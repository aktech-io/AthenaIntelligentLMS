package processor

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/card/model"
)

// FromEnv defaults to the sandbox when CARD_PROCESSOR is unset.
func TestFromEnv_DefaultSandbox(t *testing.T) {
	t.Setenv("CARD_PROCESSOR", "")
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if p.Name() != "sandbox" {
		t.Errorf("default processor = %s, want sandbox", p.Name())
	}
}

// Unknown processor names must fail loudly, not fall back silently.
func TestFromEnv_UnknownName(t *testing.T) {
	t.Setenv("CARD_PROCESSOR", "no-such-processor")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected error for unknown processor")
	}
}

// A registered adapter is selectable by env (case-insensitive).
func TestFromEnv_RegisteredAdapter(t *testing.T) {
	Register(NewPaymentology(PaymentologyConfig{}, zap.NewNop()))
	t.Setenv("CARD_PROCESSOR", "Paymentology")
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if p.Name() != "paymentology" {
		t.Errorf("processor = %s, want paymentology", p.Name())
	}
}

// Sandbox issuance is deterministic (same request → same identifiers) and
// exposes only last4 + processor ref — no PAN-shaped field exists to leak.
func TestSandbox_DeterministicIssue(t *testing.T) {
	req := IssueRequest{
		TenantID: "t1", CustomerID: "c1", AccountID: "a1",
		Type: model.CardTypeVirtual, Network: model.CardNetworkMastercard,
		Currency: "KES", CardholderName: "JANE WANJIKU",
	}
	r1, err := Sandbox{}.IssueCard(context.Background(), req)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	r2, _ := Sandbox{}.IssueCard(context.Background(), req)
	if r1 != r2 {
		t.Errorf("sandbox issuance not deterministic: %+v vs %+v", r1, r2)
	}
	if !regexp.MustCompile(`^\d{4}$`).MatchString(r1.PanLast4) {
		t.Errorf("PanLast4 = %q, want exactly 4 digits", r1.PanLast4)
	}
	if r1.ProcessorRef == "" {
		t.Error("ProcessorRef must be set")
	}
	if !r1.Active {
		t.Error("virtual card should issue Active")
	}

	// Different account → different card identity.
	req2 := req
	req2.AccountID = "a2"
	r3, _ := Sandbox{}.IssueCard(context.Background(), req2)
	if r3.ProcessorRef == r1.ProcessorRef {
		t.Error("different account must yield a different processor ref")
	}
}

// Physical cards issue pending activation; the decline rule declines.
func TestSandbox_PhysicalAndDecline(t *testing.T) {
	req := IssueRequest{
		TenantID: "t1", CustomerID: "c1", AccountID: "a1",
		Type: model.CardTypePhysical, Network: model.CardNetworkVisa,
		CardholderName: "JANE WANJIKU",
	}
	res, err := Sandbox{}.IssueCard(context.Background(), req)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if res.Active {
		t.Error("physical card should NOT issue Active")
	}

	req.CardholderName = "DECLINE ME"
	if _, err := (Sandbox{}).IssueCard(context.Background(), req); err == nil {
		t.Error("expected sandbox decline for DECLINE ME")
	}
}

// The Paymentology stub must fail fast on every call until credentials and
// the wired client exist — a misconfigured deployment can never fake-issue.
func TestPaymentology_FailsFastUntilConfigured(t *testing.T) {
	p := NewPaymentology(PaymentologyConfigFromEnv(), zap.NewNop())

	if _, err := p.IssueCard(context.Background(), IssueRequest{}); !errors.Is(err, ErrPaymentologyNotConfigured) {
		t.Errorf("IssueCard err = %v, want ErrPaymentologyNotConfigured", err)
	}
	if err := p.FreezeCard(context.Background(), "ref"); !errors.Is(err, ErrPaymentologyNotConfigured) {
		t.Errorf("FreezeCard err = %v", err)
	}
	if err := p.UnfreezeCard(context.Background(), "ref"); !errors.Is(err, ErrPaymentologyNotConfigured) {
		t.Errorf("UnfreezeCard err = %v", err)
	}
	if err := p.BlockCard(context.Background(), "ref", "LOST"); !errors.Is(err, ErrPaymentologyNotConfigured) {
		t.Errorf("BlockCard err = %v", err)
	}
	if err := p.SetLimits(context.Background(), "ref", model.SpendingLimits{}); !errors.Is(err, ErrPaymentologyNotConfigured) {
		t.Errorf("SetLimits err = %v", err)
	}
	if _, err := p.NormalizeWebhook([]byte(`{}`)); !errors.Is(err, ErrPaymentologyNotConfigured) {
		t.Errorf("NormalizeWebhook err = %v", err)
	}
}

// Sandbox webhook normalization round-trips its trivial shape.
func TestSandbox_NormalizeWebhook(t *testing.T) {
	evt, err := Sandbox{}.NormalizeWebhook([]byte(`{"ref":"sbx-card-1","kind":"card.activated","detail":{"by":"processor"}}`))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if evt.ProcessorRef != "sbx-card-1" || evt.Kind != "card.activated" {
		t.Errorf("unexpected event: %+v", evt)
	}
	if _, err := (Sandbox{}).NormalizeWebhook([]byte(`{"kind":"x"}`)); err == nil {
		t.Error("expected error for missing ref")
	}
}
