package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/athena-lms/go-services/internal/bff/gateway/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type ContactHandler struct {
	svc *service.ContactService
}

func NewContactHandler(svc *service.ContactService) *ContactHandler {
	return &ContactHandler{svc: svc}
}

func (h *ContactHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/mobile/contacts", func(r chi.Router) {
		r.Use(authMw)
		r.Get("/recent", h.GetRecentContacts)
		r.Post("/", h.CreateContact)
		r.Get("/search", h.SearchContacts)
	})
}

func (h *ContactHandler) GetRecentContacts(w http.ResponseWriter, r *http.Request) {
	userID, err := resolveUserID(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid user")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	contacts, total, err := h.svc.GetRecentContacts(r.Context(), tenantID, userID, page, size)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, dto.NewPageResponse(contacts, page, size, total))
}

func (h *ContactHandler) CreateContact(w http.ResponseWriter, r *http.Request) {
	userID, err := resolveUserID(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid user")
		return
	}

	var req service.CreateContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.CreateContact(r.Context(), tenantID, userID, req)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *ContactHandler) SearchContacts(w http.ResponseWriter, r *http.Request) {
	userID, err := resolveUserID(r)
	if err != nil {
		apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid user")
		return
	}

	query := r.URL.Query().Get("q")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	contacts, total, err := h.svc.SearchContacts(r.Context(), tenantID, userID, query, page, size)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, dto.NewPageResponse(contacts, page, size, total))
}
