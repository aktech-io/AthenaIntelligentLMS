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
	url := fmt.Sprintf("%s/api/v1/float/wallets/customer/%s", c.baseURL, customerID)
	return doJSONGet(ctx, c.client, url)
}

func (c *OverdraftClient) GetOverdraftFacility(ctx context.Context, walletID string) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/float/facilities/wallet/%s", c.baseURL, walletID)
	return doJSONGet(ctx, c.client, url)
}

func (c *OverdraftClient) CreateWallet(ctx context.Context, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/float/wallets", c.baseURL)
	return doJSONPost(ctx, c.client, url, body)
}

func (c *OverdraftClient) ApplyOverdraft(ctx context.Context, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/float/facilities", c.baseURL)
	return doJSONPost(ctx, c.client, url, body)
}

func (c *OverdraftClient) Deposit(ctx context.Context, walletID string, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/float/wallets/%s/deposit", c.baseURL, walletID)
	return doJSONPost(ctx, c.client, url, body)
}

func (c *OverdraftClient) Withdraw(ctx context.Context, walletID string, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/float/wallets/%s/withdraw", c.baseURL, walletID)
	return doJSONPost(ctx, c.client, url, body)
}

func (c *OverdraftClient) GetTransactions(ctx context.Context, walletID string, page, size int) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/float/wallets/%s/transactions?page=%d&size=%d", c.baseURL, walletID, page, size)
	return doJSONGet(ctx, c.client, url)
}

func (c *OverdraftClient) SuspendOverdraft(ctx context.Context, facilityID string) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/float/facilities/%s/suspend", c.baseURL, facilityID)
	return doJSONPost(ctx, c.client, url, nil)
}

func (c *OverdraftClient) GetCharges(ctx context.Context, facilityID string) ([]map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/float/facilities/%s/charges", c.baseURL, facilityID)
	return doJSONGetList(ctx, c.client, url)
}
