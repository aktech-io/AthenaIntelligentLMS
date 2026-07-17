// Package model defines the decision-service read model: one row of the
// append-only decision_log projection (design §2.4).
package model

import (
	"encoding/json"
	"time"
)

// Decision is one projected decision_log row. JSONB columns pass through as
// raw JSON — the projection never reinterprets what the producer recorded.
type Decision struct {
	ID               string          `json:"id"` // decision.recorded event id
	TenantID         string          `json:"tenantId"`
	DecisionType     string          `json:"decisionType"`
	SubjectType      string          `json:"subjectType"`
	SubjectID        string          `json:"subjectId"`
	CustomerID       *string         `json:"customerId,omitempty"`
	ActorType        string          `json:"actorType"`
	ActorID          string          `json:"actorId"`
	PolicyID         string          `json:"policyId"`
	PolicyVersion    int             `json:"policyVersion"`
	PolicyHash       string          `json:"policyHash"`
	Inputs           json.RawMessage `json:"inputs"`
	Outcome          string          `json:"outcome"`
	OutcomeDetail    json.RawMessage `json:"outcomeDetail,omitempty"`
	Reasons          json.RawMessage `json:"reasons"`
	Models           json.RawMessage `json:"models"`
	Variant          string          `json:"variant"`
	ParentDecisionID *string         `json:"parentDecisionId,omitempty"`
	LatencyMS        *float64        `json:"latencyMs,omitempty"`
	CorrelationID    *string         `json:"correlationId,omitempty"`
	DecidedAt        time.Time       `json:"decidedAt"`
	RecordedAt       time.Time       `json:"recordedAt"`
}

// ListFilter narrows GET /api/v1/decisions. TenantID is always enforced from
// the caller's auth context — the log is tenant-scoped by construction.
type ListFilter struct {
	TenantID     string
	DecisionType string
	SubjectID    string
	CustomerID   string
	Outcome      string
	Variant      string
	From         *time.Time
	To           *time.Time
	Page         int
	Size         int
}
