package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/origination/model"
	"github.com/athena-lms/go-services/internal/origination/service"
)

// Handler handles HTTP requests for loan origination.
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New creates a new Handler.
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes registers all loan origination routes on the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Loan approval/disbursement authority is restricted to ADMIN/MANAGER.
	// Internal SERVICE calls bypass (see auth.RequireRole). SoD (approver !=
	// creator) is enforced separately in the service layer.
	decide := auth.RequireRole("ADMIN", "MANAGER")

	r.Route("/api/v1/loan-applications", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Get("/", h.List)
		r.Get("/{id}", h.GetByID)
		r.Put("/{id}", h.Update)
		r.Post("/{id}/submit", h.Submit)
		r.Post("/{id}/review/start", h.StartReview)
		r.With(decide).Post("/{id}/review/approve", h.Approve)
		r.With(decide).Post("/{id}/review/reject", h.Reject)
		r.With(decide).Post("/{id}/disburse", h.Disburse)
		r.Post("/{id}/cancel", h.Cancel)
		r.Post("/{id}/collaterals", h.AddCollateral)
		r.Post("/{id}/notes", h.AddNote)
		r.Get("/customer/{customerId}", h.ListByCustomer)
	})
	r.Route("/api/v1/control-config", func(r chi.Router) {
		r.Get("/", h.ListControlConfig)
		r.With(auth.RequireRole("ADMIN")).Put("/", h.UpdateControlConfig)
	})
}

// ListControlConfig returns the effective loan maker-checker config.
func (h *Handler) ListControlConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	httputil.WriteJSON(w, http.StatusOK, h.svc.EffectiveControlConfig(r.Context(), tenantID))
}

// UpdateControlConfig upserts a loan control config row.
func (h *Handler) UpdateControlConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	var req struct {
		Operation string          `json:"operation"`
		Enabled   bool            `json:"enabled"`
		Threshold decimal.Decimal `json:"threshold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	if err := h.svc.UpsertControlConfig(r.Context(), tenantID, req.Operation, req.Enabled, req.Threshold); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, h.svc.EffectiveControlConfig(r.Context(), tenantID))
}

// Create handles POST /api/v1/loan-applications
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.svc.Create(r.Context(), req, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// GetByID handles GET /api/v1/loan-applications/{id}
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid application ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.GetByID(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// Update handles PUT /api/v1/loan-applications/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid application ID", r.URL.Path)
		return
	}

	var req model.CreateApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.svc.Update(r.Context(), id, req, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// Submit handles POST /api/v1/loan-applications/{id}/submit
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid application ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.svc.Submit(r.Context(), id, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// StartReview handles POST /api/v1/loan-applications/{id}/review/start
func (h *Handler) StartReview(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid application ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.svc.StartReview(r.Context(), id, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// Approve handles POST /api/v1/loan-applications/{id}/review/approve
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid application ID", r.URL.Path)
		return
	}

	var req model.ApproveApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.svc.Approve(r.Context(), id, req, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// Reject handles POST /api/v1/loan-applications/{id}/review/reject
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid application ID", r.URL.Path)
		return
	}

	var req model.RejectApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.svc.Reject(r.Context(), id, req, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// Disburse handles POST /api/v1/loan-applications/{id}/disburse
func (h *Handler) Disburse(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid application ID", r.URL.Path)
		return
	}

	var req model.DisburseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.svc.Disburse(r.Context(), id, req, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// Cancel handles POST /api/v1/loan-applications/{id}/cancel
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid application ID", r.URL.Path)
		return
	}

	var reason *string
	if r := r.URL.Query().Get("reason"); r != "" {
		reason = &r
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.svc.Cancel(r.Context(), id, reason, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// AddCollateral handles POST /api/v1/loan-applications/{id}/collaterals
func (h *Handler) AddCollateral(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid application ID", r.URL.Path)
		return
	}

	var req model.AddCollateralRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.AddCollateral(r.Context(), id, req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// AddNote handles POST /api/v1/loan-applications/{id}/notes
func (h *Handler) AddNote(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid application ID", r.URL.Path)
		return
	}

	var req model.AddNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.svc.AddNote(r.Context(), id, req, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// List handles GET /api/v1/loan-applications
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	var status *model.ApplicationStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := model.ApplicationStatus(s)
		if model.ValidApplicationStatuses[st] {
			status = &st
		}
	}

	resp, err := h.svc.List(r.Context(), tenantID, status, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// ListByCustomer handles GET /api/v1/loan-applications/customer/{customerId}
func (h *Handler) ListByCustomer(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customerId")
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.ListByCustomer(r.Context(), customerID, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// ---- helpers ----

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()

	// Typed domain errors carry an explicit status code (e.g. maker-checker
	// violations are 422 BusinessError).
	switch e := err.(type) {
	case *errors.NotFoundError:
		httputil.WriteNotFound(w, e.Message, r.URL.Path)
		return
	case *errors.BusinessError:
		httputil.WriteErrorJSON(w, e.StatusCode, http.StatusText(e.StatusCode), e.Message, r.URL.Path)
		return
	}

	if strings.Contains(msg, "not found") {
		httputil.WriteNotFound(w, msg, r.URL.Path)
		return
	}
	if strings.Contains(msg, "must be in") ||
		strings.Contains(msg, "cannot cancel") ||
		strings.Contains(msg, "is required") ||
		strings.Contains(msg, "must be positive") ||
		strings.Contains(msg, "must be between") ||
		strings.Contains(msg, "is below product minimum") ||
		strings.Contains(msg, "exceeds product maximum") ||
		strings.Contains(msg, "is not available") ||
		strings.Contains(msg, "invalid collateralType") {
		httputil.WriteUnprocessable(w, msg, r.URL.Path)
		return
	}

	h.logger.Error("Internal error", zap.Error(err), zap.String("path", r.URL.Path))
	httputil.WriteInternalError(w, "Internal server error", r.URL.Path)
}
