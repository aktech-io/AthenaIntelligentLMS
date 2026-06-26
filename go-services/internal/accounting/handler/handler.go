package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/accounting/model"
	"github.com/athena-lms/go-services/internal/accounting/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
)

// Handler provides HTTP handlers for the accounting API.
type Handler struct {
	svc    *service.AccountingService
	logger *zap.Logger
}

// New creates a new accounting handler.
func New(svc *service.AccountingService, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes mounts all accounting routes on the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/accounting", func(r chi.Router) {
		// Chart of Accounts
		r.Post("/accounts", h.createAccount)
		r.Get("/accounts", h.listAccounts)
		r.Get("/accounts/{id}", h.getAccount)
		r.Get("/accounts/code/{code}", h.getAccountByCode)
		r.Get("/accounts/{id}/balance", h.getBalance)
		r.Get("/accounts/{id}/ledger", h.getLedger)

		// Journal Entries
		r.Post("/journal-entries", h.postEntry)
		r.Get("/journal-entries", h.listEntries)
		r.Get("/journal-entries/{id}", h.getEntry)
		r.Post("/journal-entries/{id}/submit", h.submitEntry)
		r.Post("/journal-entries/{id}/approve", h.approveEntry)
		r.Post("/journal-entries/{id}/reject", h.rejectEntry)
		r.Post("/journal-entries/{id}/reverse", h.reverseEntry)

		// Trial Balance
		r.Get("/trial-balance", h.getTrialBalance)

		// Fiscal Periods
		r.Get("/periods", h.listPeriods)
		r.Post("/periods/{year}/{month}/close", h.closePeriod)
		r.Post("/periods/{year}/{month}/reopen", h.reopenPeriod)

		// Audit Log
		r.Get("/audit-log", h.listAuditLogs)
		r.Get("/audit-log/verify", h.verifyAuditChain)
		r.Get("/audit-log/{entityType}/{entityId}", h.getEntityAuditTrail)

		// Cash Flow
		r.Get("/cash-flow", h.getCashFlow)
	})
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req model.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	if req.Code == "" || req.Name == "" {
		httputil.WriteBadRequest(w, "code and name are required", r.URL.Path)
		return
	}
	if !model.ValidAccountTypes[req.AccountType] {
		httputil.WriteBadRequest(w, "invalid accountType", r.URL.Path)
		return
	}
	if !model.ValidBalanceTypes[req.BalanceType] {
		httputil.WriteBadRequest(w, "invalid balanceType", r.URL.Path)
		return
	}

	tenantID := tenantFromRequest(r)
	resp, err := h.svc.CreateAccount(r.Context(), req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromRequest(r)
	var accountType *model.AccountType
	if t := r.URL.Query().Get("type"); t != "" {
		at := model.AccountType(t)
		if !model.ValidAccountTypes[at] {
			httputil.WriteBadRequest(w, "invalid account type: "+t, r.URL.Path)
			return
		}
		accountType = &at
	}

	resp, err := h.svc.ListAccounts(r.Context(), tenantID, accountType)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid UUID", r.URL.Path)
		return
	}
	tenantID := tenantFromRequest(r)
	resp, err := h.svc.GetAccount(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getAccountByCode(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	tenantID := tenantFromRequest(r)
	resp, err := h.svc.GetAccountByCode(r.Context(), code, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) postEntry(w http.ResponseWriter, r *http.Request) {
	var req model.PostJournalEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	if req.Reference == "" {
		httputil.WriteBadRequest(w, "reference is required", r.URL.Path)
		return
	}
	if len(req.Lines) < 2 {
		httputil.WriteBadRequest(w, "at least 2 lines required", r.URL.Path)
		return
	}

	tenantID := tenantFromRequest(r)
	userID := auth.UserIDFromContext(r.Context())

	resp, err := h.svc.PostEntry(r.Context(), req, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) listEntries(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromRequest(r)
	page := queryInt(r, "page", 0)
	size := queryInt(r, "size", 20)

	var from, to *time.Time
	if f := r.URL.Query().Get("from"); f != "" {
		t, err := time.Parse("2006-01-02", f)
		if err == nil {
			from = &t
		}
	}
	if t := r.URL.Query().Get("to"); t != "" {
		parsed, err := time.Parse("2006-01-02", t)
		if err == nil {
			to = &parsed
		}
	}

	entries, total, err := h.svc.ListEntries(r.Context(), tenantID, from, to, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(entries, page, size, total))
}

func (h *Handler) getEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid UUID", r.URL.Path)
		return
	}
	tenantID := tenantFromRequest(r)
	resp, err := h.svc.GetEntry(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getBalance(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid UUID", r.URL.Path)
		return
	}
	tenantID := tenantFromRequest(r)
	now := time.Now()
	year := queryInt(r, "year", now.Year())
	month := queryInt(r, "month", int(now.Month()))

	resp, err := h.svc.GetBalance(r.Context(), id, tenantID, year, month)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getLedger(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid UUID", r.URL.Path)
		return
	}
	tenantID := tenantFromRequest(r)
	resp, err := h.svc.GetLedger(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getTrialBalance(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromRequest(r)
	now := time.Now()
	year := queryInt(r, "year", now.Year())
	month := queryInt(r, "month", int(now.Month()))

	resp, err := h.svc.GetTrialBalance(r.Context(), tenantID, year, month)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// --- helpers ---

func tenantFromRequest(r *http.Request) string {
	return auth.TenantIDOrDefault(r.Context())
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}

func (h *Handler) submitEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid UUID", r.URL.Path)
		return
	}
	tenantID := tenantFromRequest(r)
	resp, err := h.svc.SubmitForApproval(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) approveEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid UUID", r.URL.Path)
		return
	}
	tenantID := tenantFromRequest(r)
	userID := auth.UserIDFromContext(r.Context())
	resp, err := h.svc.ApproveEntry(r.Context(), id, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) rejectEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid UUID", r.URL.Path)
		return
	}
	var req model.RejectEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body", r.URL.Path)
		return
	}
	if req.Reason == "" {
		httputil.WriteBadRequest(w, "reason is required", r.URL.Path)
		return
	}
	tenantID := tenantFromRequest(r)
	resp, err := h.svc.RejectEntry(r.Context(), id, tenantID, req.Reason)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) reverseEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid UUID", r.URL.Path)
		return
	}
	var req model.ReverseEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body", r.URL.Path)
		return
	}
	if req.Reason == "" {
		httputil.WriteBadRequest(w, "reason is required", r.URL.Path)
		return
	}
	tenantID := tenantFromRequest(r)
	userID := auth.UserIDFromContext(r.Context())
	resp, err := h.svc.ReverseEntry(r.Context(), id, tenantID, userID, req.Reason)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) listPeriods(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromRequest(r)
	periods, err := h.svc.ListPeriods(r.Context(), tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	if periods == nil {
		periods = []model.FiscalPeriod{}
	}
	httputil.WriteJSON(w, http.StatusOK, periods)
}

func (h *Handler) closePeriod(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(chi.URLParam(r, "year"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid year", r.URL.Path)
		return
	}
	month, err := strconv.Atoi(chi.URLParam(r, "month"))
	if err != nil || month < 1 || month > 12 {
		httputil.WriteBadRequest(w, "invalid month", r.URL.Path)
		return
	}
	tenantID := tenantFromRequest(r)
	userID := auth.UserIDFromContext(r.Context())
	period, err := h.svc.ClosePeriod(r.Context(), tenantID, year, month, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, period)
}

func (h *Handler) reopenPeriod(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(chi.URLParam(r, "year"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid year", r.URL.Path)
		return
	}
	month, err := strconv.Atoi(chi.URLParam(r, "month"))
	if err != nil || month < 1 || month > 12 {
		httputil.WriteBadRequest(w, "invalid month", r.URL.Path)
		return
	}
	var req model.ReopenPeriodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body", r.URL.Path)
		return
	}
	if req.Reason == "" {
		httputil.WriteBadRequest(w, "reason is required", r.URL.Path)
		return
	}
	tenantID := tenantFromRequest(r)
	userID := auth.UserIDFromContext(r.Context())
	period, err := h.svc.ReopenPeriod(r.Context(), tenantID, year, month, userID, req.Reason)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, period)
}

func (h *Handler) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromRequest(r)
	page := queryInt(r, "page", 0)
	size := queryInt(r, "size", 20)

	var entityType, userID *string
	if et := r.URL.Query().Get("entityType"); et != "" {
		entityType = &et
	}
	if uid := r.URL.Query().Get("userId"); uid != "" {
		userID = &uid
	}

	var from, to *time.Time
	if f := r.URL.Query().Get("from"); f != "" {
		t, err := time.Parse("2006-01-02", f)
		if err == nil {
			from = &t
		}
	}
	if t := r.URL.Query().Get("to"); t != "" {
		parsed, err := time.Parse("2006-01-02", t)
		if err == nil {
			to = &parsed
		}
	}

	logs, total, err := h.svc.ListAuditLogs(r.Context(), tenantID, entityType, userID, from, to, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	if logs == nil {
		logs = []model.FinancialAuditLog{}
	}
	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(logs, page, size, total))
}

