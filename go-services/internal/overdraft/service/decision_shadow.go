package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/decision"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/overdraft/client"
)

// E1 v1 (design §5 increment 1, §6): ApplyOverdraft evaluates the
// overdraft.facility policy in SHADOW. The legacy credit_band_configs path
// remains the enforced decision — byte-for-byte unchanged — while the shadow
// outcome is recorded as decision.recorded through the overdraft outbox in
// the same transaction as facility creation, so the shadow-vs-actual diff can
// soak before enforcement flips (config-gated, not in v1).
//
// Shadow failure semantics: an evaluation failure must never affect the money
// path — it is logged and the application proceeds without a record. Only the
// outbox INSERT itself shares the transaction's fate, which is the point:
// facility and decision record commit atomically or not at all.

const decisionSource = "overdraft-service"

// SetDecisionEvaluator wires the decision spine evaluator (nil leaves shadow
// evaluation off, e.g. in unit tests).
func (s *WalletService) SetDecisionEvaluator(e *decision.Evaluator) {
	s.decisionEval = e
}

// buildOverdraftDecisionRequest maps one overdraft application onto the
// decision-spine request: the full input snapshot plus the scoring model's
// governance metadata. A score whose provenance says "mock" was fabricated by
// the upstream fail-open (design §1.3-2) and is marked Available=false so the
// policy's declared on_model_unavailable outcome applies — the shadow log
// records MODEL_UNAVAILABLE instead of trusting the fabricated score.
func buildOverdraftDecisionRequest(tenantID string, walletID uuid.UUID, customerID string, score client.CreditScoreResult) decision.Request {
	modelScore := float64(score.Score)
	inputs := map[string]any{
		"band":  score.Band,
		"score": score.Score,
	}
	if score.LlmProvider != "" {
		inputs["scoreProvider"] = score.LlmProvider
	}
	if score.LlmModel != "" {
		inputs["scoreModel"] = score.LlmModel
	}
	if score.ScoredAt != nil {
		inputs["scoredAt"] = score.ScoredAt.UTC().Format(time.RFC3339Nano)
	}
	return decision.Request{
		Type:        "overdraft.facility",
		TenantID:    tenantID,
		SubjectType: "wallet",
		SubjectID:   walletID.String(),
		CustomerID:  customerID,
		Actor:       decision.SystemActor(decisionSource),
		Inputs:      inputs,
		Models: []decision.ModelRef{{
			Name:      "credit_score",
			Version:   score.LlmModel,
			Role:      "scoring",
			Score:     &modelScore,
			Available: score.Trusted(),
			ScoredAt:  score.ScoredAt,
		}},
	}
}

// shadowDecisionEvents evaluates the policy in shadow and returns the
// decision.recorded events to write through the outbox (champion, plus a
// shadow-challenger outcome when the policy declares one). Never fails the
// caller: on evaluation error it logs and returns nil.
func (s *WalletService) shadowDecisionEvents(ctx context.Context, tenantID string, walletID uuid.UUID, customerID string, score client.CreditScoreResult) []*event.DomainEvent {
	if s.decisionEval == nil {
		return nil
	}
	req := buildOverdraftDecisionRequest(tenantID, walletID, customerID, score)
	out, err := s.decisionEval.Evaluate(ctx, req)
	if err != nil {
		s.logger.Warn("Shadow decision evaluation failed; money path unaffected",
			zap.String("walletId", walletID.String()), zap.Error(err))
		return nil
	}
	decidedAt := time.Now()

	var events []*event.DomainEvent
	for _, o := range append([]decision.Outcome{out}, deref(out.Challenger)...) {
		rec := decision.NewRecord(req, o, decidedAt)
		evt, err := event.NewDomainEvent(event.DecisionRecorded, decisionSource, tenantID, "", rec)
		if err != nil {
			s.logger.Warn("Failed to build decision.recorded event", zap.Error(err))
			continue
		}
		events = append(events, evt)
	}
	return events
}

func deref(o *decision.Outcome) []decision.Outcome {
	if o == nil {
		return nil
	}
	return []decision.Outcome{*o}
}
