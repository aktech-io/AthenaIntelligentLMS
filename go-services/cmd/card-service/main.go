// card-service — Nemo B1 card issuing (port 8107 in-cluster / 28107 compose).
//
// Issues and manages payment cards through a pluggable issuer-processor
// adapter (CARD_PROCESSOR: sandbox | paymentology). PCI posture: this service
// stores processor_ref + pan_last4 only — never PAN/CVV/PIN/expiry (see
// internal/card/model and internal/card/README.md).
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/card/handler"
	cardprocessor "github.com/athena-lms/go-services/internal/card/processor"
	"github.com/athena-lms/go-services/internal/card/repository"
	"github.com/athena-lms/go-services/internal/card/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/config"
	"github.com/athena-lms/go-services/internal/common/db"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/health"
	"github.com/athena-lms/go-services/internal/common/metrics"
	commonmw "github.com/athena-lms/go-services/internal/common/middleware"
	"github.com/athena-lms/go-services/internal/common/outbox"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
	"github.com/athena-lms/go-services/internal/common/tracing"
)

func init() { decimal.MarshalJSONWithoutQuotes = true }

func main() {
	// Structured JSON logging
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load("card-service")
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Distributed tracing (H1): no-op unless OTEL_EXPORTER_OTLP_ENDPOINT is set.
	shutdownTracing := tracing.Init(context.Background(), cfg.ServiceName, logger)
	defer shutdownTracing(context.Background())
	cfg.Port = envInt("PORT", 8107)
	cfg.DBName = envStr("DB_NAME", "athena_cards")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	pool, err := db.NewPool(ctx, cfg.DatabaseDSN(), logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	// Run migrations
	if exit := db.MigrateGate(cfg, "file://migrations/card", logger); exit {
		return
	}

	// RabbitMQ
	rmqConn := rabbitmq.TryConnection(cfg.RabbitMQURL(), logger)
	defer rmqConn.Close()

	// Declare topology on every (re)connect so it survives a broker restart.
	rmqConn.OnReady(func(c *rabbitmq.Connection) {
		ch, err := c.Channel()
		if err != nil {
			logger.Warn("Failed to open RabbitMQ channel for topology", zap.Error(err))
			return
		}
		defer ch.Close()
		if err := rabbitmq.DeclareTopology(ch, logger); err != nil {
			logger.Warn("Failed to declare RabbitMQ topology", zap.Error(err))
		}
	})

	// JWT
	jwtUtil, err := auth.NewJWTUtil(cfg.JWTSecret)
	if err != nil {
		logger.Fatal("Failed to initialize JWT", zap.Error(err))
	}

	// Card processor: register the real adapter (fail-fast stub until the
	// Paymentology credentials arrive), then resolve via CARD_PROCESSOR
	// (default sandbox).
	cardprocessor.Register(cardprocessor.NewPaymentology(cardprocessor.PaymentologyConfigFromEnv(), logger))
	proc, err := cardprocessor.FromEnv()
	if err != nil {
		logger.Fatal("Failed to resolve card processor", zap.Error(err))
	}
	logger.Info("Card processor selected", zap.String("processor", proc.Name()))

	// Event publishing: all domain events go through the transactional outbox
	// (written atomically with the state change) and are drained by the relay.
	publisher, err := event.NewPublisher(rmqConn, logger)
	if err != nil {
		logger.Warn("Event publisher unavailable (RabbitMQ not connected)", zap.Error(err))
	}
	defer publisher.Close()

	relay := outbox.NewRelay(pool, publisher, logger)
	metrics.MustRegister(metrics.NewOutboxCollector(relay))
	metrics.MustRegister(metrics.NewDBCollector(pool, "cards"))
	go relay.Run(ctx)

	// Domain components
	repo := repository.New(pool)
	svc := service.New(repo, proc, logger)
	h := handler.New(svc, logger)

	// Router
	r := chi.NewRouter()
	r.Use(commonmw.Recovery(logger))
	r.Use(commonmw.Logging(logger, cfg.ServiceName))

	// Health endpoint (unauthenticated — used by Docker healthcheck)
	r.Get("/actuator/health", health.Handler(pool, rmqConn))
	// Prometheus metrics (H2): unauthenticated, scraped in-cluster only.
	r.Handle("/metrics", metrics.Handler())

	// Protected routes
	authMw := auth.NewMiddleware(jwtUtil, cfg.InternalServiceKey, logger)
	r.Group(func(r chi.Router) {
		r.Use(authMw.Handler)
		h.RegisterRoutes(r)
	})

	// Server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      tracing.WrapHandler(r, cfg.ServiceName),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("Shutting down...")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	logger.Info("Starting server", zap.Int("port", cfg.Port), zap.String("service", cfg.ServiceName))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("Server failed", zap.Error(err))
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			return n
		}
	}
	return fallback
}
