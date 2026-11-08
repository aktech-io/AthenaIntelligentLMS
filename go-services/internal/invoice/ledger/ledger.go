// Package ledger implements the hash-chained, HMAC-signed invoice ledger
// (SPEC §2 LedgerEvent / Anchor) as pure functions over model.LedgerEvent so
// the repository writes links and GET /timeline re-verifies them with the
// same code.
//
//	hash = sha256(prev_hash || canonical_json(type, payload, actor, occurred_at))
//	sig  = HMAC-SHA256(key = service key, msg = hash)        (v1; ed25519 later)
//
// Canonical JSON is encoding/json over a struct whose payload has been
// round-tripped through map[string]any, so keys are sorted and number
// formatting is identical whether the payload comes from Go or back out of
// jsonb. occurred_at is truncated to microseconds (timestamptz precision).
package ledger

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/athena-lms/go-services/internal/invoice/model"
)

// GenesisPrev is prev_hash of the first event in a tenant chain.
const GenesisPrev = "0000000000000000000000000000000000000000000000000000000000000000"

type canonical struct {
	Type       string         `json:"type"`
	Payload    map[string]any `json:"payload"`
	Actor      string         `json:"actor"`
	OccurredAt string         `json:"occurredAt"`
}

// Canonicalize returns the payload as a sorted-key map, round-tripped through
// JSON so decimals/time values become their JSON primitives.
func Canonicalize(payload any) (map[string]any, error) {
	if payload == nil {
		return map[string]any{}, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// NormalizeTime truncates to microseconds in UTC (Postgres timestamptz).
func NormalizeTime(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

// Hash computes the chain hash of an event given its prev_hash.
func Hash(prevHash, typ string, payload map[string]any, actor string, occurredAt time.Time) (string, error) {
	canon, err := Canonicalize(payload)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(canonical{
		Type:       typ,
		Payload:    canon,
		Actor:      actor,
		OccurredAt: NormalizeTime(occurredAt).Format(time.RFC3339Nano),
	})
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Sign returns HMAC-SHA256(key, hash) hex.
func Sign(key []byte, hash string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(hash))
	return hex.EncodeToString(m.Sum(nil))
}

// Link fills PrevHash/Hash/Sig on ev given the previous event hash.
func Link(ev *model.LedgerEvent, prevHash string, key []byte) error {
	if prevHash == "" {
		prevHash = GenesisPrev
	}
	ev.OccurredAt = NormalizeTime(ev.OccurredAt)
	canon, err := Canonicalize(ev.Payload)
	if err != nil {
		return err
	}
	ev.Payload = canon
	h, err := Hash(prevHash, ev.Type, ev.Payload, ev.Actor, ev.OccurredAt)
	if err != nil {
		return err
	}
	ev.PrevHash = prevHash
	ev.Hash = h
	ev.Sig = Sign(key, h)
	return nil
}

// ErrChainBroken is returned by Verify when a link does not recompute.
var ErrChainBroken = errors.New("ledger chain broken")

// Verify walks events (ascending seq, one tenant chain or a contiguous
// subset) and checks that each hash recomputes from its stored prev_hash and
// that each prev_hash equals the previous event's hash. When key is non-nil
// signatures are checked too. Returns the first broken seq, or 0 when intact.
//
// The first event may have any prev_hash (the subset may start mid-chain);
// only its own hash is recomputed.
func Verify(events []model.LedgerEvent, key []byte) (ok bool, brokenSeq int64) {
	for i, ev := range events {
		h, err := Hash(ev.PrevHash, ev.Type, ev.Payload, ev.Actor, ev.OccurredAt)
		if err != nil || h != ev.Hash {
			return false, ev.Seq
		}
		if i > 0 && ev.PrevHash != events[i-1].Hash {
			return false, ev.Seq
		}
		if key != nil && !hmac.Equal([]byte(Sign(key, ev.Hash)), []byte(ev.Sig)) {
			return false, ev.Seq
		}
	}
	return true, 0
}

// MerkleRoot computes a sha256 Merkle root over the given hex hashes in
// order (odd leaf duplicated). Empty input yields the hash of nothing.
func MerkleRoot(hashes []string) string {
	if len(hashes) == 0 {
		s := sha256.Sum256(nil)
		return hex.EncodeToString(s[:])
	}
	level := make([][]byte, 0, len(hashes))
	for _, h := range hashes {
		b, err := hex.DecodeString(h)
		if err != nil {
			b = []byte(h)
		}
		level = append(level, b)
	}
	for len(level) > 1 {
		if len(level)%2 == 1 {
			level = append(level, level[len(level)-1])
		}
		next := make([][]byte, 0, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			s := sha256.Sum256(append(append([]byte{}, level[i]...), level[i+1]...))
			next = append(next, s[:])
		}
		level = next
	}
	return hex.EncodeToString(level[0])
}

// Anchorer submits a Merkle root to an external timestamping service and
// returns a receipt. v1 ships only NoneAnchorer (method NONE: root computed
// and stored, nothing submitted).
type Anchorer interface {
	Method() string
	Anchor(root string) (receipt []byte, err error)
}

// NoneAnchorer stores roots without external anchoring.
type NoneAnchorer struct{}

func (NoneAnchorer) Method() string                { return "NONE" }
func (NoneAnchorer) Anchor(string) ([]byte, error) { return nil, nil }

// OTSAnchorer is the OpenTimestamps hook.
//
// TODO(invoice-finance v2): submit `root` to an OpenTimestamps calendar
// (https://a.pool.opentimestamps.org) via the `ots stamp` protocol, store the
// incomplete .ots proof as the receipt, and upgrade it once the Bitcoin
// attestation lands (`ots upgrade`). Until then this returns an error so the
// hourly job falls back to NONE rather than recording a fake receipt.
type OTSAnchorer struct{ CalendarURL string }

func (OTSAnchorer) Method() string { return "OTS" }
func (OTSAnchorer) Anchor(string) ([]byte, error) {
	return nil, errors.New("OTS anchoring not implemented in v1")
}
