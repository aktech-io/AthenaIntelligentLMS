package consumer

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/event"
)

// fakePoster is an in-memory stand-in for *service.AccountingService that mirrors
// the real EntryExists/PostX idempotency contract: a posting is recorded under
// (sourceEvent, sourceID) and EntryExists reports whether that key was already
// posted. This lets us prove that a redelivered event is a no-op without a DB.
type fakePoster struct {
	mu         sync.Mutex
	posted     map[string]int // "sourceEvent|sourceID" -> number of times posted
	amounts    map[string]decimal.Decimal
	currencies map[string]string
}

func newFakePoster() *fakePoster { return &fakePoster{posted: map[string]int{}} }

func fkey(sourceEvent, sourceID string) string { return sourceEvent + "|" + sourceID }

func (f *fakePoster) EntryExists(_ context.Context, sourceEvent, sourceID string) bool {
	if sourceID == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posted[fkey(sourceEvent, sourceID)] > 0
}

func (f *fakePoster) record(sourceEvent, sourceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posted[fkey(sourceEvent, sourceID)]++
}

// count returns the total number of postings recorded for an event type.
func (f *fakePoster) count(sourceEvent string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for k, n := range f.posted {
		if len(k) >= len(sourceEvent) && k[:len(sourceEvent)] == sourceEvent && k[len(sourceEvent)] == '|' {
			total += n
		}
	}
	return total
}

func (f *fakePoster) PostLoanDisbursement(_ context.Context, _, applicationID string, _ decimal.Decimal) error {
	f.record("loan.disbursed", applicationID)
	return nil
}
func (f *fakePoster) PostRepayment(_ context.Context, _, paymentID string, _ decimal.Decimal, _ map[string]any) error {
	f.record("payment.completed", paymentID)
	return nil
}
func (f *fakePoster) PostPaymentReversal(_ context.Context, _, paymentID string, _ decimal.Decimal) error {
	f.record("payment.reversed", paymentID)
	return nil
}
func (f *fakePoster) PostOverdraftDrawn(_ context.Context, _, sourceID string, _ decimal.Decimal) error {
	f.record("overdraft.drawn", sourceID)
	return nil
}
func (f *fakePoster) PostOverdraftRepaid(_ context.Context, _, sourceID string, _ decimal.Decimal) error {
	f.record("overdraft.repaid", sourceID)
	return nil
}
func (f *fakePoster) PostOverdraftInterestCharged(_ context.Context, _, sourceID string, _ decimal.Decimal) error {
	f.record("overdraft.interest.charged", sourceID)
	return nil
}
func (f *fakePoster) PostOverdraftFeeCharged(_ context.Context, _, sourceID string, _ decimal.Decimal) error {
	f.record("overdraft.fee.charged", sourceID)
	return nil
}
func (f *fakePoster) PostFloatDrawn(_ context.Context, _, sourceID string, _ decimal.Decimal) error {
	f.record("float.drawn", sourceID)
	return nil
}
func (f *fakePoster) PostFloatRepaid(_ context.Context, _, sourceID string, _ decimal.Decimal) error {
	f.record("float.repaid", sourceID)
	return nil
}
func (f *fakePoster) PostTransferCharge(_ context.Context, _, sourceID string, amount decimal.Decimal, currency string) error {
	f.record("transfer.completed", sourceID)
	f.remember(sourceID, amount, currency)
	return nil
}
func (f *fakePoster) PostLoanFeeCharged(_ context.Context, _, sourceID string, amount decimal.Decimal, currency, _ string) error {
	f.record("loan.fee.charged", sourceID)
	f.remember(sourceID, amount, currency)
	return nil
}
func (f *fakePoster) PostPenaltyAccrued(_ context.Context, _, sourceID string, amount decimal.Decimal, currency string) error {
	f.record("loan.penalty.accrued", sourceID)
	f.remember(sourceID, amount, currency)
	return nil
}
func (f *fakePoster) PostLoanWriteOff(_ context.Context, _, sourceID string, amount decimal.Decimal, currency string) error {
	f.record("loan.written.off", sourceID)
	f.remember(sourceID, amount, currency)
	return nil
}

// remember stores the amount/currency a posting was called with, keyed by
// sourceID, so tests can assert the handler consumed the right payload fields.
func (f *fakePoster) remember(sourceID string, amount decimal.Decimal, currency string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.amounts == nil {
		f.amounts = map[string]decimal.Decimal{}
		f.currencies = map[string]string{}
	}
	f.amounts[sourceID] = amount
	f.currencies[sourceID] = currency
}

