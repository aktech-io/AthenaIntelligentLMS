package decision

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/athena-lms/go-services/internal/common/market"
)

// ModelDisableEnv is the emergency kill switch (design §4): a comma-separated
// list of model names to treat as unavailable, for incident response. The
// auditable path is a policy version with models.<name>.enabled: false.
const ModelDisableEnv = "DECISION_MODEL_DISABLE"

// Evaluator evaluates decision requests against versioned policies. It is
// stateless apart from the registry reference and safe for concurrent use.
type Evaluator struct {
	reg *Registry
	now func() time.Time // test seam for staleness checks
}

// NewEvaluator returns an evaluator over reg; a nil reg uses the process-wide
// DefaultRegistry (embedded defaults + DECISION_POLICY_DIR).
func NewEvaluator(reg *Registry) *Evaluator {
	if reg == nil {
		reg = DefaultRegistry()
	}
	return &Evaluator{reg: reg, now: time.Now}
}

// Evaluate resolves the effective policy for the request and evaluates it.
//
// Contract (design §2.3): every successful call yields exactly one champion
// Outcome the caller must record as a decision.recorded event through its own
// outbox in the same transaction as the state change. When the policy
// declares a shadow challenger and the subject hash-buckets into its traffic
// slice, Outcome.Challenger carries the additional log-only outcome.
//
// An error means the evaluation itself could not run (no policy registered,
// unevaluatable rule); the caller decides what that means for its path — in
// shadow mode it logs and proceeds, an enforcing caller must fail closed.
func (e *Evaluator) Evaluate(ctx context.Context, req Request) (Outcome, error) {
	start := time.Now()
	mkt := req.Market
	if mkt == "" {
		mkt = market.Current().Code
	}

	pol, err := e.reg.Resolve(req.Type, req.TenantID, mkt)
	if err != nil {
		evalErrors.WithLabelValues(req.Type, "resolve").Inc()
		return Outcome{}, err
	}

	out, err := e.evaluatePolicy(pol, req)
	if err != nil {
		evalErrors.WithLabelValues(req.Type, "evaluate").Inc()
		return Outcome{}, err
	}
	out.Variant = VariantChampion
	out.LatencyMS = float64(time.Since(start).Microseconds()) / 1000.0
	e.observe(req, out)

	// Shadow challenger (v1: log-only, never enforced, never fatal).
	if c := pol.Challenger; c != nil && c.Mode == "shadow" && HashBucket(req.SubjectID) < c.Traffic {
		if cp, cerr := e.reg.ResolveVersion(req.Type, req.TenantID, mkt, c.Version); cerr != nil {
			evalErrors.WithLabelValues(req.Type, "challenger").Inc()
		} else if co, cerr := e.evaluatePolicy(cp, req); cerr != nil {
			evalErrors.WithLabelValues(req.Type, "challenger").Inc()
		} else {
			co.Variant = "challenger:" + strconv.Itoa(c.Version)
			co.LatencyMS = float64(time.Since(start).Microseconds()) / 1000.0
			e.observe(req, co)
			out.Challenger = &co
		}
	}
	return out, nil
}

