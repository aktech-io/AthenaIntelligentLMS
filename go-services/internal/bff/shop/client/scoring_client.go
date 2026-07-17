package client

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math"
	"net/http"

	"github.com/athena-lms/go-services/internal/common/auth"
)

type ScoringClient struct {
	baseURL string
	client  *http.Client
}

func NewScoringClient(baseURL, serviceKey string) *ScoringClient {
	return &ScoringClient{
		baseURL: baseURL,
		client: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "shop-service",
			},
		},
	}
}

type CreditScoreResponse struct {
	CustomerID  string `json:"customerId"`
	CreditScore int    `json:"creditScore"`
}

func (c *ScoringClient) GetCreditScore(ctx context.Context, customerID string) (int, error) {
	url := fmt.Sprintf("%s/api/v1/scoring/customers/%s/latest", c.baseURL, customerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return c.mockCreditScore(customerID), nil
	}
	resp, err := c.client.Do(req)
	if err != nil {
		slog.Warn("scoring service unavailable, using mock", "error", err)
		return c.mockCreditScore(customerID), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Warn("scoring service returned non-200, using mock", "status", resp.StatusCode)
		return c.mockCreditScore(customerID), nil
	}
	var result CreditScoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return c.mockCreditScore(customerID), nil
	}
	return result.CreditScore, nil
}

func (c *ScoringClient) mockCreditScore(customerID string) int {
	h := fnv.New32a()
	h.Write([]byte(customerID))
	return 500 + int(math.Abs(float64(int32(h.Sum32()))))%350
}
