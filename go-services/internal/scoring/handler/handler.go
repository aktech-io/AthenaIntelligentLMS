package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/scoring/model"
	"github.com/athena-lms/go-services/internal/scoring/service"
)

// Handler exposes AI scoring HTTP endpoints.
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New creates a new Handler.
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes mounts all scoring routes on the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/scoring", func(r chi.Router) {
		r.Post("/requests", h.manualScore)
		r.Get("/requests", h.listRequests)
		r.Get("/requests/{id}", h.getRequest)
		r.Get("/applications/{applicationId}/request", h.getRequestByApplication)
		r.Get("/applications/{applicationId}/result", h.getResultByApplication)
		r.Get("/customers/{customerId}/latest", h.getLatestResultByCustomer)
	})
}

func (h *Handler) manualScore(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)

	var req model.ManualScoringRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	if req.LoanApplicationID == "" {
		httputil.WriteBadRequest(w, "loanApplicationId is required", r.URL.Path)
		return
	}
	if req.CustomerID == 0 {
		httputil.WriteBadRequest(w, "customerId is required", r.URL.Path)
		return
	}

	resp, err := h.svc.ManualScore(r.Context(), &req, tenantID)
	if err != nil {
		h.logger.Error("Failed to trigger manual score", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to trigger scoring", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) listRequests(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	requests, total, err := h.svc.ListRequests(r.Context(), tenantID, page, size)
	if err != nil {
		h.logger.Error("Failed to list scoring requests", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list scoring requests", r.URL.Path)
		return
	}
	if requests == nil {
		requests = []model.ScoringRequestResponse{}
	}

	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(requests, page, size, total))
}

func (h *Handler) getRequest(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)
	id := chi.URLParam(r, "id")

	resp, err := h.svc.GetRequest(r.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to get scoring request", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get scoring request", r.URL.Path)
		return
	}
	if resp == nil {
		httputil.WriteNotFound(w, "ScoringRequest not found with id: "+id, r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getRequestByApplication(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)
	applicationID := chi.URLParam(r, "applicationId")

	resp, err := h.svc.GetRequestByApplication(r.Context(), applicationID, tenantID)
	if err != nil {
		h.logger.Error("Failed to get scoring request by application", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get scoring request", r.URL.Path)
		return
	}
	if resp == nil {
		httputil.WriteNotFound(w, "ScoringRequest not found for application: "+applicationID, r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getResultByApplication(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)
	applicationID := chi.URLParam(r, "applicationId")

	resp, err := h.svc.GetResultByApplication(r.Context(), applicationID, tenantID)
	if err != nil {
		h.logger.Error("Failed to get scoring result by application", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get scoring result", r.URL.Path)
		return
	}
	if resp == nil {
		httputil.WriteNotFound(w, "ScoringResult not found for application: "+applicationID, r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getLatestResultByCustomer(w http.ResponseWriter, r *http.Request) {
	tenantID := resolveTenantID(r)
	customerIDStr := chi.URLParam(r, "customerId")

	// Same coercion as the event consumer's resolveCustomerID: platform
	// customer IDs are varchar (e.g. "CUST-A1B2"), stored hashed — a plain
	// ParseInt 400s every lookup for them and fail-closes overdraft approval.
	if customerIDStr == "" {
		httputil.WriteBadRequest(w, "Invalid customerId: "+customerIDStr, r.URL.Path)
		return
	}
	customerID := model.FlexibleCustomerID(customerIDStr)

	resp, err := h.svc.GetLatestResultByCustomer(r.Context(), customerID, tenantID)
	if err != nil {
		h.logger.Error("Failed to get latest scoring result by customer", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to get scoring result", r.URL.Path)
		return
	}
	if resp == nil {
		httputil.WriteNotFound(w, "ScoringResult not found for customer: "+customerIDStr, r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// resolveTenantID extracts tenant ID from context (set by auth middleware) or X-Tenant-ID header.
func resolveTenantID(r *http.Request) string {
	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" {
		return tenantID
	}
	header := r.Header.Get("X-Tenant-ID")
	if header != "" {
		return header
	}
	return "default"
}
