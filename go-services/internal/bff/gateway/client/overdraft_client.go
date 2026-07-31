package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/athena-lms/go-services/internal/common/auth"
)

type OverdraftClient struct {
	baseURL string
	client  *http.Client
}

func NewOverdraftClient(baseURL, serviceKey string) *OverdraftClient {
	return &OverdraftClient{
		baseURL: baseURL,
		client: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "mobile-gateway",
			},
		},
	}
}

func (c *OverdraftClient) GetWalletByCustomerID(ctx context.Context, customerID string) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/wallets/customer/%s", c.baseURL, customerID)
	return doJSONGet(ctx, c.client, url)
}

func (c *OverdraftClient) GetOverdraftFacility(ctx context.Context, walletID string) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/wallets/%s/overdraft", c.baseURL, walletID)
	return doJSONGet(ctx, c.client, url)
}

func (c *OverdraftClient) CreateWallet(ctx context.Context, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/wallets", c.baseURL)
	return doJSONPost(ctx, c.client, url, body)
}

func (c *OverdraftClient) ApplyOverdraft(ctx context.Context, walletID string) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/wallets/%s/overdraft/apply", c.baseURL, walletID)
	return doJSONPost(ctx, c.client, url, nil)
}

func (c *OverdraftClient) Deposit(ctx context.Context, walletID string, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/wallets/%s/deposit", c.baseURL, walletID)
	return doJSONPost(ctx, c.client, url, body)
}

func (c *OverdraftClient) Withdraw(ctx context.Context, walletID string, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/wallets/%s/withdraw", c.baseURL, walletID)
	return doJSONPost(ctx, c.client, url, body)
}

func (c *OverdraftClient) GetTransactions(ctx context.Context, walletID string, page, size int) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/wallets/%s/transactions?page=%d&size=%d", c.baseURL, walletID, page, size)
	return doJSONGet(ctx, c.client, url)
}

func (c *OverdraftClient) SuspendOverdraft(ctx context.Context, walletID string) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/wallets/%s/overdraft/suspend", c.baseURL, walletID)
	return doJSONPost(ctx, c.client, url, nil)
}

func (c *OverdraftClient) GetCharges(ctx context.Context, walletID string) ([]map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/overdraft/%s/interest-charges", c.baseURL, walletID)
	return doJSONGetList(ctx, c.client, url)
}
