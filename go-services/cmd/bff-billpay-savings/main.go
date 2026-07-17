// bff-billpay-savings is the mobile BFF bill-pay and savings service: the
// biller catalogue, saved billers, bill payment (settled via the LMS payment
// service), and savings goals with deposits/withdrawals plus a daily auto-save
// scheduler. Folded in from the AthenaMobileWallet repo (A1 Phase 0); the HTTP
// surface is unchanged.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/bff/billpay/client"
	"github.com/athena-lms/go-services/internal/bff/billpay/config"
	"github.com/athena-lms/go-services/internal/bff/billpay/handler"
	"github.com/athena-lms/go-services/internal/bff/billpay/provider"
	"github.com/athena-lms/go-services/internal/bff/billpay/publisher"
	"github.com/athena-lms/go-services/internal/bff/billpay/repository"
	"github.com/athena-lms/go-services/internal/bff/billpay/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/db"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/health"
	"github.com/athena-lms/go-services/internal/common/metrics"
	commonmw "github.com/athena-lms/go-services/internal/common/middleware"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
	"github.com/athena-lms/go-services/internal/common/tracing"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	shutdownTracing := tracing.Init(context.Background(), cfg.ServiceName, logger)
	defer shutdownTracing(context.Background())
	cfg.Port = envInt("PORT", 8112)
	cfg.DBName = envStr("DB_NAME", "athena_billpay_savings")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DatabaseDSN(), logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer pool.Close()
	sqlxDB := db.NewSQLX(pool)

	if exit := db.MigrateGate(cfg.Config, "file://migrations/bff-billpay-savings", logger); exit {
		return
	}

	rmqConn := rabbitmq.TryConnection(cfg.RabbitMQURL(), logger)
	defer rmqConn.Close()
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

	jwtUtil, err := auth.NewJWTUtil(cfg.JWTSecret)
	if err != nil {
		logger.Fatal("Failed to initialize JWT", zap.Error(err))
	}

	pub, err := event.NewPublisher(rmqConn, logger)
	if err != nil {
		logger.Warn("Event publisher unavailable (RabbitMQ not connected)", zap.Error(err))
	}
	defer pub.Close()
	eventPub := publisher.NewEventPublisher(pub)

	// Repositories
	billerRepo := repository.NewBillerRepo(sqlxDB)
	paymentRepo := repository.NewBillPaymentRepo(sqlxDB)
	savedRepo := repository.NewSavedBillerRepo(sqlxDB)
	savingsRepo := repository.NewSavingsRepo(sqlxDB)

	// LMS clients (via lms-api-gateway, X-Service-Key auth)
	accountCli := client.NewAccountClient(cfg.AccountServiceURL, cfg.InternalServiceKey)
	paymentCli := client.NewPaymentClient(cfg.PaymentServiceURL, cfg.InternalServiceKey)

	// Provider, services
	billerProvider := provider.NewBillerProvider()
	billPaySvc := service.NewBillPayService(billerRepo, paymentRepo, savedRepo, billerProvider, accountCli, paymentCli, eventPub)
	savingsSvc := service.NewSavingsService(savingsRepo, accountCli, eventPub)

	// Auto-save scheduler (daily)
	scheduler := service.NewAutoSaveScheduler(savingsRepo, savingsSvc)
	scheduler.Start(ctx)

	// Router
	r := chi.NewRouter()
	r.Use(commonmw.Recovery(logger))
	r.Use(commonmw.CORS())
	r.Use(commonmw.Logging(logger, cfg.ServiceName))

	r.Get("/actuator/health", health.Handler(pool, rmqConn))
	r.Handle("/metrics", metrics.Handler())

	authMw := auth.NewMiddleware(jwtUtil, cfg.InternalServiceKey, logger)
	handler.NewBillPayHandler(billPaySvc).Routes(r, authMw.Handler)
	handler.NewSavingsHandler(savingsSvc).Routes(r, authMw.Handler)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      tracing.WrapHandler(r, cfg.ServiceName),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

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
