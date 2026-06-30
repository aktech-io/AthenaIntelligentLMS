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
	mu     sync.Mutex
	posted map[string]int // "sourceEvent|sourceID" -> number of times posted
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
