package main

import (
	"context"
	"fmt"
	"github.com/athena-lms/go-services/internal/common/metrics"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/config"
	"github.com/athena-lms/go-services/internal/common/db"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/health"
	commonmw "github.com/athena-lms/go-services/internal/common/middleware"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
	"github.com/athena-lms/go-services/internal/float/consumer"
	floatEvent "github.com/athena-lms/go-services/internal/float/event"
	"github.com/athena-lms/go-services/internal/float/handler"
	"github.com/athena-lms/go-services/internal/float/repository"
	"github.com/athena-lms/go-services/internal/float/service"
)

func init() { decimal.MarshalJSONWithoutQuotes = true }

func main() {
	// Structured JSON logging
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load("float-service")
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}
	cfg.Port = envInt("PORT", 8092)
	cfg.DBName = envStr("DB_NAME", "athena_float")

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
		if err := db.RunMigrations(cfg.DatabaseDSN(), "file://migrations/float", logger); err != nil {
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

	// Event publisher
	pub, err := event.NewPublisher(rmqConn, logger)
	if err != nil {
		logger.Warn("Event publisher unavailable (RabbitMQ not connected)", zap.Error(err))
	}
	defer pub.Close()

	// Float components
	repo := repository.New(pool)
	floatPub := floatEvent.NewPublisher(pub, logger)
	svc := service.New(repo, floatPub, logger)
	hdl := handler.New(svc, logger)

	// JWT
	jwtUtil, err := auth.NewJWTUtil(cfg.JWTSecret)
	if err != nil {
		logger.Fatal("Failed to initialize JWT", zap.Error(err))
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
		hdl.RegisterRoutes(r)
	})

	// Event consumers (gated by RABBITMQ_CONSUME_ENABLED)
	consumeEnabled := strings.EqualFold(envStr("RABBITMQ_CONSUME_ENABLED", "true"), "true")
	if consumeEnabled {
		floatConsumer := consumer.New(svc, rmqConn, logger)

		// Declare the float inbound queue (loan.disbursed binding)
		if err := floatConsumer.DeclareFloatInboundQueue(); err != nil {
			logger.Warn("Float inbound queue unavailable (RabbitMQ not connected)", zap.Error(err))
		}

		// Start float queue consumer (account.credit.received)
		go func() {
			if err := floatConsumer.StartFloatQueueConsumer(ctx); err != nil {
				logger.Error("Float queue consumer stopped", zap.Error(err))
			}
		}()

		// Start float inbound queue consumer (loan.disbursed)
		go func() {
			if err := floatConsumer.StartInboundQueueConsumer(ctx); err != nil {
				logger.Error("Float inbound queue consumer stopped", zap.Error(err))
			}
		}()

		logger.Info("Float event consumers started (floatQueue + floatInboundQueue)")
	} else {
		logger.Info("RabbitMQ consuming disabled (RABBITMQ_CONSUME_ENABLED != true)")
	}

	// Server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
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
