// Package decision implements the Nemo decision spine (E1): an in-process,
// broker-independent policy evaluator that every automated (and recorded
// human) decision routes through, per docs/nemo/05-decision-engine-design.md.
//
// Evaluation is a library linked into each service; recording is asynchronous
// and lossless: every evaluation produces exactly one decision.recorded event
// written through the caller's transactional outbox in the SAME database
// transaction as the state change the decision caused, and projected into the
// append-only decision_log by decision-service.
//
// v1 scope (design §6): types, band-table + comparison evaluator, reason-code
// registry, policy loader with embedded YAML defaults, Prometheus metrics.
// Champion/challenger evaluation is shadow-only and never enforced.
package decision

import (
	"time"
)

// Decision outcomes. The set is closed (design §2.4): anything a policy rule
// yields must be one of these.
const (
	Approve  = "APPROVE"
	Decline  = "DECLINE"
	Refer    = "REFER"
	Flag     = "FLAG"
	NoAction = "NO_ACTION"
)

// Actor types for who made (or recorded) the decision.
const (
	ActorSystem = "SYSTEM"
	ActorHuman  = "HUMAN"
)

// Variant names. VariantChampion is the enforced (or shadow-enforced)
// evaluation; challenger outcomes carry "challenger:<version>".
const VariantChampion = "champion"

// Actor identifies who made the decision: the SYSTEM (service name as ID) or
// a HUMAN (user id), so recorded manual decisions share the same log.
type Actor struct {
	Type string `json:"type"` // SYSTEM | HUMAN
	ID   string `json:"id"`   // service name, or user id when HUMAN
}

// SystemActor is a convenience constructor for machine decisions.
func SystemActor(serviceName string) Actor { return Actor{Type: ActorSystem, ID: serviceName} }

// Request is one decision to evaluate. Inputs is the full feature snapshot at
// decision time and is persisted verbatim in the decision log — it must
// already contain everything the policy rules reference (flat map, design
// §2.2: the v1 rule language addresses top-level keys only).
type Request struct {
	Type          string // policy id, e.g. "overdraft.facility"
	TenantID      string
	Market        string // ISO market code; "" resolves via the market-pack default
	SubjectType   string // wallet | application | transaction | alert ...
	SubjectID     string
	CustomerID    string // optional; some subjects aren't customers
	Actor         Actor
	CorrelationID string         // joins the decision to the domain event stream
	Inputs        map[string]any // full feature snapshot, persisted verbatim
	Models        []ModelRef     // metadata of model calls the caller made (§4)
}

// Reason is one machine-coded reason for an outcome. Codes come from the
// append-only registry in reasons.go; order matters — Reasons[0] is the
// principal reason (adverse-action regimes require this, design §3).
type Reason struct {
	Code   string `json:"code"`
	RuleID string `json:"ruleId,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// PolicyRef pins the exact policy that decided: id, monotonically increasing
// version, and the content hash of the policy document.
type PolicyRef struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Hash    string `json:"hash"`
}

// ModelRef is the governance metadata stamped on every model touch (§4). It
// lands verbatim in decision_log.models. Available=false is the structural
// record that a model was down or its output untrusted (e.g. a fabricated
// mock score) — the evaluator then applies the policy's declared
// on_model_unavailable outcome instead of trusting the score.
type ModelRef struct {
	Name        string     `json:"name"`
	Version     string     `json:"version,omitempty"`
	RegistryRef string     `json:"registryRef,omitempty"`
	Role        string     `json:"role,omitempty"`
	Score       *float64   `json:"score,omitempty"`
	LatencyMS   float64    `json:"latencyMs,omitempty"`
	Available   bool       `json:"available"`
	ScoredAt    *time.Time `json:"scoredAt,omitempty"`
}

// Outcome is the result of one policy evaluation.
type Outcome struct {
	Decision  string         // APPROVE | DECLINE | REFER | FLAG | NO_ACTION
	Detail    map[string]any // limit, rate, fee, band, queue ...
	Reasons   []Reason       // ordered, machine-coded
	Policy    PolicyRef      // exact policy that decided
	Models    []ModelRef     // model metadata (from Request, availability applied)
	Variant   string         // champion | challenger:<version>
	LatencyMS float64        // evaluate latency

	// Challenger is the shadow challenger outcome when the policy declares one
	// and the subject hash-bucketed into its traffic slice. v1: log-only,
	// NEVER enforced (design §2.3); callers record it as an additional
	// decision.recorded event and must not act on it.
	Challenger *Outcome `json:"-"`
}
