package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
)

// ---------------------------------------------------------------------------
// stripPrefix tests
// ---------------------------------------------------------------------------

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/lms/api/v1/accounts/123", "/api/v1/accounts/123"},
		{"/lms/api/auth/login", "/api/auth/login"},
		{"/lms/api/v1/loans/", "/api/v1/loans/"},
		{"/lms", "/"},
		{"/other/path", "/other/path"},
		{"/lms/", "/"},
		{"/lms/api/fraud/alerts", "/api/fraud/alerts"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := stripPrefix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ---------------------------------------------------------------------------
// Circuit Breaker tests
// ---------------------------------------------------------------------------

func TestCircuitBreaker_StartsClosedAndAllows(t *testing.T) {
	cb := NewCircuitBreaker()
	assert.Equal(t, CircuitClosed, cb.State())
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_OpensAfterThresholdFailures(t *testing.T) {
	cb := NewCircuitBreaker()

	// Record failures up to threshold (5)
	for i := 0; i < 4; i++ {
		cb.RecordFailure()
		assert.Equal(t, CircuitClosed, cb.State(), "should still be closed after %d failures", i+1)
	}

	cb.RecordFailure() // 5th failure
	assert.Equal(t, CircuitOpen, cb.State())
	assert.False(t, cb.Allow(), "open circuit should reject requests")
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterWait(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.openStateDuration = 10 * time.Millisecond // speed up for test

	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}
	assert.Equal(t, CircuitOpen, cb.State())

	// Wait for open state to expire
	time.Sleep(20 * time.Millisecond)

	assert.True(t, cb.Allow(), "should allow after open duration expires")
	assert.Equal(t, CircuitHalfOpen, cb.State())
}

func TestCircuitBreaker_ClosesAfterHalfOpenSuccesses(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.openStateDuration = 1 * time.Millisecond

	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}
	time.Sleep(5 * time.Millisecond)
	cb.Allow() // transitions to half-open

	// Record enough successes to close
	for i := 0; i < 3; i++ {
		cb.RecordSuccess()
	}
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_ReOpensOnHalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.openStateDuration = 1 * time.Millisecond

	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}
	time.Sleep(5 * time.Millisecond)
	cb.Allow() // transitions to half-open
	assert.Equal(t, CircuitHalfOpen, cb.State())

	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.State())
}

func TestCircuitBreaker_SuccessResetsClosed(t *testing.T) {
	cb := NewCircuitBreaker()

	// Accumulate some failures
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	cb.RecordSuccess()

	// Now we need 5 more consecutive failures to open
	for i := 0; i < 4; i++ {
		cb.RecordFailure()
	}
	assert.Equal(t, CircuitClosed, cb.State(), "should still be closed, failures were reset by success")
}

// ---------------------------------------------------------------------------
// CORS middleware test
// ---------------------------------------------------------------------------

