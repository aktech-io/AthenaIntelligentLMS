package handler

import (
	"encoding/json"
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
	"github.com/athena-lms/go-services/internal/overdraft/model"
	"github.com/athena-lms/go-services/internal/overdraft/service"
)

// Handler handles HTTP requests for the overdraft service.
type Handler struct {
	walletSvc *service.WalletService
	auditSvc  *service.AuditService
	eodSvc    *service.EODService
	logger    *zap.Logger
}

// New creates a new Handler.
func New(walletSvc *service.WalletService, auditSvc *service.AuditService, eodSvc *service.EODService, logger *zap.Logger) *Handler {
	return &Handler{walletSvc: walletSvc, auditSvc: auditSvc, eodSvc: eodSvc, logger: logger}
}

// RegisterRoutes registers all overdraft routes on the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Wallet routes — support both /wallet (singular) and /wallets (plural)
	walletRoutes := func(r chi.Router) {
		r.Post("/", h.CreateWallet)
		r.Get("/", h.ListWallets)
		r.Get("/{id}", h.GetWallet)
		r.Get("/customer/{customerId}", h.GetWalletByCustomer)
		r.Post("/{id}/deposit", h.Deposit)
		r.Post("/{id}/withdraw", h.Withdraw)
		r.Get("/{id}/transactions", h.GetTransactions)
		r.Post("/{id}/overdraft/apply", h.ApplyOverdraft)
		r.Get("/{id}/overdraft", h.GetOverdraftFacility)
		r.Post("/{id}/overdraft/suspend", h.SuspendOverdraft)
	}
	r.Route("/api/v1/wallet", walletRoutes)
	r.Route("/api/v1/wallets", walletRoutes)

	r.Route("/api/v1/overdraft", func(r chi.Router) {
		r.Get("/audit", h.GetAuditLog)
		r.Get("/audit/verify", h.VerifyAuditChain)
		r.Get("/summary", h.GetOverdraftSummary)
		r.Post("/eod/run", h.RunEOD)
		r.Get("/eod/status", h.GetEODStatus)
		r.Get("/{walletId}/interest-charges", h.GetInterestCharges)
		r.Get("/{walletId}/billing-statements", h.GetBillingStatements)
	})
}

// ApplyOverdraft handles POST /api/v1/wallets/{id}/overdraft/apply
func (h *Handler) ApplyOverdraft(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid wallet ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.walletSvc.ApplyOverdraft(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// GetOverdraftFacility handles GET /api/v1/wallets/{id}/overdraft
func (h *Handler) GetOverdraftFacility(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid wallet ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.walletSvc.GetOverdraftFacility(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// SuspendOverdraft handles POST /api/v1/wallets/{id}/overdraft/suspend
func (h *Handler) SuspendOverdraft(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid wallet ID", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.walletSvc.SuspendOverdraft(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetOverdraftSummary handles GET /api/v1/overdraft/summary
func (h *Handler) GetOverdraftSummary(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.walletSvc.GetSummary(r.Context(), tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// CreateWallet handles POST /api/v1/wallet
func (h *Handler) CreateWallet(w http.ResponseWriter, r *http.Request) {
	var req model.CreateWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.walletSvc.CreateWallet(r.Context(), req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// ListWallets handles GET /api/v1/wallet
func (h *Handler) ListWallets(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.walletSvc.ListWallets(r.Context(), tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetWallet handles GET /api/v1/wallet/{id}
func (h *Handler) GetWallet(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid wallet ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.walletSvc.GetWallet(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetWalletByCustomer handles GET /api/v1/wallet/customer/{customerId}
func (h *Handler) GetWalletByCustomer(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customerId")
	tenantID := auth.TenantIDOrDefault(r.Context())

	resp, err := h.walletSvc.GetWalletByCustomer(r.Context(), customerID, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// Deposit handles POST /api/v1/wallet/{id}/deposit
func (h *Handler) Deposit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid wallet ID", r.URL.Path)
		return
	}

	var req model.WalletTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.walletSvc.Deposit(r.Context(), id, req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// Withdraw handles POST /api/v1/wallet/{id}/withdraw
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid wallet ID", r.URL.Path)
		return
	}

	var req model.WalletTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.walletSvc.Withdraw(r.Context(), id, req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetTransactions handles GET /api/v1/wallet/{id}/transactions
func (h *Handler) GetTransactions(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid wallet ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	resp, err := h.walletSvc.GetTransactions(r.Context(), id, tenantID, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetAuditLog handles GET /api/v1/overdraft/audit
func (h *Handler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	var entityType *string
	if et := r.URL.Query().Get("entityType"); et != "" {
		entityType = &et
	}

	var entityID *uuid.UUID
	if eid := r.URL.Query().Get("entityId"); eid != "" {
		parsed, err := uuid.Parse(eid)
		if err == nil {
			entityID = &parsed
		}
	}

	resp, err := h.auditSvc.GetAuditLog(r.Context(), tenantID, entityType, entityID, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// VerifyAuditChain handles GET /api/v1/overdraft/audit/verify — reports whether
// the tamper-evident audit trail is intact, or the seq of the first altered or
// missing entry, so an auditor can prove the log was not edited or back-dated.
func (h *Handler) VerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	result, err := h.auditSvc.VerifyAuditChain(r.Context(), tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

// RunEOD handles POST /api/v1/overdraft/eod/run — triggers EOD batch processing.
func (h *Handler) RunEOD(w http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("date")
	runDate := time.Now().UTC()
	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			httputil.WriteBadRequest(w, "Invalid date format, use YYYY-MM-DD", r.URL.Path)
			return
		}
		runDate = parsed
	}

	result, err := h.eodSvc.RunEOD(r.Context(), runDate)
	if err != nil {
		if strings.Contains(err.Error(), "already running") {
			httputil.WriteConflict(w, err.Error(), r.URL.Path)
			return
		}
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

// GetEODStatus handles GET /api/v1/overdraft/eod/status
func (h *Handler) GetEODStatus(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, h.eodSvc.LastRunStatus())
}

// GetInterestCharges handles GET /api/v1/overdraft/{walletId}/interest-charges
func (h *Handler) GetInterestCharges(w http.ResponseWriter, r *http.Request) {
	walletID, err := uuid.Parse(chi.URLParam(r, "walletId"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid wallet ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.walletSvc.GetInterestCharges(r.Context(), walletID, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetBillingStatements handles GET /api/v1/overdraft/{walletId}/billing-statements
func (h *Handler) GetBillingStatements(w http.ResponseWriter, r *http.Request) {
	walletID, err := uuid.Parse(chi.URLParam(r, "walletId"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid wallet ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	resp, err := h.walletSvc.GetBillingStatements(r.Context(), walletID, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// ---- helpers ----

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch e := err.(type) {
	case *cerrors.NotFoundError:
		httputil.WriteNotFound(w, e.Message, r.URL.Path)
	case *cerrors.BusinessError:
		if e.StatusCode == http.StatusBadRequest {
			httputil.WriteBadRequest(w, e.Message, r.URL.Path)
		} else if e.StatusCode == http.StatusConflict {
			httputil.WriteConflict(w, e.Message, r.URL.Path)
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
