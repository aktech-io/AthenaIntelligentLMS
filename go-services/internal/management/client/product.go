// Package client holds outbound service-to-service HTTP clients for the loan
// management service.
package client

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/httputil"
)

// ProductClient fetches loan-product penalty terms from product-service.
// It uses the shared internal-service HTTP client (service-key auth headers,
// 30s timeout) like the origination/overdraft clients.
type ProductClient struct {
	client  *httputil.ServiceClient
	baseURL string
	logger  *zap.Logger
}

// NewProductClient creates a new ProductClient.
func NewProductClient(serviceKey, baseURL string, logger *zap.Logger) *ProductClient {
	return &ProductClient{
		client:  httputil.NewServiceClient(serviceKey),
		baseURL: baseURL,
		logger:  logger,
	}
}

// NewProductClientFromEnv builds a ProductClient from the standard environment
// (PRODUCT_SERVICE_URL, LMS_INTERNAL_SERVICE_KEY). The URL default matches the
// origination service's wiring for the k3s deployment.
func NewProductClientFromEnv(logger *zap.Logger) *ProductClient {
	baseURL := os.Getenv("PRODUCT_SERVICE_URL")
	if baseURL == "" {
		baseURL = "http://product-service.lms.svc.cluster.local:8087"
	}
	return NewProductClient(os.Getenv("LMS_INTERNAL_SERVICE_KEY"), baseURL, logger)
}

// PenaltyTerms holds a loan product's penalty configuration.
type PenaltyTerms struct {
	PenaltyRate      *decimal.Decimal // % per annum
	PenaltyGraceDays *int
}

// productPenaltyResponse is the partial product-service response
// (internal/product/model.LoanProductResponse).
type productPenaltyResponse struct {
	PenaltyRate      *decimal.Decimal `json:"penaltyRate"`
	PenaltyGraceDays *int             `json:"penaltyGraceDays"`
}

// GetPenaltyTerms fetches a product's penaltyRate and penaltyGraceDays.
//
// It fails CLOSED: any transport error or non-2xx response returns an error —
// it never fabricates terms. Callers decide how to degrade (loan activation
// stores NULL and continues; the accrual job skips the loan and retries the
// backfill on the next run).
func (c *ProductClient) GetPenaltyTerms(ctx context.Context, productID uuid.UUID) (*PenaltyTerms, error) {
	if productID == uuid.Nil {
		return nil, fmt.Errorf("productId must not be nil")
	}

	url := fmt.Sprintf("%s/api/v1/products/%s", c.baseURL, productID)
	var resp productPenaltyResponse
	if err := c.client.Get(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("fetch penalty terms for product %s: %w", productID, err)
	}

	return &PenaltyTerms{
		PenaltyRate:      resp.PenaltyRate,
		PenaltyGraceDays: resp.PenaltyGraceDays,
	}, nil
}
