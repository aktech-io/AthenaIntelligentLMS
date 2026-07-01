// Package crb builds the Credit Reference Bureau borrower-performance feed from
// the loan book. It is bureau-AGNOSTIC: each active loan becomes a Record and a
// Mapper renders the Records to a bureau's template. v1 ships a generic CSV
// mapper; specific bureau (Metropol / TransUnion / Creditinfo) templates are
// added later as additional Mapper implementations, selected per-tenant by the
// regulatory profile's crb_bureau. See docs/REGULATORY_REPORTING_KE.md.
package crb

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

// Record is the canonical, bureau-agnostic borrower-performance record for a
// single loan account as of a reporting period end.
type Record struct {
	CustomerID           string
	LoanAccountRef       string
	Product              string
	Currency             string
	DisbursedAmount      decimal.Decimal
	OutstandingBalance   decimal.Decimal // principal + interest + fees + penalty
	OutstandingPrincipal decimal.Decimal
	OverdueAmount        decimal.Decimal // past-due unpaid installments as of period end
	DaysPastDue          int
	Classification       string // loan stage label (internal staging; CBK prudential
	// classification is added with H-4's correctly-banded scheme)
	AccountStatus   string
	DisbursedAt     time.Time
	MaturityDate    time.Time
	LastPaymentDate *time.Time
}

// Mapper renders canonical records to a bureau-specific representation. The
// bureau is chosen per-tenant via the regulatory profile; v1 provides CSVMapper.
type Mapper interface {
	// Bureau names the target format (e.g. "GENERIC_CSV", "METROPOL").
	Bureau() string
	// ContentType is the HTTP content type of Render's output.
	ContentType() string
	// Render serialises the records; an empty slice yields header-only output.
	Render(records []Record) ([]byte, error)
}

// CSVMapper is the default, bureau-agnostic CSV rendering. Specific bureaus add
// their own Mapper with the exact column order/format their template requires.
type CSVMapper struct{}

// Bureau identifies the generic CSV format.
func (CSVMapper) Bureau() string { return "GENERIC_CSV" }

// ContentType is text/csv.
func (CSVMapper) ContentType() string { return "text/csv" }

var csvHeader = []string{
	"customer_id", "loan_account_ref", "product", "currency",
	"disbursed_amount", "outstanding_balance", "outstanding_principal",
	"overdue_amount", "days_past_due", "classification", "account_status",
	"disbursed_at", "maturity_date", "last_payment_date",
}

// Render writes the records as RFC-4180 CSV with a stable header. Amounts use
// fixed 2dp; dates use YYYY-MM-DD; a nil last-payment date renders empty.
func (CSVMapper) Render(records []Record) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(csvHeader); err != nil {
		return nil, err
	}
	for _, r := range records {
		last := ""
		if r.LastPaymentDate != nil {
			last = r.LastPaymentDate.Format("2006-01-02")
		}
		if err := w.Write([]string{
			r.CustomerID, r.LoanAccountRef, r.Product, r.Currency,
			r.DisbursedAmount.StringFixed(2), r.OutstandingBalance.StringFixed(2),
			r.OutstandingPrincipal.StringFixed(2), r.OverdueAmount.StringFixed(2),
			strconv.Itoa(r.DaysPastDue), r.Classification, r.AccountStatus,
			r.DisbursedAt.Format("2006-01-02"), r.MaturityDate.Format("2006-01-02"), last,
		}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
