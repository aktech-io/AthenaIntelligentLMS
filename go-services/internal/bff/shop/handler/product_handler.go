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

type ProductHandler struct {
	svc *service.ProductService
}

func NewProductHandler(svc *service.ProductService) *ProductHandler {
	return &ProductHandler{svc: svc}
}

func (h *ProductHandler) Routes(r chi.Router) {
	r.Route("/api/v1/shop/products", func(r chi.Router) {
		r.Get("/categories", h.ListCategories)
		r.Get("/featured", h.ListFeatured)
		r.Get("/", h.SearchProducts)
		r.Get("/{id}", h.GetProduct)
	})
}

func (h *ProductHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	cats, err := h.svc.ListCategories(r.Context(), tenantID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cats)
}

func (h *ProductHandler) ListFeatured(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	products, err := h.svc.ListFeatured(r.Context(), tenantID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, products)
}

func (h *ProductHandler) SearchProducts(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	var categoryID *uuid.UUID
	if catStr := r.URL.Query().Get("categoryId"); catStr != "" {
		if id, err := uuid.Parse(catStr); err == nil {
			categoryID = &id
		}
	}

	query := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	products, total, err := h.svc.SearchProducts(r.Context(), tenantID, categoryID, query, sort, page, size)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewPageResponse(products, page, size, total))
}

func (h *ProductHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid product id")
		return
	}
	product, err := h.svc.GetProduct(r.Context(), id)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
