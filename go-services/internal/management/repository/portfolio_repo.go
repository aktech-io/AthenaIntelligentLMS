package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/management/crb"
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
// The parameters below are benchmark point-estimates, NOT calibrated from this
// institution's own default/recovery experience: that data does not yet exist
// (no observed defaults, no recovery/write-off capture). Calibrating them is
// blocked on data, not engineering — see docs/IFRS9_PD_LGD_CALIBRATION.md for the
// finding, the basis for these values, and the data + method to calibrate later.
// They are package-level consts so they are easy to find and tune, and the report
// exposes PD/LGD/EAD per stage so the provision is fully transparent/auditable.
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

// GetCRBFeedRecords returns one canonical CRB borrower-performance record per loan
// that was active as of periodEnd (disbursed on/before it and not closed before
// it). OverdueAmount is aggregated from past-due unpaid schedule installments
// (due on/before periodEnd, still PENDING/PARTIAL). Ordered by customer for a
// stable feed. Product is the product_id reference (product metadata lives in the
// product service); Classification is the internal loan stage (the CBK prudential
// classification is added with H-4). Borrower PII enrichment (national ID/name,
// held outside this service) is a documented follow-up.
func (r *Repository) GetCRBFeedRecords(ctx context.Context, tenantID string, periodEnd time.Time) ([]crb.Record, error) {
	const q = `
		SELECT l.customer_id, l.id::text, l.product_id::text, l.currency,
		       l.disbursed_amount,
		       (l.outstanding_principal + l.outstanding_interest + l.outstanding_fees + l.outstanding_penalty) AS outstanding_balance,
		       l.outstanding_principal,
		       COALESCE(arr.overdue, 0) AS overdue_amount,
		       l.dpd, l.stage, l.status, l.disbursed_at, l.maturity_date, l.last_repayment_date
		FROM loans l
		LEFT JOIN (
		    SELECT loan_id, SUM(total_due - total_paid) AS overdue
		    FROM loan_schedules
		    WHERE due_date <= $2 AND status IN ('PENDING','PARTIAL')
		    GROUP BY loan_id
		) arr ON arr.loan_id = l.id
		WHERE l.tenant_id = $1
		  AND l.disbursed_at <= $2
		  AND (l.closed_at IS NULL OR l.closed_at > $2)
		ORDER BY l.customer_id, l.disbursed_at`
	rows, err := r.pool.Query(ctx, q, tenantID, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("query crb feed: %w", err)
	}
	defer rows.Close()

	var out []crb.Record
	for rows.Next() {
		var rec crb.Record
		var last *time.Time
		if err := rows.Scan(
			&rec.CustomerID, &rec.LoanAccountRef, &rec.Product, &rec.Currency,
			&rec.DisbursedAmount, &rec.OutstandingBalance, &rec.OutstandingPrincipal,
			&rec.OverdueAmount, &rec.DaysPastDue, &rec.Classification, &rec.AccountStatus,
			&rec.DisbursedAt, &rec.MaturityDate, &last,
		); err != nil {
			return nil, fmt.Errorf("scan crb record: %w", err)
		}
		rec.LastPaymentDate = last
		out = append(out, rec)
	}
	return out, rows.Err()
}

// CBKBucketAgg is the aggregated exposure in one CBK PG/04 classification bucket.
type CBKBucketAgg struct {
	Class       string
	Loans       int
	Outstanding decimal.Decimal
}

// GetCBKBuckets classifies the active loan book into the CBK PG/04 five buckets by
// days-past-due (the CORRECT prudential bands, distinct from internal staging) and
// sums gross outstanding principal per bucket.
func (r *Repository) GetCBKBuckets(ctx context.Context, tenantID string) ([]CBKBucketAgg, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT
		     CASE
		         WHEN dpd <= 30  THEN 'NORMAL'
		         WHEN dpd <= 90  THEN 'WATCH'
		         WHEN dpd <= 180 THEN 'SUBSTANDARD'
		         WHEN dpd <= 360 THEN 'DOUBTFUL'
		         ELSE 'LOSS'
		     END AS class,
		     COUNT(*),
		     COALESCE(SUM(outstanding_principal), 0)
		 FROM loans
		 WHERE tenant_id = $1 AND status IN ('ACTIVE','RESTRUCTURED')
		 GROUP BY 1`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query cbk buckets: %w", err)
	}
	defer rows.Close()

	var out []CBKBucketAgg
	for rows.Next() {
		var b CBKBucketAgg
		if err := rows.Scan(&b.Class, &b.Loans, &b.Outstanding); err != nil {
			return nil, fmt.Errorf("scan cbk bucket: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
