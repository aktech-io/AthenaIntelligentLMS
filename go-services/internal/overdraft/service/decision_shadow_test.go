package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/decision"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/overdraft/client"
)

func trustedScore(band string, score int) client.CreditScoreResult {
	at := time.Now().Add(-time.Hour)
	return client.CreditScoreResult{
		Score: score, Band: band,
		LlmProvider: "openai", LlmModel: "athena-score-v1", ScoredAt: &at,
	}
}

func mockScore(band string, score int) client.CreditScoreResult {
	at := time.Now().Add(-time.Hour)
	return client.CreditScoreResult{
		Score: score, Band: band,
		LlmProvider: "mock", LlmModel: "deterministic-v1", ScoredAt: &at,
	}
}

func TestBuildOverdraftDecisionRequest(t *testing.T) {
	walletID := uuid.New()
	req := buildOverdraftDecisionRequest("t1", walletID, "CUST-1", trustedScore("B", 700))

	if req.Type != "overdraft.facility" || req.TenantID != "t1" ||
		req.SubjectType != "wallet" || req.SubjectID != walletID.String() || req.CustomerID != "CUST-1" {
		t.Errorf("request identity wrong: %+v", req)
	}
	if req.Actor.Type != decision.ActorSystem || req.Actor.ID != "overdraft-service" {
		t.Errorf("actor = %+v, want SYSTEM/overdraft-service", req.Actor)
	}
	if req.Inputs["band"] != "B" || req.Inputs["score"] != 700 || req.Inputs["scoreProvider"] != "openai" {
		t.Errorf("inputs snapshot wrong: %+v", req.Inputs)
	}
	if len(req.Models) != 1 {
		t.Fatalf("models = %+v, want exactly credit_score", req.Models)
	}
	m := req.Models[0]
	if m.Name != "credit_score" || !m.Available || m.Version != "athena-score-v1" || *m.Score != 700 || m.ScoredAt == nil {
		t.Errorf("model metadata wrong: %+v", m)
	}
}

// A stored score fabricated by the upstream mock fail-open (provider "mock",
// design §1.3-2) must be marked unavailable so the shadow decision records
// MODEL_UNAVAILABLE instead of trusting it.
func TestBuildOverdraftDecisionRequest_MockScoreIsUntrusted(t *testing.T) {
	req := buildOverdraftDecisionRequest("t1", uuid.New(), "CUST-1", mockScore("A", 800))
	if req.Models[0].Available {
		t.Error("fabricated mock score marked Available=true; shadow would trust the fail-open")
	}

	// Unknown/empty provenance (older scoring rows) stays trusted until the
	// scoring API version-header contract lands (increment 2).
	req2 := buildOverdraftDecisionRequest("t1", uuid.New(), "CUST-1", client.CreditScoreResult{Score: 700, Band: "B"})
	if !req2.Models[0].Available {
		t.Error("score with empty provenance treated as unavailable")
	}
}

func shadowWalletService(t *testing.T) *WalletService {
	t.Helper()
	svc := NewWalletService(nil, nil, nil, zap.NewNop())
	svc.SetDecisionEvaluator(decision.NewEvaluator(nil)) // embedded defaults
	return svc
}

func decodeRecord(t *testing.T, evt *event.DomainEvent) decision.Record {
	t.Helper()
	var rec decision.Record
	if err := json.Unmarshal(evt.Payload, &rec); err != nil {
		t.Fatalf("decode decision.recorded payload: %v", err)
	}
	return rec
}

