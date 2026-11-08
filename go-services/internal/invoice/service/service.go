// Package service implements the invoice-finance pipeline (SPEC §3) as an
// in-process state machine: every step runs in its own DB transaction,
// writes exactly one ledger event per status transition, and the
// orchestrator advances to the next automatic step as soon as one completes.
// Failed steps are retried by a background loop with backoff.
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/outbox"
	"github.com/athena-lms/go-services/internal/invoice/client"
	"github.com/athena-lms/go-services/internal/invoice/corebanking"
	"github.com/athena-lms/go-services/internal/invoice/ledger"
	"github.com/athena-lms/go-services/internal/invoice/model"
	"github.com/athena-lms/go-services/internal/invoice/pipeline"
	"github.com/athena-lms/go-services/internal/invoice/policy"
	"github.com/athena-lms/go-services/internal/invoice/repository"
)

// Source is the event source / system actor name.
const Source = "invoice-service"

// Config tunes the pipeline.
type Config struct {
	DemoMode               bool
	PublicPortalURL        string
	Currency               string
	SecondApprovalAbove    decimal.Decimal
	ConfirmTTL             time.Duration // buyer link validity (5d)
	ReminderAfter          []time.Duration
	OfferTTL               time.Duration
	ConfidenceFloor        float64
	OverdueGraceDays       int
	ProcessingFeeRate      decimal.Decimal
	ProcessingFeeMin       decimal.Decimal
	StepTimeout            time.Duration
	FundTimeout            time.Duration
	RetryInterval          time.Duration
	MaxAttempts            int
	ScoreMaxAge            time.Duration
	SellerBandFallback     string
	ConcentrationMinActive int
	FraudRiskThreshold     float64
	NewBuyerDays           int
	AmountSpikeMultiple    decimal.Decimal
}

// DefaultConfig returns the SPEC defaults.
func DefaultConfig() Config {
	return Config{
		Currency:               "KES",
		SecondApprovalAbove:    decimal.NewFromInt(500000),
		ConfirmTTL:             5 * 24 * time.Hour,
		ReminderAfter:          []time.Duration{2 * 24 * time.Hour, 4 * 24 * time.Hour},
		OfferTTL:               7 * 24 * time.Hour,
		ConfidenceFloor:        0.6,
		OverdueGraceDays:       5,
		ProcessingFeeRate:      decimal.RequireFromString("0.01"),
		ProcessingFeeMin:       decimal.NewFromInt(500),
		StepTimeout:            45 * time.Second,
		FundTimeout:            90 * time.Second,
		RetryInterval:          30 * time.Second,
		MaxAttempts:            20,
		ScoreMaxAge:            30 * 24 * time.Hour,
		SellerBandFallback:     "B",
		ConcentrationMinActive: 5,
		FraudRiskThreshold:     0.5,
		NewBuyerDays:           90,
		AmountSpikeMultiple:    decimal.NewFromInt(3),
	}
}

// Service is the invoice-finance application service.
type Service struct {
	repo     *repository.Repository
	clients  *client.Clients
	core     corebanking.Provider
	policy   *policy.Engine
	anchorer ledger.Anchorer
	cfg      Config
	logger   *zap.Logger

	files   sync.Map // invoice id → []byte (uploaded bytes awaiting extraction)
	running sync.Map // invoice id → struct{} (orchestrator in flight)
	now     func() time.Time
}

// New wires the service.
func New(repo *repository.Repository, clients *client.Clients, core corebanking.Provider, pol *policy.Engine, cfg Config, logger *zap.Logger) *Service {
	return &Service{repo: repo, clients: clients, core: core, policy: pol, anchorer: ledger.NoneAnchorer{}, cfg: cfg, logger: logger, now: time.Now}
}

// SetAnchorer swaps the anchoring backend (OTS hook).
func (s *Service) SetAnchorer(a ledger.Anchorer) { s.anchorer = a }

// Config exposes the effective configuration.
func (s *Service) Config() Config { return s.cfg }

// Policy exposes the policy engine.
func (s *Service) Policy() *policy.Engine { return s.policy }

// Core exposes the core-banking provider.
func (s *Service) Core() corebanking.Provider { return s.core }

// actorFrom returns the acting user (or system) for ledger events.
func actorFrom(ctx context.Context) string {
	if u := auth.UserIDFromContext(ctx); u != "" {
		return u
	}
	return Source
}