// makeEvent builds a DomainEvent envelope with a fixed ID (as RabbitMQ would
// redeliver byte-for-byte) carrying the given payload.
func makeEvent(t *testing.T, id, eventType, tenantID string, payload map[string]any) *event.DomainEvent {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return &event.DomainEvent{
		ID:       id,
		Type:     eventType,
		Version:  1,
		Source:   "test",
		TenantID: tenantID,
		Payload:  raw,
	}
}

// TestRedeliveredEventPostsExactlyOnce is the H-3 regression test: an
// at-least-once redelivery of the SAME event (identical envelope ID) must dedupe
// to a single GL posting. Before the fix these handlers built sourceID from
// time.Now(), so every redelivery produced a fresh key that EntryExists could
// never match, double-posting the cash entry.
func TestRedeliveredEventPostsExactlyOnce(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		payload   map[string]any
	}{
		{
			name:      "overdraft.repaid",
			eventType: "overdraft.repaid",
			payload:   map[string]any{"walletId": "wallet-1", "amount": 500.0},
		},
		{
			name:      "overdraft.interest.charged",
			eventType: "overdraft.interest.charged",
			payload:   map[string]any{"walletId": "wallet-1", "interestCharged": 25.0},
		},
		{
			name:      "float.repaid",
			eventType: "float.repaid",
			payload:   map[string]any{"floatAccountId": "float-1", "amount": 1000.0},
		},
		{
			name:      "transfer.completed charge",
			eventType: "transfer.completed",
			payload: map[string]any{"transferId": "txf-1", "sourceAccountId": "a", "destinationAccountId": "b",
				"amount": "1000", "chargeAmount": "25", "currency": "KES"},
		},
		{
			name:      "loan.fee.charged",
			eventType: "loan.fee.charged",
			payload: map[string]any{"applicationId": "app-1", "customerId": "c-1", "feeType": "UPFRONT",
				"feeName": "Processing Fee", "amount": "500", "currency": "KES", "reference": "FEE-REF-1"},
		},
		{
			name:      "loan.penalty.accrued",
			eventType: "loan.penalty.accrued",
			payload: map[string]any{"loanId": "loan-1", "customerId": "c-1", "amount": "75",
				"currency": "KES", "accrualDate": "2026-07-01"},
		},
		{
			name:      "loan.written.off",
			eventType: "loan.written.off",
			payload: map[string]any{"loanId": "loan-1", "customerId": "c-1", "principalWrittenOff": "800",
				"interestWrittenOff": "150", "feesWrittenOff": "30", "penaltyWrittenOff": "20",
				"totalWrittenOff": "1000", "currency": "KES", "caseId": "case-1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakePoster()
			c := New(fake, nil, zap.NewNop())
			evt := makeEvent(t, uuid.NewString(), tc.eventType, "tenant-1", tc.payload)

			// First delivery posts; redelivery (same envelope ID) must be a no-op.
			require.NoError(t, c.handle(context.Background(), evt))
			require.NoError(t, c.handle(context.Background(), evt))
			require.NoError(t, c.handle(context.Background(), evt))

			assert.Equal(t, 1, fake.count(tc.eventType),
				"%s: redelivered event must post exactly once", tc.eventType)
		})
	}
}

// TestDistinctEventsPostSeparately guards against over-deduping: two genuinely
// different events (distinct envelope IDs) for the same wallet must each post.
func TestDistinctEventsPostSeparately(t *testing.T) {
	fake := newFakePoster()
	c := New(fake, nil, zap.NewNop())

	payload := map[string]any{"walletId": "wallet-1", "amount": 500.0}
	first := makeEvent(t, uuid.NewString(), "overdraft.repaid", "tenant-1", payload)
	second := makeEvent(t, uuid.NewString(), "overdraft.repaid", "tenant-1", payload)

	require.NoError(t, c.handle(context.Background(), first))
	require.NoError(t, c.handle(context.Background(), second))

	assert.Equal(t, 2, fake.count("overdraft.repaid"),
		"two distinct repayment events must each post")
}