func TestCORSMiddleware_PreflightRequest(t *testing.T) {
	handler := newCORSMiddleware([]string{"http://localhost:3000"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/lms/api/v1/accounts/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Equal(t, "http://localhost:3000", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rr.Header().Get("Access-Control-Allow-Credentials"))
	assert.Contains(t, rr.Header().Get("Access-Control-Allow-Methods"), "GET")
	assert.Contains(t, rr.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, rr.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

// HIGH-1: a request with no Origin (non-browser client) gets NO ACAO header,
// and an Origin not on the allowlist is never reflected.
func TestCORSMiddleware_NoOriginNoHeader(t *testing.T) {
	handler := newCORSMiddleware([]string{"http://localhost:3000"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))

	// Disallowed origin is not reflected.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Origin", "https://evil.example.com")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	assert.Empty(t, rr2.Header().Get("Access-Control-Allow-Origin"))
}

// ---------------------------------------------------------------------------
// Health endpoint test
// ---------------------------------------------------------------------------

func TestHealthEndpoint(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	gw := NewGateway(logger)

	req := httptest.NewRequest(http.MethodGet, "/actuator/health", nil)
	rr := httptest.NewRecorder()

	handler := healthHandler(gw)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var body map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, "UP", body["status"])
	components, ok := body["components"].(map[string]any)
	require.True(t, ok)

	// Verify all routes are present
	assert.Contains(t, components, "account-service")
	assert.Contains(t, components, "product-service")
	assert.Contains(t, components, "loan-origination-service")
	assert.Contains(t, components, "loan-management-service")
	assert.Contains(t, components, "payment-service")
	assert.Contains(t, components, "accounting-service")
	assert.Contains(t, components, "float-service")
	assert.Contains(t, components, "collections-service")
	assert.Contains(t, components, "compliance-service")
	assert.Contains(t, components, "reporting-service")
	assert.Contains(t, components, "ai-scoring-service")
	assert.Contains(t, components, "overdraft-service")
	assert.Contains(t, components, "media-service")
	assert.Contains(t, components, "fraud-detection-service")

	// Each component should show UP status
	for _, v := range components {
		comp := v.(map[string]any)
		assert.Equal(t, "UP", comp["status"])
	}
}

func TestHealthEndpoint_ShowsDownWhenCircuitOpen(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	gw := NewGateway(logger)

	// Open the account-service circuit breaker
	cb := gw.circuitBreakers["account-service"]
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	req := httptest.NewRequest(http.MethodGet, "/actuator/health", nil)
	rr := httptest.NewRecorder()

	healthHandler(gw).ServeHTTP(rr, req)

	var body map[string]any
	json.Unmarshal(rr.Body.Bytes(), &body)

	components := body["components"].(map[string]any)
	acctComp := components["account-service"].(map[string]any)
	assert.Equal(t, "DOWN", acctComp["status"])
}

// ---------------------------------------------------------------------------
// Proxy routing integration test
// ---------------------------------------------------------------------------

func TestProxyRouting_ForwardsToUpstream(t *testing.T) {
	// Create a fake upstream that records what it receives
	var receivedPath string
	var receivedHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedHost = r.Host
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"from upstream"}`))
	}))
	defer upstream.Close()

	logger, _ := zap.NewDevelopment()

	// Create gateway with single test route pointing at our upstream
	gw := &Gateway{
		logger: logger,
		routes: []RouteConfig{
			{
				ID:         "test-service",
				PathPrefix: "/lms/api/v1/test/",
				TargetURL:  upstream.URL,
			},
		},
		circuitBreakers: map[string]*CircuitBreaker{
			"test-service": NewCircuitBreaker(),
		},
	}

	r := chi.NewRouter()
	r.Use(newCORSMiddleware([]string{"http://localhost:3000"}))

	// For this test, use a no-op auth middleware
	noopAuth := &auth.Middleware{}
	// Register with public route to skip auth
	gw.routes[0].Public = true
	gw.RegisterRoutes(r, noopAuth, nil)

	req := httptest.NewRequest(http.MethodGet, "/lms/api/v1/test/items/42", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	// StripPrefix should have removed /lms
	assert.Equal(t, "/api/v1/test/items/42", receivedPath)
	_ = receivedHost

	body, _ := io.ReadAll(rr.Body)
	assert.Contains(t, string(body), "from upstream")
}

func TestProxyRouting_CircuitBreakerRejects(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cb := NewCircuitBreaker()
	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	gw := &Gateway{
		logger: logger,
		routes: []RouteConfig{
			{
				ID:         "test-service",
				PathPrefix: "/lms/api/v1/test/",
				TargetURL:  "http://localhost:1", // won't be reached
				Public:     true,
			},
		},
		circuitBreakers: map[string]*CircuitBreaker{
			"test-service": cb,
		},
	}

	r := chi.NewRouter()
	gw.RegisterRoutes(r, &auth.Middleware{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/lms/api/v1/test/items", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)

	var body map[string]any
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "Service Unavailable", body["error"])
	assert.Contains(t, body["message"], "circuit breaker is open")
}

// ---------------------------------------------------------------------------
// defaultRoutes coverage
// ---------------------------------------------------------------------------

func TestDefaultRoutes_AllRoutesHaveRequiredFields(t *testing.T) {
	routes := defaultRoutes()
	assert.GreaterOrEqual(t, len(routes), 16, "should have at least 16 routes")

	ids := make(map[string]bool)
	for _, r := range routes {
		assert.NotEmpty(t, r.ID, "route must have an ID")
		assert.NotEmpty(t, r.PathPrefix, "route %s must have a PathPrefix", r.ID)
		assert.NotEmpty(t, r.TargetURL, "route %s must have a TargetURL", r.ID)
		assert.NotEmpty(t, r.EnvOverride, "route %s must have an EnvOverride", r.ID)
		assert.True(t, r.PathPrefix[0] == '/', "route %s PathPrefix must start with /", r.ID)
		assert.True(t, r.PathPrefix[len(r.PathPrefix)-1] == '/', "route %s PathPrefix must end with /", r.ID)

		assert.False(t, ids[r.ID], "duplicate route ID: %s", r.ID)
		ids[r.ID] = true
	}
}

func TestDefaultRoutes_AuthRouteIsPublic(t *testing.T) {
	routes := defaultRoutes()
	for _, r := range routes {
		if r.ID == "account-service-auth" {
			assert.True(t, r.Public, "auth route must be public")
			return
		}
	}
	t.Fatal("account-service-auth route not found")
}

// ---------------------------------------------------------------------------
// Route prefix matching
// ---------------------------------------------------------------------------

func TestDefaultRoutes_LoanApplicationsBeforeLoans(t *testing.T) {
	routes := defaultRoutes()
	origIdx := -1
	mgmtIdx := -1
	for i, r := range routes {
		if r.ID == "loan-origination-service" {
			origIdx = i
		}
		if r.ID == "loan-management-service" {
			mgmtIdx = i
		}
	}
	require.NotEqual(t, -1, origIdx, "loan-origination-service must exist")
	require.NotEqual(t, -1, mgmtIdx, "loan-management-service must exist")
	assert.Less(t, origIdx, mgmtIdx,
		"loan-origination-service (loan-applications) must be registered before loan-management-service (loans) for correct prefix matching")
}

// ---------------------------------------------------------------------------
// Env override for strangler fig
// ---------------------------------------------------------------------------

func TestNewGateway_EnvOverride(t *testing.T) {
	t.Setenv("ROUTE_ACCOUNT_SERVICE_URL", "http://new-host:9999")

	logger, _ := zap.NewDevelopment()
	gw := NewGateway(logger)

	for _, r := range gw.routes {
		if r.ID == "account-service" {
			assert.Equal(t, "http://new-host:9999", r.TargetURL)
			return
		}
	}
	t.Fatal("account-service route not found")
}
