package decision

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// A policy is data, not code (design §2.2) — the same move as market packs.
// Built-in defaults are embedded from policies/*.yaml so a cold service with
// no decision-service reachable still evaluates deterministically. Additional
// or overriding policies load at startup from DECISION_POLICY_DIR. ETag
// distribution from decision-service is increment 4; v1 is embedded-defaults
// (+ directory) only.

//go:embed policies/*.yaml
var builtinFS embed.FS

// ModelRequirement declares a model dependency of a policy.
type ModelRequirement struct {
	Source   string `yaml:"source"`            // e.g. "ai-scoring-service"
	Required bool   `yaml:"required"`          // unavailable ⇒ on_model_unavailable
	MaxAge   string `yaml:"max_age,omitempty"` // e.g. "30d", "720h"; stale ⇒ REFER + SCORE_STALE
	Enabled  *bool  `yaml:"enabled,omitempty"` // kill switch: false ⇒ treated as unavailable
}

// MaxAgeDuration parses MaxAge, supporting a "d" (day) suffix on top of Go
// duration syntax. Zero means "no staleness limit".
func (m ModelRequirement) MaxAgeDuration() (time.Duration, error) {
	return parseAge(m.MaxAge)
}

func parseAge(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	if strings.HasSuffix(s, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(s, "d"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid max_age %q", s)
		}
		return time.Duration(days * 24 * float64(time.Hour)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid max_age %q: %w", s, err)
	}
	return d, nil
}

// BandRow is one row of a band table. Non-structural keys (limit, rate, fee,
// queue, ...) are captured inline and become Outcome.Detail verbatim.
type BandRow struct {
	Band    string         `yaml:"band"`
	Outcome string         `yaml:"outcome"`
	Reason  string         `yaml:"reason,omitempty"`
	Detail  map[string]any `yaml:",inline"`
}

// Rule is one policy rule, evaluated in order. Exactly one of When or Table
// is set. The v1 rule language is deliberately small (design §2.2):
// comparisons and set membership over a flat Inputs map, plus band tables —
// not a general expression engine.
type Rule struct {
	ID      string    `yaml:"id"`
	When    string    `yaml:"when,omitempty"`    // "<field> <op> <literal>"
	Outcome string    `yaml:"outcome,omitempty"` // for When rules
	Reason  string    `yaml:"reason,omitempty"`
	Field   string    `yaml:"field,omitempty"` // Table match input; default "band"
	Table   []BandRow `yaml:"table,omitempty"`
}

// Challenger declares a shadow challenger version (v1: log-only, never
// enforced).
type Challenger struct {
	Version int     `yaml:"version"`
	Traffic float64 `yaml:"traffic"` // deterministic hash(subject_id) bucketing
	Mode    string  `yaml:"mode"`    // v1: must be "shadow"
}

// Policy is one versioned policy document.
type Policy struct {
	ID                 string                      `yaml:"policy"`
	Version            int                         `yaml:"version"`
	Market             string                      `yaml:"market"` // ISO code or "*"
	Tenant             string                      `yaml:"tenant"` // tenant id or "*"
	Models             map[string]ModelRequirement `yaml:"models,omitempty"`
	OnModelUnavailable string                      `yaml:"on_model_unavailable,omitempty"`
	Rules              []Rule                      `yaml:"rules"`
	Challenger         *Challenger                 `yaml:"challenger,omitempty"`

	hash string // content hash of the source document
}

// Ref returns the PolicyRef pinning this exact document.
func (p *Policy) Ref() PolicyRef { return PolicyRef{ID: p.ID, Version: p.Version, Hash: p.hash} }

var validOutcomes = map[string]bool{
	Approve: true, Decline: true, Refer: true, Flag: true, NoAction: true,
}

// failureOutcomes are the outcomes a declared failure mode may take. APPROVE
// is deliberately excluded: a model outage must never approve credit.
var failureOutcomes = map[string]bool{Decline: true, Refer: true, Flag: true, NoAction: true}

// Validate reports the first structural problem with the policy, if any.
// Reason codes are checked against the append-only registry so a policy can
// never emit an unregistered (hence untranslatable, unauditable) code.
func (p *Policy) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("decision policy: 'policy' id is required")
	}
	if p.Version < 1 {
		return fmt.Errorf("decision policy %s: version must be >= 1, got %d", p.ID, p.Version)
	}
	if p.OnModelUnavailable != "" && !failureOutcomes[p.OnModelUnavailable] {
		return fmt.Errorf("decision policy %s: on_model_unavailable %q must be one of DECLINE|REFER|FLAG|NO_ACTION", p.ID, p.OnModelUnavailable)
	}
	for name, m := range p.Models {
		if _, err := m.MaxAgeDuration(); err != nil {
			return fmt.Errorf("decision policy %s: model %s: %w", p.ID, name, err)
		}
	}
	if len(p.Rules) == 0 {
		return fmt.Errorf("decision policy %s: at least one rule is required", p.ID)
	}
	for i, r := range p.Rules {
		if r.ID == "" {
			return fmt.Errorf("decision policy %s: rule[%d] is missing an id", p.ID, i)
		}
		hasWhen, hasTable := r.When != "", len(r.Table) > 0
		if hasWhen == hasTable {
			return fmt.Errorf("decision policy %s: rule %s must have exactly one of 'when' or 'table'", p.ID, r.ID)
		}
		if hasWhen {
			if _, err := parseCondition(r.When); err != nil {
				return fmt.Errorf("decision policy %s: rule %s: %w", p.ID, r.ID, err)
			}
			if !validOutcomes[r.Outcome] {
				return fmt.Errorf("decision policy %s: rule %s: invalid outcome %q", p.ID, r.ID, r.Outcome)
			}
			if err := validateReason(r.Outcome, r.Reason); err != nil {
				return fmt.Errorf("decision policy %s: rule %s: %w", p.ID, r.ID, err)
			}
		}
		for _, row := range r.Table {
			if row.Band == "" {
				return fmt.Errorf("decision policy %s: rule %s: table row missing 'band'", p.ID, r.ID)
			}
			if !validOutcomes[row.Outcome] {
				return fmt.Errorf("decision policy %s: rule %s: band %s: invalid outcome %q", p.ID, r.ID, row.Band, row.Outcome)
			}
			if err := validateReason(row.Outcome, row.Reason); err != nil {
				return fmt.Errorf("decision policy %s: rule %s: band %s: %w", p.ID, r.ID, row.Band, err)
			}
		}
	}
	if c := p.Challenger; c != nil {
		if c.Mode != "shadow" {
			return fmt.Errorf("decision policy %s: challenger mode %q not supported in v1 (shadow only)", p.ID, c.Mode)
		}
		if c.Version < 1 {
			return fmt.Errorf("decision policy %s: challenger version must be >= 1", p.ID)
		}
		if c.Version == p.Version {
			return fmt.Errorf("decision policy %s: challenger version equals champion version %d", p.ID, p.Version)
		}
		if c.Traffic <= 0 || c.Traffic > 1 {
			return fmt.Errorf("decision policy %s: challenger traffic %v must be in (0,1]", p.ID, c.Traffic)
		}
	}
	return nil
}

