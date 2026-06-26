package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	commonerr "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/httputil"
	"github.com/athena-lms/go-services/internal/payment/model"
	"github.com/athena-lms/go-services/internal/payment/service"
)

// Handler exposes payment HTTP endpoints.
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// New creates a new Handler.
func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes mounts all payment routes on the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/payments", func(r chi.Router) {
		r.Post("/", h.initiate)
		r.Get("/", h.list)
		r.Get("/{id}", h.getByID)
		r.Get("/customer/{customerId}", h.listByCustomer)
		r.Get("/reference/{ref}", h.getByReference)
		r.Post("/{id}/process", h.process)
		r.Post("/{id}/complete", h.complete)
		r.Post("/{id}/fail", h.fail)
		r.Post("/{id}/reverse", h.reverse)
		r.Post("/methods", h.addMethod)
		r.Get("/methods/customer/{customerId}", h.getMethods)
	})
	r.Get("/api/v1/audit-log", h.listAuditLog)
}

// listAuditLog returns the payment-service audit trail, optionally filtered by
// entityType and entityId query params.
func (h *Handler) listAuditLog(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	entityType := r.URL.Query().Get("entityType")
	entityID := r.URL.Query().Get("entityId")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 50
	}
	records, err := h.svc.ListAuditLog(r.Context(), tenantID, entityType, entityID, size, page*size)
	if err != nil {
		h.logger.Error("Failed to list audit log", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list audit log", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, records)
}

func (h *Handler) initiate(w http.ResponseWriter, r *http.Request) {
	var req model.InitiatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	userID := auth.UserIDFromContext(r.Context())

	payment, err := h.svc.Initiate(r.Context(), &req, tenantID, userID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, service.ToResponse(payment))
}

func (h *Handler) getByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid payment ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	payment, err := h.svc.GetByID(r.Context(), id, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, service.ToResponse(payment))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	tenantID := auth.TenantIDOrDefault(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	var statusFilter *model.PaymentStatus
	if s := r.URL.Query().Get("status"); s != "" {
		ps := model.PaymentStatus(s)
		if model.ValidPaymentStatuses[ps] {
			statusFilter = &ps
		}
	}

	var typeFilter *model.PaymentType
	if t := r.URL.Query().Get("type"); t != "" {
		pt := model.PaymentType(t)
		if model.ValidPaymentTypes[pt] {
			typeFilter = &pt
		}
	}

	payments, total, err := h.svc.List(r.Context(), tenantID, statusFilter, typeFilter, page, size)
	if err != nil {
		h.logger.Error("Failed to list payments", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list payments", r.URL.Path)
		return
	}

	responses := make([]model.PaymentResponse, 0, len(payments))
	for i := range payments {
		responses = append(responses, service.ToResponse(&payments[i]))
	}

	httputil.WriteJSON(w, http.StatusOK, dto.NewPageResponse(responses, page, size, total))
}

func (h *Handler) listByCustomer(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customerId")
	tenantID := auth.TenantIDOrDefault(r.Context())

	payments, err := h.svc.ListByCustomer(r.Context(), customerID, tenantID)
	if err != nil {
		h.logger.Error("Failed to list payments by customer", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list payments", r.URL.Path)
		return
	}

	responses := make([]model.PaymentResponse, 0, len(payments))
	for i := range payments {
		responses = append(responses, service.ToResponse(&payments[i]))
	}

	httputil.WriteJSON(w, http.StatusOK, responses)
}

func (h *Handler) getByReference(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")

	payment, err := h.svc.GetByReference(r.Context(), ref)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, service.ToResponse(payment))
}

func (h *Handler) process(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid payment ID", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	payment, err := h.svc.Process(r.Context(), id, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, service.ToResponse(payment))
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid payment ID", r.URL.Path)
		return
	}

	var req model.CompletePaymentRequest
	// Body is optional for complete
	json.NewDecoder(r.Body).Decode(&req)

	tenantID := auth.TenantIDOrDefault(r.Context())
	payment, err := h.svc.Complete(r.Context(), id, &req, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, service.ToResponse(payment))
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid payment ID", r.URL.Path)
		return
	}

	var req model.FailPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	payment, err := h.svc.Fail(r.Context(), id, &req, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, service.ToResponse(payment))
}

func (h *Handler) reverse(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid payment ID", r.URL.Path)
		return
	}

	var req model.ReversePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	payment, err := h.svc.Reverse(r.Context(), id, &req, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, service.ToResponse(payment))
}

func (h *Handler) addMethod(w http.ResponseWriter, r *http.Request) {
	var req model.AddPaymentMethodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body", r.URL.Path)
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	method, err := h.svc.AddPaymentMethod(r.Context(), &req, tenantID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, service.ToMethodResponse(method))
}

func (h *Handler) getMethods(w http.ResponseWriter, r *http.Request) {
	customerID := chi.URLParam(r, "customerId")
	tenantID := auth.TenantIDOrDefault(r.Context())

	methods, err := h.svc.GetPaymentMethods(r.Context(), customerID, tenantID)
	if err != nil {
		h.logger.Error("Failed to list payment methods", zap.Error(err))
		httputil.WriteInternalError(w, "Failed to list payment methods", r.URL.Path)
		return
	}

	responses := make([]model.PaymentMethodResponse, 0, len(methods))
	for i := range methods {
		responses = append(responses, service.ToMethodResponse(&methods[i]))
	}

	httputil.WriteJSON(w, http.StatusOK, responses)
}

// writeError maps domain errors to HTTP responses.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch e := err.(type) {
	case *commonerr.NotFoundError:
		httputil.WriteNotFound(w, e.Message, r.URL.Path)
	case *commonerr.BusinessError:
		httputil.WriteErrorJSON(w, e.StatusCode, http.StatusText(e.StatusCode), e.Message, r.URL.Path)
	default:
		h.logger.Error("Internal error", zap.Error(err))
		httputil.WriteInternalError(w, "Internal server error", r.URL.Path)
	}
}
