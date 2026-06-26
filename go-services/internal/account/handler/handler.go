package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/account/model"
	"github.com/athena-lms/go-services/internal/account/repository"
	"github.com/athena-lms/go-services/internal/account/service"
	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
)

// Handler handles HTTP requests for the account service.
type Handler struct {
	accountSvc  *service.AccountService
	customerSvc *service.CustomerService
	transferSvc *service.TransferService
	openingSvc  *service.AccountOpeningService
	interestSvc *service.InterestService
	eodSvc      *service.EODService
	approvalSvc *service.ApprovalService
	repo        *repository.Repository
	logger      *zap.Logger
}

// SetApprovalService sets the maker-checker approval service.
func (h *Handler) SetApprovalService(svc *service.ApprovalService) { h.approvalSvc = svc }

// New creates a new Handler.
func New(accountSvc *service.AccountService, customerSvc *service.CustomerService, transferSvc *service.TransferService, logger *zap.Logger) *Handler {
	return &Handler{accountSvc: accountSvc, customerSvc: customerSvc, transferSvc: transferSvc, logger: logger}
}

// NewWithRepo creates a new Handler with direct repository access (needed for org settings).
func NewWithRepo(accountSvc *service.AccountService, customerSvc *service.CustomerService, transferSvc *service.TransferService, repo *repository.Repository, logger *zap.Logger) *Handler {
	return &Handler{accountSvc: accountSvc, customerSvc: customerSvc, transferSvc: transferSvc, repo: repo, logger: logger}
}

// SetOpeningService sets the account opening service.
func (h *Handler) SetOpeningService(svc *service.AccountOpeningService) { h.openingSvc = svc }

// SetInterestService sets the interest service.
func (h *Handler) SetInterestService(svc *service.InterestService) { h.interestSvc = svc }

// SetEODService sets the EOD service.
func (h *Handler) SetEODService(svc *service.EODService) { h.eodSvc = svc }

// RegisterRoutes registers all account service routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/accounts", func(r chi.Router) {
		r.Post("/", h.CreateAccount)
		r.Post("/open", h.OpenAccount)
		r.Get("/", h.ListAccounts)
		r.Get("/search", h.SearchAccounts)
		r.Get("/customer/{customerId}", h.GetAccountsByCustomer)
		r.Get("/{id}", h.GetAccount)
		r.Get("/{id}/balance", h.GetBalance)
		r.Post("/{id}/credit", h.Credit)
		r.Post("/{id}/debit", h.Debit)
		r.Get("/{id}/transactions", h.GetTransactions)
		r.Get("/{id}/mini-statement", h.GetMiniStatement)
		r.Get("/{id}/statement", h.GetStatement)
		r.Put("/{id}/status", h.UpdateStatus)
		r.Post("/{id}/approve", h.ApproveAccount)
		r.Post("/{id}/close", h.CloseAccount)
		r.Get("/{id}/interest-summary", h.GetInterestSummary)
		r.Post("/{id}/post-interest", h.PostInterest)
	})
	r.Route("/api/v1/eod", func(r chi.Router) {
		r.Post("/run", h.RunEOD)
		r.Get("/history", h.GetEODHistory)
	})
	r.Route("/api/v1/customers", func(r chi.Router) {
		r.Post("/", h.CreateCustomer)
		r.Get("/", h.ListCustomers)
		r.Get("/search", h.SearchCustomers)
		r.Get("/by-customer-id/{customerId}", h.GetCustomerByCustomerId)
		r.Get("/{id}", h.GetCustomer)
		r.Put("/{id}", h.UpdateCustomer)
		r.Patch("/{id}/status", h.UpdateCustomerStatus)
		r.Patch("/{id}/kyc", h.UpdateCustomerKyc)
	})
	r.Route("/api/v1/transfers", func(r chi.Router) {
		r.Post("/", h.InitiateTransfer)
		r.Get("/{id}", h.GetTransfer)
		r.Get("/account/{accountId}", h.GetTransfersByAccount)
	})
	r.Route("/api/v1/organization", func(r chi.Router) {
		r.Get("/settings", h.GetOrgSettings)
		r.Put("/settings", h.UpdateOrgSettings)
		r.Route("/branches", func(r chi.Router) {
			r.Get("/", h.ListBranches)
			r.Post("/", h.CreateBranch)
			r.Get("/{id}", h.GetBranch)
			r.Put("/{id}", h.UpdateBranch)
			r.Delete("/{id}", h.DeleteBranch)
		})
		r.Route("/users", func(r chi.Router) {
			r.Get("/", h.ListUsers)
			r.Post("/", h.CreateUser)
			r.Get("/{id}", h.GetUser)
			r.Put("/{id}", h.UpdateUser)
			r.Put("/{id}/status", h.UpdateUserStatus)
		})
	})
	r.Get("/api/v1/audit-log", h.ListAuditLog)
	r.Get("/api/v1/audit-log/verify", h.VerifyAuditChain)

	r.Route("/api/v1/control-config", func(r chi.Router) {
		r.Get("/", h.ListControlConfig)
		// Changing maker-checker config is an administrative control.
		r.With(auth.RequireRole("ADMIN")).Put("/", h.UpdateControlConfig)
	})
	r.Route("/api/v1/pending-approvals", func(r chi.Router) {
		r.Get("/", h.ListPendingApprovals)
		// Only authorising roles may approve/reject queued operations.
		r.With(auth.RequireRole("ADMIN", "MANAGER")).Post("/{id}/approve", h.ApprovePending)
		r.With(auth.RequireRole("ADMIN", "MANAGER")).Post("/{id}/reject", h.RejectPending)
	})
}

