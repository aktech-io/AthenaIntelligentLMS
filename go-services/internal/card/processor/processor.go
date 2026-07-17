// Package processor defines the card issuer-processor adapter (Nemo gap B1):
// card issuance and lifecycle as a pluggable interface so the processor
// (Paymentology — decided 2026-07-18 — with Kenya BIN sponsorship via Diamond
// Trust Bank) is an integration choice behind a stable seam, mirroring the
// eKYC provider registry idiom (internal/compliance/ekyc).
//
// The built-in sandbox processor gives deterministic results for demos and
// tests; real adapters register alongside it (from main wiring) and are
// selected with the CARD_PROCESSOR env var (default "sandbox").
//
// PCI-DSS POSTURE: adapters MUST NEVER return, log, or persist full PANs,
// CVVs, PINs, or expiry dates — the processor is the card-data environment.
// IssueResult deliberately has no field a full PAN could travel in; only the
// processor's opaque card reference and the last 4 digits cross this seam.
// PAN display uses processor-side tokenized reveal on the device (deferred
// with the app screens). See internal/card/model package comment.
package processor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/athena-lms/go-services/internal/card/model"
)

// IssueRequest asks the processor to issue one card under the tenant's card
// program. Identifiers are Nemo-side ids the processor stores as customer
// references; no cardholder document data travels here (KYC evidence is
// exchanged during program onboarding, not per-issuance).
type IssueRequest struct {
	TenantID       string
	CustomerID     string
	AccountID      string
	Type           model.CardType
	Network        model.CardNetwork
	Currency       string
	CardholderName string
	Limits         model.SpendingLimits
}

// IssueResult is everything Nemo is allowed to keep about the issued card.
// There is intentionally NO full-PAN / CVV / expiry field (PCI posture above).
type IssueResult struct {
	ProcessorRef string // processor-side opaque card id; the handle for all later calls
	PanLast4     string // last 4 digits only — permitted for display under PCI-DSS
	Network      model.CardNetwork
	// Active reports whether the card is immediately usable (virtual cards
	// typically are) or pending production/activation (physical).
	Active bool
}

// WebhookEvent is a processor callback normalized to the Nemo card domain.
// Kind is one of the card.* domain event types (internal/common/event) or
// "card.processor.<raw>" for kinds we pass through unmapped.
type WebhookEvent struct {
	ProcessorRef string
	Kind         string
	OccurredAt   time.Time
	Detail       map[string]any
}

// Processor is one card issuer-processor integration.
type Processor interface {
	Name() string
	IssueCard(ctx context.Context, req IssueRequest) (IssueResult, error)
	FreezeCard(ctx context.Context, processorRef string) error
	UnfreezeCard(ctx context.Context, processorRef string) error
	// BlockCard is the terminal block (lost/stolen/fraud). Irreversible.
	BlockCard(ctx context.Context, processorRef, reason string) error
	SetLimits(ctx context.Context, processorRef string, limits model.SpendingLimits) error
	// NormalizeWebhook maps one raw processor callback body to the Nemo card
	// domain. The HTTP webhook endpoint itself (signature verification +
	// ingestion) lands with the Paymentology credentials.
	NormalizeWebhook(body []byte) (WebhookEvent, error)
}

var registry = map[string]Processor{
	"sandbox": Sandbox{},
}

// Register adds a processor adapter (called from main wiring). Last
// registration wins for a name.
func Register(p Processor) { registry[strings.ToLower(p.Name())] = p }

// FromEnv resolves the configured processor (CARD_PROCESSOR, default sandbox).
func FromEnv() (Processor, error) {
	name := strings.ToLower(os.Getenv("CARD_PROCESSOR"))
	if name == "" {
		name = "sandbox"
	}
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("card processor: unknown processor %q", name)
	}
	return p, nil
}
