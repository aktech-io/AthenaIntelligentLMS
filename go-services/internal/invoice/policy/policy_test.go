package policy

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/athena-lms/go-services/internal/common/decision"
)

type memStore struct {
	version int
	hash    string
	yaml    string
	saved   int
}

func (m *memStore) LatestPolicy(context.Context, string, string) (int, string, string, string, time.Time, bool, error) {
	if m.version == 0 {
		return 0, "", "", "", time.Time{}, false, nil
	}
	return m.version, m.hash, m.yaml, "admin", time.Now(), true, nil
}
func (m *memStore) SavePolicy(_ context.Context, _, _ string, version int, hash, yamlDoc, _ string) error {
	m.version, m.hash, m.yaml, m.saved = version, hash, yamlDoc, m.saved+1
	return nil
}

func base() Inputs {
	return Inputs{
		SellerBand: "A", BuyerTier: "A", InvoiceScore: 80, DaysToDue: 45,
		Amount: decimal.NewFromInt(100000), BuyerConfirmed: true, ConcentrationPct: 10, ScoreAvailable: true,
	}
}

func eval(t *testing.T, e *Engine, in Inputs) decision.Outcome {
	t.Helper()
	_, out, err := e.Evaluate(context.Background(), "default", "inv-1", "CUST-1", in)
	require.NoError(t, err)
	return out
}

func TestEmbeddedDefaultLoads(t *testing.T) {
	e, err := New(context.Background(), nil, "KE")
	require.NoError(t, err)
	doc := e.Current("default")
	assert.Equal(t, 1, doc.Version)
	assert.Contains(t, doc.Hash, "sha256:")
	assert.Equal(t, "embedded", doc.Source)
	assert.Contains(t, doc.YAML, "policy: invoice.advance")
}

func TestRuleIFGate(t *testing.T) {
	e, _ := New(context.Background(), nil, "KE")
	in := base()
	in.BuyerConfirmed = false
	out := eval(t, e, in)
	assert.Equal(t, decision.Decline, out.Decision)
	assert.Equal(t, "IF-GATE", out.Reasons[0].RuleID)
	assert.Equal(t, decision.ReasonBuyerNotConfirmed, out.Reasons[0].Code)
}

func TestRuleIFFraud(t *testing.T) {
	e, _ := New(context.Background(), nil, "KE")
	in := base()
	in.FraudFlags = []string{"AMOUNT_SPIKE"}
	out := eval(t, e, in)
	assert.Equal(t, decision.Refer, out.Decision)
	assert.Equal(t, "IF-FRAUD", out.Reasons[0].RuleID)
	assert.Equal(t, decision.ReasonFraudFlag, out.Reasons[0].Code)
}

func TestRuleIFTenor(t *testing.T) {
	e, _ := New(context.Background(), nil, "KE")
	for _, days := range []int{3, 6, 121, 400} {
		in := base()
		in.DaysToDue = days
		out := eval(t, e, in)
		assert.Equal(t, decision.Decline, out.Decision, "days=%d", days)
		assert.Equal(t, "IF-TENOR", out.Reasons[0].RuleID)
		assert.Equal(t, decision.ReasonTenorOutOfRange, out.Reasons[0].Code)
	}
	for _, days := range []int{7, 60, 120} {
		in := base()
		in.DaysToDue = days
		assert.Equal(t, decision.Approve, eval(t, e, in).Decision, "days=%d", days)
	}
}

func TestRuleIFConc(t *testing.T) {
	e, _ := New(context.Background(), nil, "KE")
	in := base()
	in.ConcentrationPct = 55
	out := eval(t, e, in)
	assert.Equal(t, decision.Refer, out.Decision)
	assert.Equal(t, "IF-CONC", out.Reasons[0].RuleID)
	assert.Equal(t, decision.ReasonBuyerConcentration, out.Reasons[0].Code)
}

