package provisioning

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestClassifyCBKBands(t *testing.T) {
	cases := []struct {
		dpd  int
		code string
		rate string
	}{
		{0, "NORMAL", "0.01"},
		{30, "NORMAL", "0.01"},
		{31, "WATCH", "0.03"},
		{90, "WATCH", "0.03"},
		{91, "SUBSTANDARD", "0.2"},
		{180, "SUBSTANDARD", "0.2"},
		{181, "DOUBTFUL", "0.5"},
		{360, "DOUBTFUL", "0.5"},
		{361, "LOSS", "1"},
		{5000, "LOSS", "1"},
	}
	for _, c := range cases {
		code, rate := ClassifyCBK(c.dpd)
		if code != c.code {
			t.Errorf("dpd %d: want class %s, got %s", c.dpd, c.code, code)
		}
		if rate.String() != c.rate {
			t.Errorf("dpd %d: want rate %s, got %s", c.dpd, c.rate, rate.String())
		}
	}
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestBuildReportHigherOfAndStatutoryReserve(t *testing.T) {
	// Substandard 91-180 @ 20%: 100000 → CBK 20000. Loss @ 100%: 10000 → 10000.
	// Normal @ 1%: 1,000,000 → 10000. CBK total = 40000.
	inputs := []BucketInput{
		{Class: "NORMAL", Loans: 50, Outstanding: dec("1000000")},
		{Class: "SUBSTANDARD", Loans: 3, Outstanding: dec("100000")},
		{Class: "LOSS", Loans: 1, Outstanding: dec("10000")},
	}

	// Case A: CBK (40000) > IFRS (25000). Statutory reserve = 15000; P&L = IFRS.
	rep := BuildReport("2026-06-30", inputs, dec("25000"))
	if rep.CBKProvision.String() != "40000" {
		t.Fatalf("CBK total want 40000, got %s", rep.CBKProvision)
	}
	if rep.RequiredAllowance.String() != "40000" {
		t.Fatalf("required allowance want higher-of 40000, got %s", rep.RequiredAllowance)
	}
	if rep.PLImpairmentCharge.String() != "25000" {
		t.Fatalf("P&L charge want IFRS 25000, got %s", rep.PLImpairmentCharge)
	}
	if rep.StatutoryLoanLossReserve.String() != "15000" {
		t.Fatalf("statutory reserve want 15000, got %s", rep.StatutoryLoanLossReserve)
	}
	if len(rep.Buckets) != 5 {
		t.Fatalf("want 5 canonical buckets, got %d", len(rep.Buckets))
	}

	// Case B: IFRS (60000) > CBK (40000). No statutory reserve; allowance = IFRS.
	rep = BuildReport("2026-06-30", inputs, dec("60000"))
	if rep.RequiredAllowance.String() != "60000" {
		t.Fatalf("required allowance want higher-of 60000, got %s", rep.RequiredAllowance)
	}
	if !rep.StatutoryLoanLossReserve.IsZero() {
		t.Fatalf("statutory reserve should be zero when IFRS>CBK, got %s", rep.StatutoryLoanLossReserve)
	}
}
