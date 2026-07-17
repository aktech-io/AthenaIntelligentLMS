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

func newRouter(t *testing.T) chi.Router {
	t.Helper()
	secret := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	jwtUtil, err := auth.NewJWTUtil(secret)
	require.NoError(t, err)
	authMw := auth.NewMiddleware(jwtUtil, "test-service-key", zap.NewNop())

	r := chi.NewRouter()
	NewBillPayHandler(nil).Routes(r, authMw.Handler)
	NewSavingsHandler(nil).Routes(r, authMw.Handler)
	return r
}

// Every bill-pay and savings endpoint the app calls is authenticated: 401
// (not 404) without credentials.
func TestProtectedRoutesRequire401(t *testing.T) {
	r := newRouter(t)
	protected := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/billpay/categories"},
		{http.MethodGet, "/api/v1/billpay/billers"},
		{http.MethodPost, "/api/v1/billpay/validate"},
		{http.MethodPost, "/api/v1/billpay/pay"},
		{http.MethodGet, "/api/v1/billpay/history"},
		{http.MethodGet, "/api/v1/billpay/saved"},
		{http.MethodPost, "/api/v1/billpay/saved"},
		{http.MethodDelete, "/api/v1/billpay/saved/123"},
		{http.MethodGet, "/api/v1/savings/goals"},
		{http.MethodPost, "/api/v1/savings/goals"},
		{http.MethodPost, "/api/v1/savings/goals/123/deposit"},
		{http.MethodPost, "/api/v1/savings/goals/123/withdraw"},
		{http.MethodGet, "/api/v1/savings/goals/123/transactions"},
	}
	for _, tc := range protected {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s", tc.method, tc.path)
	}
}
