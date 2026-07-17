package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/shop/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type FavoriteHandler struct {
	svc *service.FavoriteService
}

func NewFavoriteHandler(svc *service.FavoriteService) *FavoriteHandler {
	return &FavoriteHandler{svc: svc}
}

func (h *FavoriteHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/shop/favorites", func(r chi.Router) {
		r.Use(authMw)
		r.Get("/", h.ListFavorites)
		r.Post("/{productId}/toggle", h.ToggleFavorite)
	})
}

func (h *FavoriteHandler) ListFavorites(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID, err := userIDFromContext(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid user identity")
		return
	}

	result, err := h.svc.ListFavorites(r.Context(), tenantID, userID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *FavoriteHandler) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
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

	result, err := h.svc.ToggleFavorite(r.Context(), tenantID, userID, productID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
