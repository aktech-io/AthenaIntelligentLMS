package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/athena-lms/go-services/internal/bff/gateway/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type LoanHandler struct {
	svc *service.LoanProxyService
}

func NewLoanHandler(svc *service.LoanProxyService) *LoanHandler {
	return &LoanHandler{svc: svc}
}

func (h *LoanHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/mobile/loans", func(r chi.Router) {
		r.Use(authMw)
		r.Get("/products", h.GetProducts)
		r.Post("/apply", h.ApplyForLoan)
		r.Get("/active", h.GetActiveLoans)
		r.Get("/{loanId}/schedule", h.GetLoanSchedule)
		r.Post("/repay", h.MakeRepayment)
	})
}

func (h *LoanHandler) GetProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.svc.GetLoanProducts(r.Context())
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, products)
}

func (h *LoanHandler) ApplyForLoan(w http.ResponseWriter, r *http.Request) {
	var req service.LoanApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	customerID := auth.CustomerIDStrFromContext(r.Context())
	resp, err := h.svc.ApplyForLoan(r.Context(), customerID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *LoanHandler) GetActiveLoans(w http.ResponseWriter, r *http.Request) {
	customerID := auth.CustomerIDStrFromContext(r.Context())
	loans, err := h.svc.GetActiveLoans(r.Context(), customerID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, loans)
}

func (h *LoanHandler) GetLoanSchedule(w http.ResponseWriter, r *http.Request) {
	loanID := chi.URLParam(r, "loanId")
	schedule, err := h.svc.GetLoanSchedule(r.Context(), loanID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, schedule)
}

func (h *LoanHandler) MakeRepayment(w http.ResponseWriter, r *http.Request) {
	userID, err := resolveUserID(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid user")
		return
	}

	var req service.LoanRepaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	customerID := auth.CustomerIDStrFromContext(r.Context())
	resp, err := h.svc.MakeRepayment(r.Context(), userID, customerID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
