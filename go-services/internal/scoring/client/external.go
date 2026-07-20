package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/scoring/model"
)

// AthenaScoreClient calls the external NemoScore (AthenaCreditScore) API.
//
// Fail-closed by design: any transport error, non-2xx status, or decode
// failure is returned as an error. Callers must treat a missing score as
// "no automated decision possible" (manual review), never fabricate one.
type AthenaScoreClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
	logger  *zap.Logger
}

// NewAthenaScoreClient creates a new external scoring client.
// apiKey is sent as X-Api-Key and must match a NemoScore SERVICE_API_KEYS entry.
func NewAthenaScoreClient(baseURL, apiKey string, logger *zap.Logger) *AthenaScoreClient {
	return &AthenaScoreClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		logger: logger,
	}
}

// GetScore retrieves a credit score for the given customer.
// Returns an error if the scoring API is unavailable or returns an
// unusable response — there is intentionally no mock fallback.
func (c *AthenaScoreClient) GetScore(ctx context.Context, customerID int64) (*model.ExternalScoreResponse, error) {
	url := fmt.Sprintf("%s/api/v1/credit-score/%d", c.baseURL, customerID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("scoring API request build failed: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Error("Scoring API unreachable",
			zap.Int64("customerId", customerID), zap.String("url", url), zap.Error(err))
		return nil, fmt.Errorf("scoring API unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no score available for customer %d", customerID)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error("Scoring API returned non-2xx",
			zap.Int64("customerId", customerID), zap.Int("status", resp.StatusCode))
		return nil, fmt.Errorf("scoring API returned status %d", resp.StatusCode)
	}

	var score model.ExternalScoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&score); err != nil {
		return nil, fmt.Errorf("scoring API response decode failed: %w", err)
	}
	score.CustomerID = customerID

	c.logger.Info("Got credit score for customer",
		zap.Int64("customerId", customerID),
		zap.String("finalScore", score.FinalScore.String()),
		zap.String("status", score.Status))
	return &score, nil
}

// CollectionsPriority computes a collections-priority score for a delinquent
// case (NemoScore contract 1.5.0). Same fail-closed stance as GetScore: any
// error (including 404 no-score and 409 INSUFFICIENT_DATA) means the caller
// must keep its own DPD-threshold prioritization — never fabricate a band.
func (c *AthenaScoreClient) CollectionsPriority(ctx context.Context, customerID int64, reqBody model.CollectionsPriorityRequest) (*model.CollectionsPriorityResponse, error) {
	url := fmt.Sprintf("%s/api/v1/credit-score/%d/collections-priority", c.baseURL, customerID)

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("collections-priority request marshal failed: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("collections-priority request build failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Warn("Collections-priority API unreachable",
			zap.Int64("customerId", customerID), zap.Error(err))
		return nil, fmt.Errorf("collections-priority API unreachable: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("no score available for customer %d", customerID)
	case resp.StatusCode == http.StatusConflict:
		return nil, fmt.Errorf("customer %d score is INSUFFICIENT_DATA", customerID)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		c.logger.Warn("Collections-priority API returned non-2xx",
			zap.Int64("customerId", customerID), zap.Int("status", resp.StatusCode))
		return nil, fmt.Errorf("collections-priority API returned status %d", resp.StatusCode)
	}

	var out model.CollectionsPriorityResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("collections-priority response decode failed: %w", err)
	}
	c.logger.Info("Got collections priority",
		zap.Int64("customerId", customerID),
		zap.Float64("priorityScore", out.PriorityScore),
		zap.String("priorityBand", out.PriorityBand))
	return &out, nil
}
