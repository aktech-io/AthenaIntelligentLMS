package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/shop/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type BNPLHandler struct {
	svc *service.BNPLService
}

func NewBNPLHandler(svc *service.BNPLService) *BNPLHandler {
	return &BNPLHandler{svc: svc}
}

func (h *BNPLHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/shop/bnpl", func(r chi.Router) {
		// Public endpoints.
		r.Get("/plans", h.ListPlans)
		r.Post("/calculate", h.Calculate)

		// Authenticated endpoints.
		r.Group(func(r chi.Router) {
			r.Use(authMw)
			r.Get("/eligibility", h.CheckEligibility)
		})
	})
}

func (h *BNPLHandler) ListPlans(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	plans, err := h.svc.ListPlans(r.Context(), tenantID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, plans)
}

func (h *BNPLHandler) CheckEligibility(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	customerID := auth.CustomerIDStrFromContext(r.Context())
	if customerID == "" {
		apperrors.WriteError(w, r, http.StatusBadRequest, "customer identity required")
		return
	}

	result, err := h.svc.CheckEligibility(r.Context(), tenantID, customerID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type calculateRequest struct {
	PlanID uuid.UUID `json:"planId"`
	Amount float64   `json:"amount"`
}

func (h *BNPLHandler) Calculate(w http.ResponseWriter, r *http.Request) {
	var req calculateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PlanID == uuid.Nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "planId is required")
		return
	}
	if req.Amount <= 0 {
		apperrors.WriteError(w, r, http.StatusBadRequest, "amount must be positive")
		return
	}

	result, err := h.svc.Calculate(r.Context(), req.PlanID, req.Amount)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
