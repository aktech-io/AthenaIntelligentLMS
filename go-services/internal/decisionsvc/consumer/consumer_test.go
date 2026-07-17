package consumer

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/decision"
	commonEvent "github.com/athena-lms/go-services/internal/common/event"
)

func recordedEvent(t *testing.T) *commonEvent.DomainEvent {
	t.Helper()
	score := 712.0
	at := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	rec := decision.NewRecord(
		decision.Request{
			Type:        "overdraft.facility",
			TenantID:    "t1",
			SubjectType: "wallet",
			SubjectID:   "w-1",
			CustomerID:  "c-1",
			Actor:       decision.SystemActor("overdraft-service"),
			Inputs:      map[string]any{"band": "B", "score": 712},
			Models:      []decision.ModelRef{{Name: "credit_score", Version: "v3", Available: true, Score: &score, ScoredAt: &at}},
		},
		decision.Outcome{
			Decision: decision.Approve,
			Detail:   map[string]any{"band": "B", "limit": 50000},
			Policy:   decision.PolicyRef{ID: "overdraft.facility", Version: 1, Hash: "sha256:abc"},
			Models:   []decision.ModelRef{{Name: "credit_score", Version: "v3", Available: true, Score: &score, ScoredAt: &at}},
			Variant:  decision.VariantChampion,
		},
		time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC),
	)
	evt, err := commonEvent.NewDomainEvent(commonEvent.DecisionRecorded, "overdraft-service", "t1", "corr-1", rec)
	if err != nil {
		t.Fatal(err)
	}
	return evt
}

// Project maps the library's Record wire format onto the decision_log row —
// this is the producer↔projection contract test.
func TestProject_RoundTripsLibraryRecord(t *testing.T) {
	evt := recordedEvent(t)
	d, err := Project(evt)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if d.ID != evt.ID {
		t.Errorf("row id = %s, want event id %s (idempotency key)", d.ID, evt.ID)
	}
	if d.TenantID != "t1" || d.DecisionType != "overdraft.facility" ||
		d.SubjectType != "wallet" || d.SubjectID != "w-1" {
		t.Errorf("who/what wrong: %+v", d)
	}
	if d.CustomerID == nil || *d.CustomerID != "c-1" {
		t.Errorf("customerId = %v, want c-1", d.CustomerID)
	}
	if d.ActorType != decision.ActorSystem || d.ActorID != "overdraft-service" {
		t.Errorf("actor = %s/%s", d.ActorType, d.ActorID)
	}
	if d.PolicyID != "overdraft.facility" || d.PolicyVersion != 1 || d.PolicyHash != "sha256:abc" {
		t.Errorf("policy pin wrong: %+v", d)
	}
	if d.Outcome != "APPROVE" || d.Variant != "champion" {
		t.Errorf("outcome/variant = %s/%s", d.Outcome, d.Variant)
	}
	if d.CorrelationID == nil || *d.CorrelationID != "corr-1" {
		t.Errorf("correlationId = %v, want corr-1", d.CorrelationID)
	}
	var inputs map[string]any
	if err := json.Unmarshal(d.Inputs, &inputs); err != nil || inputs["band"] != "B" {
		t.Errorf("inputs not preserved verbatim: %s", d.Inputs)
	}
	var models []map[string]any
	if err := json.Unmarshal(d.Models, &models); err != nil || len(models) != 1 || models[0]["name"] != "credit_score" {
		t.Errorf("models not preserved: %s", d.Models)
	}
}

// A redelivered event (at-least-once) projects a byte-identical row with the
// same (id, decided_at) primary key, so ON CONFLICT DO NOTHING makes the
// second write a no-op — the unit-level half of projection idempotency.
func TestProject_RedeliveryIsDeterministic(t *testing.T) {
	evt := recordedEvent(t)
	d1, err := Project(evt)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := Project(evt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d1, d2) {
		t.Errorf("redelivered event projected differently:\n%+v\n%+v", d1, d2)
	}
	if d1.ID != evt.ID || !d1.DecidedAt.Equal(d2.DecidedAt) {
		t.Error("idempotency key (id, decided_at) not stable across redeliveries")
	}
}

func TestProject_Validation(t *testing.T) {
	base := recordedEvent(t)

	mutate := func(f func(*commonEvent.DomainEvent, map[string]any)) *commonEvent.DomainEvent {
		var payload map[string]any
		if err := json.Unmarshal(base.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		evt := *base
		f(&evt, payload)
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		evt.Payload = raw
		return &evt
	}

	cases := []struct {
		name string
		evt  *commonEvent.DomainEvent
	}{
		{"non-uuid event id", mutate(func(e *commonEvent.DomainEvent, p map[string]any) { e.ID = "not-a-uuid" })},
		{"missing decisionType", mutate(func(e *commonEvent.DomainEvent, p map[string]any) { delete(p, "decisionType") })},
		{"missing outcome", mutate(func(e *commonEvent.DomainEvent, p map[string]any) { delete(p, "outcome") })},
		{"missing subjectId", mutate(func(e *commonEvent.DomainEvent, p map[string]any) { delete(p, "subjectId") })},
		{"missing decidedAt", mutate(func(e *commonEvent.DomainEvent, p map[string]any) { delete(p, "decidedAt") })},
		{"no tenant anywhere", mutate(func(e *commonEvent.DomainEvent, p map[string]any) {
			e.TenantID = ""
			delete(p, "tenantId")
		})},
		{"bad parentDecisionId", mutate(func(e *commonEvent.DomainEvent, p map[string]any) { p["parentDecisionId"] = "nope" })},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Project(c.evt); err == nil {
				t.Error("want validation error, got nil")
			}
		})
	}

	// Tenant falls back to the envelope when the payload omits it.
	evt := mutate(func(e *commonEvent.DomainEvent, p map[string]any) { delete(p, "tenantId") })
	d, err := Project(evt)
	if err != nil {
		t.Fatalf("envelope tenant fallback: %v", err)
	}
	if d.TenantID != "t1" {
		t.Errorf("tenant = %s, want envelope t1", d.TenantID)
	}

	// Defaults: empty variant → champion; nil reasons/models → empty JSON docs.
	evt2 := mutate(func(e *commonEvent.DomainEvent, p map[string]any) {
		delete(p, "variant")
		p["reasons"] = nil
		p["models"] = nil
		delete(p, "inputs")
	})
	d2, err := Project(evt2)
	if err != nil {
		t.Fatal(err)
	}
	if d2.Variant != "champion" || string(d2.Reasons) != "[]" || string(d2.Models) != "[]" || string(d2.Inputs) != "{}" {
		t.Errorf("defaults wrong: variant=%s reasons=%s models=%s inputs=%s",
			d2.Variant, d2.Reasons, d2.Models, d2.Inputs)
	}
}

// Foreign event types on the decision.# binding are acked and skipped, and a
// malformed decision.recorded never poisons the queue (ack + loud log).
func TestHandler_AckSemantics(t *testing.T) {
	h := Handler(nil, zap.NewNop()) // repo must not be touched on these paths

	other := &commonEvent.DomainEvent{ID: "x", Type: "decision.future.thing"}
	if err := h(context.Background(), other); err != nil {
		t.Errorf("foreign type: want ack (nil), got %v", err)
	}

	bad := recordedEvent(t)
	bad.Payload = json.RawMessage(`{"broken":`)
	if err := h(context.Background(), bad); err != nil {
		t.Errorf("malformed payload: want ack-and-skip (nil), got %v", err)
	}
}