// ListControlConfig returns the effective maker-checker config for the tenant.
func (h *Handler) ListControlConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	httputil.WriteJSON(w, http.StatusOK, h.approvalSvc.EffectiveConfig(r.Context(), tenantID))
}

// UpdateControlConfig upserts a control config row.
func (h *Handler) UpdateControlConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	var req struct {
		Operation string          `json:"operation"`
		Enabled   bool            `json:"enabled"`
		Threshold decimal.Decimal `json:"threshold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	// Capture prior config for the audit trail (changing a financial control is
	// itself an auditable, tamper-evident event).
	var before any
	for _, c := range h.approvalSvc.EffectiveConfig(r.Context(), tenantID) {
		if strings.EqualFold(c.Operation, req.Operation) {
			before = map[string]any{"operation": c.Operation, "enabled": c.Enabled, "threshold": c.ThresholdAmount}
			break
		}
	}
	if err := h.approvalSvc.UpsertConfig(r.Context(), tenantID, req.Operation, req.Enabled, req.Threshold); err != nil {
		h.handleError(w, r, err)
		return
	}
	audit.New(h.repo, h.logger).Record(r.Context(),
		"CONTROL_CONFIG_UPDATE", "CONTROL_CONFIG", req.Operation,
		before,
		map[string]any{"operation": req.Operation, "enabled": req.Enabled, "threshold": req.Threshold},
		nil)
	httputil.WriteJSON(w, http.StatusOK, h.approvalSvc.EffectiveConfig(r.Context(), tenantID))
}

// ListPendingApprovals returns queued approvals (optional ?status=).
func (h *Handler) ListPendingApprovals(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	status := r.URL.Query().Get("status")
	page, size := parsePagination(r)
	items, err := h.approvalSvc.ListPending(r.Context(), tenantID, status, size, page*size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, items)
}

// ApprovePending approves and executes a queued operation.
func (h *Handler) ApprovePending(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id := chi.URLParam(r, "id")
	p, err := h.approvalSvc.Approve(r.Context(), tenantID, id)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

// RejectPending declines a queued operation.
func (h *Handler) RejectPending(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id := chi.URLParam(r, "id")
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := h.approvalSvc.Reject(r.Context(), tenantID, id, req.Reason); err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "REJECTED", "id": id})
}

// ListAuditLog returns the account-service audit trail, optionally filtered by
// entityType and entityId query params.
func (h *Handler) ListAuditLog(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	entityType := r.URL.Query().Get("entityType")
	entityID := r.URL.Query().Get("entityId")
	page, size := parsePagination(r)
	records, err := h.repo.ListAuditLog(r.Context(), tenantID, entityType, entityID, size, page*size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, records)
}

// VerifyAuditChain reports whether the tamper-evident audit trail is intact, or
// the seq of the first altered/missing entry. Lets an auditor prove the log was
// not edited or back-dated.
func (h *Handler) VerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	result, err := h.repo.VerifyAuditChain(r.Context(), tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

// --- Account ---

func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	var req service.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	resp, err := h.accountSvc.CreateAccount(r.Context(), req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	page, size := parsePagination(r)
	resp, err := h.accountSvc.ListAccounts(r.Context(), tenantID, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	resp, err := h.accountSvc.GetAccount(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	resp, err := h.accountSvc.GetBalance(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) Credit(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	var req service.TransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	resp, err := h.accountSvc.Credit(r.Context(), id, req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) Debit(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	var req service.TransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	resp, err := h.accountSvc.Debit(r.Context(), id, req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetTransactions(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	page, size := parsePagination(r)
	resp, err := h.accountSvc.GetTransactionHistory(r.Context(), id, tenantID, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetMiniStatement(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	resp, err := h.accountSvc.GetMiniStatement(r.Context(), id, tenantID, 10)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetStatement(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	from, _ := time.Parse("2006-01-02", r.URL.Query().Get("startDate"))
	to, _ := time.Parse("2006-01-02", r.URL.Query().Get("endDate"))
	if from.IsZero() {
		from = time.Now().AddDate(0, -1, 0)
	}
	if to.IsZero() {
		to = time.Now()
	}
	page, size := parsePagination(r)
	resp, err := h.accountSvc.GetStatement(r.Context(), id, tenantID, from, to, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) SearchAccounts(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	q := r.URL.Query().Get("q")
	resp, err := h.accountSvc.SearchAccounts(r.Context(), q, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetAccountsByCustomer(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	customerID := chi.URLParam(r, "customerId")
	resp, err := h.accountSvc.GetByCustomerID(r.Context(), customerID, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	resp, err := h.accountSvc.UpdateStatus(r.Context(), id, req.Status, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// --- Customer ---

func (h *Handler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	var req service.CreateCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	resp, err := h.customerSvc.CreateCustomer(r.Context(), req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) ListCustomers(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	page, size := parsePagination(r)
	resp, err := h.customerSvc.ListCustomers(r.Context(), tenantID, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetCustomer(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid customer ID", r.URL.Path)
		return
	}
	resp, err := h.customerSvc.GetCustomer(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) UpdateCustomer(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid customer ID", r.URL.Path)
		return
	}
	var req service.UpdateCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	resp, err := h.customerSvc.UpdateCustomer(r.Context(), id, req, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) UpdateCustomerStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid customer ID", r.URL.Path)
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	resp, err := h.customerSvc.UpdateCustomerStatus(r.Context(), id, req.Status, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// UpdateCustomerKyc sets a customer's KYC status (PENDING/VERIFIED/REJECTED).
func (h *Handler) UpdateCustomerKyc(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid customer ID", r.URL.Path)
		return
	}
	var req struct {
		KycStatus string `json:"kycStatus"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	resp, err := h.customerSvc.UpdateKycStatus(r.Context(), id, req.KycStatus, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) SearchCustomers(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	q := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	resp, err := h.customerSvc.SearchCustomers(r.Context(), q, tenantID, limit)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetCustomerByCustomerId(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	customerID := chi.URLParam(r, "customerId")
	resp, err := h.customerSvc.GetByCustomerID(r.Context(), customerID, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// --- Transfer ---

func (h *Handler) InitiateTransfer(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())
	var req service.TransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	resp, err := h.transferSvc.InitiateTransfer(r.Context(), req, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) GetTransfer(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid transfer ID", r.URL.Path)
		return
	}
	resp, err := h.transferSvc.GetTransfer(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetTransfersByAccount(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	accountID, err := uuid.Parse(chi.URLParam(r, "accountId"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	page, size := parsePagination(r)
	resp, err := h.transferSvc.GetTransfersByAccount(r.Context(), accountID, tenantID, page, size)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// --- Organization Settings ---

func (h *Handler) GetOrgSettings(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	settings, err := h.repo.GetTenantSettings(r.Context(), tenantID)
	if err != nil {
		// If not found, return defaults
		settings = &model.TenantSettings{
			TenantID:              tenantID,
			Currency:              "KES",
			Timezone:              "Africa/Nairobi",
			SessionTimeoutMinutes: 30,
			AuditTrailEnabled:     true,
		}
	}
	httputil.WriteJSON(w, http.StatusOK, settings)
}

func (h *Handler) UpdateOrgSettings(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	var req struct {
		OrgName               *string `json:"orgName"`
		CountryCode           *string `json:"countryCode"`
		Currency              *string `json:"currency"`
		Timezone              *string `json:"timezone"`
		TwoFactorEnabled      *bool   `json:"twoFactorEnabled"`
		SessionTimeoutMinutes *int    `json:"sessionTimeoutMinutes"`
		AuditTrailEnabled     *bool   `json:"auditTrailEnabled"`
		IPWhitelistEnabled    *bool   `json:"ipWhitelistEnabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	// Start from existing or defaults
	settings, err := h.repo.GetTenantSettings(r.Context(), tenantID)
	if err != nil {
		settings = &model.TenantSettings{
			TenantID:              tenantID,
			Currency:              "KES",
			Timezone:              "Africa/Nairobi",
			SessionTimeoutMinutes: 30,
			AuditTrailEnabled:     true,
		}
	}

	// Apply updates
	if req.OrgName != nil {
		settings.OrgName = req.OrgName
	}
	if req.CountryCode != nil {
		settings.CountryCode = req.CountryCode
	}
	if req.Currency != nil {
		settings.Currency = *req.Currency
	}
	if req.Timezone != nil {
		settings.Timezone = *req.Timezone
	}
	if req.TwoFactorEnabled != nil {
		settings.TwoFactorEnabled = *req.TwoFactorEnabled
	}
	if req.SessionTimeoutMinutes != nil {
		settings.SessionTimeoutMinutes = *req.SessionTimeoutMinutes
	}
	if req.AuditTrailEnabled != nil {
		settings.AuditTrailEnabled = *req.AuditTrailEnabled
	}
	if req.IPWhitelistEnabled != nil {
		settings.IPWhitelistEnabled = *req.IPWhitelistEnabled
	}

	if err := h.repo.UpsertTenantSettings(r.Context(), settings); err != nil {
		h.logger.Error("Failed to update tenant settings", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to save settings", r.URL.Path)
		return
	}

	// Re-read to get the full record with timestamps
	saved, err := h.repo.GetTenantSettings(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to read back tenant settings", zap.Error(err))
		httputil.WriteJSON(w, http.StatusOK, settings)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, saved)
}

// --- Branch ---

func (h *Handler) ListBranches(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	branches, err := h.repo.ListBranches(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list branches", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list branches", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, branches)
}

func (h *Handler) GetBranch(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id := chi.URLParam(r, "id")
	branch, err := h.repo.GetBranch(r.Context(), tenantID, id)
	if err != nil {
		httputil.WriteNotFound(w, "Branch not found", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, branch)
}

func (h *Handler) CreateBranch(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	var branch model.Branch
	if err := json.NewDecoder(r.Body).Decode(&branch); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	branch.TenantID = tenantID
	if branch.Status == "" {
		branch.Status = "ACTIVE"
	}
	if branch.Type == "" {
		branch.Type = "BRANCH"
	}
	if branch.Country == "" {
		branch.Country = "KEN"
	}
	if err := h.repo.CreateBranch(r.Context(), &branch); err != nil {
		h.logger.Error("Failed to create branch", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to create branch", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, branch)
}

func (h *Handler) UpdateBranch(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id := chi.URLParam(r, "id")

	// Verify it exists
	existing, err := h.repo.GetBranch(r.Context(), tenantID, id)
	if err != nil || existing == nil {
		httputil.WriteNotFound(w, "Branch not found", r.URL.Path)
		return
	}

	var branch model.Branch
	if err := json.NewDecoder(r.Body).Decode(&branch); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	branch.ID = id
	branch.TenantID = tenantID
	if err := h.repo.UpdateBranch(r.Context(), &branch); err != nil {
		h.logger.Error("Failed to update branch", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to update branch", r.URL.Path)
		return
	}

	// Re-read to return full record
	updated, err := h.repo.GetBranch(r.Context(), tenantID, id)
	if err != nil {
		httputil.WriteJSON(w, http.StatusOK, branch)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteBranch(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.repo.DeleteBranch(r.Context(), tenantID, id); err != nil {
		h.logger.Error("Failed to delete branch", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to delete branch", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Users ---

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	page, size := parsePagination(r)
	users, total, err := h.repo.ListUsers(r.Context(), tenantID, size, page*size)
	if err != nil {
		h.logger.Error("Failed to list users", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list users", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"content":       users,
		"totalElements": total,
		"page":          page,
		"size":          size,
	})
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id := chi.URLParam(r, "id")
	user, err := h.repo.GetUser(r.Context(), tenantID, id)
	if err != nil {
		httputil.WriteNotFound(w, "User not found", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, user)
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	var user model.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	if user.Username == "" || user.Name == "" || user.Email == "" {
		httputil.WriteBadRequest(w, "Username, name, and email are required", r.URL.Path)
		return
	}
	user.TenantID = tenantID
	if user.Role == "" {
		user.Role = "USER"
	}
	if user.Status == "" {
		user.Status = "ACTIVE"
	}
	if err := h.repo.CreateUser(r.Context(), &user); err != nil {
		h.logger.Error("Failed to create user", zap.Error(err))
		httputil.WriteConflict(w, "User with this username already exists", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, user)
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id := chi.URLParam(r, "id")

	existing, err := h.repo.GetUser(r.Context(), tenantID, id)
	if err != nil || existing == nil {
		httputil.WriteNotFound(w, "User not found", r.URL.Path)
		return
	}

	var user model.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	user.ID = id
	user.TenantID = tenantID
	if user.Status == "" {
		user.Status = existing.Status
	}
	if err := h.repo.UpdateUser(r.Context(), &user); err != nil {
		h.logger.Error("Failed to update user", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to update user", r.URL.Path)
		return
	}

	updated, err := h.repo.GetUser(r.Context(), tenantID, id)
	if err != nil {
		httputil.WriteJSON(w, http.StatusOK, user)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) UpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	id := chi.URLParam(r, "id")

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	if req.Status != "ACTIVE" && req.Status != "INACTIVE" && req.Status != "LOCKED" {
		httputil.WriteBadRequest(w, "Status must be ACTIVE, INACTIVE, or LOCKED", r.URL.Path)
		return
	}

	if err := h.repo.UpdateUserStatus(r.Context(), tenantID, id, req.Status); err != nil {
		h.logger.Error("Failed to update user status", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to update user status", r.URL.Path)
		return
	}

	user, err := h.repo.GetUser(r.Context(), tenantID, id)
	if err != nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": req.Status})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, user)
}

// --- Account Opening / Closing / Interest ---

func (h *Handler) OpenAccount(w http.ResponseWriter, r *http.Request) {
	if h.openingSvc == nil {
		httputil.WriteInternalError(w, "Account opening service not available", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())
	var req service.OpenAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	resp, err := h.openingSvc.OpenAccount(r.Context(), req, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) ApproveAccount(w http.ResponseWriter, r *http.Request) {
	if h.openingSvc == nil {
		httputil.WriteInternalError(w, "Account opening service not available", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	resp, err := h.openingSvc.ApproveAccount(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) CloseAccount(w http.ResponseWriter, r *http.Request) {
	if h.openingSvc == nil {
		httputil.WriteInternalError(w, "Account opening service not available", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}
	resp, err := h.openingSvc.CloseAccount(r.Context(), id, req.Reason, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetInterestSummary(w http.ResponseWriter, r *http.Request) {
	if h.interestSvc == nil {
		httputil.WriteInternalError(w, "Interest service not available", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	resp, err := h.interestSvc.GetInterestSummary(r.Context(), id, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) PostInterest(w http.ResponseWriter, r *http.Request) {
	if h.interestSvc == nil {
		httputil.WriteInternalError(w, "Interest service not available", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid account ID", r.URL.Path)
		return
	}
	resp, err := h.interestSvc.PostAccruedInterest(r.Context(), id, tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) RunEOD(w http.ResponseWriter, r *http.Request) {
	if h.eodSvc == nil {
		httputil.WriteInternalError(w, "EOD service not available", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		userID = "system"
	}
	resp, err := h.eodSvc.RunEOD(r.Context(), tenantID, userID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetEODHistory(w http.ResponseWriter, r *http.Request) {
	if h.eodSvc == nil {
		httputil.WriteInternalError(w, "EOD service not available", r.URL.Path)
		return
	}
	tenantID := auth.TenantIDOrDefault(r.Context())
	runs, err := h.eodSvc.GetEODHistory(r.Context(), tenantID, 30)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, runs)
}

// --- Helpers ---

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch e := err.(type) {
	case *service.ErrPendingApproval:
		// Operation queued for dual control — 202 Accepted.
		httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
			"status":    "PENDING_APPROVAL",
			"pendingId": e.PendingID,
			"operation": e.Operation,
			"message":   "Submitted for approval — a second authoriser must approve this operation.",
		})
	case *errors.NotFoundError:
		httputil.WriteNotFound(w, e.Message, r.URL.Path)
	case *errors.BusinessError:
		httputil.WriteErrorJSON(w, e.StatusCode, http.StatusText(e.StatusCode), e.Message, r.URL.Path)
	default:
		h.logger.Error("Internal error", zap.Error(err), zap.String("path", r.URL.Path))
		httputil.WriteInternalError(w, "An unexpected error occurred", r.URL.Path)
	}
}

func parsePagination(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 || size > 100 {
		size = 20
	}
	if page < 0 {
		page = 0
	}
	return page, size
}
