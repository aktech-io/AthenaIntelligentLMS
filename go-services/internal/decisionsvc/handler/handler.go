// Package handler exposes the decision-service read API (v1 cut line §6):
// GET /api/v1/decisions — tenant-scoped, filterable, paginated. Policy CRUD,
// referral queues and the regulator export land in increment 4.
package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/decisionsvc/model"
	"github.com/athena-lms/go-services/internal/decisionsvc/repository"
)

// Handler handles HTTP requests for the decision service.
type Handler struct {
	repo   *repository.Repository
	logger *zap.Logger
}

// New creates a Handler.
func New(repo *repository.Repository, logger *zap.Logger) *Handler {
	return &Handler{repo: repo, logger: logger}
}

// RegisterRoutes registers decision routes on the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/decisions", func(r chi.Router) {
		r.Get("/", h.ListDecisions)
	})
}

// ListDecisions handles GET /api/v1/decisions.
// Query: decisionType, subjectId, customerId, outcome, variant,
// from, to (RFC3339), page, size.
func (h *Handler) ListDecisions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := model.ListFilter{
		// Tenant scope always comes from the caller's identity, never from a
		// query parameter — the regulator query wall starts here.
		TenantID:     auth.TenantIDOrDefault(r.Context()),
		DecisionType: q.Get("decisionType"),
		SubjectID:    q.Get("subjectId"),
		CustomerID:   q.Get("customerId"),
		Outcome:      q.Get("outcome"),
		Variant:      q.Get("variant"),
		Page:         intParam(q.Get("page"), 0),
		Size:         intParam(q.Get("size"), 20),
	}
	if f.Size < 1 || f.Size > 200 {
		f.Size = 20
	}
	if f.Page < 0 {
		f.Page = 0
	}
	for name, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if v := q.Get(name); v != "" {
			ts, err := time.Parse(time.RFC3339, v)
			if err != nil {
				httputil.WriteBadRequest(w, "Invalid "+name+" timestamp (want RFC3339): "+v, r.URL.Path)
				return
			}
			*dst = &ts
		}
	}

	decisions, total, err := h.repo.ListDecisions(r.Context(), f)
	if err != nil {
		h.logger.Error("Failed to list decisions", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list decisions", r.URL.Path)
		return
	}
	if decisions == nil {
		decisions = []model.Decision{}
	}
	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(decisions, f.Page, f.Size, total))
}

func intParam(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}
