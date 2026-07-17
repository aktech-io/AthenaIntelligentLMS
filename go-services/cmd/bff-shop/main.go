// bff-shop is the mobile BFF BNPL marketplace service: product catalogue,
// carts, favourites, orders, and BNPL credit plans with eligibility via the
// LMS AI-scoring service and financing via loan origination. Folded in from
// the AthenaMobileWallet repo (A1 Phase 0); the HTTP surface is unchanged.
// Not part of the Nemo concept navigation — keep behind a feature flag
// app-side.
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

	"github.com/athena-lms/go-services/internal/bff/shop/client"
	"github.com/athena-lms/go-services/internal/bff/shop/config"
	"github.com/athena-lms/go-services/internal/bff/shop/handler"
	"github.com/athena-lms/go-services/internal/bff/shop/publisher"
	"github.com/athena-lms/go-services/internal/bff/shop/repository"
	"github.com/athena-lms/go-services/internal/bff/shop/service"
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
	cfg.Port = envInt("PORT", 8113)
	cfg.DBName = envStr("DB_NAME", "athena_shop")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DatabaseDSN(), logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer pool.Close()
	sqlxDB := db.NewSQLX(pool)

	if exit := db.MigrateGate(cfg.Config, "file://migrations/bff-shop", logger); exit {
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
	categoryRepo := repository.NewCategoryRepo(sqlxDB)
	productRepo := repository.NewProductRepo(sqlxDB)
	bnplRepo := repository.NewBNPLRepo(sqlxDB)
	cartRepo := repository.NewCartRepo(sqlxDB)
	favoriteRepo := repository.NewFavoriteRepo(sqlxDB)
	orderRepo := repository.NewOrderRepo(sqlxDB)

	// LMS clients (via lms-api-gateway, X-Service-Key auth)
	accountClient := client.NewAccountClient(cfg.AccountServiceURL, cfg.InternalServiceKey)
	paymentClient := client.NewPaymentClient(cfg.PaymentServiceURL, cfg.InternalServiceKey)
	loanClient := client.NewLoanOriginationClient(cfg.LoanOriginationServiceURL, cfg.InternalServiceKey, cfg.BNPLProductID)
	scoringClient := client.NewScoringClient(cfg.AIScoringServiceURL, cfg.InternalServiceKey)

	// Services
	productSvc := service.NewProductService(categoryRepo, productRepo)
	cartSvc := service.NewCartService(cartRepo, productRepo)
	favoriteSvc := service.NewFavoriteService(favoriteRepo, productRepo)
	bnplSvc := service.NewBNPLService(bnplRepo, scoringClient)
	orderSvc := service.NewOrderService(orderRepo, cartRepo, productRepo, bnplRepo, accountClient, paymentClient, loanClient, eventPub)

	// Router
	r := chi.NewRouter()
	r.Use(commonmw.Recovery(logger))
	r.Use(commonmw.CORS())
	r.Use(commonmw.Logging(logger, cfg.ServiceName))

	r.Get("/actuator/health", health.Handler(pool, rmqConn))
	r.Handle("/metrics", metrics.Handler())

	authMw := auth.NewMiddleware(jwtUtil, cfg.InternalServiceKey, logger)
	handler.NewProductHandler(productSvc).Routes(r)
	handler.NewCartHandler(cartSvc).Routes(r, authMw.Handler)
	handler.NewFavoriteHandler(favoriteSvc).Routes(r, authMw.Handler)
	handler.NewOrderHandler(orderSvc).Routes(r, authMw.Handler)
	handler.NewBNPLHandler(bnplSvc).Routes(r, authMw.Handler)

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
