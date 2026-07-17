package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/athena-lms/go-services/internal/card/model"
)

// Sandbox is the deterministic no-vendor processor — TEST/DEMO ONLY. It
// simulates issuance without any real card network: the "PAN" never exists
// anywhere; the adapter derives a stable last4 + processor ref from the
// request so demos and tests are reproducible. Nothing sensitive is ever
// generated, returned, or stored (PCI posture: see package comment).
//
// Rules (for demos/tests):
//   - last4 and processorRef are a stable function of (tenant, customer,
//     account, type): re-issuing the same request yields the same identifiers.
//   - cardholder name "DECLINE ME" → issuance refused (processor-decline path).
//   - VIRTUAL cards come back Active; PHYSICAL come back pending activation.
//   - lifecycle calls (freeze/unfreeze/block/limits) succeed on any non-empty
//     ref; the service layer owns state, the sandbox just acknowledges.
type Sandbox struct{}

func (Sandbox) Name() string { return "sandbox" }

// ref derives the stable sandbox identifiers for a request.
func sandboxIdentity(req IssueRequest) (processorRef, last4 string) {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%s|%s|%s", req.TenantID, req.CustomerID, req.AccountID, req.Type)
	sum := h.Sum64()
	return fmt.Sprintf("sbx-card-%016x", sum), fmt.Sprintf("%04d", sum%10000)
}

func (Sandbox) IssueCard(_ context.Context, req IssueRequest) (IssueResult, error) {
	if req.CardholderName == "DECLINE ME" {
		return IssueResult{}, fmt.Errorf("sandbox: issuance declined by processor rule")
	}
	ref, last4 := sandboxIdentity(req)
	return IssueResult{
		ProcessorRef: ref,
		PanLast4:     last4,
		Network:      req.Network,
		Active:       req.Type == model.CardTypeVirtual,
	}, nil
}

func (Sandbox) FreezeCard(_ context.Context, ref string) error   { return sandboxAck(ref) }
func (Sandbox) UnfreezeCard(_ context.Context, ref string) error { return sandboxAck(ref) }

func (Sandbox) BlockCard(_ context.Context, ref, _ string) error { return sandboxAck(ref) }

func (Sandbox) SetLimits(_ context.Context, ref string, _ model.SpendingLimits) error {
	return sandboxAck(ref)
}

func sandboxAck(ref string) error {
	if ref == "" {
		return fmt.Errorf("sandbox: empty processor ref")
	}
	return nil
}

// NormalizeWebhook accepts the sandbox's own trivial JSON shape:
// {"ref": "...", "kind": "card.activated", "detail": {...}}.
func (Sandbox) NormalizeWebhook(body []byte) (WebhookEvent, error) {
	var raw struct {
		Ref    string         `json:"ref"`
		Kind   string         `json:"kind"`
		Detail map[string]any `json:"detail"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return WebhookEvent{}, fmt.Errorf("sandbox: parse webhook: %w", err)
	}
	if raw.Ref == "" || raw.Kind == "" {
		return WebhookEvent{}, fmt.Errorf("sandbox: webhook missing ref/kind")
	}
	return WebhookEvent{
		ProcessorRef: raw.Ref,
		Kind:         raw.Kind,
		OccurredAt:   time.Now().UTC(),
		Detail:       raw.Detail,
	}, nil
}
