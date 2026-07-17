package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/athena-lms/go-services/internal/bff/gateway/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type TransferHandler struct {
	svc *service.TransferService
}

func NewTransferHandler(svc *service.TransferService) *TransferHandler {
	return &TransferHandler{svc: svc}
}

func (h *TransferHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/mobile/transfers", func(r chi.Router) {
		r.Use(authMw)
		r.Post("/send", h.SendMoney)
	})
}

func (h *TransferHandler) SendMoney(w http.ResponseWriter, r *http.Request) {
	userID, err := resolveUserID(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid user")
		return
	}

	var req service.SendTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	customerID := auth.CustomerIDStrFromContext(r.Context())
	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.SendMoney(r.Context(), userID, customerID, tenantID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
