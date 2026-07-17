package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/athena-lms/go-services/internal/common/auth"
)

type LoanManagementClient struct {
	baseURL string
	client  *http.Client
}

func NewLoanManagementClient(baseURL, serviceKey string) *LoanManagementClient {
	return &LoanManagementClient{
		baseURL: baseURL,
		client: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "mobile-gateway",
			},
		},
	}
}

func (c *LoanManagementClient) GetActiveLoans(ctx context.Context, customerID string) ([]map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/loans/customer/%s?status=ACTIVE", c.baseURL, customerID)
	return doJSONGetList(ctx, c.client, url)
}

func (c *LoanManagementClient) GetLoanSchedule(ctx context.Context, loanID string) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/loans/%s/schedule", c.baseURL, loanID)
	return doJSONGet(ctx, c.client, url)
}

func (c *LoanManagementClient) MakeRepayment(ctx context.Context, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/loans/repayment", c.baseURL)
	return doJSONPost(ctx, c.client, url, body)
}
