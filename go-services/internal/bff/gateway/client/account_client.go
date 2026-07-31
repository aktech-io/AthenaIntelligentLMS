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

// ResolveAccountID returns the customer's WALLET account id (or the first
// account when no wallet exists). GET /accounts/customer/{id} returns a list.
func (c *AccountClient) ResolveAccountID(ctx context.Context, customerID string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/customer/%s", c.baseURL, customerID)
	accounts, err := c.doGetList(ctx, url)
	if err != nil {
		return "", err
	}
	if len(accounts) == 0 {
		return "", fmt.Errorf("no accounts for customer %s", customerID)
	}
	pick := accounts[0]
	for _, a := range accounts {
		if t, _ := a["accountType"].(string); t == "WALLET" {
			pick = a
			break
		}
	}
	id, _ := pick["id"].(string)
	if id == "" {
		return "", fmt.Errorf("account for customer %s has no id", customerID)
	}
	return id, nil
}

func (c *AccountClient) GetBalance(ctx context.Context, customerID string) (map[string]any, error) {
	accountID, err := c.ResolveAccountID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/api/v1/accounts/%s/balance", c.baseURL, accountID)
	return c.doGet(ctx, url)
}

func (c *AccountClient) GetTransactions(ctx context.Context, customerID string, page, size int) (map[string]any, error) {
	accountID, err := c.ResolveAccountID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/api/v1/accounts/%s/transactions?page=%d&size=%d", c.baseURL, accountID, page, size)
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

func (c *AccountClient) doGetList(ctx context.Context, url string) ([]map[string]any, error) {
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
	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

func (c *AccountClient) doPost(ctx context.Context, url string, body map[string]any) (map[string]any, error) {
	return doJSONPost(ctx, c.client, url, body)
}
