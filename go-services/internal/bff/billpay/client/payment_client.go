package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/common/auth"
)

// PaymentClient calls the payment service for payment initiation.
type PaymentClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPaymentClient creates a PaymentClient with service-key authentication.
func NewPaymentClient(baseURL, serviceKey string) *PaymentClient {
	return &PaymentClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "billpay-savings-service",
			},
		},
	}
}

type paymentRequest struct {
	TenantID        string  `json:"tenantId"`
	SourceAccountID string  `json:"sourceAccountId"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	Reference       string  `json:"reference"`
	Description     string  `json:"description"`
	PaymentType     string  `json:"paymentType"`
	Channel         string  `json:"channel"`
}

type paymentResponse struct {
	ID string `json:"id"`
}

// InitiatePayment creates a payment and returns the payment ID.
func (c *PaymentClient) InitiatePayment(ctx context.Context, tenantID string, sourceAccountID uuid.UUID, amount float64, reference, description string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/payments", c.baseURL)
	body := paymentRequest{
		TenantID:        tenantID,
		SourceAccountID: sourceAccountID.String(),
		Amount:          amount,
		Currency:        "KES",
		Reference:       reference,
		Description:     description,
		PaymentType:     "BILL_PAYMENT",
		Channel:         "MOBILE_WALLET",
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal payment request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create payment request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("payment call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("payment service returned status %d", resp.StatusCode)
	}

	var pr paymentResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", fmt.Errorf("decode payment response: %w", err)
	}
	return pr.ID, nil
}
