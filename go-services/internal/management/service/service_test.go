package service

import (
	"context"
	"database/sql"
	stderrors "errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/audit"
	cerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/management/client"
	"github.com/athena-lms/go-services/internal/management/event"
	"github.com/athena-lms/go-services/internal/management/model"
)

// ---------------------------------------------------------------------------
// In-memory repaymentStore fake (no DB) — mirrors the real repository's
// contract: lookups return pgx.ErrNoRows when missing, and inserting a
// duplicate (loan_id, payment_reference) fails with unique_violation 23505
// like the uq_repayments_loan_payment_ref partial unique index.
// ---------------------------------------------------------------------------

// fakeTx satisfies pgx.Tx for the calls ApplyRepayment makes directly on the
// transaction (Commit/Rollback); everything else panics if touched.
type fakeTx struct{ pgx.Tx }

func (fakeTx) Commit(context.Context) error   { return nil }
func (fakeTx) Rollback(context.Context) error { return nil }

type fakeStore struct {
	loan       *model.Loan
	schedules  []*model.LoanSchedule
	repayments []*model.LoanRepayment

	// hideExistingFromTxLookup simulates the check-then-insert race: the in-tx
	// dedup lookup misses, but the unique index still rejects the insert
	// (as if a concurrent transaction committed the reference in between).
	hideExistingFromTxLookup bool

	inserts     int
	loanUpdates int
}

func (f *fakeStore) BeginTx(context.Context) (pgx.Tx, error) { return fakeTx{}, nil }

func (f *fakeStore) GetLoanByIDAndTenantTx(_ context.Context, _ pgx.Tx, id uuid.UUID, tenantID string) (*model.Loan, error) {
	if f.loan != nil && f.loan.ID == id && f.loan.TenantID == tenantID {
		return f.loan, nil
	}
	return nil, pgx.ErrNoRows
}

func (f *fakeStore) findByReference(loanID uuid.UUID, reference string) (*model.LoanRepayment, error) {
	for _, rep := range f.repayments {
		if rep.LoanID == loanID && rep.PaymentReference.Valid && rep.PaymentReference.String == reference {
			return rep, nil
		}
	}
	return nil, pgx.ErrNoRows
}

func (f *fakeStore) GetRepaymentByLoanAndReference(_ context.Context, loanID uuid.UUID, reference string) (*model.LoanRepayment, error) {
	return f.findByReference(loanID, reference)
}

func (f *fakeStore) GetRepaymentByLoanAndReferenceTx(_ context.Context, _ pgx.Tx, loanID uuid.UUID, reference string) (*model.LoanRepayment, error) {
	if f.hideExistingFromTxLookup {
		return nil, pgx.ErrNoRows
	}
	return f.findByReference(loanID, reference)
}

func (f *fakeStore) GetPendingSchedulesTx(_ context.Context, _ pgx.Tx, loanID uuid.UUID) ([]*model.LoanSchedule, error) {
	var pending []*model.LoanSchedule
	for _, s := range f.schedules {
		if s.LoanID == loanID && (s.Status == model.InstallmentPending || s.Status == model.InstallmentPartial) {
			pending = append(pending, s)
		}
	}
	return pending, nil
}

func (f *fakeStore) UpdateScheduleTx(context.Context, pgx.Tx, *model.LoanSchedule) error { return nil }

func (f *fakeStore) UpdateLoanTx(context.Context, pgx.Tx, *model.Loan) error {
	f.loanUpdates++
	return nil
}

func (f *fakeStore) GetLoanByIDTx(_ context.Context, _ pgx.Tx, id uuid.UUID) (*model.Loan, error) {
	if f.loan != nil && f.loan.ID == id {
		return f.loan, nil
	}
	return nil, pgx.ErrNoRows
}

