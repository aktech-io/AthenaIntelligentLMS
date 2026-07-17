package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/athena-lms/go-services/internal/common/auth"
)

type ProductClient struct {
	baseURL string
	client  *http.Client
}

func NewProductClient(baseURL, serviceKey string) *ProductClient {
	return &ProductClient{
		baseURL: baseURL,
		client: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "mobile-gateway",
			},
		},
	}
}

func (c *ProductClient) GetLoanProducts(ctx context.Context) ([]map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/products/?page=0&size=50", c.baseURL)
	// Product service returns paginated response {content:[], page, size, ...}.
	// Extract the "content" array.
	resp, err := doJSONGet(ctx, c.client, url)
	if err != nil {
		return nil, fmt.Errorf("get loan products: %w", err)
	}
	if content, ok := resp["content"]; ok {
		if list, ok := content.([]any); ok {
			result := make([]map[string]any, 0, len(list))
			for _, item := range list {
				if m, ok := item.(map[string]any); ok {
					result = append(result, m)
				}
			}
			return result, nil
		}
	}
	return []map[string]any{}, nil
}
