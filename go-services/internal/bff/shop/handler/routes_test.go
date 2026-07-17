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
	NewProductHandler(nil).Routes(r)
	NewCartHandler(nil).Routes(r, authMw.Handler)
	NewFavoriteHandler(nil).Routes(r, authMw.Handler)
	NewOrderHandler(nil).Routes(r, authMw.Handler)
	NewBNPLHandler(nil).Routes(r, authMw.Handler)
	return r
}

func TestProtectedRoutesRequire401(t *testing.T) {
	r := newRouter(t)
	protected := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/shop/cart"},
		{http.MethodPost, "/api/v1/shop/cart"},
		{http.MethodPut, "/api/v1/shop/cart/123"},
		{http.MethodDelete, "/api/v1/shop/cart/123"},
		{http.MethodDelete, "/api/v1/shop/cart"},
		{http.MethodGet, "/api/v1/shop/favorites"},
		{http.MethodPost, "/api/v1/shop/favorites/123/toggle"},
		{http.MethodPost, "/api/v1/shop/orders"},
		{http.MethodGet, "/api/v1/shop/orders"},
		{http.MethodGet, "/api/v1/shop/orders/123"},
		{http.MethodPut, "/api/v1/shop/orders/123/status"},
	}
	for _, tc := range protected {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s", tc.method, tc.path)
	}
}

// Product browsing is deliberately public (marketplace is viewable before
// sign-in); BNPL plan listing/calculation are also outside the auth group.
func TestPublicRoutesRegistered(t *testing.T) {
	r := newRouter(t)
	expected := []string{
		"GET /api/v1/shop/products",
		"GET /api/v1/shop/products/categories",
		"GET /api/v1/shop/products/featured",
		"GET /api/v1/shop/products/{id}",
		"GET /api/v1/shop/bnpl/plans",
		"POST /api/v1/shop/bnpl/calculate",
	}
	routes := map[string]bool{}
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if len(route) > 1 && route[len(route)-1] == '/' {
			route = route[:len(route)-1]
		}
		routes[method+" "+route] = true
		return nil
	})
	require.NoError(t, err)
	for _, e := range expected {
		assert.Contains(t, routes, e)
	}
}
