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
	"go.uber.org/zap"
	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/account/event"
	"github.com/athena-lms/go-services/internal/account/handler"
	"github.com/athena-lms/go-services/internal/account/repository"
	"github.com/athena-lms/go-services/internal/account/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/config"
	"github.com/athena-lms/go-services/internal/common/db"
	commonevent "github.com/athena-lms/go-services/internal/common/event"
	commonmw "github.com/athena-lms/go-services/internal/common/middleware"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
)

func init() { decimal.MarshalJSONWithoutQuotes = true }

func main() {
	// Structured JSON logging
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load("account-service")
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}
	cfg.Port = envInt("PORT", 8086)
	cfg.DBName = envStr("DB_NAME", "athena_accounts")

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
		if err := db.RunMigrations(cfg.DatabaseDSN(), "file://migrations/account", logger); err != nil {
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
	pub, err := commonevent.NewPublisher(rmqConn, logger)
	if err != nil {
		logger.Warn("Event publisher unavailable", zap.Error(err))
	}
	defer pub.Close()
	acctPub := event.NewPublisher(pub, logger)

	// Service wiring
	repo := repository.New(pool)
	accountSvc := service.NewAccountService(repo, acctPub, logger)
	customerSvc := service.NewCustomerService(repo, acctPub, logger)
	transferSvc := service.NewTransferService(repo, acctPub, logger, "", cfg.InternalServiceKey)
	openingSvc := service.NewAccountOpeningService(repo, acctPub, logger)
	interestSvc := service.NewInterestService(repo, acctPub, logger)
	dormancySvc := service.NewDormancyService(repo, acctPub, logger)
	eodSvc := service.NewEODService(interestSvc, dormancySvc, logger)
	approvalSvc := service.NewApprovalService(repo, accountSvc, transferSvc, openingSvc, logger)
	hdlr := handler.NewWithRepo(accountSvc, customerSvc, transferSvc, repo, logger)
	hdlr.SetOpeningService(openingSvc)
	hdlr.SetInterestService(interestSvc)
	hdlr.SetEODService(eodSvc)
	hdlr.SetApprovalService(approvalSvc)

	// JWT
	jwtUtil, err := auth.NewJWTUtil(cfg.JWTSecret)
	if err != nil {
		logger.Fatal("Failed to initialize JWT", zap.Error(err))
	}

	// Auth handler (login — unauthenticated)
	authHandler, err := handler.NewAuthHandler(cfg.JWTSecret, logger)
	if err != nil {
		logger.Fatal("Failed to initialize auth handler", zap.Error(err))
	}

	// Router
	r := chi.NewRouter()
	r.Use(commonmw.Recovery(logger))
	r.Use(commonmw.Logging(logger, cfg.ServiceName))

	// Health endpoint (unauthenticated)
	r.Get("/actuator/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"UP"}`))
	})

	// Auth endpoints (unauthenticated)
	r.Post("/api/auth/login", authHandler.Login)

	// Protected routes
	authMw := auth.NewMiddleware(jwtUtil, cfg.InternalServiceKey, logger)
	r.Group(func(r chi.Router) {
		r.Use(authMw.Handler)
		r.Get("/api/auth/me", authHandler.Me)
		hdlr.RegisterRoutes(r)
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
