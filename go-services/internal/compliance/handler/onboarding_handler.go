package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	commonerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/compliance/model"
	"github.com/athena-lms/go-services/internal/compliance/service"
)

// OnboardingHandler exposes the A2 self-service onboarding API. Submissions
// arrive via the customer channel (BFF calls with the service key); the
// queue/decision endpoints are officer-facing.
type OnboardingHandler struct {
	svc    *service.OnboardingService
	logger *zap.Logger
}

// handleError maps domain errors to HTTP responses (same mapping as the
// main compliance handler).
func (h *OnboardingHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch e := err.(type) {
	case *commonerrors.NotFoundError:
		httputil.WriteNotFound(w, e.Message, r.URL.Path)
	case *commonerrors.BusinessError:
		httputil.WriteErrorJSON(w, e.StatusCode, http.StatusText(e.StatusCode), e.Message, r.URL.Path)
	default:
		h.logger.Error("Internal error", zap.Error(err), zap.String("path", r.URL.Path))
		httputil.WriteInternalError(w, "internal server error", r.URL.Path)
	}
}

// NewOnboarding creates an OnboardingHandler.
func NewOnboarding(svc *service.OnboardingService, logger *zap.Logger) *OnboardingHandler {
	return &OnboardingHandler{svc: svc, logger: logger}
}

// RegisterRoutes mounts onboarding routes. Officer decisions reuse the
// compliance.decide gate that protects KYC pass/fail.
func (h *OnboardingHandler) RegisterRoutes(r chi.Router) {
	decide := auth.RequirePermission("compliance.decide", "ADMIN", "MANAGER")
	r.Route("/api/v1/onboarding", func(r chi.Router) {
		r.Post("/", h.Submit)
		r.Get("/", h.List)
		r.Get("/{id}", h.Get)
		r.With(decide).Post("/{id}/approve", h.decideFn(true))
		r.With(decide).Post("/{id}/reject", h.decideFn(false))
	})
}

func (h *OnboardingHandler) Submit(w http.ResponseWriter, r *http.Request) {
	var req model.SubmitOnboardingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	app, err := h.svc.Submit(r.Context(), req, resolveTenantID(r))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, app)
}

func (h *OnboardingHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid application id", r.URL.Path)
		return
	}
	app, err := h.svc.Get(r.Context(), id, resolveTenantID(r))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, app)
}

func (h *OnboardingHandler) List(w http.ResponseWriter, r *http.Request) {
	var status *model.OnboardingStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := model.OnboardingStatus(s)
		status = &st
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, err := strconv.Atoi(r.URL.Query().Get("size"))
	if err != nil || size <= 0 || size > 200 {
		size = 20
	}
	resp, err := h.svc.List(r.Context(), resolveTenantID(r), status, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *OnboardingHandler) decideFn(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httputil.WriteBadRequest(w, "Invalid application id", r.URL.Path)
			return
		}
		var req model.OnboardingDecisionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
			return
		}
		officer := auth.UserIDFromContext(r.Context())
		app, err := h.svc.Decide(r.Context(), id, approve, req, resolveTenantID(r), officer)
		if err != nil {
			h.handleError(w, r, err)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, app)
	}
}
