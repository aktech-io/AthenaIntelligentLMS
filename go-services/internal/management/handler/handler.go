package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	cerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/management/crb"
	"github.com/athena-lms/go-services/internal/management/model"
	"github.com/athena-lms/go-services/internal/management/service"
)

// Handler handles HTTP requests for loan management.
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New creates a new Handler.
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes registers all loan management routes on the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/loans", func(r chi.Router) {
		r.Get("/", h.List)
		r.Get("/{id}", h.GetByID)
		r.Get("/{id}/schedule", h.GetSchedule)
		r.Get("/{id}/schedule/{installmentNo}", h.GetInstallment)
		r.Get("/{id}/repayments", h.GetRepayments)
		r.Get("/{id}/dpd", h.GetDpd)
		r.Get("/customer/{customerId}", h.ListByCustomer)
		r.Get("/portfolio-stats", h.GetPortfolioStats)
		r.Get("/par-report", h.GetPARReport)
		r.Get("/ecl-provision", h.GetECLProvisionReport)
		r.With(auth.RequireRole("ADMIN", "MANAGER")).Get("/crb-feed", h.GetCRBFeed)
		r.Post("/{id}/restructure", h.Restructure)
	})
	r.Route("/api/v1/repayments", func(r chi.Router) {
		r.Post("/", h.ApplyRepayment)
	})
	r.Get("/api/v1/audit-log", h.ListAuditLog)
	r.Get("/api/v1/audit-log/verify", h.VerifyAuditChain)
}

// GetPortfolioStats handles GET /api/v1/loans/portfolio-stats — live loan-book totals.
func (h *Handler) GetPortfolioStats(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	stats, err := h.svc.GetPortfolioStats(r.Context(), tenantID)
	if err != nil {
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "INTERNAL", "Failed to compute portfolio stats", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, stats)
}

// GetPARReport handles GET /api/v1/loans/par-report — Portfolio-at-Risk / ageing
// analysis of the active loan book (delinquency buckets + PAR ratios).
func (h *Handler) GetPARReport(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	report, err := h.svc.GetPARReport(r.Context(), tenantID)
	if err != nil {
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "INTERNAL", "Failed to compute PAR report", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, report)
}

// GetECLProvisionReport handles GET /api/v1/loans/ecl-provision — simplified
// IFRS 9 stage-based loan-loss provisioning (Expected Credit Loss) over the
// active loan book. READ-ONLY: no GL posting.
func (h *Handler) GetECLProvisionReport(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	report, err := h.svc.GetECLProvisionReport(r.Context(), tenantID)
	if err != nil {
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "INTERNAL", "Failed to compute ECL provision report", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, report)
}

// ListAuditLog handles GET /api/v1/audit-log for the loans domain.
func (h *Handler) ListAuditLog(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	entityType := r.URL.Query().Get("entityType")
	entityID := r.URL.Query().Get("entityId")
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("size")); err == nil && v > 0 {
		limit = v
	}
	offset := 0
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		offset = v * limit
	}
	records, err := h.svc.ListAuditLog(r.Context(), tenantID, entityType, entityID, limit, offset)
	if err != nil {
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "INTERNAL", "Failed to fetch audit log", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, records)
}

