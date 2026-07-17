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

type LoanOriginationClient struct {
	baseURL       string
	client        *http.Client
	bnplProductID string
}

func NewLoanOriginationClient(baseURL, serviceKey, bnplProductID string) *LoanOriginationClient {
	return &LoanOriginationClient{
		baseURL:       baseURL,
		bnplProductID: bnplProductID,
		client: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "shop-service",
			},
		},
	}
}

type CreateLoanApplicationRequest struct {
	ProductID       string  `json:"productId"`
	CustomerID      string  `json:"customerId"`
	RequestedAmount float64 `json:"requestedAmount"`
	Purpose         string  `json:"purpose"`
	OrderReference  string  `json:"orderReference"`
}

type LoanApplicationResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (c *LoanOriginationClient) CreateAndSubmitLoanApplication(ctx context.Context, customerID string, amount float64, orderRef string) (*LoanApplicationResponse, error) {
	// Step 1: Create loan application.
	createReq := CreateLoanApplicationRequest{
		ProductID:       c.bnplProductID,
		CustomerID:      customerID,
		RequestedAmount: amount,
		Purpose:         "BNPL purchase - " + orderRef,
		OrderReference:  orderRef,
	}
	body, _ := json.Marshal(createReq)
	url := fmt.Sprintf("%s/api/v1/loan-applications", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("loan origination service unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("loan application creation failed (status %d): %s", resp.StatusCode, string(respBody))
	}
	var createResp LoanApplicationResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return nil, err
	}

	// Step 2: Submit the loan application.
	submitURL := fmt.Sprintf("%s/api/v1/loan-applications/%s/submit", c.baseURL, createResp.ID)
	submitReq, err := http.NewRequestWithContext(ctx, http.MethodPost, submitURL, nil)
	if err != nil {
		return nil, err
	}
	submitReq.Header.Set("Content-Type", "application/json")
	submitResp, err := c.client.Do(submitReq)
	if err != nil {
		return nil, fmt.Errorf("loan application submit failed: %w", err)
	}
	defer submitResp.Body.Close()
	if submitResp.StatusCode != http.StatusOK && submitResp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(submitResp.Body)
		return nil, fmt.Errorf("loan application submit failed (status %d): %s", submitResp.StatusCode, string(respBody))
	}
	var submitResult LoanApplicationResponse
	if err := json.NewDecoder(submitResp.Body).Decode(&submitResult); err != nil {
		// If decode fails, return the create response (submit may not return a body).
		return &createResp, nil
	}
	return &submitResult, nil
}
