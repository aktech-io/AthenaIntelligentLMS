package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/shop/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type CartHandler struct {
	svc *service.CartService
}

func NewCartHandler(svc *service.CartService) *CartHandler {
	return &CartHandler{svc: svc}
}

func (h *CartHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/shop/cart", func(r chi.Router) {
		r.Use(authMw)
		r.Get("/", h.GetCart)
		r.Post("/", h.AddToCart)
		r.Put("/{productId}", h.UpdateCartItem)
		r.Delete("/{productId}", h.RemoveFromCart)
		r.Delete("/", h.ClearCart)
	})
}

func (h *CartHandler) GetCart(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := userIDFromContext(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}
	cart, err := h.svc.GetCart(r.Context(), tenantID, userID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cart)
}

type addToCartRequest struct {
	ProductID  uuid.UUID  `json:"productId"`
	Quantity   int        `json:"quantity"`
	BNPLPlanID *uuid.UUID `json:"bnplPlanId"`
}

func (h *CartHandler) AddToCart(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := userIDFromContext(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	var req addToCartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProductID == uuid.Nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "productId is required")
		return
	}
	if req.Quantity <= 0 {
		req.Quantity = 1
	}

	cart, err := h.svc.AddToCart(r.Context(), tenantID, userID, req.ProductID, req.Quantity, req.BNPLPlanID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cart)
}

func (h *CartHandler) UpdateCartItem(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := userIDFromContext(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	productID, err := uuid.Parse(chi.URLParam(r, "productId"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid productId")
		return
	}

	quantity, err := strconv.Atoi(r.URL.Query().Get("quantity"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "quantity parameter is required")
		return
	}

	cart, err := h.svc.UpdateCartItem(r.Context(), tenantID, userID, productID, quantity)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cart)
}

func (h *CartHandler) RemoveFromCart(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := userIDFromContext(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	productID, err := uuid.Parse(chi.URLParam(r, "productId"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid productId")
		return
	}

	cart, err := h.svc.RemoveFromCart(r.Context(), tenantID, userID, productID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cart)
}

func (h *CartHandler) ClearCart(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := userIDFromContext(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	if err := h.svc.ClearCart(r.Context(), tenantID, userID); err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func userIDFromContext(r *http.Request) (uuid.UUID, error) {
	userID := auth.MobileUserIDFromContext(r.Context())
	if userID != "" {
		return uuid.Parse(userID)
	}
	// Fall back to customerID or username.
	customerID := auth.CustomerIDStrFromContext(r.Context())
	if customerID != "" {
		if id, err := uuid.Parse(customerID); err == nil {
			return id, nil
		}
	}
	return uuid.Parse(auth.UserIDFromContext(r.Context()))
}