// AddSchedulePenaltyTx mirrors the real repository's relative UPDATE:
// penalty_due and total_due both grow by the accrued amount.
func (f *fakeStore) AddSchedulePenaltyTx(_ context.Context, _ pgx.Tx, scheduleID uuid.UUID, amount decimal.Decimal) error {
	for _, s := range f.schedules {
		if s.ID == scheduleID {
			s.PenaltyDue = s.PenaltyDue.Add(amount)
			s.TotalDue = s.TotalDue.Add(amount)
			return nil
		}
	}
	return pgx.ErrNoRows
}

// fakeProducts is an in-memory productTermsClient.
type fakeProducts struct {
	terms *client.PenaltyTerms
	err   error
	calls int
}

func (f *fakeProducts) GetPenaltyTerms(context.Context, uuid.UUID) (*client.PenaltyTerms, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.terms, nil
}

func (f *fakeStore) InsertRepaymentTx(_ context.Context, _ pgx.Tx, rep *model.LoanRepayment) (*model.LoanRepayment, error) {
	if rep.PaymentReference.Valid {
		if _, err := f.findByReference(rep.LoanID, rep.PaymentReference.String); err == nil {
			return nil, &pgconn.PgError{Code: "23505", ConstraintName: "uq_repayments_loan_payment_ref"}
		}
	}
	rep.ID = uuid.New()
	rep.CreatedAt = time.Now()
	f.repayments = append(f.repayments, rep)
	f.inserts++
	return rep, nil
}

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

func newTestService(store repaymentStore) *Service {
	logger := zap.NewNop()
	return &Service{
		store: store,
		// Publisher without a broker: publish() drops events (best-effort).
		publisher: event.NewManagementPublisher(nil, logger),
		logger:    logger,
		auditor:   audit.New(nil, logger), // nil Inserter => no-op
	}
}

func testLoan(tenantID string) *model.Loan {
	return &model.Loan{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ApplicationID:        uuid.New(),
		CustomerID:           "CUST-001",
		ProductID:            uuid.New(),
		DisbursedAmount:      decimal.NewFromInt(1000),
		OutstandingPrincipal: decimal.NewFromInt(1000),
		OutstandingInterest:  decimal.NewFromInt(100),
		OutstandingFees:      decimal.Zero,
		OutstandingPenalty:   decimal.Zero,
		Currency:             "KES",
		InterestRate:         decimal.NewFromInt(12),
		TenorMonths:          1,
		RepaymentFrequency:   model.FrequencyMonthly,
		ScheduleType:         model.ScheduleTypeEMI,
		Status:               model.LoanStatusActive,
		Stage:                model.LoanStagePerforming,
	}
}

func testSchedule(loan *model.Loan, no int, principalDue, interestDue decimal.Decimal) *model.LoanSchedule {
	return &model.LoanSchedule{
		ID:            uuid.New(),
		LoanID:        loan.ID,
		TenantID:      loan.TenantID,
		InstallmentNo: no,
		DueDate:       time.Now().AddDate(0, no, 0),
		PrincipalDue:  principalDue,
		InterestDue:   interestDue,
		TotalDue:      principalDue.Add(interestDue),
		PrincipalPaid: decimal.Zero,
		InterestPaid:  decimal.Zero,
		FeePaid:       decimal.Zero,
		PenaltyPaid:   decimal.Zero,
		TotalPaid:     decimal.Zero,
		Status:        model.InstallmentPending,
	}
}

// ---------------------------------------------------------------------------
// ApplyRepayment
// ---------------------------------------------------------------------------

func TestApplyRepayment_RejectsNonPositiveAmount(t *testing.T) {
	for name, amount := range map[string]decimal.Decimal{
		"zero":     decimal.Zero,
		"negative": decimal.NewFromInt(-500),
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{}
			svc := newTestService(store)

			resp, err := svc.ApplyRepayment(context.Background(), uuid.New(),
				&model.RepaymentRequest{Amount: amount}, "tenant1", "officer")

			require.Error(t, err)
			assert.Nil(t, resp)
			var bizErr *cerrors.BusinessError
			require.ErrorAs(t, err, &bizErr)
			assert.Equal(t, 400, bizErr.StatusCode)
			assert.Zero(t, store.inserts, "no repayment row must be written")
		})
	}
}

