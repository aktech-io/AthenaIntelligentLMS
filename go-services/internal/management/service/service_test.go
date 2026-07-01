package service

import (
	"context"
	"database/sql"
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

	inserts int
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

func (f *fakeStore) UpdateLoanTx(context.Context, pgx.Tx, *model.Loan) error { return nil }

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
