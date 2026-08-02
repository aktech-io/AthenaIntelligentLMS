package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	commonerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/compliance/service"
)

// MLHandler exposes the read-only ML observability proxy (MLflow tracking
// server + eKYC engine health) behind /api/v1/ml for the portal's Model
// Training page. MLflow itself has no auth and is never exposed via
// ingress — every route here is gated by compliance.decide, the same
// permission that guards onboarding/KYC decisions, because liveness
// training and red-team telemetry is compliance-sensitive.
type MLHandler struct {
	svc    *service.MLService
	logger *zap.Logger
}

// NewML creates an MLHandler.
func NewML(svc *service.MLService, logger *zap.Logger) *MLHandler {
	return &MLHandler{svc: svc, logger: logger}
}

// handleError maps domain errors to HTTP responses (same mapping as the
// main compliance handler).
func (h *MLHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch e := err.(type) {
	case *commonerrors.NotFoundError:
		httputil.WriteNotFound(w, e.Message, r.URL.Path)
	case *commonerrors.BusinessError:
		httputil.WriteErrorJSON(w, e.StatusCode, http.StatusText(e.StatusCode), e.Message, r.URL.Path)
	default:
		h.logger.Error("Internal error", zap.Error(err), zap.String("path", r.URL.Path))
		httputil.WriteInternalError(w, "internal server error", r.URL.Path)
	}
}

// RegisterRoutes mounts the ML observability routes. The whole group is
// gated (not just mutations): these reads expose model provenance and
// red-team results.
func (h *MLHandler) RegisterRoutes(r chi.Router) {
	decide := auth.RequirePermission("compliance.decide", "ADMIN", "MANAGER")
	r.Route("/api/v1/ml", func(r chi.Router) {
		r.Use(decide)
		r.Get("/experiments", h.ListExperiments)
		r.Get("/runs", h.SearchRuns)
		r.Get("/runs/{runId}/metric-history", h.MetricHistory)
		r.Get("/deployed-model", h.DeployedModel)
	})
}

// ListExperiments handles GET /api/v1/ml/experiments
func (h *MLHandler) ListExperiments(w http.ResponseWriter, r *http.Request) {
	exps, err := h.svc.ListMLExperiments(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"experiments": exps})
}

// SearchRuns handles GET /api/v1/ml/runs?experiment=<name>&limit=N
func (h *MLHandler) SearchRuns(w http.ResponseWriter, r *http.Request) {
	experiment := strings.TrimSpace(r.URL.Query().Get("experiment"))
	if experiment == "" {
		httputil.WriteBadRequest(w, "experiment query parameter is required", r.URL.Path)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit")) // 0 → service default
	runs, err := h.svc.SearchMLRuns(r.Context(), experiment, limit)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"experiment": experiment,
		"runs":       runs,
	})
}

// MetricHistory handles GET /api/v1/ml/runs/{runId}/metric-history?metric=<name>
func (h *MLHandler) MetricHistory(w http.ResponseWriter, r *http.Request) {
	metric := strings.TrimSpace(r.URL.Query().Get("metric"))
	if metric == "" {
		httputil.WriteBadRequest(w, "metric query parameter is required", r.URL.Path)
		return
	}
	hist, err := h.svc.MLMetricHistory(r.Context(), chi.URLParam(r, "runId"), metric)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, hist)
}

// DeployedModel handles GET /api/v1/ml/deployed-model. Engine
// unreachability is reported as {reachable:false,...} with HTTP 200 — a
// first-class state the portal renders as an amber card.
func (h *MLHandler) DeployedModel(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, h.svc.DeployedModel(r.Context()))
}
