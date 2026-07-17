package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/billpay/model"
	"github.com/athena-lms/go-services/internal/bff/billpay/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type SavingsHandler struct {
	svc *service.SavingsService
}

func NewSavingsHandler(svc *service.SavingsService) *SavingsHandler {
	return &SavingsHandler{svc: svc}
}

// Routes registers savings endpoints on the given router.
func (h *SavingsHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/savings", func(r chi.Router) {
		r.Use(authMw)
		r.Get("/goals", h.ListGoals)
		r.Post("/goals", h.CreateGoal)
		r.Post("/goals/{id}/deposit", h.Deposit)
		r.Post("/goals/{id}/withdraw", h.Withdraw)
		r.Get("/goals/{id}/transactions", h.Transactions)
	})
}

func (h *SavingsHandler) ListGoals(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := uuid.Parse(auth.MobileUserIDFromContext(r.Context()))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	goals, err := h.svc.ListGoals(r.Context(), tenantID, userID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	resp := make([]model.SavingsGoalResponse, len(goals))
	for i := range goals {
		resp[i] = goals[i].ToResponse()
	}
	if resp == nil {
		resp = []model.SavingsGoalResponse{}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *SavingsHandler) CreateGoal(w http.ResponseWriter, r *http.Request) {
	var req service.CreateGoalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := uuid.Parse(auth.MobileUserIDFromContext(r.Context()))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	goal, err := h.svc.CreateGoal(r.Context(), tenantID, userID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, goal.ToResponse())
}

func (h *SavingsHandler) Deposit(w http.ResponseWriter, r *http.Request) {
	goalID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid goal id")
		return
	}

	var req service.DepositRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := uuid.Parse(auth.MobileUserIDFromContext(r.Context()))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	goal, err := h.svc.Deposit(r.Context(), tenantID, userID, goalID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, goal.ToResponse())
}

func (h *SavingsHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	goalID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid goal id")
		return
	}

	var req service.WithdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := uuid.Parse(auth.MobileUserIDFromContext(r.Context()))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	goal, err := h.svc.Withdraw(r.Context(), tenantID, userID, goalID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, goal.ToResponse())
}

func (h *SavingsHandler) Transactions(w http.ResponseWriter, r *http.Request) {
	goalID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid goal id")
		return
	}

	userID, err := uuid.Parse(auth.MobileUserIDFromContext(r.Context()))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	txns, total, err := h.svc.GetTransactions(r.Context(), userID, goalID, page, size)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	resp := make([]model.SavingsTransactionResponse, len(txns))
	for i := range txns {
		resp[i] = txns[i].ToResponse()
	}
	writeJSON(w, http.StatusOK, dto.NewPageResponse(resp, page, size, total))
}