func TestApplyRepayment_DuplicateReferenceReturnsExisting(t *testing.T) {
	loan := testLoan("tenant1")
	sched := testSchedule(loan, 1, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	existing := &model.LoanRepayment{
		ID:               uuid.New(),
		LoanID:           loan.ID,
		TenantID:         loan.TenantID,
		Amount:           decimal.NewFromInt(500),
		Currency:         "KES",
		InterestApplied:  decimal.NewFromInt(100),
		PrincipalApplied: decimal.NewFromInt(400),
		PaymentReference: toNullString("PAY-REF-1"),
		PaymentDate:      time.Now(),
		CreatedAt:        time.Now(),
	}
	store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}, repayments: []*model.LoanRepayment{existing}}
	svc := newTestService(store)

	resp, err := svc.ApplyRepayment(context.Background(), loan.ID,
		&model.RepaymentRequest{Amount: decimal.NewFromInt(500), PaymentReference: "PAY-REF-1"},
		loan.TenantID, "officer")

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, existing.ID, resp.ID, "must return the originally recorded repayment")
	assert.Zero(t, store.inserts, "the duplicate must not be re-applied")
	assert.True(t, sched.TotalPaid.IsZero(), "schedule must not be re-allocated")
	assert.True(t, loan.OutstandingPrincipal.Equal(decimal.NewFromInt(1000)), "loan balance must be untouched")
}

func TestApplyRepayment_DuplicateReferenceRace_ReturnsWinner(t *testing.T) {
	// The in-tx dedup check misses (concurrent writer), the unique index
	// rejects the insert, and the service must re-fetch and return the winner.
	loan := testLoan("tenant1")
	sched := testSchedule(loan, 1, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	winner := &model.LoanRepayment{
		ID:               uuid.New(),
		LoanID:           loan.ID,
		TenantID:         loan.TenantID,
		Amount:           decimal.NewFromInt(500),
		Currency:         "KES",
		PaymentReference: toNullString("PAY-REF-RACE"),
		PaymentDate:      time.Now(),
		CreatedAt:        time.Now(),
	}
	store := &fakeStore{
		loan:                     loan,
		schedules:                []*model.LoanSchedule{sched},
		repayments:               []*model.LoanRepayment{winner},
		hideExistingFromTxLookup: true,
	}
	svc := newTestService(store)

	resp, err := svc.ApplyRepayment(context.Background(), loan.ID,
		&model.RepaymentRequest{Amount: decimal.NewFromInt(500), PaymentReference: "PAY-REF-RACE"},
		loan.TenantID, "officer")

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, winner.ID, resp.ID, "must return the winner's repayment after the unique violation")
	assert.Zero(t, store.inserts, "the loser's insert must not persist")
}

func TestApplyRepayment_OverpaymentRecordsUnallocatedSurplus(t *testing.T) {
	loan := testLoan("tenant1") // outstanding: 1000 principal + 100 interest
	sched := testSchedule(loan, 1, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
	svc := newTestService(store)

	// Pay 1500 against a total outstanding of 1100 -> 400 surplus.
	resp, err := svc.ApplyRepayment(context.Background(), loan.ID,
		&model.RepaymentRequest{Amount: decimal.NewFromInt(1500), PaymentReference: "PAY-OVER-1"},
		loan.TenantID, "officer")

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.InterestApplied.Equal(decimal.NewFromInt(100)), "interestApplied = %s", resp.InterestApplied)
	assert.True(t, resp.PrincipalApplied.Equal(decimal.NewFromInt(1000)), "principalApplied = %s", resp.PrincipalApplied)
	assert.True(t, resp.UnallocatedAmount.Equal(decimal.NewFromInt(400)),
		"surplus must be recorded, got %s", resp.UnallocatedAmount)

	require.Len(t, store.repayments, 1)
	assert.True(t, store.repayments[0].UnallocatedAmount.Equal(decimal.NewFromInt(400)),
		"surplus must be persisted on the repayment row")

	assert.Equal(t, model.LoanStatusClosed, loan.Status, "fully repaid loan must close")
	assert.True(t, loan.OutstandingPrincipal.IsZero())
	assert.True(t, loan.OutstandingInterest.IsZero())
}

