package main

import (
	"context"
	"fmt"
	"github.com/athena-lms/go-services/internal/common/metrics"
	"github.com/athena-lms/go-services/internal/common/tracing"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/account/event"
	"github.com/athena-lms/go-services/internal/account/handler"
	"github.com/athena-lms/go-services/internal/account/rbac"
	"github.com/athena-lms/go-services/internal/account/repository"
	"github.com/athena-lms/go-services/internal/account/service"
	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/config"
	"github.com/athena-lms/go-services/internal/common/db"
	commonevent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/health"
	commonmw "github.com/athena-lms/go-services/internal/common/middleware"
	"github.com/athena-lms/go-services/internal/common/outbox"
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

	// Distributed tracing (H1): no-op unless OTEL_EXPORTER_OTLP_ENDPOINT is set.
	shutdownTracing := tracing.Init(context.Background(), cfg.ServiceName, logger)
	defer shutdownTracing(context.Background())
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
	pub, err := commonevent.NewPublisher(rmqConn, logger)
	if err != nil {
		logger.Warn("Event publisher unavailable", zap.Error(err))
	}
	defer pub.Close()
	acctPub := event.NewPublisher(pub, logger)

	// Outbox relay: drains events the service writes transactionally (atomic with
	// the balance change) and publishes them at-least-once, surviving broker
	// outages and restarts (F27 root-cause fix).
	relay := outbox.NewRelay(pool, pub, logger)
	metrics.MustRegister(metrics.NewOutboxCollector(relay))
	go relay.Run(ctx)

	// Service wiring
	repo := repository.New(pool)
	rbacStore := rbac.NewStore(pool)
	accountSvc := service.NewAccountService(repo, acctPub, logger)
	customerSvc := service.NewCustomerService(repo, acctPub, logger)
	transferSvc := service.NewTransferService(repo, acctPub, logger, "", cfg.InternalServiceKey)
	openingSvc := service.NewAccountOpeningService(repo, acctPub, logger)
	interestSvc := service.NewInterestService(repo, acctPub, logger)
	dormancySvc := service.NewDormancyService(repo, acctPub, logger)
	eodSvc := service.NewEODService(interestSvc, dormancySvc, logger)
	approvalSvc := service.NewApprovalService(repo, accountSvc, transferSvc, openingSvc, logger)
	tenantSvc := service.NewTenantService(repo, repo, logger)
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

	// Auth handler (login — unauthenticated). The rbac store stamps effective
	// permissions into issued tokens.
	authHandler, err := handler.NewAuthHandler(cfg.JWTSecret, rbacStore, logger)
	if err != nil {
		logger.Fatal("Failed to initialize auth handler", zap.Error(err))
	}

	// Router
	r := chi.NewRouter()
	r.Use(commonmw.Recovery(logger))
	r.Use(commonmw.Logging(logger, cfg.ServiceName))

	// Health endpoint (unauthenticated)
	r.Get("/actuator/health", health.Handler(pool, rmqConn))
	// Prometheus metrics (H2): unauthenticated, scraped in-cluster only.
	r.Handle("/metrics", metrics.Handler())

	// Auth endpoints (unauthenticated)
	r.Post("/api/auth/login", authHandler.Login)

	// Protected routes
	authMw := auth.NewMiddleware(jwtUtil, cfg.InternalServiceKey, logger)
	rbacAudit := audit.New(repo, logger)
	rbacHandler := rbac.NewHandler(rbacStore, rbacAudit, logger)
	tenantHandler := handler.NewTenantHandler(tenantSvc, logger)
	r.Group(func(r chi.Router) {
		r.Use(authMw.Handler)
		r.Get("/api/auth/me", authHandler.Me)
		hdlr.RegisterRoutes(r)
		// RBAC matrix: reads open to authenticated callers; writes require rbac.manage.
		rbacHandler.RegisterRoutes(r, auth.RequirePermission("rbac.manage", "ADMIN"))
		// Tenant registry (Nemo C1): platform administration, admin-only.
		tenantHandler.RegisterRoutes(r, auth.RequirePermission("tenant.manage", "ADMIN"))
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