// appendEvent writes a ledger event and mirrors it to the outbox as a
// DomainEvent of the same type so downstream services (reporting,
// notification, accounting) can react.
func (s *Service) appendEvent(ctx context.Context, tx pgx.Tx, inv *model.Invoice, typ string, payload map[string]any, actor string) (*model.LedgerEvent, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["invoiceId"] = inv.ID.String()
	payload["status"] = inv.Status
	if actor == "" {
		actor = actorFrom(ctx)
	}
	ev := &model.LedgerEvent{TenantID: inv.TenantID, InvoiceID: &inv.ID, Type: typ, Payload: payload, Actor: actor, OccurredAt: s.now()}
	if err := s.repo.AppendEvent(ctx, tx, ev); err != nil {
		return nil, fmt.Errorf("ledger append %s: %w", typ, err)
	}
	de, err := event.NewDomainEvent(typ, Source, inv.TenantID, inv.ID.String(), map[string]any{
		"invoiceId": inv.ID.String(), "sellerCustomerId": inv.SellerCustomerID, "status": inv.Status,
		"statusReason": inv.StatusReason, "seq": ev.Seq, "hash": ev.Hash, "actor": actor, "payload": payload,
	})
	if err != nil {
		return nil, err
	}
	if err := outbox.Write(ctx, tx, de, inv.ID.String()); err != nil {
		return nil, err
	}
	return ev, nil
}