func TestApplyRepayment_PartialPaymentFollowsWaterfall(t *testing.T) {
	loan := testLoan("tenant1")
	sched := testSchedule(loan, 1, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
	svc := newTestService(store)

	resp, err := svc.ApplyRepayment(context.Background(), loan.ID,
		&model.RepaymentRequest{Amount: decimal.NewFromInt(150), PaymentReference: "PAY-PART-1"},
		loan.TenantID, "officer")

	require.NoError(t, err)
	require.NotNil(t, resp)
	// Waterfall: penalty(0) -> fee(0) -> interest(100) -> principal(50).
	assert.True(t, resp.InterestApplied.Equal(decimal.NewFromInt(100)))
	assert.True(t, resp.PrincipalApplied.Equal(decimal.NewFromInt(50)))
	assert.True(t, resp.UnallocatedAmount.IsZero())
	assert.Equal(t, model.LoanStatusActive, loan.Status)
	assert.True(t, loan.OutstandingPrincipal.Equal(decimal.NewFromInt(950)))
	assert.True(t, loan.OutstandingInterest.IsZero())
	assert.Equal(t, model.InstallmentPartial, sched.Status)
}

func TestApplyRepayment_InactiveLoanRejected(t *testing.T) {
	loan := testLoan("tenant1")
	loan.Status = model.LoanStatusClosed
	store := &fakeStore{loan: loan}
	svc := newTestService(store)

	resp, err := svc.ApplyRepayment(context.Background(), loan.ID,
		&model.RepaymentRequest{Amount: decimal.NewFromInt(100)}, loan.TenantID, "officer")

	require.Error(t, err)
	assert.Nil(t, resp)
	var bizErr *cerrors.BusinessError
	require.ErrorAs(t, err, &bizErr)
	assert.Zero(t, store.inserts)
}

func toNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// ---------------------------------------------------------------------------
// Penalty accrual (BLOCKER-2)
// ---------------------------------------------------------------------------

func decPtr(d decimal.Decimal) *decimal.Decimal { return &d }
func intPtr(i int) *int                         { return &i }

// accrualToday is a fixed accrual date so due-date arithmetic is deterministic.
var accrualToday = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

// penaltyLoan returns a test loan with penalty terms set.
// Rate 36.5% p.a. gives an exact daily factor of 0.001 (36.5/100/365).
func penaltyLoan(graceDays int) *model.Loan {
	loan := testLoan("tenant1")
	loan.PenaltyRate = decPtr(decimal.NewFromFloat(36.5))
	loan.PenaltyGraceDays = intPtr(graceDays)
	return loan
}

func overdueSchedule(loan *model.Loan, no, daysOverdue int, principalDue, interestDue decimal.Decimal) *model.LoanSchedule {
	s := testSchedule(loan, no, principalDue, interestDue)
	s.DueDate = accrualToday.AddDate(0, 0, -daysOverdue)
	return s
}

func TestAccruePenalty_DailySimplePenaltyMath(t *testing.T) {
	loan := penaltyLoan(0)
	sched := overdueSchedule(loan, 1, 10, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
	svc := newTestService(store)

	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday))

	// daily = 36.5/100/365 x (1000 + 100 unpaid) = 0.001 x 1100 = 1.10
	assert.True(t, sched.PenaltyDue.Equal(decimal.NewFromFloat(1.10)), "penaltyDue = %s", sched.PenaltyDue)
	assert.True(t, sched.TotalDue.Equal(decimal.NewFromFloat(1101.10)),
		"totalDue must grow with the accrual, got %s", sched.TotalDue)
	assert.True(t, loan.OutstandingPenalty.Equal(decimal.NewFromFloat(1.10)),
		"outstandingPenalty = %s", loan.OutstandingPenalty)
	require.NotNil(t, loan.LastPenaltyAccrualDate)
	assert.True(t, loan.LastPenaltyAccrualDate.Equal(accrualToday))
}

