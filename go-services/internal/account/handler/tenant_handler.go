package handler

import (
	"encoding/json"

	"github.com/athena-lms/go-services/internal/account/model"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/account/service"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
)

// TenantHandler exposes the tenant registry (Nemo C1): the "create neobank"
// API. All routes are platform-administration and mounted behind the
// tenant.manage permission gate (see main.go).
type TenantHandler struct {
	svc    *service.TenantService
	logger *zap.Logger
}

// NewTenantHandler creates a TenantHandler.
func NewTenantHandler(svc *service.TenantService, logger *zap.Logger) *TenantHandler {
	return &TenantHandler{svc: svc, logger: logger}
}

// RegisterRoutes mounts the tenant registry under /api/v1/tenants. gate is the
// admin authorisation middleware (auth.RequirePermission("tenant.manage", "ADMIN")).
func (h *TenantHandler) RegisterRoutes(r chi.Router, gate func(http.Handler) http.Handler) {
	r.Route("/api/v1/tenants", func(r chi.Router) {
		// Brand pack read is deliberately outside the tenant.manage gate:
		// the portal and the mobile BFF fetch it to theme themselves.
		r.Get("/{id}/brand", h.getBrand)
		r.Group(func(r chi.Router) {
			r.Use(gate)
			r.Get("/", h.list)
			r.Post("/", h.create)
			r.Get("/{id}", h.get)
			r.Post("/{id}/activate", h.activate)
			r.Post("/{id}/suspend", h.suspend)
			r.Put("/{id}/brand", h.putBrand)
		})
	})
}

// create handles POST /api/v1/tenants — the one-call "create neobank" flow.
// The response carries the admin's one-time password exactly once.
func (h *TenantHandler) create(w http.ResponseWriter, r *http.Request) {
	var req service.ProvisionTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	result, err := h.svc.Provision(r.Context(), req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, result)
}

func (h *TenantHandler) list(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.svc.List(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, tenants)
}

func (h *TenantHandler) get(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, t)
}

func (h *TenantHandler) activate(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Activate(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, t)
}

func (h *TenantHandler) suspend(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Suspend(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, t)
}

func (h *TenantHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch e := err.(type) {
	case *errors.NotFoundError:
		httputil.WriteNotFound(w, e.Message, r.URL.Path)
	case *errors.BusinessError:
		httputil.WriteErrorJSON(w, e.StatusCode, http.StatusText(e.StatusCode), e.Message, r.URL.Path)
	default:
		h.logger.Error("Tenant registry error", zap.Error(err), zap.String("path", r.URL.Path))
		httputil.WriteInternalError(w, "An unexpected error occurred", r.URL.Path)
	}
}

// getBrand handles GET /api/v1/tenants/{id}/brand — readable by any
// authenticated caller (portal, BFF via service key): the app needs its
// theme before any staff privilege exists.
func (h *TenantHandler) getBrand(w http.ResponseWriter, r *http.Request) {
	b, err := h.svc.GetBrand(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, b)
}

// putBrand handles PUT /api/v1/tenants/{id}/brand (tenant.manage gated).
func (h *TenantHandler) putBrand(w http.ResponseWriter, r *http.Request) {
	var b model.BrandPack
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	stored, err := h.svc.SetBrand(r.Context(), chi.URLParam(r, "id"), b)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, stored)
}
