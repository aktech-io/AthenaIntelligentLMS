package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
				ServiceName: "shop-service",
			},
		},
	}
}

type ResolveAccountResponse struct {
	AccountID string `json:"accountId"`
}

func (c *AccountClient) ResolveAccountID(ctx context.Context, customerID string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/customer/%s", c.baseURL, customerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("account service unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("account resolve failed (status %d): %s", resp.StatusCode, string(body))
	}
	var result ResolveAccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.AccountID, nil
}

type DebitRequest struct {
	AccountID   string  `json:"accountId"`
	Amount      float64 `json:"amount"`
	Description string  `json:"description"`
	Reference   string  `json:"reference"`
}

type DebitResponse struct {
	TransactionRef string `json:"transactionRef"`
}

func (c *AccountClient) DebitAccount(ctx context.Context, req DebitRequest) (*DebitResponse, error) {
	body, _ := json.Marshal(req)
	url := fmt.Sprintf("%s/api/v1/accounts/%s/debit", c.baseURL, req.AccountID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("account debit failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("account debit failed (status %d): %s", resp.StatusCode, string(respBody))
	}
	var result DebitResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AccountClient) CreditAccount(ctx context.Context, accountID string, amount float64, description, reference string) error {
	payload := map[string]any{
		"accountId":   accountID,
		"amount":      amount,
		"description": description,
		"reference":   reference,
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/v1/accounts/%s/credit", c.baseURL, accountID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("account credit failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("account credit failed (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}