func TestAccruePenalty_PartiallyPaidInstallmentAccruesOnUnpaidOnly(t *testing.T) {
	loan := penaltyLoan(0)
	sched := overdueSchedule(loan, 1, 10, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	sched.PrincipalPaid = decimal.NewFromInt(400)
	sched.InterestPaid = decimal.NewFromInt(100)
	sched.Status = model.InstallmentPartial
	store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
	svc := newTestService(store)

	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday))

	// unpaid = (1000-400) + (100-100) = 600 -> 0.001 x 600 = 0.60
	assert.True(t, sched.PenaltyDue.Equal(decimal.NewFromFloat(0.60)), "penaltyDue = %s", sched.PenaltyDue)
	assert.True(t, loan.OutstandingPenalty.Equal(decimal.NewFromFloat(0.60)))
}

func TestAccruePenalty_GraceDays(t *testing.T) {
	cases := []struct {
		name        string
		daysOverdue int
		graceDays   int
		accrues     bool
	}{
		{"within grace", 5, 5, false},
		{"one day past grace", 6, 5, true},
		{"not yet due", -3, 0, false},
		{"due today", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loan := penaltyLoan(tc.graceDays)
			sched := overdueSchedule(loan, 1, tc.daysOverdue, decimal.NewFromInt(1000), decimal.NewFromInt(100))
			store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
			svc := newTestService(store)

			require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday))

			if tc.accrues {
				assert.True(t, sched.PenaltyDue.Equal(decimal.NewFromFloat(1.10)), "penaltyDue = %s", sched.PenaltyDue)
			} else {
				assert.True(t, sched.PenaltyDue.IsZero(), "no penalty within grace, got %s", sched.PenaltyDue)
				assert.True(t, loan.OutstandingPenalty.IsZero())
			}
			// The day is marked processed either way.
			require.NotNil(t, loan.LastPenaltyAccrualDate)
		})
	}
}

func TestAccruePenalty_SameDayIdempotent(t *testing.T) {
	loan := penaltyLoan(0)
	sched := overdueSchedule(loan, 1, 10, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
	svc := newTestService(store)

	// First run accrues; a rerun the same day (e.g. scheduler restart) must not.
	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday))
	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday))
	assert.True(t, sched.PenaltyDue.Equal(decimal.NewFromFloat(1.10)),
		"same-day rerun must not double-accrue, got %s", sched.PenaltyDue)
	assert.True(t, loan.OutstandingPenalty.Equal(decimal.NewFromFloat(1.10)))

	// The next day accrues one more day's penalty.
	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday.AddDate(0, 0, 1)))
	assert.True(t, sched.PenaltyDue.Equal(decimal.NewFromFloat(2.20)), "penaltyDue = %s", sched.PenaltyDue)
	assert.True(t, loan.OutstandingPenalty.Equal(decimal.NewFromFloat(2.20)))
}

func TestAccruePenalty_InDuplumStopsAtPrincipal(t *testing.T) {
	loan := penaltyLoan(0)
	loan.OutstandingPenalty = loan.OutstandingPrincipal // already at the cap
	sched := overdueSchedule(loan, 1, 10, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
	svc := newTestService(store)

	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday))

	assert.True(t, sched.PenaltyDue.IsZero(), "in-duplum: no accrual once penalty >= principal")
	assert.True(t, loan.OutstandingPenalty.Equal(loan.OutstandingPrincipal))
	require.NotNil(t, loan.LastPenaltyAccrualDate, "the day still counts as processed")
}

func TestAccruePenalty_InDuplumCapsFinalIncrement(t *testing.T) {
	loan := penaltyLoan(0)
	// Headroom of 0.50 left before penalty reaches principal; the computed
	// daily accrual (1.10) must be clamped so the bucket never overshoots.
	loan.OutstandingPenalty = loan.OutstandingPrincipal.Sub(decimal.NewFromFloat(0.50))
	sched := overdueSchedule(loan, 1, 10, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
	svc := newTestService(store)

	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday))

	assert.True(t, sched.PenaltyDue.Equal(decimal.NewFromFloat(0.50)), "penaltyDue = %s", sched.PenaltyDue)
	assert.True(t, loan.OutstandingPenalty.Equal(loan.OutstandingPrincipal),
		"penalty must stop exactly at principal, got %s", loan.OutstandingPenalty)
}

