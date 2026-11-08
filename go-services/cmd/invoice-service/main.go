// invoice-service: Nemo invoice finance (docs/nemo/invoice-finance/SPEC.md).
// Follows overdraft-service's layout: config, MigrateGate, outbox relay,
// metrics, tracing, JWT/service-key auth, chi router.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
	"github.com/athena-lms/go-services/internal/common/idempotency"
	"github.com/athena-lms/go-services/internal/common/market"
	"github.com/athena-lms/go-services/internal/common/metrics"
	commonmw "github.com/athena-lms/go-services/internal/common/middleware"
	"github.com/athena-lms/go-services/internal/common/outbox"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
	"github.com/athena-lms/go-services/internal/common/tracing"
	"github.com/athena-lms/go-services/internal/invoice/client"
	"github.com/athena-lms/go-services/internal/invoice/corebanking"
	"github.com/athena-lms/go-services/internal/invoice/handler"
	"github.com/athena-lms/go-services/internal/invoice/policy"
	"github.com/athena-lms/go-services/internal/invoice/repository"
	"github.com/athena-lms/go-services/internal/invoice/service"
)

func init() { decimal.MarshalJSONWithoutQuotes = true }

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load("invoice-service")
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}
	shutdownTracing := tracing.Init(context.Background(), cfg.ServiceName, logger)
	defer shutdownTracing(context.Background())
	cfg.Port = envInt("PORT", 8101)
	cfg.DBName = envStr("DB_NAME", "athena_invoice")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DatabaseDSN(), logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer pool.Close()
	if exit := db.MigrateGate(cfg, "file://migrations/invoice", logger); exit {
		return
	}

	// RabbitMQ + topology (idempotent, re-declared on reconnect).
	rmqConn := rabbitmq.TryConnection(cfg.RabbitMQURL(), logger)
	defer rmqConn.Close()
	rmqConn.OnReady(func(c *rabbitmq.Connection) {
		ch, err := c.Channel()
		if err != nil {
			logger.Warn("RabbitMQ channel for topology", zap.Error(err))
			return
		}
		defer ch.Close()
		if err := rabbitmq.DeclareTopology(ch, logger); err != nil {
			logger.Warn("Declare RabbitMQ topology", zap.Error(err))
		}
	})
	pub, err := event.NewPublisher(rmqConn, logger)
	if err != nil {
		logger.Warn("Event publisher unavailable (RabbitMQ not connected)", zap.Error(err))
	}
	defer pub.Close()
	relay := outbox.NewRelay(pool, pub, logger)
	metrics.MustRegister(metrics.NewOutboxCollector(relay))
	go relay.Run(ctx)

	jwtUtil, err := auth.NewJWTUtil(cfg.JWTSecret)
	if err != nil {
		logger.Fatal("Failed to initialize JWT", zap.Error(err))
	}
	if cfg.InternalServiceKey == "" {
		logger.Warn("LMS_INTERNAL_SERVICE_KEY is empty: ledger signatures use an empty key and service calls will be rejected upstream")
	}

	// Upstreams.
	urls := client.URLs{
		EkycML:       envStr("EKYC_ML_URL", "http://ekyc-ml-service.lms.svc.cluster.local:8102"),
		Media:        envStr("MEDIA_SERVICE_URL", "http://media-service.lms.svc.cluster.local:8098"),
		Notification: envStr("NOTIFICATION_SERVICE_URL", "http://notification-service.lms.svc.cluster.local:8099"),
		Scoring:      envStr("AI_SCORING_URL", "http://ai-scoring-service.lms.svc.cluster.local:8096"),
		Fraud:        envStr("FRAUD_DETECTION_URL", "http://fraud-detection-service.lms.svc.cluster.local:8100"),
		Account:      envStr("ACCOUNT_SERVICE_URL", "http://account-service.lms.svc.cluster.local:8086"),
		Origination:  envStr("LOAN_ORIGINATION_URL", "http://loan-origination-service.lms.svc.cluster.local:8088"),
		LoanMgmt:     envStr("LOAN_MANAGEMENT_URL", "http://loan-management-service.lms.svc.cluster.local:8089"),
		Payment:      envStr("PAYMENT_SERVICE_URL", "http://payment-service.lms.svc.cluster.local:8090"),
		Product:      envStr("PRODUCT_SERVICE_URL", "http://product-service.lms.svc.cluster.local:8087"),
	}
	clients := client.New(urls, cfg.InternalServiceKey, logger)

	// Service config.
	scfg := service.DefaultConfig()
	scfg.DemoMode = envBool("DEMO_MODE", false)
	scfg.PublicPortalURL = envStr("PUBLIC_PORTAL_URL", "http://localhost:3001")
	scfg.Currency = envStr("DEFAULT_CURRENCY", market.Current().Currency)
	scfg.SecondApprovalAbove = envDec("DISBURSEMENT_SECOND_APPROVAL_ABOVE", scfg.SecondApprovalAbove)
	scfg.ConfirmTTL = envDur("BUYER_CONFIRM_TTL", scfg.ConfirmTTL)
	scfg.OfferTTL = envDur("OFFER_TTL", scfg.OfferTTL)
	scfg.ConfidenceFloor = envFloat("OCR_CONFIDENCE_FLOOR", scfg.ConfidenceFloor)
	scfg.OverdueGraceDays = envInt("OVERDUE_GRACE_DAYS", scfg.OverdueGraceDays)
	scfg.ProcessingFeeRate = envDec("PROCESSING_FEE_RATE", scfg.ProcessingFeeRate)
	scfg.ProcessingFeeMin = envDec("PROCESSING_FEE_MIN", scfg.ProcessingFeeMin)
	scfg.StepTimeout = envDur("PIPELINE_STEP_TIMEOUT", scfg.StepTimeout)
	scfg.FundTimeout = envDur("PIPELINE_FUND_TIMEOUT", scfg.FundTimeout)
	scfg.RetryInterval = envDur("PIPELINE_RETRY_INTERVAL", scfg.RetryInterval)
	scfg.MaxAttempts = envInt("PIPELINE_MAX_ATTEMPTS", scfg.MaxAttempts)
	scfg.SellerBandFallback = strings.ToUpper(envStr("SELLER_BAND_FALLBACK", scfg.SellerBandFallback))
	scfg.ConcentrationMinActive = envInt("CONCENTRATION_MIN_ACTIVE", scfg.ConcentrationMinActive)
	scfg.FraudRiskThreshold = envFloat("FRAUD_RISK_THRESHOLD", scfg.FraudRiskThreshold)
	scfg.NewBuyerDays = envInt("FRAUD_NEW_BUYER_DAYS", scfg.NewBuyerDays)

	// Policy engine: embedded default + latest stored version for the default tenant.
	repo := repository.New(pool, []byte(cfg.InternalServiceKey))
	pol, err := policy.New(ctx, repo, market.Current().Code)
	if err != nil {
		logger.Fatal("Failed to load invoice.advance policy", zap.Error(err))
	}
	if err := pol.Load(ctx, "default"); err != nil {
		logger.Warn("Stored policy not loaded; using embedded default", zap.Error(err))
	}

	// Core-banking provider.
	nemo := corebanking.NewNemoProvider(clients, scfg.Currency, logger)
	var webhook *corebanking.WebhookProvider
	if u := os.Getenv("COREBANKING_WEBHOOK_URL"); u != "" {
		webhook = corebanking.NewWebhookProvider(u, os.Getenv("COREBANKING_WEBHOOK_SECRET"), envDur("COREBANKING_WEBHOOK_TIMEOUT", 20*time.Second), logger)
	}
	core, err := corebanking.Select(envStr("COREBANKING_PROVIDER", "nemo"), nemo, webhook)
	if err != nil {
		logger.Fatal("Core-banking provider", zap.Error(err))
	}
	if core.Name() == "nemo" {
		// Product bootstrap is best-effort at startup and retried on first funding.
		pctx, pcancel := context.WithTimeout(ctx, 20*time.Second)
		if err := nemo.EnsureProduct(pctx); err != nil {
			logger.Warn("INVOICE_FINANCE product not ensured at startup (will retry on first funding)", zap.Error(err))
		} else {
			logger.Info("Core product ready", zap.String("code", corebanking.ProductCode), zap.String("productId", nemo.ProductID()))
		}
		pcancel()
	}
	logger.Info("Core-banking provider", zap.String("provider", core.Name()), zap.Bool("demoMode", scfg.DemoMode))

	svc := service.New(repo, clients, core, pol, scfg, logger)
	go svc.RetryLoop(ctx)
	svc.StartJobs(ctx, envInt("OVERDUE_JOB_HOUR_UTC", 1))

	// payment.completed consumer (externalReference "invoice:<id>[:ref]").
	if cfg.RabbitMQConsumeEnabled {
		consumer := event.NewConsumer(rmqConn, rabbitmq.InvoicePaymentQueue, 2, 4, idempotency.Wrap(pool, logger, svc.HandlePaymentEvent), logger)
		go func() {
			if err := consumer.Start(ctx); err != nil {
				logger.Error("payment consumer stopped", zap.Error(err))
			}
		}()
	}

	h := handler.New(svc, envDur("UPLOAD_SYNC_WAIT", 60*time.Second), envBool("TRUST_PROXY_HEADERS", true), logger)

	r := chi.NewRouter()
	r.Use(commonmw.Recovery(logger))
	r.Use(commonmw.Logging(logger, cfg.ServiceName))
	r.Use(commonmw.CORS())
	r.Get("/actuator/health", health.Handler(pool, rmqConn))
	r.Handle("/metrics", metrics.Handler())

	// Public (no JWT), rate-limited per IP.
	publicLimiter := commonmw.NewRateLimiter(commonmw.RateLimiterConfig{
		Enabled: envBool("PUBLIC_RATE_LIMIT_ENABLED", true), RPS: envFloat("PUBLIC_RATE_LIMIT_RPS", 2), Burst: envInt("PUBLIC_RATE_LIMIT_BURST", 20),
		TrustProxyHeaders: envBool("TRUST_PROXY_HEADERS", true),
	}, logger)
	defer publicLimiter.Stop()
	r.Group(func(r chi.Router) {
		r.Use(publicLimiter.Middleware)
		h.RegisterPublicRoutes(r)
	})

	authMw := auth.NewMiddleware(jwtUtil, cfg.InternalServiceKey, logger)
	r.Group(func(r chi.Router) {
		r.Use(authMw.Handler)
		h.RegisterRoutes(r)
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      tracing.WrapHandler(r, cfg.ServiceName),
		ReadTimeout:  120 * time.Second, // uploads + inline pipeline
		WriteTimeout: 120 * time.Second,
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
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func envDec(key string, fallback decimal.Decimal) decimal.Decimal {
	if v := os.Getenv(key); v != "" {
		if d, err := decimal.NewFromString(v); err == nil {
			return d
		}
	}
	return fallback
}

func envDur(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
