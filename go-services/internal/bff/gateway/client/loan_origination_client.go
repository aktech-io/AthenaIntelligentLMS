package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/athena-lms/go-services/internal/common/auth"
)

type LoanOriginationClient struct {
	baseURL string
	client  *http.Client
}

func NewLoanOriginationClient(baseURL, serviceKey string) *LoanOriginationClient {
	return &LoanOriginationClient{
		baseURL: baseURL,
		client: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "mobile-gateway",
			},
		},
	}
}

func (c *LoanOriginationClient) ApplyForLoan(ctx context.Context, body map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/loan-applications", c.baseURL)
	return doJSONPost(ctx, c.client, url, body)
}
