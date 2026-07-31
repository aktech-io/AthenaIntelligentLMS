package ekyc

import (
	"strings"
	"testing"
)

// Fail-closed provider selection (audit action 8): an unset EKYC_PROVIDER
// must never silently resolve to the auto-approving sandbox.
func TestFromEnvFailsClosedWhenUnset(t *testing.T) {
	t.Setenv("EKYC_PROVIDER", "")
	t.Setenv("EKYC_ALLOW_SANDBOX_DEFAULT", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("unset EKYC_PROVIDER must be an error, got nil")
	} else if !strings.Contains(err.Error(), "EKYC_PROVIDER not set") {
		t.Errorf("error = %v, want EKYC_PROVIDER not set", err)
	}
}

func TestFromEnvSandboxDefaultEscapeHatch(t *testing.T) {
	t.Setenv("EKYC_PROVIDER", "")

	t.Run("exact true restores the sandbox default", func(t *testing.T) {
		t.Setenv("EKYC_ALLOW_SANDBOX_DEFAULT", "true")
		p, err := FromEnv()
		if err != nil {
			t.Fatalf("FromEnv() error = %v", err)
		}
		if p.Name() != "sandbox" {
			t.Errorf("provider = %q, want sandbox", p.Name())
		}
	})

	// Same strictness as LIVENESS_ENFORCE: only the exact string "true".
	for _, v := range []string{"1", "TRUE", "yes"} {
		t.Run("rejects "+v, func(t *testing.T) {
			t.Setenv("EKYC_ALLOW_SANDBOX_DEFAULT", v)
			if _, err := FromEnv(); err == nil {
				t.Errorf("EKYC_ALLOW_SANDBOX_DEFAULT=%q must not enable the sandbox default", v)
			}
		})
	}
}

func TestFromEnvExplicitSelection(t *testing.T) {
	t.Run("explicit sandbox still works", func(t *testing.T) {
		t.Setenv("EKYC_PROVIDER", "sandbox")
		p, err := FromEnv()
		if err != nil {
			t.Fatalf("FromEnv() error = %v", err)
		}
		if p.Name() != "sandbox" {
			t.Errorf("provider = %q, want sandbox", p.Name())
		}
	})
	t.Run("unknown provider errors", func(t *testing.T) {
		t.Setenv("EKYC_PROVIDER", "no-such-vendor")
		if _, err := FromEnv(); err == nil {
			t.Fatal("want unknown-provider error, got nil")
		}
	})
}
