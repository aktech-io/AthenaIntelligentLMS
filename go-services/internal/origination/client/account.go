package client

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/httputil"
)

// AccountClient calls account-service to move money on deposit accounts (e.g.
// crediting a borrower's account on loan disbursement).
type AccountClient struct {
	client  *httputil.ServiceClient
	baseURL string
	logger  *zap.Logger
}

// NewAccountClient creates a new AccountClient.
func NewAccountClient(serviceKey, baseURL string, logger *zap.Logger) *AccountClient {
	return &AccountClient{
		client:  httputil.NewServiceClient(serviceKey),
		baseURL: baseURL,
		logger:  logger,
	}
}

type creditRequest struct {
	Amount         decimal.Decimal `json:"amount"`
	Description    string          `json:"description,omitempty"`
	Reference      string          `json:"reference,omitempty"`
	Channel        string          `json:"channel,omitempty"`
	IdempotencyKey string          `json:"idempotencyKey,omitempty"`
}

// Credit credits a deposit account. Returns an error if the call fails so the
// caller can decide whether to proceed (disbursement must NOT complete if the
// borrower's account was not actually funded).
func (c *AccountClient) Credit(ctx context.Context, accountID uuid.UUID, amount decimal.Decimal, description, reference, idempotencyKey string) error {
	url := fmt.Sprintf("%s/api/v1/accounts/%s/credit", c.baseURL, accountID)
	body := creditRequest{
		Amount:         amount,
		Description:    description,
		Reference:      reference,
		Channel:        "LOAN_DISBURSEMENT",
		IdempotencyKey: idempotencyKey,
	}
	var resp map[string]any
	if err := c.client.Post(ctx, url, body, &resp); err != nil {
		c.logger.Error("Account credit failed during disbursement",
			zap.String("accountId", accountID.String()),
			zap.String("amount", amount.String()),
			zap.Error(err))
		return fmt.Errorf("credit disbursement account: %w", err)
	}
	return nil
}
