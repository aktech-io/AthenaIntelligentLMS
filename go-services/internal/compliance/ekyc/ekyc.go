// Package ekyc defines the eKYC provider adapter (Nemo gap A2): identity
// verification as a pluggable interface so the vendor (Smile ID / Veriff
// class) is a per-market integration choice, not a code dependency. The
// built-in sandbox provider gives deterministic results for demos and tests;
// real vendor adapters register alongside it and are selected with the
// EKYC_PROVIDER env var (market packs carry the kycRuleSet id that will pick
// per-market defaults once vendor adapters exist).
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
	FullName    string
	NationalID  string
	Phone       string
	DateOfBirth string // ISO date, optional in v1
	DocumentRef string // media ref of the ID document image
	SelfieRef   string // media ref of the liveness selfie
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

// FromEnv resolves the configured provider (EKYC_PROVIDER, default sandbox).
func FromEnv() (Provider, error) {
	name := strings.ToLower(os.Getenv("EKYC_PROVIDER"))
	if name == "" {
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
