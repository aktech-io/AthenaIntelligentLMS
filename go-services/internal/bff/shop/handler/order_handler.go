package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/shop/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type OrderHandler struct {
	svc *service.OrderService
}

func NewOrderHandler(svc *service.OrderService) *OrderHandler {
	return &OrderHandler{svc: svc}
}

func (h *OrderHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/shop/orders", func(r chi.Router) {
		r.Use(authMw)
		r.Post("/", h.PlaceOrder)
		r.Get("/", h.ListOrders)
		r.Get("/{id}", h.GetOrder)
		r.Put("/{id}/status", h.UpdateStatus)
	})
}

func (h *OrderHandler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := userIDFromContext(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	var req service.PlaceOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	order, err := h.svc.PlaceOrder(r.Context(), tenantID, userID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (h *OrderHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := userIDFromContext(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 10
	}

	orders, total, err := h.svc.GetOrders(r.Context(), tenantID, userID, page, size)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewPageResponse(orders, page, size, total))
}

func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid order id")
		return
	}

	order, err := h.svc.GetOrder(r.Context(), id)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (h *OrderHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid order id")
		return
	}

	var req service.UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	order, err := h.svc.UpdateOrderStatus(r.Context(), id, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}
