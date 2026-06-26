package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/common/middleware"
	"github.com/athena-lms/go-services/internal/float/model"
	"github.com/athena-lms/go-services/internal/float/service"
)

// Handler contains the HTTP handlers for the float service.
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New creates a new Handler.
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes registers all float routes on the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/float", func(r chi.Router) {
		r.Post("/accounts", h.CreateAccount)
		r.Get("/accounts", h.ListAccounts)
		r.Get("/accounts/{id}", h.GetAccount)
		r.Post("/accounts/{id}/draw", h.Draw)
		r.Post("/accounts/{id}/repay", h.Repay)
		r.Get("/accounts/{id}/transactions", h.GetTransactions)
		r.Get("/summary", h.GetSummary)
	})
	r.Get("/api/v1/audit-log", h.ListAuditLog)
}

// ListAuditLog returns the float-service audit trail, optionally filtered by
// entityType and entityId query params.
func (h *Handler) ListAuditLog(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	entityType := r.URL.Query().Get("entityType")
	entityID := r.URL.Query().Get("entityId")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 50
	}
	records, err := h.svc.ListAuditLog(r.Context(), tenantID, entityType, entityID, size, page*size)
	if err != nil {
		h.logger.Error("Failed to list audit log", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list audit log", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, records)
}

// CreateAccount handles POST /api/v1/float/accounts
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	var req model.CreateFloatAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	resp, err := h.svc.CreateAccount(r.Context(), &req, tenantID)
	if err != nil {
		middleware.HandleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// ListAccounts handles GET /api/v1/float/accounts
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.ListAccounts(r.Context(), tenantID)
	if err != nil {
		middleware.HandleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetAccount handles GET /api/v1/float/accounts/{id}
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}

	resp, err := h.svc.GetAccount(r.Context(), id, tenantID)
	if err != nil {
		middleware.HandleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// Draw handles POST /api/v1/float/accounts/{id}/draw
func (h *Handler) Draw(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}

	var req model.FloatDrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	resp, err := h.svc.Draw(r.Context(), id, &req, tenantID)
	if err != nil {
		middleware.HandleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// Repay handles POST /api/v1/float/accounts/{id}/repay
func (h *Handler) Repay(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}

	var req model.FloatRepayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	resp, err := h.svc.Repay(r.Context(), id, &req, tenantID)
	if err != nil {
		middleware.HandleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetTransactions handles GET /api/v1/float/accounts/{id}/transactions
func (h *Handler) GetTransactions(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}
	if page < 0 {
		page = 0
	}

	resp, err := h.svc.GetTransactions(r.Context(), id, tenantID, page, size)
	if err != nil {
		middleware.HandleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetSummary handles GET /api/v1/float/summary
func (h *Handler) GetSummary(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.GetSummary(r.Context(), tenantID)
	if err != nil {
		middleware.HandleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}