// transition moves the invoice to `to` (checked against the state machine),
// persists it and writes exactly one ledger event — all inside tx.
func (s *Service) transition(ctx context.Context, tx pgx.Tx, inv *model.Invoice, to, reason, evType string, payload map[string]any) error {
	if err := pipeline.Check(inv.Status, to); err != nil {
		return err
	}
	from := inv.Status
	inv.Status, inv.StatusReason = to, reason
	inv.PipelineError, inv.PipelineNextAt = "", nil
	if pipeline.StepFor(to) == "" {
		inv.PipelineAttempts = 0
	}
	if err := s.repo.UpdateInvoice(ctx, tx, inv); err != nil {
		return err
	}
	if evType == "" {
		evType = pipeline.EventFor(from, to)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["from"] = from
	payload["to"] = to
	if reason != "" {
		payload["reason"] = reason
	}
	_, err := s.appendEvent(ctx, tx, inv, evType, payload, "")
	return err
}

// withTx runs fn inside a transaction.
func (s *Service) withTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ComputeOffer derives the Offer terms (SPEC §3.6, §5 footer): advance =
// financeable × rate (capped at max_amount), discount fee = advance × fee/day ×
// tenor, processing fee = max(1% × advance, min), reserve = total − advance.
func ComputeOffer(inv *model.Invoice, terms policy.Terms, cfg Config, now time.Time) *model.Offer {
	base := inv.FinanceableAmount()
	total := decimal.Zero
	if inv.Total != nil {
		total = *inv.Total
	}
	advance := base.Mul(terms.AdvanceRate).RoundBank(2)
	if terms.MaxAmount != nil && advance.GreaterThan(*terms.MaxAmount) {
		advance = *terms.MaxAmount
	}
	tenor := DaysToDue(inv, now)
	if tenor < 1 {
		tenor = 1
	}
	fee := advance.Mul(terms.FeePerDay).Mul(decimal.NewFromInt(int64(tenor))).RoundBank(2)
	proc := advance.Mul(cfg.ProcessingFeeRate).RoundBank(2)
	if proc.LessThan(cfg.ProcessingFeeMin) {
		proc = cfg.ProcessingFeeMin
	}
	reserve := total.Sub(advance)
	if reserve.IsNegative() {
		reserve = decimal.Zero
	}
	return &model.Offer{
		InvoiceID: inv.ID, AdvanceRate: terms.AdvanceRate, AdvanceAmount: advance, DiscountFeePerDay: terms.FeePerDay,
		TenorDays: tenor, FeeAmount: fee, ProcessingFee: proc, NetDisbursement: advance.Sub(fee).Sub(proc),
		ReserveAmount: reserve, ExpiresAt: now.Add(cfg.OfferTTL), Status: model.OfferOffered,
	}
}

// DaysToDue is whole days from now to the invoice due date (0 when unknown).
func DaysToDue(inv *model.Invoice, now time.Time) int {
	if inv.DueDate == nil {
		return 0
	}
	day := func(t time.Time) time.Time {
		t = t.UTC()
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	return int(day(*inv.DueDate).Sub(day(now)).Hours() / 24)
}

// ApplyPayment splits a payment across fees, principal and reserve release
// (SPEC §3.9): fees first, then principal, surplus releases reserve to the
// seller. Returns the applied parts and whether the advance is now covered.
func ApplyPayment(a *model.Advance, amount decimal.Decimal) (fees, principal, release decimal.Decimal, covered bool) {
	rem := amount
	feesOwed := a.FeesAccrued.Sub(a.FeesRepaid)
	if feesOwed.IsPositive() {
		fees = decimal.Min(rem, feesOwed)
		rem = rem.Sub(fees)
	}
	prinOwed := a.Principal.Sub(a.Repaid)
	if prinOwed.IsPositive() && rem.IsPositive() {
		principal = decimal.Min(rem, prinOwed)
		rem = rem.Sub(principal)
	}
	if rem.IsPositive() {
		release = rem
	}
	covered = a.FeesAccrued.Sub(a.FeesRepaid).Sub(fees).LessThanOrEqual(decimal.Zero) &&
		a.Principal.Sub(a.Repaid).Sub(principal).LessThanOrEqual(decimal.Zero)
	return
}

// FraudInputs is what the local fraud rules read.
type FraudInputs struct {
	SellerEmail     string
	BuyerEmail      string
	BuyerCreatedAt  time.Time
	BuyerTier       string
	BuyerDisputes   int
	Amount          decimal.Decimal
	SellerAvg6m     decimal.Decimal
	SellerCount6m   int
	RemoteRiskScore float64
	RemoteRules     []string
	RemoteAvailable bool
}

// LocalFraudFlags applies SPEC §3.5's local rules. New-buyer is only a flag
// when the buyer also looks weak (tier D or prior disputes) so a first
// invoice from a fresh buyer does not refer every demo.
func LocalFraudFlags(in FraudInputs, cfg Config, now time.Time) []string {
	var flags []string
	if sd, bd := emailDomain(in.SellerEmail), emailDomain(in.BuyerEmail); sd != "" && sd == bd && !publicMailDomain(sd) {
		flags = append(flags, "SAME_PARTY_DOMAIN")
	}
	if !in.BuyerCreatedAt.IsZero() && now.Sub(in.BuyerCreatedAt) < time.Duration(cfg.NewBuyerDays)*24*time.Hour && (in.BuyerTier == model.TierD || in.BuyerDisputes > 0) {
		flags = append(flags, "NEW_BUYER")
	}
	if in.SellerCount6m >= 3 && in.SellerAvg6m.IsPositive() && in.Amount.GreaterThan(in.SellerAvg6m.Mul(cfg.AmountSpikeMultiple)) {
		flags = append(flags, "AMOUNT_SPIKE")
	}
	if in.RemoteAvailable && in.RemoteRiskScore >= cfg.FraudRiskThreshold {
		flags = append(flags, "FRAUD_SERVICE_RISK")
	}
	for _, r := range in.RemoteRules {
		flags = append(flags, "RULE_"+r)
	}
	return flags
}

func emailDomain(e string) string {
	for i := len(e) - 1; i >= 0; i-- {
		if e[i] == '@' {
			return lower(e[i+1:])
		}
	}
	return ""
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func publicMailDomain(d string) bool {
	switch d {
	case "gmail.com", "yahoo.com", "outlook.com", "hotmail.com", "icloud.com", "live.com", "proton.me", "protonmail.com":
		return true
	}
	return false
}

// InvoiceScore combines aging, amount-vs-turnover and OCR confidence into a
// 0–100 score (SPEC §3.4).
func InvoiceScore(inv *model.Invoice, daysToDue int, sellerAvg decimal.Decimal, sellerCount int) int {
	score := 100
	switch {
	case daysToDue < 7:
		score -= 25
	case daysToDue < 15:
		score -= 10
	case daysToDue > 90:
		score -= 10
	}
	if sellerCount >= 3 && sellerAvg.IsPositive() && inv.Total != nil {
		ratio := inv.Total.Div(sellerAvg)
		if ratio.GreaterThan(decimal.NewFromInt(2)) {
			score -= 15
		} else if ratio.GreaterThan(decimal.NewFromFloat(1.5)) {
			score -= 5
		}
	}
	if n := len(inv.OCR.Confidence); n > 0 {
		sum := 0.0
		for _, c := range inv.OCR.Confidence {
			sum += c
		}
		if mean := sum / float64(n); mean < 0.6 {
			score -= 20
		} else if mean < 0.75 {
			score -= 10
		}
	} else {
		score -= 20 // nothing extracted
	}
	score -= 5 * len(inv.NeedsConfirm)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// Inv is a short accessor used by jobs.
func (s *Service) Inv(ctx context.Context, id uuid.UUID) (*model.Invoice, error) {
	return s.repo.GetInvoice(ctx, nil, "", id)
}