// TestTransferCompletedChargePosting verifies the HIGH-2 pipeline end of the
// consumer: only chargeAmount > 0 produces a GL posting, and the handler passes
// the charge (not the principal) and the event currency to the poster.
func TestTransferCompletedChargePosting(t *testing.T) {
	t.Run("zero or absent charge posts nothing", func(t *testing.T) {
		for _, payload := range []map[string]any{
			{"transferId": "txf-1", "amount": "1000", "chargeAmount": "0", "currency": "KES"},
			{"transferId": "txf-2", "amount": "1000"}, // pre-fix producers: no charge fields
		} {
			fake := newFakePoster()
			c := New(fake, nil, zap.NewNop())
			evt := makeEvent(t, uuid.NewString(), "transfer.completed", "tenant-1", payload)
			require.NoError(t, c.handle(context.Background(), evt))
			assert.Equal(t, 0, fake.count("transfer.completed"),
				"transfer without a positive charge must not post")
		}
	})

	t.Run("positive charge posts the charge with the event currency", func(t *testing.T) {
		fake := newFakePoster()
		c := New(fake, nil, zap.NewNop())
		payload := map[string]any{"transferId": "txf-3", "amount": "1000", "chargeAmount": "25.50", "currency": "UGX"}
		evt := makeEvent(t, uuid.NewString(), "transfer.completed", "tenant-1", payload)

		require.NoError(t, c.handle(context.Background(), evt))
		require.Equal(t, 1, fake.count("transfer.completed"))
		assert.True(t, fake.amounts["TXF-CHG-txf-3"].Equal(decimal.RequireFromString("25.50")),
			"posted amount must be the charge, not the transfer principal")
		assert.Equal(t, "UGX", fake.currencies["TXF-CHG-txf-3"])
	})
}

// TestPenaltyAccrualNaturalKey verifies the (loanId, accrualDate) dedup key:
// the same loan/day accrual must post once even across distinct envelope IDs
// (e.g. the accrual job re-ran and re-published), while accruals for different
// days post separately.
func TestPenaltyAccrualNaturalKey(t *testing.T) {
	fake := newFakePoster()
	c := New(fake, nil, zap.NewNop())

	day1 := map[string]any{"loanId": "loan-9", "amount": "50", "currency": "KES", "accrualDate": "2026-07-01"}
	day2 := map[string]any{"loanId": "loan-9", "amount": "50", "currency": "KES", "accrualDate": "2026-07-02"}

	require.NoError(t, c.handle(context.Background(), makeEvent(t, uuid.NewString(), "loan.penalty.accrued", "tenant-1", day1)))
	require.NoError(t, c.handle(context.Background(), makeEvent(t, uuid.NewString(), "loan.penalty.accrued", "tenant-1", day1)))
	require.NoError(t, c.handle(context.Background(), makeEvent(t, uuid.NewString(), "loan.penalty.accrued", "tenant-1", day2)))

	assert.Equal(t, 1, fake.posted[fkey("loan.penalty.accrued", "PEN-ACCR-loan-9-2026-07-01")],
		"same loan/day accrual must post exactly once")
	assert.Equal(t, 1, fake.posted[fkey("loan.penalty.accrued", "PEN-ACCR-loan-9-2026-07-02")],
		"a different accrual day is a distinct posting")
}

// TestWriteOffPostsOncePerLoan: a loan is written off at most once — even two
// genuinely distinct loan.written.off events for the same loan must dedup, and
// the posted amount is totalWrittenOff.
func TestWriteOffPostsOncePerLoan(t *testing.T) {
	fake := newFakePoster()
	c := New(fake, nil, zap.NewNop())

	payload := map[string]any{
		"loanId": "loan-7", "principalWrittenOff": "800", "interestWrittenOff": "150",
		"feesWrittenOff": "30", "penaltyWrittenOff": "20", "totalWrittenOff": "1000", "currency": "KES",
	}
	require.NoError(t, c.handle(context.Background(), makeEvent(t, uuid.NewString(), "loan.written.off", "tenant-1", payload)))
	require.NoError(t, c.handle(context.Background(), makeEvent(t, uuid.NewString(), "loan.written.off", "tenant-1", payload)))

	assert.Equal(t, 1, fake.count("loan.written.off"))
	assert.True(t, fake.amounts["WOFF-loan-7"].Equal(decimal.RequireFromString("1000")),
		"write-off must post totalWrittenOff")
}

// TestLoanFeeChargedDedupsOnReference: fees dedup on the business reference, so
// a re-published fee event (distinct envelope ID, same reference) posts once.
func TestLoanFeeChargedDedupsOnReference(t *testing.T) {
	fake := newFakePoster()
	c := New(fake, nil, zap.NewNop())

	payload := map[string]any{"applicationId": "app-1", "feeType": "UPFRONT", "feeName": "Processing Fee",
		"amount": "500", "currency": "KES", "reference": "FEE-REF-9"}
	require.NoError(t, c.handle(context.Background(), makeEvent(t, uuid.NewString(), "loan.fee.charged", "tenant-1", payload)))
	require.NoError(t, c.handle(context.Background(), makeEvent(t, uuid.NewString(), "loan.fee.charged", "tenant-1", payload)))

	assert.Equal(t, 1, fake.count("loan.fee.charged"))
	assert.True(t, fake.amounts["LOAN-FEE-FEE-REF-9"].Equal(decimal.RequireFromString("500")))
}
