package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	commonerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/compliance/model"
	"github.com/athena-lms/go-services/internal/compliance/service"
)

// Handler handles HTTP requests for the compliance service.
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New creates a new Handler.
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes registers all compliance routes on the given chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Regulated compliance decisions (SAR filing, AML alert resolution, KYC
	// pass/fail) are gated to ADMIN/MANAGER. Internal SERVICE calls always pass
	// (see auth.RequireRole), so system-generated alerts/events are unaffected.
	// Data entry (KYC create/update) and all reads stay open to staff.
	// RBAC: gated by the compliance.decide permission (matrix-managed), falling
	// back to ADMIN/MANAGER for tokens issued before permissions were stamped.
	decide := auth.RequirePermission("compliance.decide", "ADMIN", "MANAGER")

	r.Route("/api/v1/compliance", func(r chi.Router) {
		// AML Alerts
		r.Post("/alerts", h.CreateAlert)
		r.Get("/alerts", h.ListAlerts)
		r.Get("/alerts/{id}", h.GetAlert)
		r.With(decide).Post("/alerts/{id}/resolve", h.ResolveAlert)
		r.With(decide).Post("/alerts/{id}/sar", h.FileSar)
		r.Get("/alerts/{id}/sar", h.GetSarForAlert)

		// KYC
		r.Post("/kyc", h.CreateOrUpdateKyc)
		r.Get("/kyc/{customerId}", h.GetKyc)
		r.With(decide).Post("/kyc/{customerId}/pass", h.PassKyc)
		r.With(decide).Post("/kyc/{customerId}/fail", h.FailKyc)

		// Events
		r.Get("/events", h.ListEvents)

		// Summary
		r.Get("/summary", h.GetSummary)
	})
}

// ─── AML Alerts ─────────────────────────────────────────────────────────────

// CreateAlert handles POST /api/v1/compliance/alerts
func (h *Handler) CreateAlert(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)

	var req model.CreateAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	alert, err := h.svc.CreateAlert(r.Context(), req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, alert)
}

// ListAlerts handles GET /api/v1/compliance/alerts
func (h *Handler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)
	page, size := parsePagination(r)

	var status *model.AlertStatus
	if s := r.URL.Query().Get("status"); s != "" {
		if !model.ValidAlertStatus(s) {
			httputil.WriteBadRequest(w, "invalid alert status: "+s, r.URL.Path)
			return
		}
		st := model.AlertStatus(s)
		status = &st
	}

	resp, err := h.svc.ListAlerts(r.Context(), tenantID, status, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetAlert handles GET /api/v1/compliance/alerts/{id}
func (h *Handler) GetAlert(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid alert ID", r.URL.Path)
		return
	}

	alert, err := h.svc.GetAlert(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, alert)
}

// ResolveAlert handles POST /api/v1/compliance/alerts/{id}/resolve
func (h *Handler) ResolveAlert(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid alert ID", r.URL.Path)
		return
	}

	var req model.ResolveAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	alert, err := h.svc.ResolveAlert(r.Context(), id, req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, alert)
}

// FileSar handles POST /api/v1/compliance/alerts/{id}/sar
func (h *Handler) FileSar(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid alert ID", r.URL.Path)
		return
	}

	var req model.FileSarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	sar, err := h.svc.FileSar(r.Context(), id, req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, sar)
}

// GetSarForAlert handles GET /api/v1/compliance/alerts/{id}/sar
func (h *Handler) GetSarForAlert(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid alert ID", r.URL.Path)
		return
	}

	sar, err := h.svc.GetSarForAlert(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, sar)
}

// ─── KYC ────────────────────────────────────────────────────────────────────

// CreateOrUpdateKyc handles POST /api/v1/compliance/kyc
func (h *Handler) CreateOrUpdateKyc(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)

	var req model.KycRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	rec, err := h.svc.CreateOrUpdateKyc(r.Context(), req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, rec)
}

// GetKyc handles GET /api/v1/compliance/kyc/{customerId}
func (h *Handler) GetKyc(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)
	customerID := chi.URLParam(r, "customerId")

	rec, err := h.svc.GetKyc(r.Context(), customerID, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, rec)
}

// PassKyc handles POST /api/v1/compliance/kyc/{customerId}/pass
func (h *Handler) PassKyc(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)
	customerID := chi.URLParam(r, "customerId")

	rec, err := h.svc.PassKyc(r.Context(), customerID, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, rec)
}

// FailKyc handles POST /api/v1/compliance/kyc/{customerId}/fail
func (h *Handler) FailKyc(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)
	customerID := chi.URLParam(r, "customerId")

	var req model.ResolveAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	rec, err := h.svc.FailKyc(r.Context(), customerID, req.ResolutionNotes, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, rec)
}

// ─── Events ─────────────────────────────────────────────────────────────────

// ListEvents handles GET /api/v1/compliance/events
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)
	page, size := parsePagination(r)

	resp, err := h.svc.ListEvents(r.Context(), tenantID, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// ─── Summary ────────────────────────────────────────────────────────────────

// GetSummary handles GET /api/v1/compliance/summary
func (h *Handler) GetSummary(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)

	summary, err := h.svc.GetSummary(r.Context(), tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, summary)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// resolveTenantID extracts the tenant ID from the X-Tenant-Id header or context.
func resolveTenantID(r *http.Request) string {
	if tid := r.Header.Get("X-Tenant-Id"); tid != "" {
		return tid
	}
	return auth.TenantIDOrDefault(r.Context())
}

// parsePagination extracts page and size query params with defaults.
func parsePagination(r *http.Request) (int, int) {
	page := 0
	size := 20

	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n >= 0 {
			page = n
		}
	}
	if s := r.URL.Query().Get("size"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 100 {
			size = n
		}
	}
	return page, size
}

// handleError maps domain errors to HTTP responses.
func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
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
