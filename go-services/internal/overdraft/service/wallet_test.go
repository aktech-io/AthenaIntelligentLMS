package service

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/overdraft/model"
)

func dec(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	v, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("bad decimal %q: %v", s, err)
	}
	return v
}

func assertDecEqual(t *testing.T, name string, got, want decimal.Decimal) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s = %s, want %s", name, got.String(), want.String())
	}
}

// Full waterfall: deposit covers fees, interest and principal, and the surplus
// stays on the wallet. Verifies BLOCKER-6: fee and interest allocations must
// NOT credit the wallet balance.
func TestApplyDepositWaterfall_FullSettlement(t *testing.T) {
	facility := &model.OverdraftFacility{
		DrawnPrincipal:  dec(t, "50"),
		AccruedInterest: dec(t, "5"),
	}
	facility.RecalculateDrawnAmount()
	fees := []model.OverdraftFee{
		{ID: uuid.New(), Amount: dec(t, "10"), AmountPaid: decimal.Zero, Status: "PENDING"},
	}

	alloc := applyDepositWaterfall(dec(t, "-50"), dec(t, "100"), facility, fees)

	assertDecEqual(t, "FeesRepaid", alloc.FeesRepaid, dec(t, "10"))
	assertDecEqual(t, "InterestRepaid", alloc.InterestRepaid, dec(t, "5"))
	assertDecEqual(t, "PrincipalRepaid", alloc.PrincipalRepaid, dec(t, "50"))
	// balanceAfter = -50 + (100 - 10 - 5) = 35, NOT -50 + 100 = 50.
	assertDecEqual(t, "BalanceAfter", alloc.BalanceAfter, dec(t, "35"))

	assertDecEqual(t, "facility.DrawnPrincipal", facility.DrawnPrincipal, decimal.Zero)
	assertDecEqual(t, "facility.AccruedInterest", facility.AccruedInterest, decimal.Zero)
	assertDecEqual(t, "facility.DrawnAmount", facility.DrawnAmount, decimal.Zero)

	if fees[0].Status != "CHARGED" {
		t.Errorf("fully paid fee status = %s, want CHARGED", fees[0].Status)
	}
	assertDecEqual(t, "fee.AmountPaid", fees[0].AmountPaid, dec(t, "10"))
	if fees[0].ChargedAt == nil {
		t.Error("fully paid fee should have ChargedAt set")
	}
	if len(alloc.PaidFees) != 1 {
		t.Fatalf("PaidFees len = %d, want 1", len(alloc.PaidFees))
	}
}

// The exact double-credit scenario from the audit (BLOCKER-6): deposit 100
// against a -50 balance with 10 of the drawn amount being accrued interest.
// The old code produced balance +50 AND cleared the 10 interest — the 10
// existed in two places. Correct: the customer keeps 40.
func TestApplyDepositWaterfall_NoDoubleCredit(t *testing.T) {
	facility := &model.OverdraftFacility{
		DrawnPrincipal:  dec(t, "50"),
		AccruedInterest: dec(t, "10"),
	}
	facility.RecalculateDrawnAmount() // drawn = 60

	alloc := applyDepositWaterfall(dec(t, "-50"), dec(t, "100"), facility, nil)

	assertDecEqual(t, "InterestRepaid", alloc.InterestRepaid, dec(t, "10"))
	assertDecEqual(t, "PrincipalRepaid", alloc.PrincipalRepaid, dec(t, "50"))
	// balanceAfter = -50 + 100 - 10 = 40 (old buggy code: 50)
	assertDecEqual(t, "BalanceAfter", alloc.BalanceAfter, dec(t, "40"))
	assertDecEqual(t, "facility.DrawnAmount", facility.DrawnAmount, decimal.Zero)

	// Conservation: deposit == balance movement + fee income + interest income.
	movement := alloc.BalanceAfter.Sub(dec(t, "-50"))
	total := movement.Add(alloc.FeesRepaid).Add(alloc.InterestRepaid)
	assertDecEqual(t, "conservation (movement+fees+interest)", total, dec(t, "100"))
}

