package client

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/httputil"
)

// CreditScoreResult holds the credit score and band for a customer.
type CreditScoreResult struct {
	Score int    `json:"score"`
	Band  string `json:"band"`
}

// ScoringClient fetches credit scores from the AI scoring service.
type ScoringClient struct {
	baseURL string
	client  *httputil.ServiceClient
	logger  *zap.Logger
}

// NewScoringClient creates a new ScoringClient.
func NewScoringClient(baseURL, serviceKey string, logger *zap.Logger) *ScoringClient {
	return &ScoringClient{
		baseURL: baseURL,
		client:  httputil.NewServiceClient(serviceKey),
		logger:  logger,
	}
}

// GetLatestScore fetches the latest credit score for a customer.
//
// It fails CLOSED (HIGH-6): any transport error, non-2xx response, or
// incomplete payload returns an error so callers reject the credit decision.
// It must never fabricate a score — a mocked score here would silently approve
// real credit facilities whenever the scoring service is down or misconfigured.
func (s *ScoringClient) GetLatestScore(ctx context.Context, customerID string) (CreditScoreResult, error) {
	url := fmt.Sprintf("%s/api/v1/scoring/customers/%s/latest", s.baseURL, customerID)

	var resp struct {
		FinalScore *int    `json:"finalScore"`
		ScoreBand  *string `json:"scoreBand"`
	}

	if err := s.client.Get(ctx, url, &resp); err != nil {
		s.logger.Warn("AI scoring unavailable",
			zap.String("customerId", customerID),
			zap.Error(err))
		return CreditScoreResult{}, fmt.Errorf("fetch credit score for customer %s: %w", customerID, err)
	}

	if resp.FinalScore == nil || resp.ScoreBand == nil {
		s.logger.Warn("Incomplete scoring response", zap.String("customerId", customerID))
		return CreditScoreResult{}, fmt.Errorf("incomplete scoring response for customer %s", customerID)
	}

	s.logger.Info("Got credit score",
		zap.String("customerId", customerID),
		zap.Int("score", *resp.FinalScore),
		zap.String("band", *resp.ScoreBand))
	return CreditScoreResult{Score: *resp.FinalScore, Band: *resp.ScoreBand}, nil
}
