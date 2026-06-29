package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// pass is a terminal handler that records that authorisation succeeded.
func pass(hit *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hit = true
		w.WriteHeader(http.StatusOK)
	})
}

func runRequirePermission(t *testing.T, setup func(r *http.Request) *http.Request, perm string, fallback ...string) (status int, reached bool) {
	t.Helper()
	var hit bool
	h := RequirePermission(perm, fallback...)(pass(&hit))
	req := httptest.NewRequest("POST", "/api/v1/thing", nil)
	if setup != nil {
		req = setup(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, hit
}

// (1) SERVICE role always passes, regardless of permission/claim presence.
func TestRequirePermission_ServiceBypass(t *testing.T) {
	status, reached := runRequirePermission(t, func(r *http.Request) *http.Request {
		return r.WithContext(WithRoles(r.Context(), []string{"SERVICE", "ADMIN"}))
	}, "compliance.decide")
	if status != http.StatusOK || !reached {
		t.Fatalf("SERVICE should pass: status=%d reached=%v", status, reached)
	}
}

// (2a) RBAC-aware token holding the permission passes.
func TestRequirePermission_HasPermission(t *testing.T) {
	status, reached := runRequirePermission(t, func(r *http.Request) *http.Request {
		ctx := WithRoles(r.Context(), []string{"MANAGER", "USER"})
		ctx = WithPermissions(ctx, []string{"compliance.decide", "product.manage"})
		return r.WithContext(ctx)
	}, "compliance.decide", "ADMIN", "MANAGER")
	if status != http.StatusOK || !reached {
		t.Fatalf("token with permission should pass: status=%d", status)
	}
}

// (2b) RBAC-aware token WITHOUT the permission is denied — even if its role
// would have passed the legacy fallback. The claim is authoritative.
func TestRequirePermission_HasClaimButMissingPermission_Denied(t *testing.T) {
	status, reached := runRequirePermission(t, func(r *http.Request) *http.Request {
		ctx := WithRoles(r.Context(), []string{"ADMIN", "USER"}) // role would pass fallback
		ctx = WithPermissions(ctx, []string{"product.manage"})   // but claim lacks the perm
		return r.WithContext(ctx)
	}, "compliance.decide", "ADMIN", "MANAGER")
	if status != http.StatusForbidden || reached {
		t.Fatalf("RBAC token lacking the permission must be denied: status=%d reached=%v", status, reached)
	}
}

// (3a) Legacy token (no permissions claim) falls back to a role check — pass.
func TestRequirePermission_FallbackRolePass(t *testing.T) {
	status, reached := runRequirePermission(t, func(r *http.Request) *http.Request {
		return r.WithContext(WithRoles(r.Context(), []string{"MANAGER", "USER"}))
	}, "compliance.decide", "ADMIN", "MANAGER")
	if status != http.StatusOK || !reached {
		t.Fatalf("legacy token with allowed role should pass via fallback: status=%d", status)
	}
}

// (3b) Legacy token whose role is not in the fallback set is denied.
func TestRequirePermission_FallbackRoleDenied(t *testing.T) {
	status, reached := runRequirePermission(t, func(r *http.Request) *http.Request {
		return r.WithContext(WithRoles(r.Context(), []string{"OFFICER", "USER"}))
	}, "compliance.decide", "ADMIN", "MANAGER")
	if status != http.StatusForbidden || reached {
		t.Fatalf("legacy token without allowed role must be denied: status=%d reached=%v", status, reached)
	}
}

// No roles, no permissions, no fallback match -> denied.
func TestRequirePermission_NothingDenied(t *testing.T) {
	status, reached := runRequirePermission(t, nil, "compliance.decide", "ADMIN", "MANAGER")
	if status != http.StatusForbidden || reached {
		t.Fatalf("anonymous-ish context must be denied: status=%d reached=%v", status, reached)
	}
}
