package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/athena-lms/go-services/internal/common/auth"
)

type AccountClient struct {
	baseURL string
	client  *http.Client
}

func NewAccountClient(baseURL, serviceKey string) *AccountClient {
	return &AccountClient{
		baseURL: baseURL,
		client: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "mobile-gateway",
			},
		},
	}
}

func (c *AccountClient) ResolveAccountID(ctx context.Context, customerID string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/customer/%s", c.baseURL, customerID)
	resp, err := c.doGet(ctx, url)
	if err != nil {
		return "", err
	}
	id, _ := resp["id"].(string)
	if id == "" {
		// Try accountId field
		id, _ = resp["accountId"].(string)
	}
	return id, nil
}

func (c *AccountClient) GetBalance(ctx context.Context, customerID string) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/customer/%s/balance", c.baseURL, customerID)
	return c.doGet(ctx, url)
}

func (c *AccountClient) GetTransactions(ctx context.Context, customerID string, page, size int) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/customer/%s/transactions?page=%d&size=%d", c.baseURL, customerID, page, size)
	return c.doGet(ctx, url)
}

func (c *AccountClient) CreditAccount(ctx context.Context, accountID string, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%s/credit", c.baseURL, accountID)
	return c.doPost(ctx, url, body)
}

func (c *AccountClient) DebitAccount(ctx context.Context, accountID string, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%s/debit", c.baseURL, accountID)
	return c.doPost(ctx, url, body)
}

func (c *AccountClient) doGet(ctx context.Context, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("account service request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("account service returned %d", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

func (c *AccountClient) doPost(ctx context.Context, url string, body map[string]any) (map[string]any, error) {
	return doJSONPost(ctx, c.client, url, body)
}
