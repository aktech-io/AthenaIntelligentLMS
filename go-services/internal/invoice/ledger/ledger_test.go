package ledger

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/athena-lms/go-services/internal/invoice/model"
)

var key = []byte("test-service-key")

func init() { decimal.MarshalJSONWithoutQuotes = true } // as every cmd/*/main.go does

func chain(t *testing.T, n int) []model.LedgerEvent {
	t.Helper()
	prev := ""
	var out []model.LedgerEvent
	base := time.Date(2026, 11, 8, 10, 0, 0, 123456789, time.UTC)
	for i := 0; i < n; i++ {
		ev := model.LedgerEvent{
			Seq:        int64(i + 1),
			TenantID:   "default",
			Type:       "invoice.uploaded",
			Payload:    map[string]any{"amount": decimal.RequireFromString("1500.50"), "i": i},
			Actor:      "officer",
			OccurredAt: base.Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, Link(&ev, prev, key))
		prev = ev.Hash
		out = append(out, ev)
	}
	return out
}

func TestLinkAndVerifyIntact(t *testing.T) {
	evs := chain(t, 5)
	assert.Equal(t, GenesisPrev, evs[0].PrevHash)
	ok, broken := Verify(evs, key)
	assert.True(t, ok)
	assert.Zero(t, broken)
	// occurred_at normalised to microseconds (timestamptz precision)
	assert.Equal(t, 123456000, evs[0].OccurredAt.Nanosecond())
}

func TestVerifyDetectsPayloadTamper(t *testing.T) {
	evs := chain(t, 4)
	evs[2].Payload["amount"] = 99.0
	ok, broken := Verify(evs, key)
	assert.False(t, ok)
	assert.Equal(t, int64(3), broken)
}

func TestVerifyDetectsDeletedLink(t *testing.T) {
	evs := chain(t, 4)
	cut := append(evs[:1:1], evs[2:]...) // drop seq 2
	ok, broken := Verify(cut, key)
	assert.False(t, ok)
	assert.Equal(t, int64(3), broken)
}

func TestVerifyDetectsBadSignature(t *testing.T) {
	evs := chain(t, 2)
	ok, _ := Verify(evs, []byte("other-key"))
	assert.False(t, ok)
	ok, _ = Verify(evs, nil) // no key: structure only
	assert.True(t, ok)
}

func TestCanonicalIsStableAcrossJSONRoundTrip(t *testing.T) {
	// Simulate a payload coming back from jsonb as generic types.
	evs := chain(t, 1)
	rt := evs[0]
	rt.Payload = map[string]any{"i": float64(0), "amount": 1500.5}
	h, err := Hash(rt.PrevHash, rt.Type, rt.Payload, rt.Actor, rt.OccurredAt)
	require.NoError(t, err)
	assert.Equal(t, evs[0].Hash, h)
}

func TestMerkleRoot(t *testing.T) {
	evs := chain(t, 3)
	hs := []string{evs[0].Hash, evs[1].Hash, evs[2].Hash}
	r1 := MerkleRoot(hs)
	assert.Len(t, r1, 64)
	assert.Equal(t, r1, MerkleRoot(hs))
	assert.NotEqual(t, r1, MerkleRoot(hs[:2]))
	assert.NotEqual(t, r1, MerkleRoot([]string{hs[1], hs[0], hs[2]}))
	assert.Len(t, MerkleRoot(nil), 64)
}
