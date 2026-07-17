package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

// TestSubmitOnboardingSuccess — happy path decodes the application and stamps
// the service-auth + tenant headers.
func TestSubmitOnboardingSuccess(t *testing.T) {
	var gotKey, gotTenant, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Service-Key")
		gotTenant = r.Header.Get("X-Service-Tenant")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"abc","status":"AUTO_APPROVED"}`))
	}))
	defer srv.Close()

	c := NewComplianceClient(srv.URL, "sk-test")
	ctx := auth.WithTenantID(context.Background(), "acme")
	resp, err := c.SubmitOnboarding(ctx, map[string]any{"phone": "+254700000001"})

	require.NoError(t, err)
	assert.Equal(t, "AUTO_APPROVED", resp["status"])
	assert.Equal(t, "sk-test", gotKey)
	assert.Equal(t, "acme", gotTenant)
	assert.Equal(t, "/api/v1/onboarding", gotPath)
}

// TestComplianceDownstream5xxMapsTo502 — a compliance-side 500 must surface
// as 502 Bad Gateway, never a silent pass or opaque 500.
func TestComplianceDownstream5xxMapsTo502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewComplianceClient(srv.URL, "sk-test")
	_, err := c.SubmitOnboarding(context.Background(), map[string]any{})

	var be *apperrors.BusinessError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, http.StatusBadGateway, be.StatusCode)
}

// TestComplianceUnreachableMapsTo503 — connection refused must surface as 503.
func TestComplianceUnreachableMapsTo503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // dead endpoint

	c := NewComplianceClient(srv.URL, "sk-test")
	_, err := c.GetOnboarding(context.Background(), "some-id")

	var be *apperrors.BusinessError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, http.StatusServiceUnavailable, be.StatusCode)
}

// TestCompliance404MapsToNotFound — downstream 404 becomes a NotFoundError
// with the downstream message preserved.
func TestCompliance404MapsToNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status":404,"error":"Not Found","message":"Onboarding application not found","path":"/api/v1/onboarding/x"}`))
	}))
	defer srv.Close()

	c := NewComplianceClient(srv.URL, "sk-test")
	_, err := c.GetOnboarding(context.Background(), "x")

	var nf *apperrors.NotFoundError
	require.ErrorAs(t, err, &nf)
	assert.Equal(t, "Onboarding application not found", nf.Message)
}

// TestCompliance4xxPassthrough — downstream 400/422 keep status + message.
func TestCompliance4xxPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"status":422,"error":"Unprocessable Entity","message":"nationalId failed verification","path":"/api/v1/onboarding"}`))
	}))
	defer srv.Close()

	c := NewComplianceClient(srv.URL, "sk-test")
	_, err := c.SubmitOnboarding(context.Background(), map[string]any{})

	var be *apperrors.BusinessError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, http.StatusUnprocessableEntity, be.StatusCode)
	assert.Equal(t, "nationalId failed verification", be.Message)
}

// TestComplianceGarbageBodyMapsTo502 — a 2xx with a non-JSON body is a broken
// downstream, surfaced as 502 rather than a decode-shaped 500.
func TestComplianceGarbageBodyMapsTo502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>proxy error</html>"))
	}))
	defer srv.Close()

	c := NewComplianceClient(srv.URL, "sk-test")
	_, err := c.SubmitOnboarding(context.Background(), map[string]any{})

	var be *apperrors.BusinessError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, http.StatusBadGateway, be.StatusCode)
	assert.True(t, strings.Contains(be.Message, "unreadable"))
}
