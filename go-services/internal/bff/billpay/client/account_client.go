package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/common/auth"
)

// AccountClient calls the account service for debit/credit operations.
type AccountClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAccountClient creates an AccountClient with service-key authentication.
func NewAccountClient(baseURL, serviceKey string) *AccountClient {
	return &AccountClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "billpay-savings-service",
			},
		},
	}
}

type accountRequest struct {
	Amount         float64 `json:"amount"`
	Reference      string  `json:"reference"`
	Description    string  `json:"description"`
	Channel        string  `json:"channel"`
	IdempotencyKey string  `json:"idempotencyKey"`
}

// Debit debits an account by the given amount.
func (c *AccountClient) Debit(ctx context.Context, accountID uuid.UUID, amount float64, reference, description string) error {
	return c.postAccountAction(ctx, accountID, "debit", amount, reference, description)
}

// Credit credits an account by the given amount.
func (c *AccountClient) Credit(ctx context.Context, accountID uuid.UUID, amount float64, reference, description string) error {
	return c.postAccountAction(ctx, accountID, "credit", amount, reference, description)
}

func (c *AccountClient) postAccountAction(ctx context.Context, accountID uuid.UUID, action string, amount float64, reference, description string) error {
	url := fmt.Sprintf("%s/api/v1/accounts/%s/%s", c.baseURL, accountID, action)
	body := accountRequest{
		Amount:         amount,
		Reference:      reference,
		Description:    description,
		Channel:        "BILLPAY_SAVINGS",
		IdempotencyKey: uuid.New().String(),
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal account request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create account request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("account %s call failed: %w", action, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("account %s returned status %d", action, resp.StatusCode)
	}
	return nil
}