// VerifyAuditChain handles GET /api/v1/audit-log/verify for the loans domain —
// confirms the tamper-evident audit chain is intact or reports the first
// tampered/missing seq.
func (h *Handler) VerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	result, err := h.svc.VerifyAuditChain(r.Context(), tenantID)
	if err != nil {
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "INTERNAL", "Failed to verify audit chain", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

// List handles GET /api/v1/loans
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	var status *model.LoanStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := model.LoanStatus(s)
		status = &st
	}

	var customerID *string
	if c := r.URL.Query().Get("customerId"); c != "" {
		customerID = &c
	}

	resp, err := h.svc.List(r.Context(), tenantID, status, customerID, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetByID handles GET /api/v1/loans/{id}
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid loan ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.GetByID(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetSchedule handles GET /api/v1/loans/{id}/schedule
func (h *Handler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid loan ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.GetSchedule(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetInstallment handles GET /api/v1/loans/{id}/schedule/{installmentNo}
func (h *Handler) GetInstallment(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid loan ID", r.URL.Path)
		return
	}

	installmentNo, err := strconv.Atoi(chi.URLParam(r, "installmentNo"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid installment number", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.GetInstallment(r.Context(), id, installmentNo, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetRepayments handles GET /api/v1/loans/{id}/repayments
func (h *Handler) GetRepayments(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid loan ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.GetRepayments(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetDpd handles GET /api/v1/loans/{id}/dpd
func (h *Handler) GetDpd(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid loan ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.svc.GetDpd(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// ListByCustomer handles GET /api/v1/loans/customer/{customerId}
func (h *Handler) ListByCustomer(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customerId")
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.ListByCustomer(r.Context(), customerID, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// ApplyRepayment handles POST /api/v1/repayments
func (h *Handler) ApplyRepayment(w http.ResponseWriter, r *http.Request) {
	var req repaymentHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	loanID, err := uuid.Parse(req.LoanID)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid loanId", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.svc.ApplyRepayment(r.Context(), loanID, &req.RepaymentRequest, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// Restructure handles POST /api/v1/loans/{id}/restructure
func (h *Handler) Restructure(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid loan ID", r.URL.Path)
		return
	}

	var req model.RestructureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.svc.Restructure(r.Context(), id, &req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// ---- helpers ----

// repaymentHTTPRequest wraps RepaymentRequest with a loanId field.
type repaymentHTTPRequest struct {
	LoanID string `json:"loanId"`
	model.RepaymentRequest
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch e := err.(type) {
	case *cerrors.NotFoundError:
		httputil.WriteNotFound(w, e.Message, r.URL.Path)
	case *cerrors.BusinessError:
		if e.StatusCode == http.StatusBadRequest {
			httputil.WriteBadRequest(w, e.Message, r.URL.Path)
		} else {
			httputil.WriteUnprocessable(w, e.Message, r.URL.Path)
		}
	default:
		msg := err.Error()
		if strings.Contains(msg, "not found") {
			httputil.WriteNotFound(w, msg, r.URL.Path)
			return
		}
		h.logger.Error("Internal error", zap.Error(err), zap.String("path", r.URL.Path))
		httputil.WriteInternalError(w, "Internal server error", r.URL.Path)
	}
}

// GetCRBFeed handles GET /api/v1/loans/crb-feed?period=YYYY-MM — the Credit
// Reference Bureau borrower-performance feed for a reporting period. ADMIN/MANAGER
// only (it contains borrower PII). v1 renders the generic, bureau-agnostic CSV;
// the tenant's regulatory profile will select the concrete bureau template in a
// follow-up.
func (h *Handler) GetCRBFeed(w http.ResponseWriter, r *http.Request) {
	periodEnd, label, err := parsePeriodEnd(r.URL.Query().Get("period"))
	if err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	records, err := h.svc.CRBFeedRecords(r.Context(), auth.TenantIDOrDefault(r.Context()), periodEnd)
	if err != nil {
		h.logger.Error("crb feed query failed", zap.Error(err), zap.String("path", r.URL.Path))
		httputil.WriteInternalError(w, "Could not build CRB feed", r.URL.Path)
		return
	}
	data, err := crb.CSVMapper{}.Render(records)
	if err != nil {
		h.logger.Error("crb feed render failed", zap.Error(err), zap.String("path", r.URL.Path))
		httputil.WriteInternalError(w, "Could not render CRB feed", r.URL.Path)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=crb-feed-%s.csv", label))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// parsePeriodEnd parses a YYYY-MM period into its inclusive end instant (end of
// that month). An empty period defaults to the current month. Returns the end
// instant and the YYYY-MM label.
func parsePeriodEnd(period string) (time.Time, string, error) {
	if period == "" {
		period = time.Now().UTC().Format("2006-01")
	}
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid period %q, expected YYYY-MM", period)
	}
	end := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0).Add(-time.Nanosecond)
	return end, period, nil
}
