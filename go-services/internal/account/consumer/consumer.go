// Package consumer contains the account-service event consumers.
//
// MobileUserConsumer ports the Java-era account MobileUserRegisteredListener:
// when the mobile BFF publishes mobile.user.registered (a new app user finished
// OTP registration), it auto-provisions the Customer record and a default
// WALLET account so the dashboard balance is populated immediately.
package consumer

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/account/model"
	"github.com/athena-lms/go-services/internal/account/repository"
	"github.com/athena-lms/go-services/internal/account/service"
	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/idempotency"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
)

// MobileUserRegisteredPayload is the payload published by the mobile BFF
// (internal/bff/gateway/publisher.PublishUserRegistered): userId, phoneNumber
// and customerId. firstName/lastName/tenantId are optional Java-era fields kept
// for compatibility.
type MobileUserRegisteredPayload struct {
	UserID      string `json:"userId"`
	PhoneNumber string `json:"phoneNumber"`
	CustomerID  string `json:"customerId"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	TenantID    string `json:"tenantId"`
}

// CustomerService is the slice of the customer service this consumer drives.
type CustomerService interface {
	CreateCustomer(ctx context.Context, req service.CreateCustomerRequest, tenantID string) (*model.Customer, error)
}

// AccountService is the slice of the account service this consumer drives.
type AccountService interface {
	CreateAccount(ctx context.Context, req service.CreateAccountRequest, tenantID string) (*model.Account, error)
}

// Repo is the read-side used for the idempotent exists-checks.
type Repo interface {
	CustomerExistsByCustomerIDAndTenant(ctx context.Context, customerID, tenantID string) (bool, error)
	GetAccountsByCustomer(ctx context.Context, customerID, tenantID string) ([]*model.Account, error)
}

var (
	_ CustomerService = (*service.CustomerService)(nil)
	_ AccountService  = (*service.AccountService)(nil)
	_ Repo            = (*repository.Repository)(nil)
)

// MobileUserConsumer consumes the account mobile queue (bound to mobile.#) and
// auto-creates a Customer + WALLET account on mobile.user.registered.
type MobileUserConsumer struct {
	consumer    *commonEvent.Consumer
	customerSvc CustomerService
	accountSvc  AccountService
	repo        Repo
	logger      *zap.Logger
}

// NewMobileUserConsumer creates the consumer for the account mobile queue.
//
// The handler is wrapped with idempotency.Wrap so a redelivered event (delivery
// is at-least-once) is acked-and-skipped rather than processed twice. On top of
// the event-ID guard the handler itself is idempotent: it re-checks whether the
// customer/account already exist before creating either, and the DB backstops
// with uq_customer_tenant (tenant_id, customer_id), so a replay can never
// produce duplicate customers or a second auto-provisioned account.
func NewMobileUserConsumer(conn *rabbitmq.Connection, pool *pgxpool.Pool,
	customerSvc CustomerService, accountSvc AccountService, repo Repo, logger *zap.Logger) *MobileUserConsumer {
	c := &MobileUserConsumer{
		customerSvc: customerSvc,
		accountSvc:  accountSvc,
		repo:        repo,
		logger:      logger,
	}
	handler := idempotency.Wrap(pool, logger, c.handle)
	c.consumer = commonEvent.NewConsumer(conn, rabbitmq.AccountMobileQueue, 3, 5, handler, logger)
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

	c.logger.Info("Processing mobile.user.registered",
		zap.String("customerId", payload.CustomerID), zap.String("tenantId", tenantID))

	if err := c.ensureCustomer(ctx, payload, tenantID); err != nil {
		return err
	}
	return c.ensureAccount(ctx, payload, tenantID)
}

// ensureCustomer creates the Customer record if it does not already exist.
// A creation error is retried once against a fresh exists-check so a
// concurrent/racing create (unique violation on uq_customer_tenant) is treated
// as success rather than poisoning the queue.
func (c *MobileUserConsumer) ensureCustomer(ctx context.Context, payload MobileUserRegisteredPayload, tenantID string) error {
	exists, err := c.repo.CustomerExistsByCustomerIDAndTenant(ctx, payload.CustomerID, tenantID)
	if err != nil {
		return err // requeue: transient DB failure
	}
	if exists {
		c.logger.Debug("Customer already exists, skipping create",
			zap.String("customerId", payload.CustomerID), zap.String("tenantId", tenantID))
		return nil
	}

	firstName := payload.FirstName
	if firstName == "" {
		firstName = "Mobile"
	}
	lastName := payload.LastName
	if lastName == "" {
		lastName = "User"
	}
	var phone *string
	if payload.PhoneNumber != "" {
		phone = &payload.PhoneNumber
	}
	source := "MOBILE"

	req := service.CreateCustomerRequest{
		CustomerID: payload.CustomerID,
		FirstName:  firstName,
		LastName:   lastName,
		Phone:      phone,
		Source:     &source,
	}
	if _, err := c.customerSvc.CreateCustomer(ctx, req, tenantID); err != nil {
		// Lost a race with another creator? If the customer exists now the
		// outcome we wanted is in place — proceed instead of requeueing.
		if exists, checkErr := c.repo.CustomerExistsByCustomerIDAndTenant(ctx, payload.CustomerID, tenantID); checkErr == nil && exists {
			c.logger.Info("Customer created concurrently, continuing",
				zap.String("customerId", payload.CustomerID), zap.String("tenantId", tenantID))
			return nil
		}
		return err // requeue
	}

	c.logger.Info("Auto-created customer for mobile user",
		zap.String("customerId", payload.CustomerID), zap.String("tenantId", tenantID))
	return nil
}

// ensureAccount provisions the default WALLET account (tier 0, market default
// currency) unless the customer already has any account.
func (c *MobileUserConsumer) ensureAccount(ctx context.Context, payload MobileUserRegisteredPayload, tenantID string) error {
	accounts, err := c.repo.GetAccountsByCustomer(ctx, payload.CustomerID, tenantID)
	if err != nil {
		return err // requeue: transient DB failure
	}
	if len(accounts) > 0 {
		c.logger.Info("Account already exists for customer, skipping",
			zap.String("customerId", payload.CustomerID), zap.String("tenantId", tenantID))
		return nil
	}

	accountName := "Mobile Wallet"
	if payload.PhoneNumber != "" {
		accountName = "Mobile Wallet - " + payload.PhoneNumber
	}

	req := service.CreateAccountRequest{
		CustomerID:  payload.CustomerID,
		AccountType: string(model.AccountTypeWallet),
		Currency:    "", // defaults to the market pack currency
		KycTier:     0,  // fresh registrations start at tier 0 limits
		AccountName: accountName,
	}
	account, err := c.accountSvc.CreateAccount(ctx, req, tenantID)
	if err != nil {
		return err // requeue
	}

	c.logger.Info("Auto-provisioned WALLET account for mobile user",
		zap.String("customerId", payload.CustomerID),
		zap.String("accountNumber", account.AccountNumber),
		zap.String("tenantId", tenantID))
	return nil
}