// evaluatePolicy runs one policy document against the request: declared model
// dependencies first (fail-closed), then rules in order; if nothing decides,
// fail closed with POLICY_NO_MATCH.
func (e *Evaluator) evaluatePolicy(pol *Policy, req Request) (Outcome, error) {
	models := effectiveModels(pol, req.Models)

	// Model dependencies, deterministically ordered by name.
	names := make([]string, 0, len(pol.Models))
	for name := range pol.Models {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		mreq := pol.Models[name]
		ref := findModel(models, name)
		available := ref != nil && ref.Available
		if mreq.Required && !available {
			// Never a fabricated score: the declared failure outcome applies
			// (design §4 — this deletes the mock-score fail-open by construction).
			return Outcome{
				Decision: pol.OnModelUnavailable,
				Detail:   map[string]any{"model": name},
				Reasons:  []Reason{{Code: ReasonModelUnavailable, Detail: name}},
				Policy:   pol.Ref(),
				Models:   models,
			}, nil
		}
		if available {
			maxAge, _ := mreq.MaxAgeDuration() // validated at load
			if maxAge > 0 {
				// A score with no timestamp cannot prove freshness: treat it
				// as stale rather than silently reuse it (design §2.2).
				if ref.ScoredAt == nil || e.now().Sub(*ref.ScoredAt) > maxAge {
					return Outcome{
						Decision: Refer,
						Detail:   map[string]any{"model": name, "maxAge": mreq.MaxAge},
						Reasons:  []Reason{{Code: ReasonScoreStale, Detail: name}},
						Policy:   pol.Ref(),
						Models:   models,
					}, nil
				}
			}
		}
	}

	for _, rule := range pol.Rules {
		if rule.When != "" {
			cond, err := parseCondition(rule.When) // validated at load; cheap
			if err != nil {
				return Outcome{}, fmt.Errorf("policy %s rule %s: %w", pol.ID, rule.ID, err)
			}
			match, err := cond.eval(req.Inputs)
			if err != nil {
				return Outcome{}, fmt.Errorf("policy %s rule %s: %w", pol.ID, rule.ID, err)
			}
			if match {
				out := Outcome{Decision: rule.Outcome, Policy: pol.Ref(), Models: models}
				if rule.Reason != "" {
					out.Reasons = []Reason{{Code: rule.Reason, RuleID: rule.ID}}
				}
				return out, nil
			}
			continue
		}

		// Band table.
		field := rule.Field
		if field == "" {
			field = "band"
		}
		band, _ := req.Inputs[field].(string)
		for _, row := range rule.Table {
			if row.Band != band {
				continue
			}
			detail := map[string]any{"band": row.Band}
			for k, v := range row.Detail {
				detail[k] = v
			}
			out := Outcome{Decision: row.Outcome, Detail: detail, Policy: pol.Ref(), Models: models}
			if row.Reason != "" {
				out.Reasons = []Reason{{Code: row.Reason, RuleID: rule.ID, Detail: "band=" + row.Band}}
			}
			return out, nil
		}
		// No row for this band: fall through to the next rule.
	}

	// Nothing decided: fail closed, never approve by omission.
	return Outcome{
		Decision: Decline,
		Reasons:  []Reason{{Code: ReasonPolicyNoMatch}},
		Policy:   pol.Ref(),
		Models:   models,
	}, nil
}

// effectiveModels copies the caller's model metadata, applying policy and
// emergency kill switches so the recorded availability reflects what the
// evaluation actually trusted.
func effectiveModels(pol *Policy, in []ModelRef) []ModelRef {
	disabled := map[string]bool{}
	for name, m := range pol.Models {
		if m.Enabled != nil && !*m.Enabled {
			disabled[name] = true
		}
	}
	for _, name := range strings.Split(os.Getenv(ModelDisableEnv), ",") {
		if name = strings.TrimSpace(name); name != "" {
			disabled[name] = true
		}
	}
	out := make([]ModelRef, len(in))
	copy(out, in)
	for i := range out {
		if disabled[out[i].Name] {
			out[i].Available = false
		}
	}
	return out
}

func findModel(models []ModelRef, name string) *ModelRef {
	for i := range models {
		if models[i].Name == name {
			return &models[i]
		}
	}
	return nil
}

// HashBucket deterministically maps a subject id to [0, 1) for
// champion/challenger traffic splitting (design §2.3). The same subject
// always lands in the same bucket, across processes and restarts.
func HashBucket(subjectID string) float64 {
	sum := sha256.Sum256([]byte(subjectID))
	return float64(binary.BigEndian.Uint64(sum[:8])) / float64(1<<64)
}

// observe records the Prometheus series for one outcome (design §4).
func (e *Evaluator) observe(req Request, out Outcome) {
	outcomesTotal.WithLabelValues(req.Type, out.Decision, out.Variant).Inc()
	latencyMS.WithLabelValues(req.Type).Observe(out.LatencyMS)
	for _, m := range out.Models {
		if m.Available && m.Score != nil {
			modelScore.WithLabelValues(m.Name, m.Version).Observe(*m.Score)
		}
	}
}
