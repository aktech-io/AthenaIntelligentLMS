package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/athena-lms/go-services/internal/bff/gateway/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type DashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(svc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

func (h *DashboardHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/mobile/dashboard", func(r chi.Router) {
		r.Use(authMw)
		r.Get("/", h.GetDashboard)
	})
}

func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	userID, err := resolveUserID(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid user")
		return
	}

	customerID := auth.CustomerIDStrFromContext(r.Context())
	resp, err := h.svc.GetDashboard(r.Context(), userID, customerID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
