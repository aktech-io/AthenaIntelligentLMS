package crb

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestCSVMapperRendersHeaderAndRows(t *testing.T) {
	last := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	recs := []Record{{
		CustomerID:           "CUST-1",
		LoanAccountRef:       "loan-abc",
		Product:              "prod-x",
		Currency:             "KES",
		DisbursedAmount:      decimal.RequireFromString("10000"),
		OutstandingBalance:   decimal.RequireFromString("7500.50"),
		OutstandingPrincipal: decimal.RequireFromString("7000"),
		OverdueAmount:        decimal.RequireFromString("1200.25"),
		DaysPastDue:          45,
		Classification:       "SUBSTANDARD",
		AccountStatus:        "ACTIVE",
		DisbursedAt:          time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		MaturityDate:         time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
		LastPaymentDate:      &last,
	}}

	out, err := CSVMapper{}.Render(recs)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(out))).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want header + 1 row, got %d rows", len(rows))
	}
	if len(rows[0]) != len(csvHeader) {
		t.Fatalf("header width %d != %d", len(rows[0]), len(csvHeader))
	}
	got := rows[1]
	if got[0] != "CUST-1" || got[4] != "10000.00" || got[7] != "1200.25" || got[8] != "45" {
		t.Fatalf("row not rendered as expected: %v", got)
	}
	if got[13] != "2026-05-20" {
		t.Fatalf("last_payment_date want 2026-05-20, got %q", got[13])
	}
}

func TestCSVMapperEmptyIsHeaderOnly(t *testing.T) {
	out, err := CSVMapper{}.Render(nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := csv.NewReader(strings.NewReader(string(out))).ReadAll()
	if len(rows) != 1 {
		t.Fatalf("empty feed should be header-only, got %d rows", len(rows))
	}
}
