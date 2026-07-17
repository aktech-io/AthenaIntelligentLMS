package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/scoring/client"
	"github.com/athena-lms/go-services/internal/scoring/event"
	"github.com/athena-lms/go-services/internal/scoring/model"
	"github.com/athena-lms/go-services/internal/scoring/repository"
)

// Service contains the business logic for AI scoring.
type Service struct {
	repo      *repository.Repository
	client    *client.AthenaScoreClient
	publisher *event.Publisher
	logger    *zap.Logger
}

// New creates a new scoring Service.
func New(repo *repository.Repository, client *client.AthenaScoreClient, publisher *event.Publisher, logger *zap.Logger) *Service {
	return &Service{
		repo:      repo,
		client:    client,
		publisher: publisher,
		logger:    logger,
	}
}

// TriggerScoring runs the scoring pipeline for a loan application.
// Idempotent: skips if already COMPLETED.
func (s *Service) TriggerScoring(ctx context.Context, loanApplicationID string, customerID int64, triggerEvent, tenantID string) {
	// Idempotency: skip if already COMPLETED
	existing, err := s.repo.FindLatestRequestByLoan(ctx, loanApplicationID)
	if err != nil {
		s.logger.Error("Failed to check existing scoring request",
			zap.String("loanApplicationId", loanApplicationID), zap.Error(err))
		return
	}
	if existing != nil && existing.Status == model.ScoringStatusCompleted {
		s.logger.Info("Scoring already COMPLETED, skipping",
			zap.String("loanApplicationId", loanApplicationID))
		return
	}

	var req *model.ScoringRequest
	if existing != nil {
		req = existing
	} else {
		req = &model.ScoringRequest{
			TenantID:          tenantID,
			LoanApplicationID: loanApplicationID,
			CustomerID:        customerID,
			TriggerEvent:      &triggerEvent,
			Status:            model.ScoringStatusPending,
		}
		if _, err := s.repo.CreateRequest(ctx, req); err != nil {
			s.logger.Error("Failed to create scoring request",
				zap.String("loanApplicationId", loanApplicationID), zap.Error(err))
			return
		}
	}

	req.Status = model.ScoringStatusInProgress
	if err := s.repo.UpdateRequest(ctx, req); err != nil {
		s.logger.Error("Failed to update scoring request to IN_PROGRESS",
			zap.String("id", req.ID), zap.Error(err))
		return
	}

	score, err := s.client.GetScore(ctx, customerID)
	if err != nil || score == nil {
		req.Status = model.ScoringStatusFailed
		errMsg := "Failed to retrieve score from AthenaCreditScore API"; req.ErrorMessage = &errMsg
		if err != nil {
			errStr := err.Error(); req.ErrorMessage = &errStr
		}
		if updateErr := s.repo.UpdateRequest(ctx, req); updateErr != nil {
			s.logger.Error("Failed to update request to FAILED", zap.Error(updateErr))
		}
		s.logger.Warn("Scoring FAILED",
			zap.String("loanApplicationId", loanApplicationID),
			zap.Int64("customerId", customerID))
		return
	}

	if score.Status == "INSUFFICIENT_DATA" {
		req.Status = model.ScoringStatusSkipped
		msg := "insufficient data for automated scoring — route to manual review"
		req.ErrorMessage = &msg
		if err := s.repo.UpdateRequest(ctx, req); err != nil {
			s.logger.Error("Failed to update request to SKIPPED", zap.Error(err))
		}
		s.logger.Warn("Scoring SKIPPED: insufficient data",
			zap.String("loanApplicationId", loanApplicationID),
			zap.Int64("customerId", customerID),
			zap.String("dataSufficiency", score.DataSufficiency))
		return
	}

	// Normalize the band to the canonical enum before persisting/publishing
	// (the engine emits display labels like "Very Good").
	score.ScoreBand = string(model.ScoreBandFromString(score.ScoreBand))

	reasoningJSON := serializeToJSON(score.Reasoning)
	rawResponseJSON := serializeToJSON(score)

	scoredAt := parseScoredAt(score.ScoredAt)

	result := &model.ScoringResult{
		TenantID:          tenantID,
		RequestID:         req.ID,
		LoanApplicationID: loanApplicationID,
		CustomerID:        customerID,
		BaseScore:         score.BaseScore,
		CrbContribution:   score.CrbContribution,
		LlmAdjustment:     score.LlmAdjustment,
		PdProbability:     score.PdProbability,
		FinalScore:        score.FinalScore,
		ScoreBand:         score.ScoreBand,
		Reasoning:         reasoningJSON,
		LlmProvider:       score.LlmProvider,
		LlmModel:          score.LlmModel,
		RawResponse:       rawResponseJSON,
		ScoredAt:          scoredAt,
	}

	if _, err := s.repo.CreateResult(ctx, result); err != nil {
		s.logger.Error("Failed to save scoring result",
			zap.String("loanApplicationId", loanApplicationID), zap.Error(err))
		req.Status = model.ScoringStatusFailed
		saveErr := "Failed to save scoring result: " + err.Error(); req.ErrorMessage = &saveErr
		s.repo.UpdateRequest(ctx, req)
		return
	}

	now := time.Now().UTC()
	req.Status = model.ScoringStatusCompleted
	req.CompletedAt = &now
	if err := s.repo.UpdateRequest(ctx, req); err != nil {
		s.logger.Error("Failed to update request to COMPLETED", zap.Error(err))
	}

	s.publisher.PublishCreditAssessed(ctx,
		loanApplicationID, customerID,
		score.FinalScore, score.ScoreBand,
		score.PdProbability, tenantID,
	)

	s.logger.Info("Scoring COMPLETED",
		zap.String("loanApplicationId", loanApplicationID),
		zap.String("finalScore", score.FinalScore.String()),
		zap.String("scoreBand", score.ScoreBand),
	)
}