// validateReason enforces that DECLINE/REFER always carry a registered
// adverse-action code (design §3), and that any provided code is registered.
func validateReason(outcome, reason string) error {
	if reason == "" {
		if outcome == Decline || outcome == Refer {
			return fmt.Errorf("outcome %s requires a reason code", outcome)
		}
		return nil
	}
	if !KnownReason(reason) {
		return fmt.Errorf("reason code %q is not in the registry", reason)
	}
	return nil
}

// normalize fills structural defaults after parsing.
func (p *Policy) normalize() {
	if p.Tenant == "" {
		p.Tenant = "*"
	}
	if p.Market == "" {
		p.Market = "*"
	}
	if p.OnModelUnavailable == "" {
		// Fail closed unless the policy explicitly declares otherwise.
		p.OnModelUnavailable = Decline
	}
}

// ─── Registry ────────────────────────────────────────────────────────────────

type policyKey struct {
	id     string
	tenant string
	market string
}

// Registry holds parsed policies keyed by (id, tenant, market); each cell
// keeps every version so challengers can pin an older/newer version.
type Registry struct {
	mu       sync.RWMutex
	policies map[policyKey]map[int]*Policy // key → version → policy
}

// NewRegistry returns an empty registry (tests, tooling).
func NewRegistry() *Registry {
	return &Registry{policies: map[policyKey]map[int]*Policy{}}
}

var (
	defaultRegistry     *Registry
	defaultRegistryOnce sync.Once
)

