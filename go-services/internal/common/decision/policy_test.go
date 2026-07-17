package decision

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const basePolicy = `
policy: overdraft.facility
version: 1
market: KE
tenant: "*"
rules:
  - id: OD-BAND
    table:
      - { band: A, limit: 100, outcome: APPROVE }
      - { band: D, outcome: DECLINE, reason: SCORE_BAND_LOW }
`

func TestRegisterYAML_ValidPolicy(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterYAML([]byte(basePolicy), "test"); err != nil {
		t.Fatalf("RegisterYAML: %v", err)
	}
	p, err := r.Resolve("overdraft.facility", "tenant-a", "KE")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Version != 1 || p.ID != "overdraft.facility" {
		t.Errorf("resolved %s v%d, want overdraft.facility v1", p.ID, p.Version)
	}
	if !strings.HasPrefix(p.Ref().Hash, "sha256:") || len(p.Ref().Hash) != len("sha256:")+64 {
		t.Errorf("policy hash %q is not a sha256 content hash", p.Ref().Hash)
	}
	// Fail-closed default when on_model_unavailable is not declared.
	if p.OnModelUnavailable != Decline {
		t.Errorf("OnModelUnavailable default = %q, want DECLINE", p.OnModelUnavailable)
	}
	// Inline detail keys captured.
	if p.Rules[0].Table[0].Detail["limit"] != 100 {
		t.Errorf("band A limit detail = %v, want 100", p.Rules[0].Table[0].Detail["limit"])
	}
}

func TestRegisterYAML_ContentHashPinsBytes(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterYAML([]byte(basePolicy), "a"); err != nil {
		t.Fatal(err)
	}
	p1, _ := r.Resolve("overdraft.facility", "*", "KE")
	hash1 := p1.Ref().Hash

	// Same semantics, different bytes (comment) ⇒ different hash: the hash
	// pins the exact document, not its parsed meaning.
	r2 := NewRegistry()
	if err := r2.RegisterYAML([]byte("# comment\n"+basePolicy), "b"); err != nil {
		t.Fatal(err)
	}
	p2, _ := r2.Resolve("overdraft.facility", "*", "KE")
	if p2.Ref().Hash == hash1 {
		t.Error("different documents produced the same content hash")
	}

	// Identical bytes ⇒ identical hash (deterministic).
	r3 := NewRegistry()
	if err := r3.RegisterYAML([]byte(basePolicy), "c"); err != nil {
		t.Fatal(err)
	}
	p3, _ := r3.Resolve("overdraft.facility", "*", "KE")
	if p3.Ref().Hash != hash1 {
		t.Error("identical documents produced different content hashes")
	}
}

