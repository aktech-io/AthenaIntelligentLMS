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

type PaymentClient struct {
	baseURL string
	client  *http.Client
}

func NewPaymentClient(baseURL, serviceKey string) *PaymentClient {
	return &PaymentClient{
		baseURL: baseURL,
		client: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "shop-service",
			},
		},
	}
}

type InitiatePaymentRequest struct {
	OrderID     string  `json:"orderId"`
	Amount      float64 `json:"amount"`
	PaymentType string  `json:"paymentType"`
	Reference   string  `json:"reference"`
	CustomerID  string  `json:"customerId"`
}

type InitiatePaymentResponse struct {
	PaymentID string `json:"paymentId"`
	Status    string `json:"status"`
}

func (c *PaymentClient) InitiatePayment(ctx context.Context, req InitiatePaymentRequest) (*InitiatePaymentResponse, error) {
	body, _ := json.Marshal(req)
	url := fmt.Sprintf("%s/api/v1/payments/", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("payment service unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("payment initiation failed (status %d): %s", resp.StatusCode, string(respBody))
	}
	var result InitiatePaymentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
