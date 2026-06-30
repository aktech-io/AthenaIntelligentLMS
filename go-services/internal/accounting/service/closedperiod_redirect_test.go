package service

import (
	"testing"
	"time"
)

// TestResolveOpenPostingDate covers H-2: system entries must never land in a
// closed fiscal period — they are redirected to the current open period.
func TestResolveOpenPostingDate(t *testing.T) {
	jan := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)

	// 1. Preferred period open → pass through unchanged, no redirect.
	got, redir, ok := resolveOpenPostingDate(jan, now, func(_, _ int) bool { return false })
	if !ok || redir || !got.Equal(jan) {
		t.Fatalf("open period should pass through: got=%v redir=%v ok=%v", got, redir, ok)
	}

	// 2. Preferred closed, current month (now) open → redirect to now's date.
	closedJan := func(y, m int) bool { return y == 2026 && m == 1 }
	got, redir, ok = resolveOpenPostingDate(jan, now, closedJan)
	if !ok || !redir || got.Year() != 2026 || got.Month() != time.March {
		t.Fatalf("should redirect to current open month: got=%v redir=%v ok=%v", got, redir, ok)
	}

	// 3. Preferred AND current closed → walk forward to first open month (April),
	//    dated the 1st.
	closedJanMar := func(y, m int) bool { return y == 2026 && (m == 1 || m == 3) }
	got, redir, ok = resolveOpenPostingDate(jan, now, closedJanMar)
	if !ok || !redir || got.Year() != 2026 || got.Month() != time.April || got.Day() != 1 {
		t.Fatalf("should walk forward to next open month start: got=%v redir=%v ok=%v", got, redir, ok)
	}

	// 4. No open period anywhere → fail closed (ok=false), never post into a
	//    closed period.
	if _, _, ok = resolveOpenPostingDate(jan, now, func(_, _ int) bool { return true }); ok {
		t.Fatalf("all-closed should return ok=false (fail closed)")
	}
}
