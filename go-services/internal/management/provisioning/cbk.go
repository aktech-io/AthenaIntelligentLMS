// Package provisioning computes CBK PG/04 prudential loan-loss provisioning and
// reconciles it against IFRS 9 ECL. The regulator requires holding the HIGHER of
// the two: the IFRS 9 ECL is the accounting allowance that hits P&L, and any
// EXCESS of the CBK prudential provision over IFRS is held as a non-distributable
// statutory loan-loss reserve within equity (an appropriation of retained
// earnings, not a P&L charge). This package is the computation layer (H-4a);
// posting the movement to the GL is a separate increment (H-4b).
//
// NOTE: the CBK 5-bucket day-past-due bands here are the CORRECT prudential bands
// and are deliberately DISTINCT from the internal loan staging in
// internal/management/model (which uses tighter bands for collections/IFRS
// staging). CONFIRM the rates/bands against the current CBK/PG/04; they are
// consts for easy tuning. Specific provisions on Substandard+ are, under CBK,
// computed net of the realizable value of securities; v1 treats exposures as
// unsecured (no collateral data) — conservative (higher provision). Collateral
// netting is a documented follow-up. See docs/REGULATORY_REPORTING_KE.md.
package provisioning

import "github.com/shopspring/decimal"

// cbkBands is the CBK PG/04 five-bucket classification, ordered, with the minimum
// provision rate applied to gross outstanding. maxDPD is the inclusive upper day
// bound of the bucket (the last bucket is open-ended).
var cbkBands = []struct {
	code   string
	desc   string
	maxDPD int
	rate   float64
}{
	{"NORMAL", "Normal (dpd 0-30)", 30, 0.01},
	{"WATCH", "Watch (dpd 31-90)", 90, 0.03},
	{"SUBSTANDARD", "Substandard (dpd 91-180)", 180, 0.20},
	{"DOUBTFUL", "Doubtful (dpd 181-360)", 360, 0.50},
	{"LOSS", "Loss (dpd 360+)", 1 << 30, 1.00},
}

// ClassifyCBK returns the CBK class code and minimum provision rate for a
// days-past-due value.
func ClassifyCBK(dpd int) (code string, rate decimal.Decimal) {
	for _, b := range cbkBands {
		if dpd <= b.maxDPD {
			return b.code, decimal.NewFromFloat(b.rate)
		}
	}
	return "LOSS", decimal.NewFromInt(1)
}

// BucketInput is aggregated exposure for one CBK class (from the repository).
type BucketInput struct {
	Class       string
	Loans       int
	Outstanding decimal.Decimal
}

// Bucket is a CBK class with its computed prudential provision.
type Bucket struct {
	Class         string          `json:"class"`
	Description   string          `json:"description"`
	Loans         int             `json:"loans"`
	Outstanding   decimal.Decimal `json:"outstanding"`
	ProvisionRate decimal.Decimal `json:"provisionRate"`
	Provision     decimal.Decimal `json:"provision"`
}

// Report reconciles CBK prudential provisioning with IFRS 9 ECL.
type Report struct {
	AsOf                     string          `json:"asOf"`
	Buckets                  []Bucket        `json:"buckets"`
	CBKProvision             decimal.Decimal `json:"cbkProvision"`
	IFRSECLProvision         decimal.Decimal `json:"ifrsEclProvision"`
	RequiredAllowance        decimal.Decimal `json:"requiredAllowance"`        // higher-of(CBK, IFRS)
	PLImpairmentCharge       decimal.Decimal `json:"plImpairmentCharge"`       // = IFRS ECL (hits P&L)
	StatutoryLoanLossReserve decimal.Decimal `json:"statutoryLoanLossReserve"` // max(0, CBK - IFRS) → equity
	Basis                    string          `json:"basis"`
}

// BuildReport applies the CBK band rates to the aggregated exposures and
// reconciles against the IFRS 9 ECL total: the required allowance is the higher
// of the two, the P&L impairment charge is the IFRS ECL, and the excess of CBK
// over IFRS is the statutory loan-loss reserve held in equity. Buckets are
// returned in canonical band order (zero-filled for absent classes).
func BuildReport(asOf string, inputs []BucketInput, ifrsECL decimal.Decimal) *Report {
	byClass := make(map[string]BucketInput, len(inputs))
	for _, in := range inputs {
		byClass[in.Class] = in
	}

	buckets := make([]Bucket, 0, len(cbkBands))
	cbkTotal := decimal.Zero
	for _, b := range cbkBands {
		agg := byClass[b.code] // zero value if the class is absent
		out := agg.Outstanding.Round(2)
		rate := decimal.NewFromFloat(b.rate)
		prov := out.Mul(rate).Round(2)
		cbkTotal = cbkTotal.Add(prov)
		buckets = append(buckets, Bucket{
			Class: b.code, Description: b.desc, Loans: agg.Loans,
			Outstanding: out, ProvisionRate: rate, Provision: prov,
		})
	}
	cbkTotal = cbkTotal.Round(2)
	ifrs := ifrsECL.Round(2)

	statutory := cbkTotal.Sub(ifrs)
	if statutory.IsNegative() {
		statutory = decimal.Zero
	}
	return &Report{
		AsOf:                     asOf,
		Buckets:                  buckets,
		CBKProvision:             cbkTotal,
		IFRSECLProvision:         ifrs,
		RequiredAllowance:        decimal.Max(cbkTotal, ifrs),
		PLImpairmentCharge:       ifrs,
		StatutoryLoanLossReserve: statutory,
		Basis:                    "higher-of(IFRS 9 ECL, CBK PG/04 prudential provision); IFRS ECL is the P&L allowance, and the excess of CBK over IFRS is held as a non-distributable statutory loan-loss reserve in equity",
	}
}
