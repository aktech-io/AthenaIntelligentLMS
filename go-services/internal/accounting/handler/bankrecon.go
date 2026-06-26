package handler

import (
	"encoding/json"
	"net/http"

	"github.com/athena-lms/go-services/internal/accounting/model"
	"github.com/athena-lms/go-services/internal/common/httputil"
)

// importBankStatement ingests a JSON array of externally-provided bank
// statement lines for the tenant. Role-gated (ADMIN/MANAGER/ACCOUNTANT).
func (h *Handler) importBankStatement(w http.ResponseWriter, r *http.Request) {
	var reqs []model.BankStatementLineRequest
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body: "+err.Error(), r.URL.Path)
		return
	}
	if len(reqs) == 0 {
		httputil.WriteBadRequest(w, "at least one statement line is required", r.URL.Path)
		return
	}

	tenantID := tenantFromRequest(r)
	resp, err := h.svc.ImportBankStatement(r.Context(), reqs, tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// getBankReconciliation matches bank statement lines against the GL Cash
// account ledger and returns the reconciliation report. Read, open.
func (h *Handler) getBankReconciliation(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromRequest(r)
	resp, err := h.svc.GetBankReconciliation(r.Context(), tenantID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}