// A partial payment must NOT mark the whole fee CHARGED; it accumulates in
// AmountPaid and allocates against the unpaid remainder (BLOCKER-6, second bug).
func TestApplyDepositWaterfall_PartialFeePayment(t *testing.T) {
	facility := &model.OverdraftFacility{
		DrawnPrincipal:  dec(t, "30"),
		AccruedInterest: decimal.Zero,
	}
	facility.RecalculateDrawnAmount()
	fees := []model.OverdraftFee{
		// 20 fee with 5 already paid: outstanding remainder is 15.
		{ID: uuid.New(), Amount: dec(t, "20"), AmountPaid: dec(t, "5"), Status: "PENDING"},
	}

	alloc := applyDepositWaterfall(dec(t, "-30"), dec(t, "10"), facility, fees)

	assertDecEqual(t, "FeesRepaid", alloc.FeesRepaid, dec(t, "10"))
	assertDecEqual(t, "fee.AmountPaid", fees[0].AmountPaid, dec(t, "15"))
	if fees[0].Status != "PENDING" {
		t.Errorf("partially paid fee status = %s, want PENDING", fees[0].Status)
	}
	if fees[0].ChargedAt != nil {
		t.Error("partially paid fee must not have ChargedAt set")
	}
	// Whole deposit went to the fee: balance unchanged, principal untouched.
	assertDecEqual(t, "BalanceAfter", alloc.BalanceAfter, dec(t, "-30"))
	assertDecEqual(t, "facility.DrawnPrincipal", facility.DrawnPrincipal, dec(t, "30"))
	assertDecEqual(t, "InterestRepaid", alloc.InterestRepaid, decimal.Zero)
	assertDecEqual(t, "PrincipalRepaid", alloc.PrincipalRepaid, decimal.Zero)
}

// Waterfall order is fees -> interest -> principal; a deposit smaller than
// fees+interest never reaches principal.
func TestApplyDepositWaterfall_OrderFeesInterestPrincipal(t *testing.T) {
	facility := &model.OverdraftFacility{
		DrawnPrincipal:  dec(t, "50"),
		AccruedInterest: dec(t, "8"),
	}
	facility.RecalculateDrawnAmount()
	fees := []model.OverdraftFee{
		{ID: uuid.New(), Amount: dec(t, "10"), AmountPaid: decimal.Zero, Status: "PENDING"},
	}

	alloc := applyDepositWaterfall(dec(t, "-50"), dec(t, "12"), facility, fees)

	assertDecEqual(t, "FeesRepaid", alloc.FeesRepaid, dec(t, "10"))
	assertDecEqual(t, "InterestRepaid", alloc.InterestRepaid, dec(t, "2"))
	assertDecEqual(t, "PrincipalRepaid", alloc.PrincipalRepaid, decimal.Zero)
	assertDecEqual(t, "facility.AccruedInterest", facility.AccruedInterest, dec(t, "6"))
	assertDecEqual(t, "facility.DrawnPrincipal", facility.DrawnPrincipal, dec(t, "50"))
	// Everything went to fees+interest: balance is unchanged.
	assertDecEqual(t, "BalanceAfter", alloc.BalanceAfter, dec(t, "-50"))
}

// BLOCKER-7: status enforcement on the credit rail. Deposits stay allowed on
// FROZEN/SUSPENDED wallets (they reduce exposure); CLOSED rejects.
func TestCheckDepositAllowed(t *testing.T) {
	cases := []struct {
		status  string
		wantErr bool
	}{
		{"ACTIVE", false},
		{"SUSPENDED", false},
		{"FROZEN", false},
		{"CLOSED", true},
	}
	for _, c := range cases {
		err := checkDepositAllowed(c.status)
		if (err != nil) != c.wantErr {
			t.Errorf("checkDepositAllowed(%s) err = %v, wantErr %v", c.status, err, c.wantErr)
		}
	}
}

// BLOCKER-7: status enforcement on the debit rail. Only ACTIVE may withdraw.
func TestCheckWithdrawAllowed(t *testing.T) {
	cases := []struct {
		status  string
		wantErr bool
	}{
		{"ACTIVE", false},
		{"SUSPENDED", true},
		{"FROZEN", true},
		{"CLOSED", true},
	}
	for _, c := range cases {
		err := checkWithdrawAllowed(c.status)
		if (err != nil) != c.wantErr {
			t.Errorf("checkWithdrawAllowed(%s) err = %v, wantErr %v", c.status, err, c.wantErr)
		}
	}
}

// HIGH-6: with no scoring client configured, ApplyOverdraft must reject the
// application instead of fabricating {650, "B"} and approving a facility.
func TestApplyOverdraft_NilScoringClientFailsClosed(t *testing.T) {
	svc := NewWalletService(nil, nil, nil, zap.NewNop())

	resp, err := svc.ApplyOverdraft(context.Background(), uuid.New(), "tenant-a")
	if resp != nil {
		t.Fatal("expected no facility to be approved without a scoring client")
	}
	var be *errors.BusinessError
	if !stderrors.As(err, &be) {
		t.Fatalf("expected BusinessError, got %T: %v", err, err)
	}
}
