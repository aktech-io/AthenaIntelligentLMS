// Package handler exposes the invoice-finance HTTP API (SPEC §6).
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	cerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/invoice/model"
	"github.com/athena-lms/go-services/internal/invoice/pipeline"
	"github.com/athena-lms/go-services/internal/invoice/service"
)

const maxUpload = 15 << 20

// Handler serves the invoice routes.
type Handler struct {
	svc        *service.Service
	logger     *zap.Logger
	syncWait   time.Duration
	officer    func(http.Handler) http.Handler
	manager    func(http.Handler) http.Handler
	admin      func(http.Handler) http.Handler
	trustProxy bool
}

// New creates a Handler. syncWait bounds inline pipeline execution on upload.
func New(svc *service.Service, syncWait time.Duration, trustProxy bool, logger *zap.Logger) *Handler {
	return &Handler{
		svc: svc, logger: logger, syncWait: syncWait, trustProxy: trustProxy,
		officer: auth.RequireRole("ADMIN", "MANAGER", "OFFICER"),
		manager: auth.RequireRole("ADMIN", "MANAGER"),
		admin:   auth.RequireRole("ADMIN"),
	}
}

// RegisterRoutes mounts the protected routes (behind the JWT middleware).
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/invoices", func(r chi.Router) {
		r.With(h.officer).Post("/", h.Upload)
		r.Get("/", h.List)
		r.Get("/queue", h.Queue)
		r.Get("/stats", h.Stats)
		r.Get("/anchors", h.Anchors)
		r.Get("/{id}", h.Get)
		r.Get("/{id}/timeline", h.Timeline)
		r.With(h.officer).Post("/{id}/resend-confirmation", h.Resend)
		r.With(h.officer).Post("/{id}/offer/accept", h.Accept)
		r.With(h.manager).Post("/{id}/approve", h.Approve)
		r.With(h.manager).Post("/{id}/reject", h.Reject)
		r.With(h.officer).Post("/{id}/fund", h.Fund)
		r.With(h.officer).Post("/{id}/payments", h.Payment)
		r.With(h.manager).Post("/{id}/cancel", h.Cancel)
	})
	r.Route("/api/v1/buyers", func(r chi.Router) {
		r.Get("/", h.Buyers)
		r.Get("/{id}", h.Buyer)
		r.With(h.manager).Patch("/{id}", h.PatchBuyer)
	})
	r.Route("/api/v1/invoice-policy", func(r chi.Router) {
		r.Get("/", h.GetPolicy)
		r.With(h.admin).Put("/", h.PutPolicy)
	})
}

// RegisterPublicRoutes mounts the no-JWT routes (rate-limit them upstream).
func (h *Handler) RegisterPublicRoutes(r chi.Router) {
	r.Route("/api/public/invoices", func(r chi.Router) {
		r.Get("/confirm/{token}", h.PublicConfirmGet)
		r.Post("/confirm/{token}", h.PublicConfirmPost)
		r.Get("/verify/{fingerprint}", h.PublicVerify)
		r.Post("/verify", h.PublicVerifyUpload)
	})
}

// ─── Invoices ────────────────────────────────────────────────────────────────

// Upload handles POST /api/v1/invoices (multipart).
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		httputil.WriteBadRequest(w, "invalid multipart form: "+err.Error(), r.URL.Path)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		httputil.WriteBadRequest(w, "file is required", r.URL.Path)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		httputil.WriteBadRequest(w, "could not read file", r.URL.Path)
		return
	}
	in := service.UploadInput{
		FileName: hdr.Filename, Data: data, SellerCustomerID: r.FormValue("sellerCustomerId"), BuyerEmail: r.FormValue("buyerEmail"),
		SellerName: r.FormValue("sellerName"), SellerEmail: r.FormValue("sellerEmail"), InvoiceNumber: r.FormValue("invoiceNumber"),
	}
	if v := strings.TrimSpace(r.FormValue("requestedAmount")); v != "" {
		d, err := decimal.NewFromString(v)
		if err != nil || !d.IsPositive() {
			httputil.WriteBadRequest(w, "requestedAmount must be a positive number", r.URL.Path)
			return
		}
		in.RequestedAmount = &d
	}
	resp, err := h.svc.Upload(r.Context(), in, h.syncWait)
	if err != nil {
		var dup *service.ErrDuplicate
		if errors.As(err, &dup) {
			httputil.WriteJSON(w, http.StatusConflict, map[string]any{
				"error": "Conflict", "message": "duplicate invoice", "duplicateOf": dup.DuplicateOf, "invoice": dup.Invoice, "path": r.URL.Path, "status": 409,
			})
			return
		}
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// List handles GET /api/v1/invoices.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := model.ListFilter{Status: strings.ToUpper(q.Get("status")), SellerID: q.Get("sellerId")}
	f.Page, _ = strconv.Atoi(q.Get("page"))
	f.Size, _ = strconv.Atoi(q.Get("size"))
	if b := q.Get("buyerId"); b != "" {
		id, err := uuid.Parse(b)
		if err != nil {
			httputil.WriteBadRequest(w, "invalid buyerId", r.URL.Path)
			return
		}
		f.BuyerID = &id
	}
	page, err := h.svc.List(r.Context(), f)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, page)
}

