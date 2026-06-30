// Package handler exposes the per-tenant regulatory profile over HTTP.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/regulatory/model"
	"github.com/athena-lms/go-services/internal/regulatory/service"
)

// Handler serves the regulatory-profile endpoints.
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New creates a new Handler.
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes mounts the regulatory routes. Read is open (any authenticated
// caller); mutation is ADMIN-only and audited.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/regulatory", func(r chi.Router) {
		r.Get("/profile", h.getProfile)
		r.With(auth.RequireRole("ADMIN")).Put("/profile", h.updateProfile)
	})
}

// getProfile returns the tenant's active regulatory profile, seeding a default
// DCP profile on first access.
func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	p, err := h.svc.GetOrCreateForTenant(r.Context(), tenantID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

// updateProfile applies a partial update to the tenant's regulatory profile.
func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var req model.UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "Bad Request", "Invalid JSON body", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	actor := auth.UserIDFromContext(r.Context())
	p, err := h.svc.UpdateForTenant(r.Context(), tenantID, actor, &req)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

// writeErr maps a service error to 400 (validation) or 500 (everything else).
func (h *Handler) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, service.ErrValidation) {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	h.logger.Error("regulatory profile request failed", zap.Error(err))
	httputil.WriteErrorJSON(w, http.StatusInternalServerError, "Internal Server Error", "Could not process regulatory profile", r.URL.Path)
}