func TestRegisterYAML_ValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{"missing id", "version: 1\nrules: [{id: R, when: \"a == 1\", outcome: APPROVE}]", "'policy' id is required"},
		{"bad version", "policy: p\nversion: 0\nrules: [{id: R, when: \"a == 1\", outcome: APPROVE}]", "version must be >= 1"},
		{"no rules", "policy: p\nversion: 1", "at least one rule"},
		{"rule missing id", "policy: p\nversion: 1\nrules: [{when: \"a == 1\", outcome: APPROVE}]", "missing an id"},
		{"rule with both when and table", "policy: p\nversion: 1\nrules: [{id: R, when: \"a == 1\", outcome: APPROVE, table: [{band: A, outcome: APPROVE}]}]", "exactly one of"},
		{"bad outcome", "policy: p\nversion: 1\nrules: [{id: R, when: \"a == 1\", outcome: MAYBE}]", "invalid outcome"},
		{"bad condition", "policy: p\nversion: 1\nrules: [{id: R, when: \"a ~ 1\", outcome: APPROVE}]", "unsupported operator"},
		{"decline without reason", "policy: p\nversion: 1\nrules: [{id: R, when: \"a == 1\", outcome: DECLINE}]", "requires a reason code"},
		{"refer without reason in table", "policy: p\nversion: 1\nrules: [{id: R, table: [{band: C, outcome: REFER}]}]", "requires a reason code"},
		{"unregistered reason", "policy: p\nversion: 1\nrules: [{id: R, when: \"a == 1\", outcome: DECLINE, reason: MADE_UP}]", "not in the registry"},
		{"table row missing band", "policy: p\nversion: 1\nrules: [{id: R, table: [{outcome: APPROVE}]}]", "missing 'band'"},
		{"on_model_unavailable approve", "policy: p\nversion: 1\non_model_unavailable: APPROVE\nrules: [{id: R, when: \"a == 1\", outcome: APPROVE}]", "on_model_unavailable"},
		{"bad max_age", "policy: p\nversion: 1\nmodels: {m: {required: true, max_age: soon}}\nrules: [{id: R, when: \"a == 1\", outcome: APPROVE}]", "invalid max_age"},
		{"challenger enforce mode", "policy: p\nversion: 2\nchallenger: {version: 1, traffic: 0.5, mode: enforce}\nrules: [{id: R, when: \"a == 1\", outcome: APPROVE}]", "shadow only"},
		{"challenger same version", "policy: p\nversion: 2\nchallenger: {version: 2, traffic: 0.5, mode: shadow}\nrules: [{id: R, when: \"a == 1\", outcome: APPROVE}]", "equals champion"},
		{"challenger bad traffic", "policy: p\nversion: 2\nchallenger: {version: 1, traffic: 1.5, mode: shadow}\nrules: [{id: R, when: \"a == 1\", outcome: APPROVE}]", "traffic"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := NewRegistry().RegisterYAML([]byte(c.yaml), c.name)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

// Resolution order: tenant+market → tenant → market default → platform
// default (design §2.2), and the highest version wins within a cell.
func TestResolve_ResolutionOrder(t *testing.T) {
	mk := func(tenant, market string, version int) string {
		return `
policy: p
version: ` + itoa(version) + `
market: "` + market + `"
tenant: "` + tenant + `"
rules:
  - id: R
    table: [{ band: A, outcome: APPROVE, cell: "` + tenant + `/` + market + `/v` + itoa(version) + `" }]
`
	}
	r := NewRegistry()
	for _, doc := range []string{
		mk("*", "*", 1),
		mk("*", "KE", 2),
		mk("t1", "*", 3),
		mk("t1", "KE", 4),
		mk("t1", "KE", 5), // higher version in the same cell
	} {
		if err := r.RegisterYAML([]byte(doc), "res"); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		tenant, market string
		wantCell       string
	}{
		{"t1", "KE", "t1/KE/v5"}, // exact cell, latest version
		{"t1", "ET", "t1/*/v3"},  // tenant override, other market
		{"t2", "KE", "*/KE/v2"},  // market default
		{"t2", "ET", "*/*/v1"},   // platform default
	}
	for _, c := range cases {
		p, err := r.Resolve("p", c.tenant, c.market)
		if err != nil {
			t.Fatalf("Resolve(%s,%s): %v", c.tenant, c.market, err)
		}
		got := p.Rules[0].Table[0].Detail["cell"]
		if got != c.wantCell {
			t.Errorf("Resolve(%s,%s) = %v, want %s", c.tenant, c.market, got, c.wantCell)
		}
	}

	if _, err := r.Resolve("unknown.policy", "t1", "KE"); err == nil {
		t.Error("Resolve(unknown.policy): want error, got nil")
	}

	// ResolveVersion pins an exact version through the same walk.
	p, err := r.ResolveVersion("p", "t1", "KE", 4)
	if err != nil {
		t.Fatalf("ResolveVersion: %v", err)
	}
	if p.Version != 4 {
		t.Errorf("ResolveVersion = v%d, want v4", p.Version)
	}
	if _, err := r.ResolveVersion("p", "t2", "ET", 9); err == nil {
		t.Error("ResolveVersion(v9): want error, got nil")
	}
}

func TestLoadDir_OverridesBuiltins(t *testing.T) {
	dir := t.TempDir()
	doc := strings.Replace(basePolicy, "version: 1", "version: 7", 1)
	if err := os.WriteFile(filepath.Join(dir, "p.yaml"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := r.RegisterYAML([]byte(basePolicy), "builtin"); err != nil {
		t.Fatal(err)
	}
	if err := r.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	p, err := r.Resolve("overdraft.facility", "any", "KE")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 7 {
		t.Errorf("after LoadDir, resolved v%d, want v7 (dir policy wins by version)", p.Version)
	}
}

// The embedded platform defaults must always parse, validate, and cover the
// overdraft.facility decision — a service with no decision-service reachable
// still evaluates deterministically (design §2.2).
func TestDefaultRegistry_EmbeddedOverdraftPolicy(t *testing.T) {
	p, err := DefaultRegistry().Resolve("overdraft.facility", "any-tenant", "KE")
	if err != nil {
		t.Fatalf("embedded overdraft.facility policy missing: %v", err)
	}
	if p.OnModelUnavailable != Decline {
		t.Errorf("embedded policy on_model_unavailable = %q, want DECLINE (fail-closed, HIGH-6)", p.OnModelUnavailable)
	}
	m, ok := p.Models["credit_score"]
	if !ok || !m.Required {
		t.Fatalf("embedded policy must declare credit_score as a required model, got %+v", p.Models)
	}
	// Band table mirrors the seeded system credit_band_configs (shadow parity).
	want := map[string]int{"A": 100000, "B": 50000, "C": 20000, "D": 5000}
	rows := p.Rules[0].Table
	if len(rows) != len(want) {
		t.Fatalf("embedded band table has %d rows, want %d", len(rows), len(want))
	}
	for _, row := range rows {
		if row.Outcome != Approve {
			t.Errorf("band %s outcome = %s, want APPROVE (v1 mirrors legacy)", row.Band, row.Outcome)
		}
		if row.Detail["limit"] != want[row.Band] {
			t.Errorf("band %s limit = %v, want %d", row.Band, row.Detail["limit"], want[row.Band])
		}
	}
}

func TestParseAge(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "0s", false},
		{"30d", "720h0m0s", false},
		{"1.5d", "36h0m0s", false},
		{"720h", "720h0m0s", false},
		{"90m", "1h30m0s", false},
		{"soon", "", true},
		{"xd", "", true},
	}
	for _, c := range cases {
		d, err := parseAge(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseAge(%q): want error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAge(%q): %v", c.in, err)
			continue
		}
		if d.String() != c.want {
			t.Errorf("parseAge(%q) = %s, want %s", c.in, d, c.want)
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
