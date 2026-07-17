package decision

import (
	"time"
)

// Record is the payload of a decision.recorded event — the single write
// contract between every producing service and the decision-service
// projection into decision_log (design §2.4). The DomainEvent envelope
// contributes the identity (event id = decision_log.id, the idempotency key),
// the tenant and the correlation id; Record carries everything else the
// regulator's one-SELECT needs: who/what/inputs/policy/outcome/why/how-fast.
type Record struct {
	DecisionType  string         `json:"decisionType"`
	TenantID      string         `json:"tenantId"`
	SubjectType   string         `json:"subjectType"`
	SubjectID     string         `json:"subjectId"`
	CustomerID    string         `json:"customerId,omitempty"`
	ActorType     string         `json:"actorType"`
	ActorID       string         `json:"actorId"`
	PolicyID      string         `json:"policyId"`
	PolicyVersion int            `json:"policyVersion"`
	PolicyHash    string         `json:"policyHash"`
	Inputs        map[string]any `json:"inputs"`
	Outcome       string         `json:"outcome"`
	OutcomeDetail map[string]any `json:"outcomeDetail,omitempty"`
	Reasons       []Reason       `json:"reasons"`
	Models        []ModelRef     `json:"models"`
	Variant       string         `json:"variant"`
	// ParentDecisionID links a human review verdict back to the REFER that
	// queued it (event id of the parent decision). Empty for first decisions.
	ParentDecisionID string    `json:"parentDecisionId,omitempty"`
	LatencyMS        float64   `json:"latencyMs"`
	DecidedAt        time.Time `json:"decidedAt"`
}

// NewRecord builds the decision.recorded payload for one evaluated outcome.
// For a shadow challenger outcome (out.Challenger), call NewRecord again with
// that outcome — each recorded outcome is its own event.
func NewRecord(req Request, out Outcome, decidedAt time.Time) Record {
	return Record{
		DecisionType:  req.Type,
		TenantID:      req.TenantID,
		SubjectType:   req.SubjectType,
		SubjectID:     req.SubjectID,
		CustomerID:    req.CustomerID,
		ActorType:     req.Actor.Type,
		ActorID:       req.Actor.ID,
		PolicyID:      out.Policy.ID,
		PolicyVersion: out.Policy.Version,
		PolicyHash:    out.Policy.Hash,
		Inputs:        req.Inputs,
		Outcome:       out.Decision,
		OutcomeDetail: out.Detail,
		Reasons:       out.Reasons,
		Models:        out.Models,
		Variant:       out.Variant,
		LatencyMS:     out.LatencyMS,
		DecidedAt:     decidedAt.UTC(),
	}
}