func TestRuleIFBandCells(t *testing.T) {
	e, _ := New(context.Background(), nil, "KE")
	cases := []struct {
		seller, buyer, want string
		rate, fee           string
	}{
		{"A", "A", decision.Approve, "0.9", "0.0005"},
		{"A", "B", decision.Approve, "0.9", "0.0005"},
		{"A", "C", decision.Approve, "0.8", "0.0007"},
		{"A", "D", decision.Refer, "0.7", "0.0009"},
		{"B", "C", decision.Approve, "0.8", "0.0007"},
		{"B", "D", decision.Refer, "0.8", "0.0007"},
		{"C", "B", decision.Approve, "0.7", "0.0009"},
		{"C", "C", decision.Refer, "0.7", "0.0009"},
		{"C", "D", decision.Decline, "0.7", "0.0009"},
		{"D", "A", decision.Refer, "0.7", "0.0009"},
		{"D", "D", decision.Decline, "", ""},
	}
	for _, c := range cases {
		in := base()
		in.SellerBand, in.BuyerTier = c.seller, c.buyer
		out := eval(t, e, in)
		assert.Equal(t, c.want, out.Decision, "%s/%s", c.seller, c.buyer)
		assert.Equal(t, c.seller+"/"+c.buyer, out.Detail["band"])
		if c.want != decision.Approve {
			assert.Equal(t, "IF-BAND", out.Reasons[0].RuleID)
		}
		terms, ok := TermsFrom(out.Detail)
		if c.rate == "" {
			assert.False(t, ok)
			continue
		}
		require.True(t, ok, "%s/%s", c.seller, c.buyer)
		assert.True(t, terms.AdvanceRate.Equal(decimal.RequireFromString(c.rate)), "rate %s", terms.AdvanceRate)
		assert.True(t, terms.FeePerDay.Equal(decimal.RequireFromString(c.fee)), "fee %s", terms.FeePerDay)
	}
}

func TestUnknownBandFailsClosed(t *testing.T) {
	e, _ := New(context.Background(), nil, "KE")
	in := base()
	in.SellerBand = "Z"
	out := eval(t, e, in)
	assert.Equal(t, decision.Decline, out.Decision)
	assert.Equal(t, decision.ReasonPolicyNoMatch, out.Reasons[0].Code)
}

func TestPutBumpsVersionAndActivates(t *testing.T) {
	st := &memStore{}
	e, err := New(context.Background(), st, "KE")
	require.NoError(t, err)

	// Tighten: C/B becomes REFER. Version in body is ignored and bumped.
	doc, err := e.Put(context.Background(), "default", "admin", `
policy: invoice.advance
version: 99
market: KE
rules:
  - id: IF-GATE
    when: buyer_confirmed == false
    outcome: DECLINE
    reason: BUYER_NOT_CONFIRMED
  - id: IF-BAND
    field: band_cell
    table:
      - { band: A/A, advance_rate: 0.95, fee_per_day: 0.0004, outcome: APPROVE }
      - { band: C/B, advance_rate: 0.70, fee_per_day: 0.0009, outcome: REFER, reason: BUYER_TIER_LOW }
`)
	require.NoError(t, err)
	assert.Equal(t, 2, doc.Version)
	assert.Equal(t, 1, st.saved)
	assert.Equal(t, "database", e.Current("default").Source)
	assert.Contains(t, doc.YAML, "version: 2")

	in := base()
	in.SellerBand, in.BuyerTier = "C", "B"
	assert.Equal(t, decision.Refer, eval(t, e, in).Decision)
	out := eval(t, e, base())
	assert.Equal(t, decision.Approve, out.Decision)
	assert.Equal(t, 2, out.Policy.Version)
	assert.Equal(t, doc.Hash, out.Policy.Hash)

	// Other tenants still see the embedded default.
	_, out2, err := e.Evaluate(context.Background(), "other", "inv", "c", base())
	require.NoError(t, err)
	assert.Equal(t, 1, out2.Policy.Version)

	// Reload from store on a fresh engine.
	e2, err := New(context.Background(), st, "KE")
	require.NoError(t, err)
	require.NoError(t, e2.Load(context.Background(), "default"))
	assert.Equal(t, 2, e2.Current("default").Version)
}

func TestPutRejectsInvalid(t *testing.T) {
	e, _ := New(context.Background(), nil, "KE")
	_, err := e.Put(context.Background(), "default", "admin", "policy: other\nrules: []")
	assert.Error(t, err)
	_, err = e.Put(context.Background(), "default", "admin", "rules:\n  - id: X\n    when: foo ~~ 1\n    outcome: APPROVE\n")
	assert.Error(t, err)
	_, err = e.Put(context.Background(), "default", "admin", "rules:\n  - id: X\n    when: foo == 1\n    outcome: DECLINE\n    reason: NOT_A_CODE\n")
	assert.Error(t, err)
}

func TestNonKEMarketStillResolves(t *testing.T) {
	e, err := New(context.Background(), nil, "UG")
	require.NoError(t, err)
	assert.Equal(t, decision.Approve, eval(t, e, base()).Decision)
}
