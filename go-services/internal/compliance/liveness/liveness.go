// Package liveness defines the passive-liveness (PAD) provider seam
// (docs/nemo/08 Tier-3, docs/nemo/09 Stage 0): frame scoring as a pluggable
// interface so the PAD engine (in-house MiniFASNet, an iBeta-certified
// bridge SDK, VeriFayda in Ethiopia) is a per-deployment integration choice,
// not a code dependency. The in-house scorer registers from main wiring and
// is selected with the LIVENESS_PROVIDER env var (default inhouse); the eKYC
// "inhouse" provider calls whichever implementation is behind the seam —
// frame fetching and the shadow/enforce decision logic stay with the caller.
package liveness

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Result is one PAD scoring verdict. The caller (the eKYC provider's
// shadow/enforce logic) interprets it; providers only report facts.
type Result struct {
	LiveScore float64 // P(live) in [0,1]
	Label     string  // provider label, e.g. LIVE | SPOOF | UNKNOWN
	Provider  string  // registry name of the provider that scored
	AuditRef  string  // provider-side id for audit
}

// Provider is one PAD engine integration.
type Provider interface {
	Name() string
	Score(ctx context.Context, frames [][]byte) (Result, error)
}

var registry = map[string]Provider{}

// Register adds a PAD provider (called from the adapter's init or main
// wiring). Last registration wins for a name.
func Register(p Provider) { registry[strings.ToLower(p.Name())] = p }

// FromEnv resolves the configured provider (LIVENESS_PROVIDER, default
// inhouse).
func FromEnv() (Provider, error) {
	name := strings.ToLower(os.Getenv("LIVENESS_PROVIDER"))
	if name == "" {
		name = "inhouse"
	}
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("liveness: unknown provider %q", name)
	}
	return p, nil
}