func (h *Handler) getEntityAuditTrail(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityId")
	tenantID := tenantFromRequest(r)
	logs, err := h.svc.GetEntityAuditTrail(r.Context(), tenantID, entityType, entityID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	if logs == nil {
		logs = []model.FinancialAuditLog{}
	}
	httputil.WriteJSON(w, http.StatusOK, logs)
}

// verifyAuditChain reports whether the tamper-evident financial_audit_log chain
// is intact, or the seq of the first altered/missing entry. Lets an auditor
// prove the log was not edited or back-dated.
func (h *Handler) verifyAuditChain(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromRequest(r)
	result, err := h.svc.VerifyAuditChain(r.Context(), tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) getCashFlow(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromRequest(r)
	now := time.Now()
	year := queryInt(r, "year", now.Year())
	month := queryInt(r, "month", int(now.Month()))
	resp, err := h.svc.GetCashFlow(r.Context(), tenantID, year, month)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch e := err.(type) {
	case *errors.BusinessError:
		if e.StatusCode == http.StatusForbidden {
			httputil.WriteForbidden(w, e.Message, r.URL.Path)
		} else {
			httputil.WriteErrorJSON(w, e.StatusCode, "Unprocessable Entity", e.Message, r.URL.Path)
		}
	case *errors.NotFoundError:
		httputil.WriteNotFound(w, e.Message, r.URL.Path)
	default:
		h.logger.Error("Internal error", zap.Error(err), zap.String("path", r.URL.Path))
		httputil.WriteInternalError(w, "internal server error", r.URL.Path)
	}
}