// GetRequest returns a ScoringRequest by ID.
func (s *Service) GetRequest(ctx context.Context, id, tenantID string) (*model.ScoringRequestResponse, error) {
	req, err := s.repo.FindRequestByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, nil
	}
	resp := toRequestResponse(req)
	return &resp, nil
}

// GetRequestByApplication returns the latest ScoringRequest for a loan application.
func (s *Service) GetRequestByApplication(ctx context.Context, applicationID, tenantID string) (*model.ScoringRequestResponse, error) {
	req, err := s.repo.FindLatestRequestByLoan(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, nil
	}
	resp := toRequestResponse(req)
	return &resp, nil
}

// GetResultByApplication returns the latest ScoringResult for a loan application.
func (s *Service) GetResultByApplication(ctx context.Context, applicationID, tenantID string) (*model.ScoringResultResponse, error) {
	res, err := s.repo.FindLatestResultByLoan(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	resp := toResultResponse(res)
	return &resp, nil
}

// GetLatestResultByCustomer returns the latest ScoringResult for a customer.
func (s *Service) GetLatestResultByCustomer(ctx context.Context, customerID int64, tenantID string) (*model.ScoringResultResponse, error) {
	res, err := s.repo.FindLatestResultByCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	resp := toResultResponse(res)
	return &resp, nil
}

// ListRequests returns paginated ScoringRequests for a tenant.
func (s *Service) ListRequests(ctx context.Context, tenantID string, page, size int) ([]model.ScoringRequestResponse, int64, error) {
	requests, total, err := s.repo.ListRequestsByTenant(ctx, tenantID, page, size)
	if err != nil {
		return nil, 0, err
	}
	responses := make([]model.ScoringRequestResponse, len(requests))
	for i, req := range requests {
		responses[i] = toRequestResponse(&req)
	}
	return responses, total, nil
}

// ManualScore creates a new scoring request and triggers scoring.
func (s *Service) ManualScore(ctx context.Context, req *model.ManualScoringRequest, tenantID string) (*model.ScoringRequestResponse, error) {
	triggerEvent := req.TriggerEvent
	if triggerEvent == "" {
		triggerEvent = "MANUAL"
	}

	scoringReq := &model.ScoringRequest{
		TenantID:          tenantID,
		LoanApplicationID: req.LoanApplicationID,
		CustomerID:        req.CustomerID,
		Status:            model.ScoringStatusPending,
		TriggerEvent:      &triggerEvent,
	}

	if _, err := s.repo.CreateRequest(ctx, scoringReq); err != nil {
		return nil, err
	}

	s.TriggerScoring(ctx, req.LoanApplicationID, req.CustomerID, triggerEvent, tenantID)

	// Re-fetch the request to get the latest status
	updated, err := s.repo.FindRequestByID(ctx, scoringReq.ID)
	if err != nil || updated == nil {
		resp := toRequestResponse(scoringReq)
		return &resp, nil
	}
	resp := toRequestResponse(updated)
	return &resp, nil
}

// --- Mapping helpers ---

func toRequestResponse(req *model.ScoringRequest) model.ScoringRequestResponse {
	return model.ScoringRequestResponse{
		ID:                req.ID,
		TenantID:          req.TenantID,
		LoanApplicationID: req.LoanApplicationID,
		CustomerID:        req.CustomerID,
		Status:            req.Status,
		TriggerEvent:      req.TriggerEvent,
		RequestedAt:       req.RequestedAt,
		CompletedAt:       req.CompletedAt,
		ErrorMessage:      strPtrVal(req.ErrorMessage),
		CreatedAt:         req.CreatedAt,
	}
}

func toResultResponse(res *model.ScoringResult) model.ScoringResultResponse {
	return model.ScoringResultResponse{
		ID:                res.ID,
		RequestID:         res.RequestID,
		LoanApplicationID: res.LoanApplicationID,
		CustomerID:        res.CustomerID,
		BaseScore:         res.BaseScore,
		CrbContribution:   res.CrbContribution,
		LlmAdjustment:     res.LlmAdjustment,
		PdProbability:     res.PdProbability,
		FinalScore:        res.FinalScore,
		ScoreBand:         res.ScoreBand,
		LlmProvider:       res.LlmProvider,
		LlmModel:          res.LlmModel,
		ScoredAt:          res.ScoredAt,
		CreatedAt:         res.CreatedAt,
		Reasoning:         parseReasoningList(res.Reasoning),
	}
}

func parseReasoningList(reasoningJSON string) []string {
	if reasoningJSON == "" {
		return []string{}
	}
	var list []string
	if err := json.Unmarshal([]byte(reasoningJSON), &list); err != nil {
		// If not valid JSON array, return as a single-element list
		return []string{reasoningJSON}
	}
	return list
}

func serializeToJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func parseScoredAt(s string) *time.Time {
	if s == "" {
		return nil
	}
	// Try RFC3339 first
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return &t
	}
	// Try RFC3339Nano
	t, err = time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return &t
	}
	// Try date-only
	t, err = time.Parse("2006-01-02", strings.TrimSpace(s))
	if err == nil {
		return &t
	}
	now := time.Now().UTC()
	return &now
}

func strPtrVal(s *string) string {
	if s == nil { return "" }
	return *s
}