func TestAccruePenalty_SkipsNonAccruingStatuses(t *testing.T) {
	for _, status := range []model.LoanStatus{model.LoanStatusClosed, model.LoanStatusWrittenOff, model.LoanStatusDefault} {
		t.Run(string(status), func(t *testing.T) {
			loan := penaltyLoan(0)
			loan.Status = status
			sched := overdueSchedule(loan, 1, 10, decimal.NewFromInt(1000), decimal.NewFromInt(100))
			store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
			svc := newTestService(store)

			require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday))
			assert.True(t, sched.PenaltyDue.IsZero())
			assert.Nil(t, loan.LastPenaltyAccrualDate)
			assert.Zero(t, store.loanUpdates)
		})
	}
}

func TestAccruePenalty_BackfillsNullTermsFromProductOnce(t *testing.T) {
	loan := testLoan("tenant1") // NULL penalty terms (legacy loan)
	sched := overdueSchedule(loan, 1, 10, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
	products := &fakeProducts{terms: &client.PenaltyTerms{
		PenaltyRate:      decPtr(decimal.NewFromFloat(36.5)),
		PenaltyGraceDays: intPtr(0),
	}}
	svc := newTestService(store)
	svc.products = products

	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday))
	assert.Equal(t, 1, products.calls)
	require.NotNil(t, loan.PenaltyRate)
	assert.True(t, loan.PenaltyRate.Equal(decimal.NewFromFloat(36.5)), "terms must be backfilled onto the loan")
	assert.True(t, sched.PenaltyDue.Equal(decimal.NewFromFloat(1.10)))

	// Next day: terms are already on the loan — no refetch.
	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday.AddDate(0, 0, 1)))
	assert.Equal(t, 1, products.calls, "backfill must happen once")
}

func TestAccruePenalty_ProductFetchFailureSkipsAndRetries(t *testing.T) {
	loan := testLoan("tenant1") // NULL penalty terms
	sched := overdueSchedule(loan, 1, 10, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
	products := &fakeProducts{err: stderrors.New("product-service down")}
	svc := newTestService(store)
	svc.products = products

	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday))

	assert.True(t, sched.PenaltyDue.IsZero(), "no rate is ever fabricated")
	assert.Nil(t, loan.PenaltyRate)
	assert.Nil(t, loan.LastPenaltyAccrualDate, "day is not marked processed, so the backfill retries next run")
	assert.Zero(t, store.loanUpdates)
}

func TestAccruePenalty_ProductWithoutPenaltyPersistsZeroRate(t *testing.T) {
	loan := testLoan("tenant1") // NULL penalty terms
	sched := overdueSchedule(loan, 1, 10, decimal.NewFromInt(1000), decimal.NewFromInt(100))
	store := &fakeStore{loan: loan, schedules: []*model.LoanSchedule{sched}}
	products := &fakeProducts{terms: &client.PenaltyTerms{}} // product defines no penalty
	svc := newTestService(store)
	svc.products = products

	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday))
	assert.True(t, sched.PenaltyDue.IsZero())
	require.NotNil(t, loan.PenaltyRate)
	assert.True(t, loan.PenaltyRate.IsZero(), "an explicit zero is persisted so the backfill happens once")

	require.NoError(t, svc.AccruePenaltyForLoan(context.Background(), loan.ID, accrualToday.AddDate(0, 0, 1)))
	assert.Equal(t, 1, products.calls)
}

// ---------------------------------------------------------------------------
// Write-off (BLOCKER-4)
// ---------------------------------------------------------------------------

