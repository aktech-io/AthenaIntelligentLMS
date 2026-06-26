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

	"github.com/athena-lms/go-services/internal/collections/consumer"
	collectionsEvent "github.com/athena-lms/go-services/internal/collections/event"
	"github.com/athena-lms/go-services/internal/collections/handler"
	"github.com/athena-lms/go-services/internal/collections/repository"
	"github.com/athena-lms/go-services/internal/collections/scheduler"
	"github.com/athena-lms/go-services/internal/collections/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/config"
	"github.com/athena-lms/go-services/internal/common/db"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/health"
	commonmw "github.com/athena-lms/go-services/internal/common/middleware"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
)

func init() { decimal.MarshalJSONWithoutQuotes = true }

func main() {
	// Structured JSON logging
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load("collections-service")
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}
	cfg.Port = envInt("PORT", 8093)
	cfg.DBName = envStr("DB_NAME", "athena_collections")

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
		if err := db.RunMigrations(cfg.DatabaseDSN(), "file://migrations/collections", logger); err != nil {
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

	// Repositories
	caseRepo := repository.NewCollectionCaseRepository(pool)
	actionRepo := repository.NewCollectionActionRepository(pool)
	ptpRepo := repository.NewPtpRepository(pool)
	strategyRepo := repository.NewStrategyRepository(pool)
	officerRepo := repository.NewOfficerRepository(pool)
	auditRepo := repository.NewAuditRepository(pool)

	// Event publisher
	pub, err := event.NewPublisher(rmqConn, logger)
	if err != nil {
		logger.Warn("Event publisher unavailable (RabbitMQ not connected)", zap.Error(err))
	}
	defer pub.Close()
	collPub := collectionsEvent.NewPublisher(pub, logger)

	// Service
	svc := service.NewCollectionsService(caseRepo, actionRepo, ptpRepo, strategyRepo, officerRepo, auditRepo, collPub, logger)

	// PTP scheduler + follow-up SLA checker
	ptpScheduler := scheduler.NewPtpCheckScheduler(ptpRepo, svc, logger)
	ptpScheduler.Start()
	defer ptpScheduler.Stop()

	// Consumer (gated by RABBITMQ_CONSUME_ENABLED)
	if cfg.RabbitMQConsumeEnabled {
		listener := consumer.NewCollectionsEventListener(svc, logger)
		cons := event.NewConsumer(rmqConn, rabbitmq.CollectionsQueue, 3, 5, listener.Handle, logger)
		go func() {
			if err := cons.Start(ctx); err != nil {
				logger.Error("Consumer exited", zap.Error(err))
			}
		}()
	} else {
		logger.Info("RabbitMQ consumer disabled (RABBITMQ_CONSUME_ENABLED=false)")
	}

	// Router
	r := chi.NewRouter()
	r.Use(commonmw.Recovery(logger))
	r.Use(commonmw.Logging(logger, cfg.ServiceName))

	// Health endpoint (unauthenticated -- used by Docker healthcheck)
	r.Get("/actuator/health", health.Handler(pool, rmqConn))

	// Protected routes
	h := handler.NewHandler(svc, logger)
	authMw := auth.NewMiddleware(jwtUtil, cfg.InternalServiceKey, logger)
	r.Group(func(r chi.Router) {
		r.Use(authMw.Handler)
		h.Routes(r)
	})

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
