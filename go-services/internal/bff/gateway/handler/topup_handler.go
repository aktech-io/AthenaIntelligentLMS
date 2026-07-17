package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/athena-lms/go-services/internal/bff/gateway/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type TopUpHandler struct {
	svc *service.TopUpService
}

func NewTopUpHandler(svc *service.TopUpService) *TopUpHandler {
	return &TopUpHandler{svc: svc}
}

func (h *TopUpHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/mobile/topup", func(r chi.Router) {
		r.Use(authMw)
		r.Post("/", h.TopUp)
	})
}

func (h *TopUpHandler) TopUp(w http.ResponseWriter, r *http.Request) {
	var req service.TopUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	customerID := auth.CustomerIDStrFromContext(r.Context())
	resp, err := h.svc.TopUp(r.Context(), customerID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
