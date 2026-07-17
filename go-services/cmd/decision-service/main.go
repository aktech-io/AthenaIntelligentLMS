// decision-service is the thin control plane of the Nemo decision spine (E1).
//
// v1 (design §6): it hosts the decision_log projection — consuming
// decision.recorded events from every producer's outbox, idempotently, into
// the partitioned append-only decision_log — and the tenant-scoped read API
// (GET /api/v1/decisions). Policy CRUD/approval, referral queues, ETag policy
// distribution and the regulator export are increment 4; evaluation itself
// never lives here (it is the internal/common/decision library, in-process in
// each service).
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

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/config"
	"github.com/athena-lms/go-services/internal/common/db"
	"github.com/athena-lms/go-services/internal/common/health"
	"github.com/athena-lms/go-services/internal/common/metrics"
	commonmw "github.com/athena-lms/go-services/internal/common/middleware"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
	"github.com/athena-lms/go-services/internal/common/tracing"
	"github.com/athena-lms/go-services/internal/decisionsvc/consumer"
	"github.com/athena-lms/go-services/internal/decisionsvc/handler"
	"github.com/athena-lms/go-services/internal/decisionsvc/repository"
)

func init() { decimal.MarshalJSONWithoutQuotes = true }

func main() {
	// Structured JSON logging
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load("decision-service")
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Distributed tracing (H1): no-op unless OTEL_EXPORTER_OTLP_ENDPOINT is set.
	shutdownTracing := tracing.Init(context.Background(), cfg.ServiceName, logger)
	defer shutdownTracing(context.Background())
	cfg.Port = envInt("PORT", 8106)
	cfg.DBName = envStr("DB_NAME", "athena_decision")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	pool, err := db.NewPool(ctx, cfg.DatabaseDSN(), logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	// Run migrations
	if cfg.MigrateOnStartup {
		if err := db.RunMigrations(cfg.DatabaseDSN(), "file://migrations/decision", logger); err != nil {
			logger.Warn("Migration failed (may be first run)", zap.Error(err))
		}
	}

	// RabbitMQ
	rmqConn := rabbitmq.TryConnection(cfg.RabbitMQURL(), logger)
	defer rmqConn.Close()

	// Declare topology on every (re)connect so it survives a broker restart
	// (re-creates the exchange/queues/bindings idempotently).
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

	// Wire up decision components
	repo := repository.New(pool)
	h := handler.New(repo, logger)

	// Projection consumer: decision.recorded → decision_log (idempotent).
	if cfg.RabbitMQConsumeEnabled {
		cons := consumer.New(rmqConn, pool, repo, logger)
		go func() {
			if err := cons.Start(ctx); err != nil {
				logger.Error("Decision projection consumer stopped", zap.Error(err))
			}
		}()
	}

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
