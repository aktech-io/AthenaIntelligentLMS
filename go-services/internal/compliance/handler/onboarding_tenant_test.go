package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/athena-lms/go-services/internal/common/auth"
)

// staffTenant: privileged callers may retarget or widen the tenant scope for
// the officer queue; everyone else stays pinned to their own tenant.
func TestStaffTenant(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		tenant string
		roles  []string
		want   string
	}{
		{"no param keeps own tenant", "/api/v1/onboarding", "admin", []string{"ADMIN"}, "admin"},
		{"admin override to explicit tenant", "/api/v1/onboarding?tenantId=default", "admin", []string{"ADMIN"}, "default"},
		{"manager wildcard widens to all", "/api/v1/onboarding?tenantId=*", "admin", []string{"MANAGER"}, ""},
		{"service key override honored", "/api/v1/onboarding?tenantId=default", "other", []string{"SERVICE", "ADMIN"}, "default"},
		{"officer cannot override", "/api/v1/onboarding?tenantId=default", "admin", []string{"OFFICER"}, "admin"},
		{"officer wildcard ignored", "/api/v1/onboarding?tenantId=*", "admin", []string{"OFFICER"}, "admin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", tc.url, nil)
			ctx := auth.WithTenantID(context.Background(), tc.tenant)
			ctx = auth.WithRoles(ctx, tc.roles)
			r = r.WithContext(ctx)
			if got := staffTenant(r); got != tc.want {
				t.Errorf("staffTenant = %q, want %q", got, tc.want)
			}
		})
	}
}
