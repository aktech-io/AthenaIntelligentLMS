// Package ekyc defines the eKYC provider adapter (Nemo gap A2): identity
// verification as a pluggable interface so the vendor (Smile ID / Veriff
// class) is a per-market integration choice, not a code dependency. The
// built-in sandbox provider gives deterministic results for demos and tests;
// real vendor adapters register alongside it and are selected with the
// EKYC_PROVIDER env var (market packs carry the kycRuleSet id that will pick
// per-market defaults once vendor adapters exist).
//
// Fail-closed selection: EKYC_PROVIDER must be set explicitly — the sandbox
// provider auto-approves anything with media refs, so an unset variable is a
// startup error, never a silent sandbox default. The dev escape hatch is
// EKYC_ALLOW_SANDBOX_DEFAULT=true (exact string), which restores the sandbox
// default for local demos; no deploy manifest may set it.
package ekyc

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Request is one identity-verification attempt for an onboarding applicant.
// Document and selfie images live in the media service; only their refs
// travel here.
type Request struct {
	FullName     string
	NationalID   string
	DocumentType string // NATIONAL_ID | PASSPORT (market-pack taxonomy)
	OCRProfile   string // ekyc-ml extraction profile id from the market pack
	Phone        string
	DateOfBirth  string // ISO date, optional in v1
	DocumentRef  string // media ref of the ID document image
	SelfieRef    string // media ref of the liveness selfie
	// SelfieFrameRefs are the Tier-1 challenge frames (docs/nemo/08), when
	// the app captured a live challenge; empty for single-shot selfies.
	SelfieFrameRefs []string
}

// Result is the provider's verdict. The risk-tiering policy interprets it;
// providers only report facts.
type Result struct {
	DocumentVerified bool    // document authentic + fields match
	LivenessPassed   bool    // selfie is a live person
	FaceMatchScore   float64 // document photo vs selfie, 0..1
	SanctionsHit     bool
	PEPHit           bool
	ProviderRef      string // vendor-side id for audit

	// Passive-PAD observability (docs/nemo/08). LivenessMode is "" when no
	// PAD ran, "shadow" (scored, never decides), "shadow-error" (engine
	// unreachable in shadow — tolerated), or "enforce". LivenessScore is
	// P(live) in [0,1], -1 when unavailable. LivenessProvider/LivenessAuditRef
	// identify which PAD engine scored (liveness.Provider seam) and its
	// audit reference; both empty when no PAD ran or the engine failed.
	LivenessScore    float64
	LivenessMode     string
	LivenessProvider string
	LivenessAuditRef string
}

// Provider is one eKYC vendor integration.
type Provider interface {
	Name() string
	Verify(ctx context.Context, req Request) (Result, error)
}

var registry = map[string]Provider{
	"sandbox": Sandbox{},
}

// Register adds a vendor adapter (called from the adapter's init or main
// wiring). Last registration wins for a name.
func Register(p Provider) { registry[strings.ToLower(p.Name())] = p }

// FromEnv resolves the configured provider from EKYC_PROVIDER. An unset
// variable is an error (fail closed — see the package docstring), unless
// EKYC_ALLOW_SANDBOX_DEFAULT=true explicitly opts a dev environment back
// into the auto-approving sandbox default.
func FromEnv() (Provider, error) {
	name := strings.ToLower(os.Getenv("EKYC_PROVIDER"))
	if name == "" {
		if os.Getenv("EKYC_ALLOW_SANDBOX_DEFAULT") != "true" {
			return nil, fmt.Errorf("ekyc: EKYC_PROVIDER not set; set inhouse, sandbox or a registered vendor — refusing to default to the auto-approving sandbox (dev escape hatch: EKYC_ALLOW_SANDBOX_DEFAULT=true)")
		}
		name = "sandbox"
	}
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("ekyc: unknown provider %q", name)
	}
	return p, nil
}

// Sandbox is the deterministic no-vendor provider. Rules (for demos/tests):
//   - national id ending "99"  → sanctions hit
//   - national id ending "88"  → PEP hit
//   - empty DocumentRef        → document not verified
//   - empty SelfieRef          → liveness failed
//   - otherwise                → verified, face match 0.97
type Sandbox struct{}

func (Sandbox) Name() string { return "sandbox" }

func (Sandbox) Verify(_ context.Context, req Request) (Result, error) {
	r := Result{
		DocumentVerified: req.DocumentRef != "",
		LivenessPassed:   req.SelfieRef != "",
		SanctionsHit:     strings.HasSuffix(req.NationalID, "99"),
		PEPHit:           strings.HasSuffix(req.NationalID, "88"),
		ProviderRef:      "sandbox-" + req.NationalID,
	}
	if r.DocumentVerified && r.LivenessPassed {
		r.FaceMatchScore = 0.97
	}
	return r, nil
}
