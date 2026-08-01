// Package consumer contains the overdraft-service event consumers.
//
// MobileUserConsumer ports the Java-era overdraft MobileUserRegisteredListener:
// when the mobile BFF publishes mobile.user.registered it auto-provisions the
// customer wallet so the dashboard's overdraft panel has something to show.
package consumer

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/idempotency"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
	"github.com/athena-lms/go-services/internal/overdraft/model"
	"github.com/athena-lms/go-services/internal/overdraft/repository"
	"github.com/athena-lms/go-services/internal/overdraft/service"
)

// MobileUserRegisteredPayload is the payload published by the mobile BFF
// (internal/bff/gateway/publisher.PublishUserRegistered). tenantId is a
// Java-era compatibility fallback; the envelope carries the tenant normally.
type MobileUserRegisteredPayload struct {
	UserID      string `json:"userId"`
	PhoneNumber string `json:"phoneNumber"`
	CustomerID  string `json:"customerId"`
	TenantID    string `json:"tenantId"`
}

// WalletService is the slice of the overdraft service this consumer drives.
type WalletService interface {
	CreateWallet(ctx context.Context, req model.CreateWalletRequest, tenantID string) (*model.WalletResponse, error)
}

// Repo is the read-side used for the idempotent exists-check.
type Repo interface {
	WalletExistsByTenantAndCustomer(ctx context.Context, tenantID, customerID string) (bool, error)
}

var (
	_ WalletService = (*service.WalletService)(nil)
	_ Repo          = (*repository.Repository)(nil)
)

// MobileUserConsumer consumes the overdraft mobile queue (bound to mobile.#)
// and auto-creates a customer wallet on mobile.user.registered.
type MobileUserConsumer struct {
	consumer  *commonEvent.Consumer
	walletSvc WalletService
	repo      Repo
	logger    *zap.Logger
}

// NewMobileUserConsumer creates the consumer for the overdraft mobile queue.
//
// The handler is wrapped with idempotency.Wrap so a redelivered event (delivery
// is at-least-once) is acked-and-skipped rather than processed twice. On top of
// that the handler is idempotent in itself: it checks (and on failure
// re-checks) wallet existence before creating, and the wallets table's
// per-customer uniqueness backstops concurrent creates — a replay can never
// produce a second wallet.
func NewMobileUserConsumer(conn *rabbitmq.Connection, pool *pgxpool.Pool,
	walletSvc WalletService, repo Repo, logger *zap.Logger) *MobileUserConsumer {
	c := &MobileUserConsumer{
		walletSvc: walletSvc,
		repo:      repo,
		logger:    logger,
	}
	handler := idempotency.Wrap(pool, logger, c.handle)
	c.consumer = commonEvent.NewConsumer(conn, rabbitmq.OverdraftMobileQueue, 3, 5, handler, logger)
	return c
}

// Start begins consuming messages. Blocks until ctx is cancelled.
func (c *MobileUserConsumer) Start(ctx context.Context) error {
	return c.consumer.Start(ctx)
}

// handle processes a single domain event. Return nil to ack, error to
// nack+requeue. The queue is bound to mobile.#, so unrelated mobile events
// (transfers etc.) are acked and ignored.
func (c *MobileUserConsumer) handle(ctx context.Context, evt *commonEvent.DomainEvent) error {
	if evt.Type != commonEvent.MobileUserRegistered {
		c.logger.Debug("Ignoring event type", zap.String("type", evt.Type))
		return nil
	}

	var payload MobileUserRegisteredPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		c.logger.Error("Failed to unmarshal mobile.user.registered payload", zap.Error(err))
		return nil // don't requeue malformed payloads
	}

	if payload.CustomerID == "" {
		c.logger.Warn("mobile.user.registered missing customerId, skipping", zap.String("eventId", evt.ID))
		return nil
	}

	tenantID := evt.TenantID
	if tenantID == "" {
		tenantID = payload.TenantID
	}
	if tenantID == "" {
		c.logger.Warn("mobile.user.registered missing tenantId, skipping",
			zap.String("eventId", evt.ID), zap.String("customerId", payload.CustomerID))
		return nil
	}

	exists, err := c.repo.WalletExistsByTenantAndCustomer(ctx, tenantID, payload.CustomerID)
	if err != nil {
		return err // requeue: transient DB failure
	}
	if exists {
		c.logger.Info("Wallet already exists for customer, skipping",
			zap.String("customerId", payload.CustomerID), zap.String("tenantId", tenantID))
		return nil
	}

	req := model.CreateWalletRequest{
		CustomerID: payload.CustomerID,
		Currency:   "", // defaults to the market pack currency
	}
	if _, err := c.walletSvc.CreateWallet(ctx, req, tenantID); err != nil {
		// Lost a race with another creator? If the wallet exists now the
		// outcome we wanted is in place — ack instead of requeueing.
		if exists, checkErr := c.repo.WalletExistsByTenantAndCustomer(ctx, tenantID, payload.CustomerID); checkErr == nil && exists {
			c.logger.Info("Wallet created concurrently, continuing",
				zap.String("customerId", payload.CustomerID), zap.String("tenantId", tenantID))
			return nil
		}
		return err // requeue
	}

	c.logger.Info("Auto-provisioned wallet for mobile user",
		zap.String("customerId", payload.CustomerID), zap.String("tenantId", tenantID))
	return nil
}