func (h *Handler) idParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "invalid id", r.URL.Path)
		return uuid.Nil, false
	}
	return id, true
}

// Get handles GET /api/v1/invoices/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r)
	if !ok {
		return
	}
	d, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, d)
}

// Timeline handles GET /api/v1/invoices/{id}/timeline.
func (h *Handler) Timeline(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r)
	if !ok {
		return
	}
	tl, err := h.svc.Timeline(r.Context(), id)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, tl)
}

// Resend handles POST /api/v1/invoices/{id}/resend-confirmation {buyerEmail?}.
func (h *Handler) Resend(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r)
	if !ok {
		return
	}
	var body struct {
		BuyerEmail string `json:"buyerEmail"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	token, err := h.svc.ResendConfirmation(r.Context(), id, body.BuyerEmail)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	resp := map[string]any{"invoiceId": id, "sent": true}
	if h.svc.Config().DemoMode {
		resp["confirmUrl"] = h.svc.ConfirmURL(token)
		resp["demoOtp"] = "000000"
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) detailAction(w http.ResponseWriter, r *http.Request, fn func(id uuid.UUID, note model.NoteRequest) (*model.InvoiceDetail, error)) {
	id, ok := h.idParam(w, r)
	if !ok {
		return
	}
	var note model.NoteRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&note)
	}
	d, err := fn(id, note)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, d)
}

// Accept handles POST /api/v1/invoices/{id}/offer/accept.
func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) {
	h.detailAction(w, r, func(id uuid.UUID, _ model.NoteRequest) (*model.InvoiceDetail, error) {
		return h.svc.AcceptOffer(r.Context(), id)
	})
}

// Approve handles POST /api/v1/invoices/{id}/approve {note}.
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	h.detailAction(w, r, func(id uuid.UUID, n model.NoteRequest) (*model.InvoiceDetail, error) {
		return h.svc.Approve(r.Context(), id, n.Note)
	})
}

// Reject handles POST /api/v1/invoices/{id}/reject {reason}.
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	h.detailAction(w, r, func(id uuid.UUID, n model.NoteRequest) (*model.InvoiceDetail, error) {
		reason := n.Reason
		if reason == "" {
			reason = n.Note
		}
		return h.svc.Reject(r.Context(), id, reason)
	})
}

// Cancel handles POST /api/v1/invoices/{id}/cancel {reason}.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	h.detailAction(w, r, func(id uuid.UUID, n model.NoteRequest) (*model.InvoiceDetail, error) {
		return h.svc.Cancel(r.Context(), id, n.Reason+n.Note)
	})
}

// Fund handles POST /api/v1/invoices/{id}/fund.
func (h *Handler) Fund(w http.ResponseWriter, r *http.Request) {
	h.detailAction(w, r, func(id uuid.UUID, _ model.NoteRequest) (*model.InvoiceDetail, error) {
		return h.svc.Fund(r.Context(), id)
	})
}

// Payment handles POST /api/v1/invoices/{id}/payments {amount, reference, payer}.
func (h *Handler) Payment(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r)
	if !ok {
		return
	}
	var req model.PaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid body: "+err.Error(), r.URL.Path)
		return
	}
	d, err := h.svc.RecordPayment(r.Context(), id, req, "API")
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, d)
}

// Queue handles GET /api/v1/invoices/queue.
func (h *Handler) Queue(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Queue(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

// Stats handles GET /api/v1/invoices/stats.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Stats(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, st)
}

// Anchors handles GET /api/v1/invoices/anchors.
func (h *Handler) Anchors(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.Anchors(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"items": a, "count": len(a), "method": "NONE"})
}

// ─── Buyers ──────────────────────────────────────────────────────────────────

// Buyers handles GET /api/v1/buyers.
func (h *Handler) Buyers(w http.ResponseWriter, r *http.Request) {
	b, err := h.svc.Buyers(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"items": b, "count": len(b)})
}

// Buyer handles GET /api/v1/buyers/{id}.
func (h *Handler) Buyer(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r)
	if !ok {
		return
	}
	b, err := h.svc.Buyer(r.Context(), id)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, b)
}

// PatchBuyer handles PATCH /api/v1/buyers/{id} {tier, limitAmount}.
func (h *Handler) PatchBuyer(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r)
	if !ok {
		return
	}
	var p model.BuyerPatch
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		httputil.WriteBadRequest(w, "invalid body: "+err.Error(), r.URL.Path)
		return
	}
	b, err := h.svc.PatchBuyer(r.Context(), id, p)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, b)
}

// ─── Policy ──────────────────────────────────────────────────────────────────

// GetPolicy handles GET /api/v1/invoice-policy.
func (h *Handler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, h.svc.PolicyView(r.Context()))
}

// PutPolicy handles PUT /api/v1/invoice-policy: JSON {yaml} or raw text/yaml body.
func (h *Handler) PutPolicy(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		httputil.WriteBadRequest(w, "could not read body", r.URL.Path)
		return
	}
	doc := string(body)
	if strings.Contains(r.Header.Get("Content-Type"), "json") {
		var p model.PolicyPut
		if err := json.Unmarshal(body, &p); err != nil {
			httputil.WriteBadRequest(w, "invalid JSON: "+err.Error(), r.URL.Path)
			return
		}
		doc = p.YAML
	}
	view, err := h.svc.PutPolicy(r.Context(), doc)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, view)
}

// ─── Public ──────────────────────────────────────────────────────────────────

// PublicConfirmGet handles GET /api/public/invoices/confirm/{token}.
func (h *Handler) PublicConfirmGet(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.PublicConfirmView(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, v)
}

// PublicConfirmPost handles POST /api/public/invoices/confirm/{token}.
func (h *Handler) PublicConfirmPost(w http.ResponseWriter, r *http.Request) {
	var req model.PublicConfirmRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid body: "+err.Error(), r.URL.Path)
		return
	}
	v, err := h.svc.PublicConfirm(r.Context(), chi.URLParam(r, "token"), req, h.clientIP(r), r.UserAgent())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, v)
}

// PublicVerify handles GET /api/public/invoices/verify/{fingerprint}.
func (h *Handler) PublicVerify(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.PublicVerify(r.Context(), chi.URLParam(r, "fingerprint"))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, v)
}

// PublicVerifyUpload handles POST /api/public/invoices/verify (multipart file).
func (h *Handler) PublicVerifyUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		httputil.WriteBadRequest(w, "invalid multipart form", r.URL.Path)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httputil.WriteBadRequest(w, "file is required", r.URL.Path)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		httputil.WriteBadRequest(w, "could not read file", r.URL.Path)
		return
	}
	v, err := h.svc.PublicVerifyBytes(r.Context(), data)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, v)
}

func (h *Handler) clientIP(r *http.Request) string {
	if h.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	var nf *cerrors.NotFoundError
	var be *cerrors.BusinessError
	var it *pipeline.ErrIllegalTransition
	switch {
	case errors.As(err, &nf):
		httputil.WriteNotFound(w, nf.Message, r.URL.Path)
	case errors.As(err, &be):
		switch be.StatusCode {
		case http.StatusBadRequest:
			httputil.WriteBadRequest(w, be.Message, r.URL.Path)
		case http.StatusConflict:
			httputil.WriteConflict(w, be.Message, r.URL.Path)
		case http.StatusForbidden:
			httputil.WriteForbidden(w, be.Message, r.URL.Path)
		case http.StatusTooManyRequests:
			httputil.WriteTooManyRequests(w, be.Message, r.URL.Path)
		default:
			httputil.WriteUnprocessable(w, be.Message, r.URL.Path)
		}
	case errors.As(err, &it):
		httputil.WriteConflict(w, it.Error(), r.URL.Path)
	default:
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteNotFound(w, err.Error(), r.URL.Path)
			return
		}
		h.logger.Error("Internal error", zap.Error(err), zap.String("path", r.URL.Path))
		httputil.WriteInternalError(w, "Internal server error", r.URL.Path)
	}
}
