package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/product/model"
	"github.com/athena-lms/go-services/internal/product/service"
)

// DepositHandler provides HTTP handlers for deposit product endpoints.
type DepositHandler struct {
	svc    *service.DepositService
	logger *zap.Logger
}

// NewDepositHandler creates a new DepositHandler.
func NewDepositHandler(svc *service.DepositService, logger *zap.Logger) *DepositHandler {
	return &DepositHandler{svc: svc, logger: logger}
}

// RegisterRoutes registers all deposit product routes.
func (h *DepositHandler) RegisterRoutes(r chi.Router) {
	// Deposit-product mutations are restricted to ADMIN and MANAGER; reads stay open.
	admin := auth.RequireRole("ADMIN", "MANAGER")

	r.Route("/api/v1/deposit-products", func(r chi.Router) {
		// Mutations: create/update/activate/deactivate — ADMIN/MANAGER only.
		r.With(admin).Post("/", h.createDepositProduct)
		r.With(admin).Put("/{id}", h.updateDepositProduct)
		r.With(admin).Post("/{id}/activate", h.activateDepositProduct)
		r.With(admin).Post("/{id}/deactivate", h.deactivateDepositProduct)
		// Reads stay open.
		r.Get("/", h.listDepositProducts)
		r.Get("/{id}", h.getDepositProduct)
	})
}

func (h *DepositHandler) createDepositProduct(w http.ResponseWriter, r *http.Request) {
	var req model.CreateDepositProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	username := auth.UserIDFromContext(r.Context())
	if username == "" {
		username = "system"
	}

	resp, err := h.svc.CreateDepositProduct(r.Context(), req, tenantID, username)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *DepositHandler) listDepositProducts(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 0)
	size := queryInt(r, "size", 20)
	tenantID := auth.TenantIDOrDefault(r.Context())

	products, total, err := h.svc.ListDepositProducts(r.Context(), tenantID, page, size)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(products, page, size, total))
}

func (h *DepositHandler) getDepositProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid product ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.GetDepositProduct(r.Context(), id, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *DepositHandler) updateDepositProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid product ID", r.URL.Path)
		return
	}

	var req model.CreateDepositProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.UpdateDepositProduct(r.Context(), id, req, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *DepositHandler) activateDepositProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid product ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.ActivateDepositProduct(r.Context(), id, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *DepositHandler) deactivateDepositProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid product ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.DeactivateDepositProduct(r.Context(), id, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *DepositHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	// Reuse the same error pattern from the main handler
	writeServiceError(w, r, err, h.logger)
}

// writeServiceError is a shared error handler for deposit and product handlers.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error, logger *zap.Logger) {
	switch e := err.(type) {
	case *service.NotFoundError:
		httputil.WriteNotFound(w, e.Msg, r.URL.Path)
	case *service.ConflictError:
		httputil.WriteConflict(w, e.Msg, r.URL.Path)
	case *service.BusinessError:
		if e.Status == http.StatusBadRequest {
			httputil.WriteBadRequest(w, e.Msg, r.URL.Path)
		} else {
			httputil.WriteUnprocessable(w, e.Msg, r.URL.Path)
		}
	default:
		logger.Error("Internal error", zap.Error(err), zap.String("path", r.URL.Path))
		httputil.WriteInternalError(w, "Internal server error", r.URL.Path)
	}
}
