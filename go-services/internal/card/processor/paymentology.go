package processor

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/card/model"
)

// Paymentology is the real issuer-processor adapter (founder decision
// 2026-07-18: Paymentology, Kenya BIN sponsorship via Diamond Trust Bank).
//
// STATUS: faithful-but-stub. The commercial deal is in progress; until API
// credentials and the partner integration pack arrive, every method fails
// fast with ErrPaymentologyNotConfigured so a misconfigured deployment
// (CARD_PROCESSOR=paymentology without credentials) surfaces immediately
// instead of silently pretending to issue cards.
//
// The config surface below is modeled on public knowledge of the
// Paymentology (ex-Tutuka) API family and MUST be verified against the
// partner docs when they arrive:
//   - Their classic issuing API ("CompanyRef/TerminalID" SOAP-era surface,
//     e.g. CreateLinkedCard / UpdateLimit / StopCard / UnstopCard) and the
//     newer REST platform both authenticate per-program; we model that as
//     ProgramID + APIKey/APISecret.
//   - Requests are HMAC-signed on the newer surface (hence APISecret) and
//     webhooks carry their own signing secret (WebhookSecret). ⚠ VERIFY:
//     exact scheme (HMAC-SHA256 over body vs canonical string, header names)
//     against their docs.
//   - BaseURL differs per environment (sandbox vs production, and possibly
//     per region). ⚠ VERIFY the Kenya/DTB program endpoint.
//
// PCI note: even when wired, this adapter must only ever map the processor's
// response down to IssueResult (ref + last4). If their issue response carries
// a full PAN (some flows do for virtual cards), it must be dropped on the
// floor here — never logged, never returned. Tokenized PAN reveal for the app
// is a separate processor-side widget/SDK flow (deferred).
type Paymentology struct {
	cfg    PaymentologyConfig
	logger *zap.Logger
}

// PaymentologyConfig is the credential/config surface for the adapter.
type PaymentologyConfig struct {
	BaseURL       string // PAYMENTOLOGY_BASE_URL   — environment API endpoint (⚠ verify per-region URL)
	ProgramID     string // PAYMENTOLOGY_PROGRAM_ID — card program / company reference for the DTB BIN
	APIKey        string // PAYMENTOLOGY_API_KEY    — API identity
	APISecret     string // PAYMENTOLOGY_API_SECRET — HMAC signing secret (⚠ verify signature scheme)
	WebhookSecret string // PAYMENTOLOGY_WEBHOOK_SECRET — callback signature verification
}

// Configured reports whether the minimum credential set is present.
func (c PaymentologyConfig) Configured() bool {
	return c.BaseURL != "" && c.ProgramID != "" && c.APIKey != "" && c.APISecret != ""
}

// PaymentologyConfigFromEnv reads the adapter config from the environment.
func PaymentologyConfigFromEnv() PaymentologyConfig {
	return PaymentologyConfig{
		BaseURL:       os.Getenv("PAYMENTOLOGY_BASE_URL"),
		ProgramID:     os.Getenv("PAYMENTOLOGY_PROGRAM_ID"),
		APIKey:        os.Getenv("PAYMENTOLOGY_API_KEY"),
		APISecret:     os.Getenv("PAYMENTOLOGY_API_SECRET"),
		WebhookSecret: os.Getenv("PAYMENTOLOGY_WEBHOOK_SECRET"),
	}
}

// NewPaymentology builds the adapter. It is safe to construct and register
// without credentials — calls fail fast until Configured() is true AND the
// TODO-marked request implementations below are wired.
func NewPaymentology(cfg PaymentologyConfig, logger *zap.Logger) *Paymentology {
	if !cfg.Configured() {
		logger.Info("Paymentology adapter registered WITHOUT credentials — all calls will fail fast until PAYMENTOLOGY_* env vars are set")
	}
	return &Paymentology{cfg: cfg, logger: logger}
}

func (p *Paymentology) Name() string { return "paymentology" }

// ErrPaymentologyNotConfigured is returned by every call until credentials
// exist and the HTTP client is implemented.
var ErrPaymentologyNotConfigured = fmt.Errorf(
	"paymentology: adapter not configured — awaiting API credentials from the Paymentology/DTB commercial deal (set PAYMENTOLOGY_BASE_URL/PROGRAM_ID/API_KEY/API_SECRET, then wire the TODO calls in internal/card/processor/paymentology.go)")

// notReady centralises the fail-fast: even with credentials present the stub
// refuses to run, because the request implementations are not wired yet.
func (p *Paymentology) notReady(op string) error {
	return fmt.Errorf("%s: %w", op, ErrPaymentologyNotConfigured)
}

// IssueCard will create a card under the DTB-sponsored program.
//
// TODO(paymentology): wire the create-card call.
//   - Classic surface: CreateLinkedCard / CreateVirtualCard style op with
//     (programRef=cfg.ProgramID, customer reference=req.CustomerID,
//     cardholder name, currency). ⚠ VERIFY op name + REST path vs docs.
//   - Map response -> IssueResult{ProcessorRef: their card/tracking id,
//     PanLast4: last 4 only, Active: per response state}. DROP any full PAN
//     in the response (PCI — see type comment).
//   - Idempotency: pass a client reference (tenant+customer+account hash or
//     the Nemo card id) so retries don't double-issue. ⚠ VERIFY their
//     idempotency mechanism (client transaction id?).
func (p *Paymentology) IssueCard(_ context.Context, _ IssueRequest) (IssueResult, error) {
	return IssueResult{}, p.notReady("issue card")
}

// FreezeCard will place a temporary hold.
// TODO(paymentology): classic surface calls this StopCard with a reversible
// stop-reason code; REST surface a status PATCH. ⚠ VERIFY reversible vs
// permanent stop-reason taxonomy so freeze maps to the reversible one.
func (p *Paymentology) FreezeCard(_ context.Context, _ string) error {
	return p.notReady("freeze card")
}

// UnfreezeCard lifts a temporary hold.
// TODO(paymentology): UnstopCard / status PATCH back to active. ⚠ VERIFY
// that only reversible stops can be lifted (blocked cards must stay blocked).
func (p *Paymentology) UnfreezeCard(_ context.Context, _ string) error {
	return p.notReady("unfreeze card")
}

// BlockCard permanently stops the card (lost/stolen/fraud).
// TODO(paymentology): StopCard with a permanent reason code mapped from our
// reason (LOST/STOLEN/FRAUD). ⚠ VERIFY reason-code enum values.
func (p *Paymentology) BlockCard(_ context.Context, _ string, _ string) error {
	return p.notReady("block card")
}

// SetLimits pushes per-card spending controls.
// TODO(paymentology): map model.SpendingLimits (perTransaction/daily/monthly
// + channel toggles) onto their limit/usage-control API (UpdateLimit /
// card-controls resource). ⚠ VERIFY which controls are program-level vs
// card-level on the DTB program — card-level overrides may be a subset.
func (p *Paymentology) SetLimits(_ context.Context, _ string, _ model.SpendingLimits) error {
	return p.notReady("set limits")
}

// NormalizeWebhook verifies and maps a Paymentology callback.
// TODO(paymentology): (1) verify signature with cfg.WebhookSecret (⚠ VERIFY
// scheme + header); (2) map their event kinds (card issued/activated,
// authorization, stop/unstop, ...) onto card.* domain kinds; unknown kinds
// pass through as "card.processor.<theirKind>" for the audit trail.
func (p *Paymentology) NormalizeWebhook(_ []byte) (WebhookEvent, error) {
	return WebhookEvent{}, p.notReady("normalize webhook")
}
