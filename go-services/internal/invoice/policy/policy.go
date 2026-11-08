// Package policy wraps common/decision for the invoice.advance policy: it
// owns a private registry seeded with the embedded default and replaced by
// the latest version stored through PUT /api/v1/invoice-policy, exposes the
// current document (yaml + version + hash), and builds the decision Request
// from pipeline inputs (SPEC §3.6, §5).
package policy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"

	"github.com/athena-lms/go-services/internal/common/decision"
)

// ID is the policy id (SPEC §5).
const ID = "invoice.advance"

// BuiltinFile is the embedded default document name.
const BuiltinFile = "invoice.advance.ke.yaml"

// Source is the service name stamped as the decision actor.
const Source = "invoice-service"

// Doc is one policy document version.
type Doc struct {
	Version   int
	Hash      string
	YAML      string
	Source    string // embedded | database
	UpdatedAt time.Time
	UpdatedBy string
}

// Inputs is the feature snapshot the policy reads (SPEC §3.6).
type Inputs struct {
	SellerBand       string
	BuyerTier        string
	InvoiceScore     int
	DaysToDue        int
	Amount           decimal.Decimal
	FraudFlags       []string
	BuyerConfirmed   bool
	ConcentrationPct float64
	ScoreAvailable   bool
	ScoreProvider    string
	ScoredAt         *time.Time
}

// Map flattens Inputs for the v1 rule language (top-level keys only).
func (in Inputs) Map() map[string]any {
	amt, _ := in.Amount.Float64()
	m := map[string]any{
		"seller_band":       in.SellerBand,
		"buyer_tier":        in.BuyerTier,
		"band_cell":         in.SellerBand + "/" + in.BuyerTier,
		"invoice_score":     in.InvoiceScore,
		"days_to_due":       in.DaysToDue,
		"amount":            amt,
		"fraud_flags":       len(in.FraudFlags),
		"fraud_flag_codes":  strings.Join(in.FraudFlags, ","),
		"buyer_confirmed":   in.BuyerConfirmed,
		"concentration_pct": in.ConcentrationPct,
		"score_available":   in.ScoreAvailable,
	}
	if in.ScoreProvider != "" {
		m["score_provider"] = in.ScoreProvider
	}
	if in.ScoredAt != nil {
		m["scored_at"] = in.ScoredAt.UTC().Format(time.RFC3339)
	}
	return m
}

// Terms is what the band table yields for an APPROVE/REFER outcome.
type Terms struct {
	AdvanceRate decimal.Decimal
	FeePerDay   decimal.Decimal
	MaxAmount   *decimal.Decimal
}

// TermsFrom extracts advance_rate / fee_per_day / max_amount from
// Outcome.Detail. ok=false when the outcome carries no terms (gate rules).
func TermsFrom(detail map[string]any) (Terms, bool) {
	var t Terms
	rate, ok1 := toDecimal(detail["advance_rate"])
	fee, ok2 := toDecimal(detail["fee_per_day"])
	if !ok1 || !ok2 {
		return t, false
	}
	t.AdvanceRate, t.FeePerDay = rate, fee
	if mx, ok := toDecimal(detail["max_amount"]); ok {
		t.MaxAmount = &mx
	}
	return t, true
}

func toDecimal(v any) (decimal.Decimal, bool) {
	switch n := v.(type) {
	case float64:
		return decimal.NewFromFloat(n), true
	case int:
		return decimal.NewFromInt(int64(n)), true
	case int64:
		return decimal.NewFromInt(n), true
	case string:
		d, err := decimal.NewFromString(n)
		return d, err == nil
	case decimal.Decimal:
		return n, true
	}
	return decimal.Zero, false
}

// Store persists policy versions (implemented by the repository).
type Store interface {
	LatestPolicy(ctx context.Context, tenantID, policyID string) (version int, hash, yamlDoc, createdBy string, createdAt time.Time, found bool, err error)
	SavePolicy(ctx context.Context, tenantID, policyID string, version int, hash, yamlDoc, createdBy string) error
}

// Engine is the invoice policy engine: one registry per tenant override.
type Engine struct {
	mu       sync.RWMutex
	market   string
	store    Store
	builtin  Doc
	current  map[string]*entry // tenant → entry ("*" for default)
	evaluate func(ctx context.Context, reg *decision.Registry, req decision.Request) (decision.Outcome, error)
}

type entry struct {
	doc Doc
	reg *decision.Registry
}

// New loads the embedded default and, when store is non-nil, the stored
// default-tenant override. market is the ISO code the policy resolves under.
func New(ctx context.Context, store Store, market string) (*Engine, error) {
	raw, err := decision.BuiltinYAML(BuiltinFile)
	if err != nil {
		return nil, fmt.Errorf("invoice policy: embedded default: %w", err)
	}
	if market == "" {
		market = "KE"
	}
	if market != "KE" {
		// The embedded default is authored for KE; re-home it under the
		// running market so resolution (id, tenant, market) still finds it.
		var generic map[string]any
		if err := yaml.Unmarshal(raw, &generic); err != nil {
			return nil, err
		}
		generic["market"] = market
		if raw, err = yaml.Marshal(generic); err != nil {
			return nil, err
		}
	}
	reg := decision.NewRegistry()
	if err := reg.RegisterYAML(raw, BuiltinFile); err != nil {
		return nil, err
	}
	e := &Engine{
		market: market,
		store:  store,
		builtin: Doc{
			Version: versionOf(raw), Hash: hashOf(raw), YAML: string(raw), Source: "embedded", UpdatedAt: time.Time{},
		},
		current: map[string]*entry{},
		evaluate: func(ctx context.Context, reg *decision.Registry, req decision.Request) (decision.Outcome, error) {
			return decision.NewEvaluator(reg).Evaluate(ctx, req)
		},
	}
	e.current["*"] = &entry{doc: e.builtin, reg: reg}
	return e, nil
}

