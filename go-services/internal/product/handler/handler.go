package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/product/model"
	"github.com/athena-lms/go-services/internal/product/service"
)

// Handler provides HTTP handlers for product and charge endpoints.
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New creates a new Handler.
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes registers all product-service routes on the given chi.Router.
// All routes are under /api/v1/products and /api/v1/charges and /api/v1/product-templates.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Product routes
	r.Route("/api/v1/products", func(r chi.Router) {
		r.Post("/", h.createProduct)
		r.Get("/", h.listProducts)
		r.Post("/from-template/{code}", h.createFromTemplate)
		r.Get("/{id}", h.getProduct)
		r.Put("/{id}", h.updateProduct)
		r.Post("/{id}/activate", h.activateProduct)
		r.Post("/{id}/deactivate", h.deactivateProduct)
		r.Post("/{id}/pause", h.pauseProduct)
		r.Post("/{id}/simulate", h.simulateSchedule)
		r.Get("/{id}/versions", h.getProductVersions)
	})

	// Charge routes
	r.Route("/api/v1/charges", func(r chi.Router) {
		r.Post("/", h.createCharge)
		r.Get("/", h.listCharges)
		r.Get("/calculate", h.calculateCharge)
		r.Get("/{id}", h.getCharge)
		r.Put("/{id}", h.updateCharge)
		r.Delete("/{id}", h.deleteCharge)
	})

	// Template routes
	r.Route("/api/v1/product-templates", func(r chi.Router) {
		r.Get("/", h.listTemplates)
		r.Get("/{code}", h.getTemplate)
	})

	// Audit trail
	r.Get("/api/v1/audit-log", h.listAuditLog)
	r.Get("/api/v1/audit-log/verify", h.verifyAuditChain)
}

// listAuditLog returns the product-service audit trail, optionally filtered by
// entityType and entityId query params.
func (h *Handler) listAuditLog(w http.ResponseWriter, r *http.Request) {
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

// verifyAuditChain reports whether the tamper-evident audit trail is intact, or
// the seq of the first altered/missing entry. Lets an auditor prove the log was
// not edited or back-dated.
func (h *Handler) verifyAuditChain(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	result, err := h.svc.VerifyAuditChain(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to verify audit chain", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to verify audit chain", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

// ─── Product Handlers ───────────────────────────────────────────────────────

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var req model.CreateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	username := auth.UserIDFromContext(r.Context())
	if username == "" {
		username = "system"
	}

	resp, err := h.svc.CreateProduct(r.Context(), req, tenantID, username)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 0)
	size := queryInt(r, "size", 20)
	tenantID := auth.TenantIDOrDefault(r.Context())

	products, total, err := h.svc.ListProducts(r.Context(), tenantID, page, size)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(products, page, size, total))
}

func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid product ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.GetProduct(r.Context(), id, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid product ID", r.URL.Path)
		return
	}

	var req model.CreateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	username := auth.UserIDFromContext(r.Context())
	if username == "" {
		username = "system"
	}
	changeReason := r.URL.Query().Get("changeReason")
	if changeReason == "" {
		changeReason = "Product update"
	}

	resp, err := h.svc.UpdateProduct(r.Context(), id, req, tenantID, username, changeReason)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) activateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid product ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	username := auth.UserIDFromContext(r.Context())
	if username == "" {
		username = "system"
	}

	resp, err := h.svc.ActivateProduct(r.Context(), id, tenantID, username)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) deactivateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid product ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.DeactivateProduct(r.Context(), id, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) pauseProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid product ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	username := auth.UserIDFromContext(r.Context())
	if username == "" {
		username = "system"
	}

	resp, err := h.svc.PauseProduct(r.Context(), id, tenantID, username)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) simulateSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid product ID", r.URL.Path)
		return
	}

	var req model.SimulateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.SimulateSchedule(r.Context(), id, req, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getProductVersions(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid product ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	versions, err := h.svc.GetProductVersions(r.Context(), id, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, versions)
}

func (h *Handler) createFromTemplate(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	tenantID := auth.TenantIDOrDefault(r.Context())
	username := auth.UserIDFromContext(r.Context())
	if username == "" {
		username = "system"
	}

	resp, err := h.svc.CreateFromTemplate(r.Context(), code, tenantID, username)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// ─── Charge Handlers ────────────────────────────────────────────────────────

func (h *Handler) createCharge(w http.ResponseWriter, r *http.Request) {
	var req model.CreateChargeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.CreateCharge(r.Context(), req, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) listCharges(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 0)
	size := queryInt(r, "size", 20)
	tenantID := auth.TenantIDOrDefault(r.Context())

	charges, total, err := h.svc.ListCharges(r.Context(), tenantID, page, size)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(charges, page, size, total))
}

func (h *Handler) getCharge(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid charge ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.GetCharge(r.Context(), id, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) updateCharge(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid charge ID", r.URL.Path)
		return
	}

	var req model.CreateChargeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.UpdateCharge(r.Context(), id, req, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) deleteCharge(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid charge ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	if err := h.svc.DeleteCharge(r.Context(), id, tenantID); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) calculateCharge(w http.ResponseWriter, r *http.Request) {
	txnType := r.URL.Query().Get("transactionType")
	amountStr := r.URL.Query().Get("amount")
	if txnType == "" || amountStr == "" {
		httputil.WriteBadRequest(w, "transactionType and amount are required", r.URL.Path)
		return
	}

	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid amount: "+amountStr, r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.CalculateCharge(r.Context(), txnType, amount, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// ─── Template Handlers ──────────────────────────────────────────────────────

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.svc.ListTemplates(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, templates)
}

func (h *Handler) getTemplate(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	tmpl, err := h.svc.GetTemplate(r.Context(), code)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, tmpl)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var bErr *service.BusinessError
	var cErr *service.ConflictError
	var nErr *service.NotFoundError

	switch {
	case errors.As(err, &nErr):
		httputil.WriteNotFound(w, nErr.Msg, r.URL.Path)
	case errors.As(err, &cErr):
		httputil.WriteConflict(w, cErr.Msg, r.URL.Path)
	case errors.As(err, &bErr):
		switch bErr.Status {
		case http.StatusBadRequest:
			httputil.WriteBadRequest(w, bErr.Msg, r.URL.Path)
		case http.StatusConflict:
			httputil.WriteConflict(w, bErr.Msg, r.URL.Path)
		default:
			httputil.WriteUnprocessable(w, bErr.Msg, r.URL.Path)
		}
	default:
		h.logger.Error("Internal error", zap.Error(err), zap.String("path", r.URL.Path))
		httputil.WriteInternalError(w, "Internal server error", r.URL.Path)
	}
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
