package handler

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
)

// newRouter mounts every gateway handler exactly as cmd/bff-gateway does.
// Services are nil — these tests exercise routing and the auth contract only,
// which the middleware settles before any handler body runs.
func newRouter(t *testing.T) chi.Router {
	t.Helper()
	secret := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	jwtUtil, err := auth.NewJWTUtil(secret)
	require.NoError(t, err)
	authMw := auth.NewMiddleware(jwtUtil, "test-service-key", zap.NewNop())

	r := chi.NewRouter()
	NewAuthHandler(nil).Routes(r, authMw.Handler)
	NewProfileHandler(nil).Routes(r, authMw.Handler)
	NewDashboardHandler(nil).Routes(r, authMw.Handler)
	NewContactHandler(nil).Routes(r, authMw.Handler)
	NewTransferHandler(nil).Routes(r, authMw.Handler)
	NewTopUpHandler(nil).Routes(r, authMw.Handler)
	NewLoanHandler(nil).Routes(r, authMw.Handler)
	NewOverdraftHandler(nil).Routes(r, authMw.Handler)
	return r
}

// TestProtectedRoutesRequire401 locks the app-facing auth contract: every
// authenticated endpoint the Flutter app calls must answer 401 (not 404) when
// no credentials are presented.
func TestProtectedRoutesRequire401(t *testing.T) {
	r := newRouter(t)
	protected := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/mobile/auth/pin/setup"},
		{http.MethodPost, "/api/v1/mobile/auth/pin/verify"},
		{http.MethodPost, "/api/v1/mobile/auth/device/register"},
		{http.MethodGet, "/api/v1/mobile/dashboard"},
		{http.MethodGet, "/api/v1/mobile/profile"},
		{http.MethodPut, "/api/v1/mobile/profile"},
		{http.MethodGet, "/api/v1/mobile/profile/preferences"},
		{http.MethodGet, "/api/v1/mobile/profile/employment"},
		{http.MethodGet, "/api/v1/mobile/contacts/recent"},
		{http.MethodGet, "/api/v1/mobile/contacts/search"},
		{http.MethodPost, "/api/v1/mobile/transfers/send"},
		{http.MethodPost, "/api/v1/mobile/topup"},
		{http.MethodGet, "/api/v1/mobile/loans/products"},
		{http.MethodPost, "/api/v1/mobile/loans/apply"},
		{http.MethodGet, "/api/v1/mobile/loans/active"},
		{http.MethodPost, "/api/v1/mobile/loans/repay"},
		{http.MethodGet, "/api/v1/mobile/overdraft"},
		{http.MethodPost, "/api/v1/mobile/overdraft/setup"},
		{http.MethodGet, "/api/v1/mobile/overdraft/charges"},
	}
	for _, tc := range protected {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s", tc.method, tc.path)
	}
}

// TestPublicAuthRoutesRegistered locks the unauthenticated onboarding paths.
// (Invoking them would need a real service; registration is asserted via chi.)
func TestPublicAuthRoutesRegistered(t *testing.T) {
	r := newRouter(t)
	public := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/mobile/auth/otp/send"},
		{http.MethodPost, "/api/v1/mobile/auth/otp/verify"},
		{http.MethodPost, "/api/v1/mobile/auth/token/refresh"},
	}
	routes := walkRoutes(t, r)
	for _, tc := range public {
		assert.Contains(t, routes, tc.method+" "+tc.path)
	}
}

// TestInvalidTokenRejected — a garbage bearer token must yield 401.
func TestInvalidTokenRejected(t *testing.T) {
	r := newRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/dashboard", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func walkRoutes(t *testing.T, r chi.Router) map[string]bool {
	t.Helper()
	routes := map[string]bool{}
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes[method+" "+trimTrailingSlash(route)] = true
		return nil
	})
	require.NoError(t, err)
	return routes
}

func trimTrailingSlash(s string) string {
	if len(s) > 1 && s[len(s)-1] == '/' {
		return s[:len(s)-1]
	}
	return s
}