// Load pulls the latest stored version for tenantID (if any) into memory.
func (e *Engine) Load(ctx context.Context, tenantID string) error {
	if e.store == nil {
		return nil
	}
	version, hash, doc, by, at, found, err := e.store.LatestPolicy(ctx, tenantID, ID)
	if err != nil || !found {
		return err
	}
	reg := decision.NewRegistry()
	if err := reg.RegisterYAML([]byte(doc), "db:"+ID); err != nil {
		return fmt.Errorf("invoice policy: stored v%d invalid: %w", version, err)
	}
	e.mu.Lock()
	e.current[tenantID] = &entry{doc: Doc{Version: version, Hash: hash, YAML: doc, Source: "database", UpdatedAt: at, UpdatedBy: by}, reg: reg}
	e.mu.Unlock()
	return nil
}

func (e *Engine) entryFor(tenantID string) *entry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if en, ok := e.current[tenantID]; ok {
		return en
	}
	return e.current["*"]
}

// Current returns the effective document for a tenant.
func (e *Engine) Current(tenantID string) Doc { return e.entryFor(tenantID).doc }

// Put validates a new YAML document, bumps the version to current+1 (the
// submitted `version:` is overwritten so operators cannot replay an old
// number), computes the content hash, stores and activates it.
func (e *Engine) Put(ctx context.Context, tenantID, actor, yamlDoc string) (Doc, error) {
	var generic map[string]any
	if err := yaml.Unmarshal([]byte(yamlDoc), &generic); err != nil {
		return Doc{}, fmt.Errorf("invalid yaml: %w", err)
	}
	if generic == nil {
		return Doc{}, fmt.Errorf("empty policy document")
	}
	if id, _ := generic["policy"].(string); id != "" && id != ID {
		return Doc{}, fmt.Errorf("policy id must be %q, got %q", ID, id)
	}
	cur := e.Current(tenantID)
	generic["policy"] = ID
	generic["version"] = cur.Version + 1
	if _, ok := generic["market"]; !ok {
		generic["market"] = e.market
	}
	generic["tenant"] = tenantID
	if tenantID == "" {
		generic["tenant"] = "*"
	}
	out, err := yaml.Marshal(generic)
	if err != nil {
		return Doc{}, err
	}
	reg := decision.NewRegistry()
	if err := reg.RegisterYAML(out, "put:"+ID); err != nil {
		return Doc{}, err
	}
	// Smoke-evaluate so a document that parses but cannot evaluate the
	// documented inputs is rejected at PUT time, not at decision time.
	if _, err := e.evaluate(ctx, reg, e.Request(tenantID, "smoke", "", Inputs{SellerBand: "A", BuyerTier: "A", DaysToDue: 30, BuyerConfirmed: true, Amount: decimal.NewFromInt(1000)})); err != nil {
		return Doc{}, fmt.Errorf("policy does not evaluate: %w", err)
	}
	doc := Doc{Version: cur.Version + 1, Hash: hashOf(out), YAML: string(out), Source: "database", UpdatedAt: time.Now().UTC(), UpdatedBy: actor}
	if e.store != nil {
		if err := e.store.SavePolicy(ctx, tenantID, ID, doc.Version, doc.Hash, doc.YAML, actor); err != nil {
			return Doc{}, err
		}
	}
	e.mu.Lock()
	e.current[tenantID] = &entry{doc: doc, reg: reg}
	e.mu.Unlock()
	return doc, nil
}

// Request builds the decision request for an invoice.
func (e *Engine) Request(tenantID, invoiceID, sellerCustomerID string, in Inputs) decision.Request {
	req := decision.Request{
		Type:          ID,
		TenantID:      tenantID,
		Market:        e.market,
		SubjectType:   "invoice",
		SubjectID:     invoiceID,
		CustomerID:    sellerCustomerID,
		Actor:         decision.SystemActor(Source),
		CorrelationID: invoiceID,
		Inputs:        in.Map(),
	}
	if in.ScoreAvailable || in.ScoreProvider != "" {
		req.Models = []decision.ModelRef{{
			Name: "credit_score", Version: in.ScoreProvider, Role: "scoring",
			Available: in.ScoreAvailable, ScoredAt: in.ScoredAt,
		}}
	}
	return req
}

// Evaluate runs the tenant's effective policy against the inputs.
func (e *Engine) Evaluate(ctx context.Context, tenantID, invoiceID, sellerCustomerID string, in Inputs) (decision.Request, decision.Outcome, error) {
	en := e.entryFor(tenantID)
	req := e.Request(tenantID, invoiceID, sellerCustomerID, in)
	// The stored document was registered under the tenant's own id; the
	// embedded default under "*". Resolution walks tenant → "*" so both work.
	out, err := e.evaluate(ctx, en.reg, req)
	return req, out, err
}

func hashOf(raw []byte) string {
	s := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(s[:])
}

func versionOf(raw []byte) int {
	var v struct {
		Version int `yaml:"version"`
	}
	_ = yaml.Unmarshal(raw, &v)
	if v.Version < 1 {
		return 1
	}
	return v.Version
}
