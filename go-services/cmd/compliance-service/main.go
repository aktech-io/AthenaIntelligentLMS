package main

import (
	"context"
	"fmt"
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
	"github.com/athena-lms/go-services/internal/compliance/consumer"
	complianceevent "github.com/athena-lms/go-services/internal/compliance/event"
	"github.com/athena-lms/go-services/internal/compliance/handler"
	"github.com/athena-lms/go-services/internal/compliance/repository"
	"github.com/athena-lms/go-services/internal/compliance/service"
)

func init() { decimal.MarshalJSONWithoutQuotes = true }

func main() {
	// Structured JSON logging
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load("compliance-service")
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}
	cfg.Port = envInt("PORT", 8094)
	cfg.DBName = envStr("DB_NAME", "athena_compliance")

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
		if err := db.RunMigrations(cfg.DatabaseDSN(), "file://migrations/compliance", logger); err != nil {
			logger.Warn("Migration failed (may be first run)", zap.Error(err))
		}
	}

	// RabbitMQ
	rmqConn := rabbitmq.TryConnection(cfg.RabbitMQURL(), logger)
	defer rmqConn.Close()

	// Declare topology (only if connected)
	if rmqConn.IsConnected() {
		ch, err := rmqConn.Channel()
		if err != nil {
			logger.Warn("Failed to open RabbitMQ channel", zap.Error(err))
		} else {
			if err := rabbitmq.DeclareTopology(ch, logger); err != nil {
				logger.Warn("Failed to declare RabbitMQ topology", zap.Error(err))
			}
			ch.Close()
		}
	}

	// Event publisher
	pub, err := event.NewPublisher(rmqConn, logger)
	if err != nil {
		logger.Warn("Event publisher unavailable (RabbitMQ not connected)", zap.Error(err))
	}
	defer pub.Close()

	// Compliance wiring
	repo := repository.New(pool)
	compPub := complianceevent.NewPublisher(pub, logger)
	svc := service.New(repo, compPub, logger)
	hdlr := handler.New(svc, logger)

	// JWT
	jwtUtil, err := auth.NewJWTUtil(cfg.JWTSecret)
	if err != nil {
		logger.Fatal("Failed to initialize JWT", zap.Error(err))
	}

	// Router
	r := chi.NewRouter()
	r.Use(commonmw.Recovery(logger))
	r.Use(commonmw.Logging(logger, cfg.ServiceName))

	// Health endpoint (unauthenticated -- used by Docker healthcheck)
	r.Get("/actuator/health", health.Handler(pool, rmqConn))

	// Protected routes
	authMw := auth.NewMiddleware(jwtUtil, cfg.InternalServiceKey, logger)
	r.Group(func(r chi.Router) {
		r.Use(authMw.Handler)
		hdlr.RegisterRoutes(r)
	})

	// Consumer (gated by RABBITMQ_CONSUME_ENABLED)
	if envBool("RABBITMQ_CONSUME_ENABLED", true) {
		cons := consumer.New(svc, repo, logger)
		evtConsumer := event.NewConsumer(rmqConn, rabbitmq.ComplianceQueue, 3, 5, cons.Handle, logger)
		go func() {
			if err := evtConsumer.Start(ctx); err != nil {
				logger.Error("Consumer stopped with error", zap.Error(err))
			}
		}()
		logger.Info("Compliance event consumer started", zap.String("queue", rabbitmq.ComplianceQueue))
	} else {
		logger.Info("Compliance event consumer disabled (RABBITMQ_CONSUME_ENABLED=false)")
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

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		return strings.EqualFold(v, "true") || v == "1"
	}
	return fallback
}
