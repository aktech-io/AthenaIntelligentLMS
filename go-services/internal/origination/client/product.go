package client

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/httputil"
)

// ProductClient calls product-service to validate products and fetch schedule config.
type ProductClient struct {
	client *httputil.ServiceClient
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

// AmountLimits holds the min and max amount from a product.
type AmountLimits struct {
	MinAmount *decimal.Decimal
	MaxAmount *decimal.Decimal
}

// ScheduleConfig holds the schedule configuration from a product.
type ScheduleConfig struct {
	ScheduleType       *string
	RepaymentFrequency *string
}

// AuthConfig holds the per-product maker-checker override from a product.
type AuthConfig struct {
	RequiresTwoPersonAuth bool
	AuthThresholdAmount   *decimal.Decimal
}

// productResponse is the partial response from product-service.
type productResponse struct {
	Status                string                 `json:"status"`
	MinAmount             *decimal.Decimal       `json:"minAmount"`
	MaxAmount             *decimal.Decimal       `json:"maxAmount"`
	ScheduleType          *string                `json:"scheduleType"`
	RepaymentFrequency    *string                `json:"repaymentFrequency"`
	RequiresTwoPersonAuth bool                   `json:"requiresTwoPersonAuth"`
	AuthThresholdAmount   *decimal.Decimal       `json:"authThresholdAmount"`
	Configuration         map[string]interface{} `json:"configuration"`
}

// ValidateAndGetAmountLimits validates that the product is ACTIVE and returns its amount limits.
// Fails open on network/auth errors.
func (c *ProductClient) ValidateAndGetAmountLimits(ctx context.Context, productID uuid.UUID) (*AmountLimits, error) {
	if productID == uuid.Nil {
		return nil, fmt.Errorf("productId must not be null")
	}

	url := fmt.Sprintf("%s/api/v1/products/%s", c.baseURL, productID)
	var resp productResponse
	err := c.client.Get(ctx, url, &resp)
	if err != nil {
		c.logger.Warn("Product service unavailable, skipping validation",
			zap.String("productId", productID.String()),
			zap.Error(err))
		return &AmountLimits{}, nil
	}

	if resp.Status != "ACTIVE" {
		return nil, fmt.Errorf("product %s is not available for new applications (status=%s)", productID, resp.Status)
	}

	return &AmountLimits{
		MinAmount: resp.MinAmount,
		MaxAmount: resp.MaxAmount,
	}, nil
}

// GetProductScheduleConfig fetches the schedule configuration for a product.
// Returns nil values on failure (fail-open).
func (c *ProductClient) GetProductScheduleConfig(ctx context.Context, productID uuid.UUID) *ScheduleConfig {
	if productID == uuid.Nil {
		return &ScheduleConfig{}
	}

	url := fmt.Sprintf("%s/api/v1/products/%s", c.baseURL, productID)
	var resp productResponse
	err := c.client.Get(ctx, url, &resp)
	if err != nil {
		c.logger.Warn("Could not fetch schedule config",
			zap.String("productId", productID.String()),
			zap.Error(err))
		return &ScheduleConfig{}
	}

	sc := &ScheduleConfig{
		ScheduleType:       resp.ScheduleType,
		RepaymentFrequency: resp.RepaymentFrequency,
	}

	// Check nested configuration map
	if resp.Configuration != nil {
		if sc.ScheduleType == nil {
			if v, ok := resp.Configuration["scheduleType"].(string); ok {
				sc.ScheduleType = &v
			}
		}
		if sc.RepaymentFrequency == nil {
			if v, ok := resp.Configuration["repaymentFrequency"].(string); ok {
				sc.RepaymentFrequency = &v
			}
		}
	}

	return sc
}

// GetProductAuthConfig fetches the per-product maker-checker override for a product.
// Returns a zero-value AuthConfig on any error (fail-open: never fails the caller).
func (c *ProductClient) GetProductAuthConfig(ctx context.Context, productID uuid.UUID) *AuthConfig {
	if productID == uuid.Nil {
		return &AuthConfig{}
	}

	url := fmt.Sprintf("%s/api/v1/products/%s", c.baseURL, productID)
	var resp productResponse
	err := c.client.Get(ctx, url, &resp)
	if err != nil {
		c.logger.Warn("Could not fetch product auth config",
			zap.String("productId", productID.String()),
			zap.Error(err))
		return &AuthConfig{}
	}

	return &AuthConfig{
		RequiresTwoPersonAuth: resp.RequiresTwoPersonAuth,
		AuthThresholdAmount:   resp.AuthThresholdAmount,
	}
}
