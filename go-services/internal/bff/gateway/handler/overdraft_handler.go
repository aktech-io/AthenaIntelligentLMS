package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/athena-lms/go-services/internal/bff/gateway/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type OverdraftHandler struct {
	svc *service.OverdraftProxyService
}

func NewOverdraftHandler(svc *service.OverdraftProxyService) *OverdraftHandler {
	return &OverdraftHandler{svc: svc}
}

func (h *OverdraftHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/mobile/overdraft", func(r chi.Router) {
		r.Use(authMw)
		r.Get("/", h.GetOverdraftStatus)
		r.Post("/setup", h.SetupOverdraft)
		r.Post("/deposit", h.Deposit)
		r.Post("/withdraw", h.Withdraw)
		r.Get("/transactions", h.GetTransactions)
		r.Post("/suspend", h.SuspendOverdraft)
		r.Get("/charges", h.GetCharges)
	})
}

func (h *OverdraftHandler) GetOverdraftStatus(w http.ResponseWriter, r *http.Request) {
	customerID := auth.CustomerIDStrFromContext(r.Context())
	resp, err := h.svc.GetOverdraftStatus(r.Context(), customerID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *OverdraftHandler) SetupOverdraft(w http.ResponseWriter, r *http.Request) {
	customerID := auth.CustomerIDStrFromContext(r.Context())
	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.SetupOverdraft(r.Context(), customerID, tenantID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *OverdraftHandler) Deposit(w http.ResponseWriter, r *http.Request) {
	userID, err := resolveUserID(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid user")
		return
	}

	var req service.OverdraftDepositRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	customerID := auth.CustomerIDStrFromContext(r.Context())
	resp, err := h.svc.Deposit(r.Context(), userID, customerID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *OverdraftHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, err := resolveUserID(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid user")
		return
	}

	var req service.OverdraftWithdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	customerID := auth.CustomerIDStrFromContext(r.Context())
	resp, err := h.svc.Withdraw(r.Context(), userID, customerID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *OverdraftHandler) GetTransactions(w http.ResponseWriter, r *http.Request) {
	customerID := auth.CustomerIDStrFromContext(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	resp, err := h.svc.GetTransactions(r.Context(), customerID, page, size)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *OverdraftHandler) SuspendOverdraft(w http.ResponseWriter, r *http.Request) {
	customerID := auth.CustomerIDStrFromContext(r.Context())
	resp, err := h.svc.SuspendOverdraft(r.Context(), customerID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *OverdraftHandler) GetCharges(w http.ResponseWriter, r *http.Request) {
	customerID := auth.CustomerIDStrFromContext(r.Context())
	charges, err := h.svc.GetCharges(r.Context(), customerID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, charges)
}
