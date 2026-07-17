package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/billpay/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type BillPayHandler struct {
	svc *service.BillPayService
}

func NewBillPayHandler(svc *service.BillPayService) *BillPayHandler {
	return &BillPayHandler{svc: svc}
}

// Routes registers bill pay endpoints on the given router.
func (h *BillPayHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/billpay", func(r chi.Router) {
		r.Use(authMw)
		r.Get("/categories", h.ListCategories)
		r.Get("/billers", h.ListBillers)
		r.Post("/validate", h.Validate)
		r.Post("/pay", h.Pay)
		r.Get("/history", h.History)
		r.Get("/saved", h.ListSaved)
		r.Post("/saved", h.SaveBiller)
		r.Delete("/saved/{id}", h.DeleteSaved)
	})
}

func (h *BillPayHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	categories, err := h.svc.ListCategories(r.Context(), tenantID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	resp := make([]interface{}, 0, len(categories))
	for i := range categories {
		resp = append(resp, categories[i].ToResponse())
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *BillPayHandler) ListBillers(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	query := r.URL.Query().Get("q")

	var categoryID *uuid.UUID
	if catStr := r.URL.Query().Get("categoryId"); catStr != "" {
		id, err := uuid.Parse(catStr)
		if err != nil {
			apperrors.WriteError(w, r, http.StatusBadRequest, "invalid categoryId")
			return
		}
		categoryID = &id
	}

	billers, err := h.svc.ListBillers(r.Context(), tenantID, categoryID, query)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	resp := make([]interface{}, 0, len(billers))
	for i := range billers {
		resp = append(resp, billers[i].ToResponse())
	}
	writeJSON(w, http.StatusOK, resp)
}

type validateRequest struct {
	BillerCode    string `json:"billerCode"`
	AccountNumber string `json:"accountNumber"`
}

func (h *BillPayHandler) Validate(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BillerCode == "" || req.AccountNumber == "" {
		apperrors.WriteError(w, r, http.StatusBadRequest, "billerCode and accountNumber are required")
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	result, err := h.svc.ValidateBiller(r.Context(), tenantID, req.BillerCode, req.AccountNumber)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *BillPayHandler) Pay(w http.ResponseWriter, r *http.Request) {
	var req service.PayBillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BillerID == uuid.Nil || req.AccountNumber == "" || req.Amount <= 0 || req.SourceAccountID == uuid.Nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "billerId, accountNumber, amount, and sourceAccountId are required")
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := uuid.Parse(auth.MobileUserIDFromContext(r.Context()))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	payment, err := h.svc.PayBill(r.Context(), tenantID, userID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, payment.ToResponse())
}

func (h *BillPayHandler) History(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
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

	payments, total, err := h.svc.GetHistory(r.Context(), tenantID, userID, page, size)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	responses := make([]interface{}, 0, len(payments))
	for i := range payments {
		responses = append(responses, payments[i].ToResponse())
	}
	writeJSON(w, http.StatusOK, dto.NewPageResponse(responses, page, size, total))
}

func (h *BillPayHandler) ListSaved(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := uuid.Parse(auth.MobileUserIDFromContext(r.Context()))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	saved, err := h.svc.ListSavedBillers(r.Context(), tenantID, userID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	resp := make([]interface{}, 0, len(saved))
	for i := range saved {
		resp = append(resp, saved[i].ToResponse())
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *BillPayHandler) SaveBiller(w http.ResponseWriter, r *http.Request) {
	var req service.SaveBillerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BillerID == uuid.Nil || req.AccountNumber == "" {
		apperrors.WriteError(w, r, http.StatusBadRequest, "billerId and accountNumber are required")
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := uuid.Parse(auth.MobileUserIDFromContext(r.Context()))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	saved, err := h.svc.SaveBiller(r.Context(), tenantID, userID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, saved.ToResponse())
}

func (h *BillPayHandler) DeleteSaved(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid id")
		return
	}

	userID, err := uuid.Parse(auth.MobileUserIDFromContext(r.Context()))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	if err := h.svc.DeleteSavedBiller(r.Context(), id, userID); err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
