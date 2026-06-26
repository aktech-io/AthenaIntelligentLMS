package repository

import (
	"context"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// PortfolioStats is a live aggregate of the loan book for a tenant.
type PortfolioStats struct {
	TotalLoans       int             `json:"totalLoans"`
	ActiveLoans      int             `json:"activeLoans"`
	ClosedLoans      int             `json:"closedLoans"`
	DefaultedLoans   int             `json:"defaultedLoans"`
	TotalDisbursed   decimal.Decimal `json:"totalDisbursed"`
	TotalOutstanding decimal.Decimal `json:"totalOutstanding"`
}

// GetPortfolioStats computes live portfolio totals grouped by loan status.
func (r *Repository) GetPortfolioStats(ctx context.Context, tenantID string) (*PortfolioStats, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT status, COUNT(*),
		        COALESCE(SUM(disbursed_amount), 0),
		        COALESCE(SUM(outstanding_principal), 0)
		 FROM loans WHERE tenant_id = $1 GROUP BY status`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := &PortfolioStats{TotalDisbursed: decimal.Zero, TotalOutstanding: decimal.Zero}
	for rows.Next() {
		var status string
		var count int
		var disbursed, outstanding decimal.Decimal
		if err := rows.Scan(&status, &count, &disbursed, &outstanding); err != nil {
			return nil, err
		}
		stats.TotalLoans += count
		stats.TotalDisbursed = stats.TotalDisbursed.Add(disbursed)
		stats.TotalOutstanding = stats.TotalOutstanding.Add(outstanding)
		switch strings.ToUpper(status) {
		case "ACTIVE", "RESTRUCTURED":
			stats.ActiveLoans += count
		case "CLOSED", "PAID_OFF", "SETTLED":
			stats.ClosedLoans += count
		case "WRITTEN_OFF", "DEFAULTED":
			stats.DefaultedLoans += count
		}
	}
	return stats, rows.Err()
}

// AgeingBucket is one delinquency band of the active loan book.
type AgeingBucket struct {
	Bucket      string          `json:"bucket"` // Current, 1-30, 31-60, 61-90, 90+
	Loans       int             `json:"loans"`
	Outstanding decimal.Decimal `json:"outstanding"`
}

// PARReport is a Portfolio-at-Risk / ageing analysis of the active loan book —
// a standard audit/regulatory delinquency report. PARn = share of outstanding
// principal held by loans more than n days past due.
type PARReport struct {
	AsOf             string          `json:"asOf"`
	ActiveLoans      int             `json:"activeLoans"`
	TotalOutstanding decimal.Decimal `json:"totalOutstanding"`
	Buckets          []AgeingBucket  `json:"buckets"`
	PAR1             float64         `json:"par1"`
	PAR30            float64         `json:"par30"`
	PAR60            float64         `json:"par60"`
	PAR90            float64         `json:"par90"`
}

// GetPARReport buckets the active loan book by days-past-due and computes PAR
// ratios over outstanding principal.
func (r *Repository) GetPARReport(ctx context.Context, tenantID string) (*PARReport, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT
		     CASE
		         WHEN dpd <= 0            THEN 'Current'
		         WHEN dpd BETWEEN 1 AND 30  THEN '1-30'
		         WHEN dpd BETWEEN 31 AND 60 THEN '31-60'
		         WHEN dpd BETWEEN 61 AND 90 THEN '61-90'
		         ELSE '90+'
		     END AS bucket,
		     COUNT(*),
		     COALESCE(SUM(outstanding_principal), 0)
		 FROM loans
		 WHERE tenant_id = $1 AND status IN ('ACTIVE', 'RESTRUCTURED')
		 GROUP BY 1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	order := []string{"Current", "1-30", "31-60", "61-90", "90+"}
	byBucket := map[string]AgeingBucket{}
	total := decimal.Zero
	var active int
	for rows.Next() {
		var b string
		var count int
		var outstanding decimal.Decimal
		if err := rows.Scan(&b, &count, &outstanding); err != nil {
			return nil, err
		}
		byBucket[b] = AgeingBucket{Bucket: b, Loans: count, Outstanding: outstanding}
		total = total.Add(outstanding)
		active += count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rep := &PARReport{
		AsOf:             time.Now().UTC().Format("2006-01-02"),
		ActiveLoans:      active,
		TotalOutstanding: total,
		Buckets:          make([]AgeingBucket, 0, len(order)),
	}
	// Outstanding past each DPD threshold, for PAR ratios.
	pastDue := map[string]decimal.Decimal{"1-30": decimal.Zero, "31-60": decimal.Zero, "61-90": decimal.Zero, "90+": decimal.Zero}
	for _, name := range order {
		b, ok := byBucket[name]
		if !ok {
			b = AgeingBucket{Bucket: name, Loans: 0, Outstanding: decimal.Zero}
		}
		rep.Buckets = append(rep.Buckets, b)
		if name != "Current" {
			pastDue[name] = b.Outstanding
		}
	}
	ratio := func(num decimal.Decimal) float64 {
		if total.IsZero() {
			return 0
		}
		v, _ := num.Div(total).Mul(decimal.NewFromInt(100)).Round(2).Float64()
		return v
	}
	atLeast1 := pastDue["1-30"].Add(pastDue["31-60"]).Add(pastDue["61-90"]).Add(pastDue["90+"])
	atLeast30 := pastDue["31-60"].Add(pastDue["61-90"]).Add(pastDue["90+"])
	atLeast60 := pastDue["61-90"].Add(pastDue["90+"])
	rep.PAR1 = ratio(atLeast1)
	rep.PAR30 = ratio(atLeast30)
	rep.PAR60 = ratio(atLeast60)
	rep.PAR90 = ratio(pastDue["90+"])
	return rep, nil
}

// IFRS 9 stage-based Expected Credit Loss (ECL) parameters.
//
// ECL = EAD × PD × LGD, per IFRS 9 stage:
//   - EAD (exposure at default) = gross outstanding principal of the stage.
//   - PD  (probability of default): 12-month for Stage 1, lifetime for 2 & 3.
//   - LGD (loss given default): the share of exposure not recovered on default.
//
// The parameters below are point-estimates (not yet calibrated from historical
// default/recovery data — that calibration is the remaining follow-up). They are
// package-level consts so they are easy to find and tune, and the report exposes
// PD/LGD/EAD per stage so the provision is fully transparent/auditable.
const (
	// Probability of default per stage.
	PDStage1 = 0.02 // 2%  — performing (12-month PD)
	PDStage2 = 0.20 // 20% — significant increase in credit risk (lifetime PD)
	PDStage3 = 1.00 // 100% — credit-impaired / in default (lifetime PD)

	// Loss given default — unsecured baseline (Basel foundation-IRB ~45%).
	LGD = 0.45
)

// ECLStageProvision is the loan-loss provision for one IFRS 9 stage,
// decomposed into its EAD × PD × LGD components.
type ECLStageProvision struct {
	Stage            string          `json:"stage"`            // Stage 1, Stage 2, Stage 3
	Description      string          `json:"description"`      // human-readable stage meaning + DPD band
	Loans            int             `json:"loans"`            // number of loans in the stage
	GrossOutstanding decimal.Decimal `json:"grossOutstanding"` // gross outstanding principal
	EAD              decimal.Decimal `json:"ead"`              // exposure at default (= gross outstanding)
	PD               decimal.Decimal `json:"pd"`               // probability of default (fraction)
	LGD              decimal.Decimal `json:"lgd"`              // loss given default (fraction)
	ProvisionRate    decimal.Decimal `json:"provisionRate"`    // effective ECL rate = PD × LGD
	Provision        decimal.Decimal `json:"provision"`        // ECL = EAD × PD × LGD
}

// ECLProvisionReport is a simplified IFRS 9 stage-based loan-loss provisioning
// (Expected Credit Loss) report over the active loan book. READ-ONLY: it does
// not post to the general ledger. See the ECLRateStageN consts for the model.
type ECLProvisionReport struct {
	AsOf             string              `json:"asOf"`
	Stages           []ECLStageProvision `json:"stages"`
	TotalOutstanding decimal.Decimal     `json:"totalOutstanding"`
	TotalProvision   decimal.Decimal     `json:"totalProvision"`
	CoverageRatio    float64             `json:"coverageRatio"` // totalProvision / totalOutstanding, %
}

// GetECLProvisionReport buckets the active loan book into IFRS 9 stages by
// days-past-due and applies a flat provision (ECL) rate per stage to gross
// outstanding principal.
func (r *Repository) GetECLProvisionReport(ctx context.Context, tenantID string) (*ECLProvisionReport, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT
		     CASE
		         WHEN dpd <= 30            THEN 'Stage 1'
		         WHEN dpd BETWEEN 31 AND 90 THEN 'Stage 2'
		         ELSE 'Stage 3'
		     END AS stage,
		     COUNT(*),
		     COALESCE(SUM(outstanding_principal), 0)
		 FROM loans
		 WHERE tenant_id = $1 AND status IN ('ACTIVE', 'RESTRUCTURED')
		 GROUP BY 1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type stageAgg struct {
		loans       int
		outstanding decimal.Decimal
	}
	byStage := map[string]stageAgg{}
	for rows.Next() {
		var stage string
		var count int
		var outstanding decimal.Decimal
		if err := rows.Scan(&stage, &count, &outstanding); err != nil {
			return nil, err
		}
		byStage[stage] = stageAgg{loans: count, outstanding: outstanding}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	stageDefs := []struct {
		name string
		desc string
		pd   float64
	}{
		{"Stage 1", "Performing (dpd 0-30) — 12-month ECL", PDStage1},
		{"Stage 2", "Significant increase in credit risk (dpd 31-90) — lifetime ECL", PDStage2},
		{"Stage 3", "Credit-impaired / non-performing (dpd 90+) — lifetime ECL", PDStage3},
	}

	lgd := decimal.NewFromFloat(LGD)
	rep := &ECLProvisionReport{
		AsOf:             time.Now().UTC().Format("2006-01-02"),
		Stages:           make([]ECLStageProvision, 0, len(stageDefs)),
		TotalOutstanding: decimal.Zero,
		TotalProvision:   decimal.Zero,
	}
	for _, def := range stageDefs {
		agg := byStage[def.name] // zero value (0 loans, zero outstanding) if stage absent
		ead := agg.outstanding.Round(2)
		pd := decimal.NewFromFloat(def.pd)
		effRate := pd.Mul(lgd)                 // effective ECL rate = PD × LGD
		provision := ead.Mul(effRate).Round(2) // ECL = EAD × PD × LGD
		rep.Stages = append(rep.Stages, ECLStageProvision{
			Stage:            def.name,
			Description:      def.desc,
			Loans:            agg.loans,
			GrossOutstanding: ead,
			EAD:              ead,
			PD:               pd,
			LGD:              lgd,
			ProvisionRate:    effRate,
			Provision:        provision,
		})
		rep.TotalOutstanding = rep.TotalOutstanding.Add(ead)
		rep.TotalProvision = rep.TotalProvision.Add(provision)
	}
	rep.TotalOutstanding = rep.TotalOutstanding.Round(2)
	rep.TotalProvision = rep.TotalProvision.Round(2)
	if !rep.TotalOutstanding.IsZero() {
		cov, _ := rep.TotalProvision.Div(rep.TotalOutstanding).Mul(decimal.NewFromInt(100)).Round(2).Float64()
		rep.CoverageRatio = cov
	}
	return rep, nil
}
