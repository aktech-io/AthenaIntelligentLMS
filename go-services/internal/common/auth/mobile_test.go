package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// testSecret is a base64-encoded 32-byte key (matches NewJWTUtil's minimum).
var testSecret = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func newTestJWT(t *testing.T) *JWTUtil {
	t.Helper()
	j, err := NewJWTUtil(testSecret)
	require.NoError(t, err)
	return j
}

func TestMobileTokenRoundTrip(t *testing.T) {
	j := newTestJWT(t)

	tok, err := j.GenerateToken("+254712345678", []string{"MOBILE_USER"},
		"MOB-ABCD1234", "acme", "6f1e8a04-98a8-4d8e-9f3c-2f6a4a1b2c3d", time.Minute)
	require.NoError(t, err)

	claims, err := j.ValidateToken(tok)
	require.NoError(t, err)
	assert.Equal(t, "+254712345678", claims.Subject)
	assert.Equal(t, []string{"MOBILE_USER"}, claims.Roles)
	assert.Equal(t, "MOB-ABCD1234", claims.CustomerID)
	assert.Equal(t, "acme", claims.TenantID)
	assert.Equal(t, "6f1e8a04-98a8-4d8e-9f3c-2f6a4a1b2c3d", claims.UserID)

	// The staff-side parser must read the same token (shared middleware).
	pc, err := j.ParseToken(tok)
	require.NoError(t, err)
	assert.Equal(t, "+254712345678", pc.Username)
	assert.Equal(t, "acme", pc.TenantID)
	assert.Equal(t, "MOB-ABCD1234", pc.CustomerIDStr)
	assert.Equal(t, "6f1e8a04-98a8-4d8e-9f3c-2f6a4a1b2c3d", pc.MobileUserID)
}

func TestMobileRefreshTokenCarriesJTI(t *testing.T) {
	j := newTestJWT(t)

	tok, err := j.GenerateRefreshToken("+254712345678", "jti-123", "acme", time.Hour)
	require.NoError(t, err)

	claims, err := j.ValidateToken(tok)
	require.NoError(t, err)
	assert.Equal(t, "jti-123", claims.ID)
	assert.Equal(t, "acme", claims.TenantID)
}

func TestMobileTokenExpired(t *testing.T) {
	j := newTestJWT(t)

	tok, err := j.GenerateToken("sub", nil, "", "default", "", -time.Minute)
	require.NoError(t, err)

	_, err = j.ValidateToken(tok)
	assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestMobileTokenWrongKey(t *testing.T) {
	j := newTestJWT(t)
	other, err := NewJWTUtil(base64.StdEncoding.EncodeToString([]byte("ffffffffffffffffffffffffffffffff")))
	require.NoError(t, err)

	tok, err := other.GenerateToken("sub", nil, "", "default", "", time.Minute)
	require.NoError(t, err)

	_, err = j.ValidateToken(tok)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

func TestMiddlewarePopulatesMobileContext(t *testing.T) {
	j := newTestJWT(t)
	mw := NewMiddleware(j, "svc-key", zap.NewNop())

	tok, err := j.GenerateToken("+254700000001", []string{"MOBILE_USER"},
		"MOB-XYZ", "acme", "11111111-2222-3333-4444-555555555555", time.Minute)
	require.NoError(t, err)

	var got struct{ tenant, mobileUser, customer, user string }
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		got.tenant = TenantIDOrDefault(ctx)
		got.mobileUser = MobileUserIDFromContext(ctx)
		got.customer = CustomerIDStrFromContext(ctx)
		got.user = UserIDFromContext(ctx)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mobile/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "acme", got.tenant)
	assert.Equal(t, "11111111-2222-3333-4444-555555555555", got.mobileUser)
	assert.Equal(t, "MOB-XYZ", got.customer)
	assert.Equal(t, "+254700000001", got.user)
}

func TestServiceKeyTransportStampsHeaders(t *testing.T) {
	var seen http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	}))
	defer upstream.Close()

	client := &http.Client{Transport: &ServiceKeyTransport{ServiceKey: "sk-1", ServiceName: "bff-gateway"}}
	req, err := http.NewRequestWithContext(WithTenantID(context.Background(), "acme"),
		http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, "sk-1", seen.Get("X-Service-Key"))
	assert.Equal(t, "acme", seen.Get("X-Service-Tenant"))
	assert.Equal(t, "bff-gateway", seen.Get("X-Service-User"))
}

func TestServiceKeyTransportDefaultTenant(t *testing.T) {
	var seen http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	}))
	defer upstream.Close()

	client := &http.Client{Transport: &ServiceKeyTransport{ServiceKey: "sk-1", ServiceName: "bff-shop"}}
	resp, err := client.Get(upstream.URL)
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, "default", seen.Get("X-Service-Tenant"))
}
