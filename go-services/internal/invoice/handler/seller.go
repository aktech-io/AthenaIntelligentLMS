package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/invoice/model"
)

// Seller auth mode (SPEC §6 "Auth modes").
//
// Staff tokens come from account-service and carry ADMIN/MANAGER/OFFICER.
// Customer tokens come from the mobile BFF (POST /api/v1/mobile/auth/otp/verify)
// and carry: sub=<phone>, roles=["MOBILE_USER"], customerId="MOB-XXXXXXXX"
// (the account-service customer business id, auto-created by account-service
// on the mobile.user.registered event), userId=<mobile user UUID>, tenantId.
//
// A request is a *seller* when its token has no staff role and carries a
// customerId; the seller id is that customerId, which is exactly what
// Invoice.SellerCustomerID stores. Sellers see only their own invoices; a
// foreign invoice answers 404 (never 403) so ids cannot be enumerated.

// staffRoles are the roles that make a token "staff" (plus SERVICE for
// service-to-service calls, which behave as ADMIN).
var staffRoles = map[string]bool{"ADMIN": true, "MANAGER": true, "OFFICER": true, "SERVICE": true}

// Principal is the caller as derived from the token in context.
type Principal struct {
	Staff        bool   // holds a staff role (or none at all and no customer id)
	SellerID     string // customerId claim when the caller is a seller
	MobileUserID string // mobile user UUID (informational)
}

// Seller reports whether the caller is scoped to a single seller.
func (p Principal) Seller() bool { return !p.Staff && p.SellerID != "" }

// principalFrom classifies the caller from the auth context populated by
// auth.Middleware. Tokens with a staff role are staff regardless of any
// customerId. Tokens without a staff role but with a customerId are sellers.
// Tokens with neither are treated as (role-less) staff so the existing role
// gates decide, exactly as before.
func principalFrom(ctx context.Context) Principal {
	p := Principal{MobileUserID: auth.MobileUserIDFromContext(ctx)}
	for _, r := range auth.RolesFromContext(ctx) {
		if staffRoles[strings.ToUpper(strings.TrimSpace(r))] {
			p.Staff = true
			return p
		}
	}
	if cid := strings.TrimSpace(auth.CustomerIDStrFromContext(ctx)); cid != "" {
		p.SellerID = cid
		return p
	}
	p.Staff = true
	return p
}

// withSellerActor makes ledger/audit "actor" fields carry the seller's
// customer id instead of the token subject (the phone number) so no PII lands
// in the ledger and the actor is traceable to the seller record.
func withSellerActor(ctx context.Context, p Principal) context.Context {
	return auth.WithUserID(ctx, p.SellerID)
}

// sellerOr lets sellers through (scoped, with the seller actor set) and
// applies staffMw (may be nil = any authenticated staff) to everybody else.
func (h *Handler) sellerOr(staffMw func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		staff := next
		if staffMw != nil {
			staff = staffMw(next)
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := principalFrom(r.Context())
			if p.Seller() {
				next.ServeHTTP(w, r.WithContext(withSellerActor(r.Context(), p)))
				return
			}
			staff.ServeHTTP(w, r)
		})
	}
}

// sellerOnly admits sellers only (staff get 403: they use the unscoped routes).
func (h *Handler) sellerOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := principalFrom(r.Context())
		if !p.Seller() {
			httputil.WriteForbidden(w, "this endpoint is for seller (customer) tokens; staff use /api/v1/invoices", r.URL.Path)
			return
		}
		next.ServeHTTP(w, r.WithContext(withSellerActor(r.Context(), p)))
	})
}

// staffOnly rejects seller tokens (403). Role gates still apply after it.
func (h *Handler) staffOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principalFrom(r.Context()).Seller() {
			httputil.WriteForbidden(w, "You do not have permission to perform this operation.", r.URL.Path)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireSellerProfile answers 409 SELLER_PROFILE_MISSING (and returns false)
// when account-service has no customer for the seller yet.
func (h *Handler) requireSellerProfile(w http.ResponseWriter, r *http.Request, p Principal) bool {
	ok, err := h.svc.SellerProfileExists(r.Context(), p.SellerID)
	if err != nil {
		h.handleError(w, r, err)
		return false
	}
	if !ok {
		httputil.WriteJSON(w, http.StatusConflict, map[string]any{
			"status": http.StatusConflict, "error": "Conflict", "code": "SELLER_PROFILE_MISSING",
			"message":    fmt.Sprintf("no customer profile exists for %s yet; complete onboarding before using invoice finance", p.SellerID),
			"customerId": p.SellerID, "path": r.URL.Path,
		})
		return false
	}
	return true
}

// authorizeInvoice enforces ownership for sellers. It returns the invoice
// (nil for staff, who bypass the check) and false after writing a 404 when the
// invoice is missing or belongs to someone else.
func (h *Handler) authorizeInvoice(w http.ResponseWriter, r *http.Request, id uuid.UUID) (*model.Invoice, bool) {
	p := principalFrom(r.Context())
	if !p.Seller() {
		return nil, true
	}
	inv, err := h.svc.Owner(r.Context(), id)
	if err != nil || inv == nil || inv.SellerCustomerID != p.SellerID {
		httputil.WriteNotFound(w, "invoice not found", r.URL.Path)
		return nil, false
	}
	return inv, true
}

// fundedStatuses are the states in which a seller may no longer cancel.
var fundedStatuses = map[string]bool{
	model.StatusFunded: true, model.StatusPartiallyPaid: true, model.StatusPaid: true, model.StatusOverdue: true,
}

// Mine handles GET /api/v1/invoices/mine?status&page&size (seller only).
func (h *Handler) Mine(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	q := r.URL.Query()
	f := model.ListFilter{Status: strings.ToUpper(q.Get("status")), SellerID: p.SellerID}
	f.Page, _ = strconv.Atoi(q.Get("page"))
	f.Size, _ = strconv.Atoi(q.Get("size"))
	page, err := h.svc.List(r.Context(), f)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, page)
}

// MineSummary handles GET /api/v1/invoices/mine/summary (seller dashboard).
func (h *Handler) MineSummary(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	if !h.requireSellerProfile(w, r, p) {
		return
	}
	sum, err := h.svc.SellerSummary(r.Context(), p.SellerID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sum)
}

// Document handles GET /api/v1/invoices/{id}/document: streams the stored
// file (PDF) from media-service for staff or the owning seller.
func (h *Handler) Document(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r)
	if !ok {
		return
	}
	if _, ok := h.authorizeInvoice(w, r, id); !ok {
		return
	}
	name, data, err := h.svc.Document(r.Context(), id)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	ct := http.DetectContentType(data)
	if strings.HasSuffix(strings.ToLower(name), ".pdf") || ct == "application/octet-stream" {
		ct = "application/pdf"
	}
	if name == "" {
		name = id.String() + ".pdf"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Disposition", `inline; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
