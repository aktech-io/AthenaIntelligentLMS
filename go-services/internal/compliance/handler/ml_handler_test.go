package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/compliance/service"
)

// newMLRouter mounts the ML routes over a service pointed at the given
// upstreams (empty engine URL → deployed-model answers without any HTTP
// call, which keeps the RBAC tests hermetic).
func newMLRouter(mlflowURL, engineURL string) *chi.Mux {
	r := chi.NewRouter()
	NewML(service.NewML(mlflowURL, engineURL, zap.NewNop()), zap.NewNop()).RegisterRoutes(r)
	return r
}

func mlGet(t *testing.T, router http.Handler, path string, perms []string, roles []string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	ctx := req.Context()
	if roles != nil {
		ctx = auth.WithRoles(ctx, roles)
	}
	if perms != nil {
		ctx = auth.WithPermissions(ctx, perms)
	}
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestMLRoutes_RequireComplianceDecide(t *testing.T) {
	router := newMLRouter("http://unused", "")

	// No permission, no fallback role → 403 on every route.
	for _, path := range []string{
		"/api/v1/ml/experiments",
		"/api/v1/ml/runs?experiment=liveness-training",
		"/api/v1/ml/runs/abc/metric-history?metric=loss",
		"/api/v1/ml/deployed-model",
	} {
		rec := mlGet(t, router, path, []string{"product.manage"}, []string{"OFFICER"})
		assert.Equal(t, http.StatusForbidden, rec.Code, "path %s should be gated", path)
	}
}

func TestMLDeployedModel_WithPermission(t *testing.T) {
	router := newMLRouter("http://unused", "")

	rec := mlGet(t, router, "/api/v1/ml/deployed-model",
		[]string{"compliance.decide"}, []string{"OFFICER"})
	require.Equal(t, http.StatusOK, rec.Code)

	var dm struct {
		Reachable bool   `json:"reachable"`
		Message   string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dm))
	assert.False(t, dm.Reachable) // engine URL unset → first-class amber state
	assert.NotEmpty(t, dm.Message)
}

func TestMLDeployedModel_LegacyRoleFallback(t *testing.T) {
	// Tokens without a permissions claim fall back to ADMIN/MANAGER,
	// mirroring the onboarding decide routes.
	router := newMLRouter("http://unused", "")
	rec := mlGet(t, router, "/api/v1/ml/deployed-model", nil, []string{"MANAGER"})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMLSearchRuns_MissingExperimentParam(t *testing.T) {
	router := newMLRouter("http://unused", "")
	rec := mlGet(t, router, "/api/v1/ml/runs", []string{"compliance.decide"}, nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMLMetricHistory_MissingMetricParam(t *testing.T) {
	router := newMLRouter("http://unused", "")
	rec := mlGet(t, router, "/api/v1/ml/runs/abc/metric-history", []string{"compliance.decide"}, nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMLRuns_MLflowDown_Returns503(t *testing.T) {
	// A closed upstream must degrade to a fast, clear 503 — never hang.
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()

	router := newMLRouter(deadURL, "")
	rec := mlGet(t, router, "/api/v1/ml/runs?experiment=liveness-training",
		[]string{"compliance.decide"}, nil)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "MLflow")
}