func TestWriteOffLoan_TransitionsFromEachValidStatus(t *testing.T) {
	for _, status := range []model.LoanStatus{model.LoanStatusActive, model.LoanStatusRestructured, model.LoanStatusDefault} {
		t.Run(string(status), func(t *testing.T) {
			loan := testLoan("tenant1")
			loan.Status = status
			loan.OutstandingPenalty = decimal.NewFromInt(25)
			store := &fakeStore{loan: loan}
			svc := newTestService(store)

			require.NoError(t, svc.WriteOffLoan(context.Background(), loan.ID, "tenant1", "case-1"))

			assert.Equal(t, model.LoanStatusWrittenOff, loan.Status)
			require.NotNil(t, loan.WrittenOffAt)
			assert.Equal(t, 1, store.loanUpdates)
			// Outstanding buckets are kept — they are the recovery claim.
			assert.True(t, loan.OutstandingPrincipal.Equal(decimal.NewFromInt(1000)))
			assert.True(t, loan.OutstandingInterest.Equal(decimal.NewFromInt(100)))
			assert.True(t, loan.OutstandingPenalty.Equal(decimal.NewFromInt(25)))
		})
	}
}

func TestWriteOffLoan_AlreadyWrittenOffIsIdempotentNoOp(t *testing.T) {
	loan := testLoan("tenant1")
	loan.Status = model.LoanStatusWrittenOff
	writtenOffAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	loan.WrittenOffAt = &writtenOffAt
	store := &fakeStore{loan: loan}
	svc := newTestService(store)

	require.NoError(t, svc.WriteOffLoan(context.Background(), loan.ID, "tenant1", "case-1"),
		"redelivered write-off approval must ack, not error")
	assert.Zero(t, store.loanUpdates, "no second update")
	assert.True(t, loan.WrittenOffAt.Equal(writtenOffAt), "original timestamp preserved")
}

func TestWriteOffLoan_InvalidFromStatusRejected(t *testing.T) {
	loan := testLoan("tenant1")
	loan.Status = model.LoanStatusClosed
	store := &fakeStore{loan: loan}
	svc := newTestService(store)

	err := svc.WriteOffLoan(context.Background(), loan.ID, "tenant1", "case-1")

	var bizErr *cerrors.BusinessError
	require.ErrorAs(t, err, &bizErr)
	assert.Equal(t, model.LoanStatusClosed, loan.Status, "status unchanged")
	assert.Zero(t, store.loanUpdates)
}

func TestWriteOffLoan_MissingLoanNotFound(t *testing.T) {
	store := &fakeStore{}
	svc := newTestService(store)

	err := svc.WriteOffLoan(context.Background(), uuid.New(), "tenant1", "case-1")

	var nfErr *cerrors.NotFoundError
	require.ErrorAs(t, err, &nfErr)
}

func TestWriteOffLoan_TenantMismatchNotFound(t *testing.T) {
	loan := testLoan("tenant1")
	store := &fakeStore{loan: loan}
	svc := newTestService(store)

	err := svc.WriteOffLoan(context.Background(), loan.ID, "other-tenant", "case-1")

	var nfErr *cerrors.NotFoundError
	require.ErrorAs(t, err, &nfErr)
	assert.Equal(t, model.LoanStatusActive, loan.Status, "status unchanged")
}

func TestApplyRepayment_WrittenOffLoanRejectedAsBusinessError(t *testing.T) {
	// Post-write-off recoveries are a later feature: for now a repayment
	// against a WRITTEN_OFF loan must fail with a clean BusinessError.
	loan := testLoan("tenant1")
	loan.Status = model.LoanStatusWrittenOff
	store := &fakeStore{loan: loan}
	svc := newTestService(store)

	resp, err := svc.ApplyRepayment(context.Background(), loan.ID,
		&model.RepaymentRequest{Amount: decimal.NewFromInt(100)}, loan.TenantID, "officer")

	require.Error(t, err)
	assert.Nil(t, resp)
	var bizErr *cerrors.BusinessError
	require.ErrorAs(t, err, &bizErr)
	assert.Zero(t, store.inserts)
}
