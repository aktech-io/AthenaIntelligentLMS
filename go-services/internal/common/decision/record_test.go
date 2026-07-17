package decision

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewRecord_CarriesFullContract(t *testing.T) {
	score := 712.0
	at := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	req := Request{
		Type:        "overdraft.facility",
		TenantID:    "t1",
		SubjectType: "wallet",
		SubjectID:   "w-9",
		CustomerID:  "c-9",
		Actor:       Actor{Type: ActorHuman, ID: "officer-1"},
		Inputs:      map[string]any{"band": "B", "score": 712},
		Models:      []ModelRef{{Name: "credit_score", Version: "v3", Available: true, Score: &score, ScoredAt: &at}},
	}
	out := Outcome{
		Decision:  Approve,
		Detail:    map[string]any{"band": "B", "limit": 50000},
		Reasons:   []Reason{},
		Policy:    PolicyRef{ID: "overdraft.facility", Version: 4, Hash: "sha256:abc"},
		Models:    req.Models,
		Variant:   VariantChampion,
		LatencyMS: 0.42,
	}
	decidedAt := time.Date(2026, 7, 17, 9, 30, 0, 0, time.FixedZone("EAT", 3*3600))

	rec := NewRecord(req, out, decidedAt)

	if rec.DecisionType != "overdraft.facility" || rec.TenantID != "t1" ||
		rec.SubjectType != "wallet" || rec.SubjectID != "w-9" || rec.CustomerID != "c-9" {
		t.Errorf("who/what fields wrong: %+v", rec)
	}
	if rec.ActorType != ActorHuman || rec.ActorID != "officer-1" {
		t.Errorf("actor = %s/%s, want HUMAN/officer-1", rec.ActorType, rec.ActorID)
	}
	if rec.PolicyID != "overdraft.facility" || rec.PolicyVersion != 4 || rec.PolicyHash != "sha256:abc" {
		t.Errorf("policy pin wrong: %+v", rec)
	}
	if rec.Outcome != Approve || rec.OutcomeDetail["limit"] != 50000 || rec.Variant != VariantChampion {
		t.Errorf("outcome fields wrong: %+v", rec)
	}
	if rec.LatencyMS != 0.42 {
		t.Errorf("latency = %v, want 0.42", rec.LatencyMS)
	}
	if !rec.DecidedAt.Equal(decidedAt) || rec.DecidedAt.Location() != time.UTC {
		t.Errorf("decidedAt = %v, want UTC-normalized %v", rec.DecidedAt, decidedAt)
	}

	// The JSON field names are the wire contract with the decision_log
	// projection — pin them so a rename is a deliberate act.
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"decisionType", "tenantId", "subjectType", "subjectId", "customerId",
		"actorType", "actorId", "policyId", "policyVersion", "policyHash",
		"inputs", "outcome", "reasons", "models", "variant", "latencyMs", "decidedAt",
	} {
		if _, ok := m[key]; !ok {
			t.Errorf("decision.recorded payload missing key %q", key)
		}
	}
}
