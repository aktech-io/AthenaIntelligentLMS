// Package handler exposes the card-issuing HTTP API (Nemo B1).
//
// Staff API (JWT via the LMS gateway, /lms prefix stripped upstream):
//
//	POST /api/v1/cards               issue a card             (ADMIN|MANAGER|OFFICER)
//	GET  /api/v1/cards?customerId=   list tenant cards        (ADMIN|MANAGER|OFFICER)
//	GET  /api/v1/cards/{id}          card detail              (ADMIN|MANAGER|OFFICER)
//	GET  /api/v1/cards/{id}/events   audit trail              (ADMIN|MANAGER|OFFICER)
//	POST /api/v1/cards/{id}/freeze   reversible hold          (ADMIN|MANAGER|OFFICER)
//	POST /api/v1/cards/{id}/unfreeze lift hold                (ADMIN|MANAGER|OFFICER)
//	POST /api/v1/cards/{id}/block    terminal block           (ADMIN|MANAGER)
//	PUT  /api/v1/cards/{id}/limits   spending controls        (ADMIN|MANAGER)
//
// The mobile BFF proxies the same resource shapes with X-Service-Key
// (SERVICE role passes RequireRole) and customer-scoped filtering; BFF
// endpoints land with the app card screens.
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/card/model"
	"github.com/athena-lms/go-services/internal/card/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
)

// Handler contains the HTTP handlers for the card service.
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New creates a Handler.
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes registers all card routes (chain after auth middleware).
func (h *Handler) RegisterRoutes(r chi.Router) {
	staff := auth.RequireRole("ADMIN", "MANAGER", "OFFICER")
	senior := auth.RequireRole("ADMIN", "MANAGER")

	r.Route("/api/v1/cards", func(r chi.Router) {
		r.With(staff).Post("/", h.IssueCard)
		r.With(staff).Get("/", h.ListCards)
		r.With(staff).Get("/{id}", h.GetCard)
		r.With(staff).Get("/{id}/events", h.ListCardEvents)
		r.With(staff).Post("/{id}/freeze", h.FreezeCard)
		r.With(staff).Post("/{id}/unfreeze", h.UnfreezeCard)
		r.With(senior).Post("/{id}/block", h.BlockCard)
		r.With(senior).Put("/{id}/limits", h.SetLimits)
	})
}

// IssueCard handles POST /api/v1/cards.
func (h *Handler) IssueCard(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	var req model.IssueCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	card, err := h.svc.IssueCard(r.Context(), &req, tenantID)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, card)
}

// ListCards handles GET /api/v1/cards?customerId=.
func (h *Handler) ListCards(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	var customerID *uuid.UUID
	if raw := r.URL.Query().Get("customerId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httputil.WriteBadRequest(w, "Invalid customerId", r.URL.Path)
			return
		}
		customerID = &id
	}

	cards, err := h.svc.ListCards(r.Context(), tenantID, customerID)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}
	if cards == nil {
		cards = []model.Card{}
	}
	httputil.WriteJSON(w, http.StatusOK, cards)
}

// GetCard handles GET /api/v1/cards/{id}.
func (h *Handler) GetCard(w http.ResponseWriter, r *http.Request) {
	h.withCard(w, r, func(tenantID string, id uuid.UUID) (any, error) {
		return h.svc.GetCard(r.Context(), tenantID, id)
	})
}

// ListCardEvents handles GET /api/v1/cards/{id}/events.
func (h *Handler) ListCardEvents(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 || size > 200 {
		size = 50
	}
	if page < 0 {
		page = 0
	}
	h.withCard(w, r, func(tenantID string, id uuid.UUID) (any, error) {
		events, err := h.svc.ListCardEvents(r.Context(), tenantID, id, size, page*size)
		if events == nil && err == nil {
			events = []model.CardEvent{}
		}
		return events, err
	})
}

// FreezeCard handles POST /api/v1/cards/{id}/freeze.
func (h *Handler) FreezeCard(w http.ResponseWriter, r *http.Request) {
	h.withCard(w, r, func(tenantID string, id uuid.UUID) (any, error) {
		return h.svc.FreezeCard(r.Context(), tenantID, id)
	})
}

// UnfreezeCard handles POST /api/v1/cards/{id}/unfreeze.
func (h *Handler) UnfreezeCard(w http.ResponseWriter, r *http.Request) {
	h.withCard(w, r, func(tenantID string, id uuid.UUID) (any, error) {
		return h.svc.UnfreezeCard(r.Context(), tenantID, id)
	})
}

// BlockCard handles POST /api/v1/cards/{id}/block.
func (h *Handler) BlockCard(w http.ResponseWriter, r *http.Request) {
	var req model.BlockCardRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
			return
		}
	}
	h.withCard(w, r, func(tenantID string, id uuid.UUID) (any, error) {
		return h.svc.BlockCard(r.Context(), tenantID, id, req.Reason)
	})
}

// SetLimits handles PUT /api/v1/cards/{id}/limits.
func (h *Handler) SetLimits(w http.ResponseWriter, r *http.Request) {
	var req model.SetLimitsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}
	h.withCard(w, r, func(tenantID string, id uuid.UUID) (any, error) {
		return h.svc.SetLimits(r.Context(), tenantID, id, req.Limits)
	})
}

// withCard parses {id}, runs fn tenant-scoped, and writes the JSON result.
func (h *Handler) withCard(w http.ResponseWriter, r *http.Request, fn func(tenantID string, id uuid.UUID) (any, error)) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid card ID", r.URL.Path)
		return
	}
	resp, err := fn(tenantID, id)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}