// The shadow path emits exactly one decision.recorded event per application
// (no challenger in the embedded v1 policy), mirroring the legacy outcome on
// a trusted score.
func TestShadowDecisionEvents_TrustedScoreApproves(t *testing.T) {
	svc := shadowWalletService(t)
	walletID := uuid.New()

	events := svc.shadowDecisionEvents(context.Background(), "t1", walletID, "CUST-1", trustedScore("B", 700))
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	evt := events[0]
	if evt.Type != event.DecisionRecorded || evt.TenantID != "t1" {
		t.Errorf("event envelope wrong: type=%s tenant=%s", evt.Type, evt.TenantID)
	}
	rec := decodeRecord(t, evt)
	if rec.Outcome != decision.Approve || rec.Variant != decision.VariantChampion {
		t.Errorf("outcome = %s/%s, want APPROVE/champion", rec.Outcome, rec.Variant)
	}
	// Embedded policy mirrors the seeded band configs: B ⇒ limit 50000.
	if rec.OutcomeDetail["limit"] != float64(50000) {
		t.Errorf("band B shadow limit = %v, want 50000 (parity with credit_band_configs)", rec.OutcomeDetail["limit"])
	}
	if rec.PolicyID != "overdraft.facility" || rec.PolicyVersion != 1 || rec.PolicyHash == "" {
		t.Errorf("policy pin wrong: %s v%d %q", rec.PolicyID, rec.PolicyVersion, rec.PolicyHash)
	}
	if rec.ActorType != decision.ActorSystem || rec.SubjectID != walletID.String() {
		t.Errorf("who/what wrong: %+v", rec)
	}
}

// The fabricated-mock case: legacy still approves off the stored score (money
// path untouched), but the shadow record captures the fail-open as
// MODEL_UNAVAILABLE ⇒ DECLINE per the policy's declared failure semantics.
func TestShadowDecisionEvents_MockScoreRecordsModelUnavailable(t *testing.T) {
	svc := shadowWalletService(t)

	events := svc.shadowDecisionEvents(context.Background(), "t1", uuid.New(), "CUST-1", mockScore("A", 800))
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	rec := decodeRecord(t, events[0])
	if rec.Outcome != decision.Decline {
		t.Fatalf("outcome = %s, want DECLINE (on_model_unavailable)", rec.Outcome)
	}
	if len(rec.Reasons) == 0 || rec.Reasons[0].Code != decision.ReasonModelUnavailable {
		t.Errorf("reasons = %+v, want MODEL_UNAVAILABLE first", rec.Reasons)
	}
	if len(rec.Models) != 1 || rec.Models[0].Available {
		t.Errorf("recorded model = %+v, want credit_score unavailable", rec.Models)
	}
}

// A stale stored score is silently reused by the legacy path; the shadow
// record surfaces it as REFER/SCORE_STALE (policy max_age 30d).
func TestShadowDecisionEvents_StaleScoreRefers(t *testing.T) {
	svc := shadowWalletService(t)
	at := time.Now().Add(-31 * 24 * time.Hour)
	stale := client.CreditScoreResult{Score: 700, Band: "B", LlmProvider: "openai", ScoredAt: &at}

	events := svc.shadowDecisionEvents(context.Background(), "t1", uuid.New(), "CUST-1", stale)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	rec := decodeRecord(t, events[0])
	if rec.Outcome != decision.Refer || rec.Reasons[0].Code != decision.ReasonScoreStale {
		t.Errorf("outcome = %s/%+v, want REFER/SCORE_STALE", rec.Outcome, rec.Reasons)
	}
}

// Shadow failures must never affect the money path: with no evaluator wired,
// or an evaluation error, the application proceeds with zero events.
func TestShadowDecisionEvents_NeverFailsTheCaller(t *testing.T) {
	// No evaluator (unit-test / legacy deployments).
	svc := NewWalletService(nil, nil, nil, zap.NewNop())
	if events := svc.shadowDecisionEvents(context.Background(), "t1", uuid.New(), "C", trustedScore("B", 700)); events != nil {
		t.Errorf("nil evaluator produced events: %+v", events)
	}

	// Evaluator over an empty registry: resolution fails, shadow logs and
	// returns nil instead of erroring.
	svc2 := NewWalletService(nil, nil, nil, zap.NewNop())
	svc2.SetDecisionEvaluator(decision.NewEvaluator(decision.NewRegistry()))
	if events := svc2.shadowDecisionEvents(context.Background(), "t1", uuid.New(), "C", trustedScore("B", 700)); events != nil {
		t.Errorf("failed evaluation produced events: %+v", events)
	}
}
