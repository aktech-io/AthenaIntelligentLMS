package decision

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"
)

// testPolicy exercises every v1 construct: a required model with staleness,
// a when-rule guard, and a band table with APPROVE/REFER/DECLINE tiers.
const testPolicy = `
policy: test.facility
version: 1
market: "*"
tenant: "*"
models:
  credit_score:
    source: ai-scoring-service
    required: true
    max_age: 30d
on_model_unavailable: DECLINE
rules:
  - id: T-KYC
    when: "kyc_status != 'PASSED'"
    outcome: DECLINE
    reason: KYC_INCOMPLETE
  - id: T-BAND
    table:
      - { band: A, limit: 100000, rate: 0.15, outcome: APPROVE }
      - { band: B, limit: 50000,  rate: 0.20, outcome: APPROVE }
      - { band: C, limit: 20000,  rate: 0.25, outcome: REFER, queue: credit-review, reason: SCORE_BAND_LOW }
      - { band: D, outcome: DECLINE, reason: SCORE_BAND_LOW }
`

func testEvaluator(t *testing.T, docs ...string) *Evaluator {
	t.Helper()
	r := NewRegistry()
	for i, doc := range docs {
		if err := r.RegisterYAML([]byte(doc), fmt.Sprintf("test-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	return NewEvaluator(r)
}

func freshScore(score float64) []ModelRef {
	at := time.Now().Add(-time.Hour)
	return []ModelRef{{
		Name: "credit_score", Version: "athena-v1", Available: true,
		Score: &score, ScoredAt: &at,
	}}
}

func request(band string, models []ModelRef) Request {
	return Request{
		Type:        "test.facility",
		TenantID:    "t1",
		Market:      "KE",
		SubjectType: "wallet",
		SubjectID:   "w-1",
		CustomerID:  "c-1",
		Actor:       SystemActor("test-service"),
		Inputs:      map[string]any{"band": band, "kyc_status": "PASSED", "score": 700},
		Models:      models,
	}
}

func TestEvaluate_BandTable(t *testing.T) {
	e := testEvaluator(t, testPolicy)
	cases := []struct {
		band        string
		want        string
		wantLimit   any
		wantReason  string
		wantReasons int
	}{
		{"A", Approve, 100000, "", 0},
		{"B", Approve, 50000, "", 0},
		{"C", Refer, 20000, ReasonScoreBandLow, 1},
		{"D", Decline, nil, ReasonScoreBandLow, 1},
		// Unknown band: no row matches, no later rule decides ⇒ fail closed.
		{"E", Decline, nil, ReasonPolicyNoMatch, 1},
		{"", Decline, nil, ReasonPolicyNoMatch, 1},
	}
	for _, c := range cases {
		t.Run("band-"+c.band, func(t *testing.T) {
			out, err := e.Evaluate(context.Background(), request(c.band, freshScore(700)))
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if out.Decision != c.want {
				t.Fatalf("band %q decision = %s, want %s", c.band, out.Decision, c.want)
			}
			if c.wantLimit != nil && out.Detail["limit"] != c.wantLimit {
				t.Errorf("band %q limit = %v, want %v", c.band, out.Detail["limit"], c.wantLimit)
			}
			if len(out.Reasons) != c.wantReasons {
				t.Fatalf("band %q reasons = %+v, want %d", c.band, out.Reasons, c.wantReasons)
			}
			if c.wantReason != "" && out.Reasons[0].Code != c.wantReason {
				t.Errorf("band %q reason = %s, want %s", c.band, out.Reasons[0].Code, c.wantReason)
			}
			if out.Variant != VariantChampion {
				t.Errorf("variant = %q, want champion", out.Variant)
			}
			if out.Policy.ID != "test.facility" || out.Policy.Version != 1 || out.Policy.Hash == "" {
				t.Errorf("outcome policy ref not pinned: %+v", out.Policy)
			}
		})
	}
}

func TestEvaluate_WhenRuleWinsInOrder(t *testing.T) {
	e := testEvaluator(t, testPolicy)
	req := request("A", freshScore(700))
	req.Inputs["kyc_status"] = "PENDING"
	out, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != Decline {
		t.Fatalf("decision = %s, want DECLINE (KYC guard ordered before band table)", out.Decision)
	}
	if out.Reasons[0].Code != ReasonKYCIncomplete || out.Reasons[0].RuleID != "T-KYC" {
		t.Errorf("reason = %+v, want KYC_INCOMPLETE from rule T-KYC", out.Reasons[0])
	}

	// Missing KYC input is treated as not-passed (fail closed), not approved.
	req2 := request("A", freshScore(700))
	delete(req2.Inputs, "kyc_status")
	out2, err := e.Evaluate(context.Background(), req2)
	if err != nil {
		t.Fatal(err)
	}
	if out2.Decision != Decline {
		t.Errorf("missing kyc_status decision = %s, want DECLINE", out2.Decision)
	}
}

func TestEvaluate_ModelUnavailable(t *testing.T) {
	e := testEvaluator(t, testPolicy)
	score := 700.0
	cases := []struct {
		name   string
		models []ModelRef
	}{
		{"no model metadata at all", nil},
		{"model marked unavailable (fabricated mock score)", []ModelRef{{
			Name: "credit_score", Version: "deterministic-v1", Available: false, Score: &score,
		}}},
		{"different model only", []ModelRef{{Name: "fraud_ml", Available: true}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := e.Evaluate(context.Background(), request("A", c.models))
			if err != nil {
				t.Fatal(err)
			}
			if out.Decision != Decline {
				t.Fatalf("decision = %s, want DECLINE (on_model_unavailable)", out.Decision)
			}
			if out.Reasons[0].Code != ReasonModelUnavailable {
				t.Errorf("reason = %s, want MODEL_UNAVAILABLE", out.Reasons[0].Code)
			}
		})
	}
}

func TestEvaluate_OnModelUnavailableRefer(t *testing.T) {
	doc := `
policy: refer.policy
version: 1
models:
  credit_score: {required: true}
on_model_unavailable: REFER
rules:
  - id: R
    table: [{ band: A, outcome: APPROVE }]
`
	e := testEvaluator(t, doc)
	req := request("A", nil)
	req.Type = "refer.policy"
	out, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != Refer {
		t.Errorf("decision = %s, want REFER (declared failure outcome)", out.Decision)
	}
}

func TestEvaluate_StaleScoreRefers(t *testing.T) {
	e := testEvaluator(t, testPolicy)
	e.now = func() time.Time { return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC) }
	score := 700.0

	mkModels := func(age time.Duration) []ModelRef {
		at := e.now().Add(-age)
		return []ModelRef{{Name: "credit_score", Available: true, Score: &score, ScoredAt: &at}}
	}

	// Exactly at max_age (30d) is still fresh; a second past it is stale.
	out, err := e.Evaluate(context.Background(), request("A", mkModels(30*24*time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != Approve {
		t.Errorf("exactly-at-max-age decision = %s, want APPROVE", out.Decision)
	}

	out, err = e.Evaluate(context.Background(), request("A", mkModels(30*24*time.Hour+time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != Refer || out.Reasons[0].Code != ReasonScoreStale {
		t.Errorf("stale decision = %s/%+v, want REFER/SCORE_STALE", out.Decision, out.Reasons)
	}

	// A score with no timestamp cannot prove freshness ⇒ stale.
	out, err = e.Evaluate(context.Background(), request("A", []ModelRef{{Name: "credit_score", Available: true, Score: &score}}))
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != Refer || out.Reasons[0].Code != ReasonScoreStale {
		t.Errorf("no-timestamp decision = %s/%+v, want REFER/SCORE_STALE", out.Decision, out.Reasons)
	}
}

func TestEvaluate_KillSwitches(t *testing.T) {
	// Emergency env kill switch.
	t.Setenv(ModelDisableEnv, "other_model, credit_score")
	e := testEvaluator(t, testPolicy)
	out, err := e.Evaluate(context.Background(), request("A", freshScore(700)))
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != Decline || out.Reasons[0].Code != ReasonModelUnavailable {
		t.Fatalf("env-disabled model: decision = %s/%+v, want DECLINE/MODEL_UNAVAILABLE", out.Decision, out.Reasons)
	}
	// The recorded model metadata reflects that the output was not trusted.
	if len(out.Models) != 1 || out.Models[0].Available {
		t.Errorf("recorded models = %+v, want credit_score marked unavailable", out.Models)
	}

	t.Setenv(ModelDisableEnv, "")
	// Policy-declared kill switch (enabled: false ⇒ auditable version bump).
	doc := `
policy: killswitch.policy
version: 1
models:
  credit_score: {required: true, enabled: false}
on_model_unavailable: DECLINE
rules:
  - id: R
    table: [{ band: A, outcome: APPROVE }]
`
	e2 := testEvaluator(t, doc)
	req := request("A", freshScore(700))
	req.Type = "killswitch.policy"
	out2, err := e2.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if out2.Decision != Decline || out2.Reasons[0].Code != ReasonModelUnavailable {
		t.Errorf("policy-disabled model: decision = %s, want DECLINE/MODEL_UNAVAILABLE", out2.Decision)
	}
}

func TestEvaluate_UnknownPolicyErrors(t *testing.T) {
	e := testEvaluator(t, testPolicy)
	req := request("A", freshScore(700))
	req.Type = "no.such.policy"
	if _, err := e.Evaluate(context.Background(), req); err == nil {
		t.Fatal("want error for unregistered policy id, got nil")
	}
}

func TestHashBucket_DeterministicAndUniform(t *testing.T) {
	// Determinism: same subject id, same bucket — across calls (and, because
	// it's sha256 over the bytes, across processes and restarts).
	for _, id := range []string{"", "w-1", "550e8400-e29b-41d4-a716-446655440000"} {
		a, b := HashBucket(id), HashBucket(id)
		if a != b {
			t.Fatalf("HashBucket(%q) not deterministic: %v vs %v", id, a, b)
		}
		if a < 0 || a >= 1 {
			t.Fatalf("HashBucket(%q) = %v, want [0,1)", id, a)
		}
	}
	// Distinct ids spread out (coarse uniformity check).
	n, below := 10000, 0
	for i := 0; i < n; i++ {
		if HashBucket(fmt.Sprintf("subject-%d", i)) < 0.10 {
			below++
		}
	}
	got := float64(below) / float64(n)
	if math.Abs(got-0.10) > 0.02 {
		t.Errorf("10%% traffic slice captured %.3f of subjects, want ~0.10", got)
	}
}

func TestEvaluate_ChallengerShadow(t *testing.T) {
	champion := `
policy: chal.policy
version: 1
rules:
  - id: R
    table: [{ band: C, outcome: APPROVE }]
challenger: {version: 2, traffic: 1.0, mode: shadow}
`
	challenger := `
policy: chal.policy
version: 2
rules:
  - id: R
    table: [{ band: C, outcome: REFER, queue: credit-review, reason: SCORE_BAND_LOW }]
`
	e := testEvaluator(t, champion, challenger)
	req := request("C", nil)
	req.Type = "chal.policy"
	out, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	// The champion outcome is what the caller may act on — unchanged.
	if out.Decision != Approve || out.Variant != VariantChampion {
		t.Fatalf("champion outcome = %s/%s, want APPROVE/champion", out.Decision, out.Variant)
	}
	// The challenger outcome rides along, shadow-only.
	if out.Challenger == nil {
		t.Fatal("challenger outcome missing at traffic=1.0")
	}
	if out.Challenger.Decision != Refer || out.Challenger.Variant != "challenger:2" {
		t.Errorf("challenger outcome = %s/%s, want REFER/challenger:2", out.Challenger.Decision, out.Challenger.Variant)
	}
	if out.Challenger.Policy.Version != 2 || out.Challenger.Policy.Hash == out.Policy.Hash {
		t.Errorf("challenger policy ref not pinned to v2: %+v", out.Challenger.Policy)
	}

	// Traffic 0-ish: bucket must exclude subjects above the slice.
	tiny := `
policy: tiny.policy
version: 1
rules:
  - id: R
    table: [{ band: C, outcome: APPROVE }]
challenger: {version: 2, traffic: 0.000001, mode: shadow}
`
	tinyV2 := `
policy: tiny.policy
version: 2
rules:
  - id: R
    table: [{ band: C, outcome: APPROVE }]
`
	e2 := testEvaluator(t, tiny, tinyV2)
	// Find a subject that hash-buckets above the slice (virtually all do).
	req2 := request("C", nil)
	req2.Type = "tiny.policy"
	req2.SubjectID = "definitely-above-the-slice"
	if HashBucket(req2.SubjectID) < 0.000001 {
		t.Skip("astronomically unlucky subject id")
	}
	out2, err := e2.Evaluate(context.Background(), req2)
	if err != nil {
		t.Fatal(err)
	}
	if out2.Challenger != nil {
		t.Error("subject outside the traffic slice must not evaluate the challenger")
	}
}

// Challenger resolution/evaluation failures must never fail the champion.
func TestEvaluate_ChallengerFailureIsNotFatal(t *testing.T) {
	champion := `
policy: chalmiss.policy
version: 3
rules:
  - id: R
    table: [{ band: C, outcome: APPROVE }]
challenger: {version: 2, traffic: 1.0, mode: shadow}
`
	e := testEvaluator(t, champion) // v2 never registered
	req := request("C", nil)
	req.Type = "chalmiss.policy"
	out, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("champion must survive challenger resolution failure: %v", err)
	}
	if out.Decision != Approve || out.Challenger != nil {
		t.Errorf("outcome = %s challenger=%v, want APPROVE with nil challenger", out.Decision, out.Challenger)
	}
}

// The evaluator gates money decisions and is shared across request goroutines:
// it must be race-free under concurrent use (run with -race).
func TestEvaluate_ConcurrentUse(t *testing.T) {
	e := testEvaluator(t, testPolicy)
	var wg sync.WaitGroup
	bands := []string{"A", "B", "C", "D", "E"}
	wants := map[string]string{"A": Approve, "B": Approve, "C": Refer, "D": Decline, "E": Decline}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			band := bands[i%len(bands)]
			req := request(band, freshScore(700))
			req.SubjectID = fmt.Sprintf("w-%d", i)
			out, err := e.Evaluate(context.Background(), req)
			if err != nil {
				t.Errorf("Evaluate: %v", err)
				return
			}
			if out.Decision != wants[band] {
				t.Errorf("band %s decision = %s, want %s", band, out.Decision, wants[band])
			}
		}(i)
	}
	wg.Wait()
}
