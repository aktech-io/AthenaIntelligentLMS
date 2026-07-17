// bff-gateway is the mobile BFF entry point: app-user auth (OTP/PIN/JWT with
// refresh rotation and a device registry), dashboard aggregation, P2P
// transfers, top-up, contacts, and loan/overdraft proxying into the LMS via
// the lms-api-gateway. Folded in from the AthenaMobileWallet repo (A1 Phase 0);
// the HTTP surface is unchanged — the Flutter app talks to it as before.
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

	"github.com/athena-lms/go-services/internal/bff/gateway/client"
	"github.com/athena-lms/go-services/internal/bff/gateway/config"
	"github.com/athena-lms/go-services/internal/bff/gateway/handler"
	"github.com/athena-lms/go-services/internal/bff/gateway/publisher"
	"github.com/athena-lms/go-services/internal/bff/gateway/repository"
	"github.com/athena-lms/go-services/internal/bff/gateway/service"
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
	// The BFF domain packages log via log/slog; emit those as JSON too so the
	// service produces one consistent structured stream.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	shutdownTracing := tracing.Init(context.Background(), cfg.ServiceName, logger)
	defer shutdownTracing(context.Background())
	cfg.Port = envInt("PORT", 8110)
	cfg.DBName = envStr("DB_NAME", "athena_mobile_gateway")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database (single pgx pool; sqlx handle shares it for the repositories)
	pool, err := db.NewPool(ctx, cfg.DatabaseDSN(), logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer pool.Close()
	sqlxDB := db.NewSQLX(pool)

	if exit := db.MigrateGate(cfg.Config, "file://migrations/bff-gateway", logger); exit {
		return
	}

	// RabbitMQ
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

	// JWT (validates app-user tokens and issues new ones)
	jwtUtil, err := auth.NewJWTUtil(cfg.JWTSecret)
	if err != nil {
		logger.Fatal("Failed to initialize JWT", zap.Error(err))
	}

	// Event publisher
	pub, err := event.NewPublisher(rmqConn, logger)
	if err != nil {
		logger.Warn("Event publisher unavailable (RabbitMQ not connected)", zap.Error(err))
	}
	defer pub.Close()
	eventPublisher := publisher.NewEventPublisher(pub)

	// Repositories
	userRepo := repository.NewUserRepo(sqlxDB)
	otpRepo := repository.NewOTPRepo(sqlxDB)
	tokenRepo := repository.NewTokenRepo(sqlxDB)
	deviceRepo := repository.NewDeviceRepo(sqlxDB)
	contactRepo := repository.NewContactRepo(sqlxDB)
	preferenceRepo := repository.NewPreferenceRepo(sqlxDB)
	employmentRepo := repository.NewEmploymentRepo(sqlxDB)

	// HTTP clients into the LMS (via lms-api-gateway, X-Service-Key auth)
	accountClient := client.NewAccountClient(cfg.AccountServiceURL, cfg.InternalServiceKey)
	notifClient := client.NewNotificationClient(cfg.NotificationServiceURL, cfg.InternalServiceKey)
	overdraftClient := client.NewOverdraftClient(cfg.OverdraftServiceURL, cfg.InternalServiceKey)
	paymentClient := client.NewPaymentClient(cfg.PaymentServiceURL, cfg.InternalServiceKey)
	productClient := client.NewProductClient(cfg.ProductServiceURL, cfg.InternalServiceKey)
	originationClient := client.NewLoanOriginationClient(cfg.LoanOriginationServiceURL, cfg.InternalServiceKey)
	managementClient := client.NewLoanManagementClient(cfg.LoanManagementServiceURL, cfg.InternalServiceKey)

	// Services
	authSvc := service.NewAuthService(cfg, userRepo, otpRepo, tokenRepo, deviceRepo, jwtUtil, notifClient, eventPublisher)
	profileSvc := service.NewProfileService(userRepo, preferenceRepo, employmentRepo)
	dashboardSvc := service.NewDashboardService(userRepo, accountClient, notifClient, overdraftClient)
	contactSvc := service.NewContactService(contactRepo, userRepo)
	transferSvc := service.NewTransferService(userRepo, accountClient, authSvc, eventPublisher)
	topUpSvc := service.NewTopUpService(paymentClient, accountClient)
	loanSvc := service.NewLoanProxyService(productClient, originationClient, managementClient, authSvc)
	overdraftSvc := service.NewOverdraftProxyService(overdraftClient, authSvc)

	// Router
	r := chi.NewRouter()
	r.Use(commonmw.Recovery(logger))
	r.Use(commonmw.CORS())
	r.Use(commonmw.Logging(logger, cfg.ServiceName))

	r.Get("/actuator/health", health.Handler(pool, rmqConn))
	r.Handle("/metrics", metrics.Handler())

	authMw := auth.NewMiddleware(jwtUtil, cfg.InternalServiceKey, logger)
	handler.NewAuthHandler(authSvc).Routes(r, authMw.Handler)
	handler.NewProfileHandler(profileSvc).Routes(r, authMw.Handler)
	handler.NewDashboardHandler(dashboardSvc).Routes(r, authMw.Handler)
	handler.NewContactHandler(contactSvc).Routes(r, authMw.Handler)
	handler.NewTransferHandler(transferSvc).Routes(r, authMw.Handler)
	handler.NewTopUpHandler(topUpSvc).Routes(r, authMw.Handler)
	handler.NewLoanHandler(loanSvc).Routes(r, authMw.Handler)
	handler.NewOverdraftHandler(overdraftSvc).Routes(r, authMw.Handler)

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