// DefaultRegistry returns the process-wide registry: embedded defaults plus
// any packs in DECISION_POLICY_DIR (loaded once, mirroring common/market). A
// broken embedded policy is a build defect and panics at first use — a
// service must not run with silently missing decision policies.
func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		r := NewRegistry()
		if err := r.loadBuiltins(); err != nil {
			panic(err)
		}
		if dir := os.Getenv("DECISION_POLICY_DIR"); dir != "" {
			if err := r.LoadDir(dir); err != nil {
				panic(err)
			}
		}
		defaultRegistry = r
	})
	return defaultRegistry
}

func (r *Registry) loadBuiltins() error {
	entries, err := builtinFS.ReadDir("policies")
	if err != nil {
		return fmt.Errorf("decision: read embedded policies: %w", err)
	}
	for _, e := range entries {
		raw, err := builtinFS.ReadFile("policies/" + e.Name())
		if err != nil {
			return fmt.Errorf("decision: read %s: %w", e.Name(), err)
		}
		if err := r.RegisterYAML(raw, e.Name()); err != nil {
			return err
		}
	}
	return nil
}

// LoadDir registers every *.yaml policy in dir, overriding same-coordinate
// same-version built-ins. Exported for tests and tooling.
func (r *Registry) LoadDir(dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return err
	}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("decision: read %s: %w", path, err)
		}
		if err := r.RegisterYAML(raw, path); err != nil {
			return err
		}
	}
	return nil
}

// RegisterYAML parses, validates, hashes and registers one policy document.
// The content hash pins the exact bytes that decided (decision_log.policy_hash).
func (r *Registry) RegisterYAML(raw []byte, source string) error {
	var p Policy
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("decision: parse %s: %w", source, err)
	}
	p.normalize()
	if err := p.Validate(); err != nil {
		return fmt.Errorf("decision: %s: %w", source, err)
	}
	sum := sha256.Sum256(raw)
	p.hash = "sha256:" + hex.EncodeToString(sum[:])

	key := policyKey{id: p.ID, tenant: p.Tenant, market: p.Market}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.policies[key] == nil {
		r.policies[key] = map[int]*Policy{}
	}
	r.policies[key][p.Version] = &p
	return nil
}

// Resolve returns the effective champion policy for (id, tenant, market): the
// most specific matching cell, walking tenant+market → tenant → market
// default → platform default (design §2.2). Within a cell the champion is the
// highest version that is not referenced as another version's shadow
// challenger — a registered challenger document (e.g. v4 declared by v3's
// challenger stanza) is evaluated in shadow only and must never resolve as
// the enforced policy until promoted (its referencing stanza removed).
func (r *Registry) Resolve(id, tenant, market string) (*Policy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, key := range resolutionOrder(id, tenant, market) {
		if versions := r.policies[key]; len(versions) > 0 {
			return championVersion(versions), nil
		}
	}
	return nil, fmt.Errorf("decision: no policy registered for %s (tenant=%s market=%s)", id, tenant, market)
}

// ResolveVersion returns a specific version at the same resolution
// coordinates (used to locate a challenger's document).
func (r *Registry) ResolveVersion(id, tenant, market string, version int) (*Policy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, key := range resolutionOrder(id, tenant, market) {
		if p, ok := r.policies[key][version]; ok {
			return p, nil
		}
	}
	return nil, fmt.Errorf("decision: no policy %s version %d (tenant=%s market=%s)", id, version, tenant, market)
}

func resolutionOrder(id, tenant, market string) []policyKey {
	return []policyKey{
		{id: id, tenant: tenant, market: market},
		{id: id, tenant: tenant, market: "*"},
		{id: id, tenant: "*", market: market},
		{id: id, tenant: "*", market: "*"},
	}
}

func championVersion(versions map[int]*Policy) *Policy {
	shadowOnly := map[int]bool{}
	for _, p := range versions {
		if p.Challenger != nil {
			shadowOnly[p.Challenger.Version] = true
		}
	}
	var best *Policy
	for _, p := range versions {
		if shadowOnly[p.Version] {
			continue
		}
		if best == nil || p.Version > best.Version {
			best = p
		}
	}
	if best == nil {
		// Degenerate: every version is referenced as a challenger. Fail safe
		// to the highest version rather than resolving nothing.
		for _, p := range versions {
			if best == nil || p.Version > best.Version {
				best = p
			}
		}
	}
	return best
}
