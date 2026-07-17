package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/notification/model"
	"github.com/athena-lms/go-services/internal/bff/notification/service"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type NotificationHandler struct {
	svc *service.NotificationService
}

func NewNotificationHandler(svc *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// Routes registers notification endpoints on the given router.
func (h *NotificationHandler) Routes(r chi.Router, authMw func(http.Handler) http.Handler) {
	r.Route("/api/v1/notifications", func(r chi.Router) {
		// Public endpoints.
		r.Post("/sms/otp", h.SendOtp)
		r.Post("/send", h.SendNotification)
		r.Get("/user/{userId}/unread-count", h.UnreadCount)

		// Authenticated endpoints.
		r.Group(func(r chi.Router) {
			r.Use(authMw)
			r.Get("/user/{userId}", h.GetUserNotifications)
			r.Post("/user/{userId}/mark-all-read", h.MarkAllRead)
		})
	})
}

type sendOtpRequest struct {
	PhoneNumber string `json:"phoneNumber" validate:"required"`
	OTP         string `json:"otp" validate:"required"`
}

func (h *NotificationHandler) SendOtp(w http.ResponseWriter, r *http.Request) {
	var req sendOtpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PhoneNumber == "" || req.OTP == "" {
		apperrors.WriteError(w, r, http.StatusBadRequest, "phoneNumber and otp are required")
		return
	}

	if err := h.svc.SendOtpSms(r.Context(), req.PhoneNumber, req.OTP); err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":      "sent",
		"phoneNumber": req.PhoneNumber,
	})
}

type sendNotificationRequest struct {
	UserID            uuid.UUID         `json:"userId" validate:"required"`
	TemplateCode      string            `json:"templateCode" validate:"required"`
	Channel           string            `json:"channel" validate:"required"`
	Variables         map[string]string `json:"variables"`
	RecipientOverride string            `json:"recipientOverride"`
}

func (h *NotificationHandler) SendNotification(w http.ResponseWriter, r *http.Request) {
	var req sendNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TemplateCode == "" || req.Channel == "" {
		apperrors.WriteError(w, r, http.StatusBadRequest, "templateCode and channel are required")
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	if err := h.svc.SendByTemplate(r.Context(), tenantID, req.UserID, req.TemplateCode, req.Variables, req.Channel, req.RecipientOverride); err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":       "sent",
		"templateCode": req.TemplateCode,
	})
}

func (h *NotificationHandler) GetUserNotifications(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid userId")
		return
	}

	category := r.URL.Query().Get("category")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 20
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	notifs, total, err := h.svc.GetUserNotifications(r.Context(), tenantID, userID, category, page, size)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	responses := make([]model.NotificationResponse, len(notifs))
	for i := range notifs {
		responses[i] = notifs[i].ToResponse()
	}

	writeJSON(w, http.StatusOK, dto.NewPageResponse(responses, page, size, total))
}

func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid userId")
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	count, err := h.svc.MarkAllRead(r.Context(), tenantID, userID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"updatedCount": count,
	})
}

func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		apperrors.WriteError(w, r, http.StatusBadRequest, "invalid userId")
		return
	}

	tenantID := auth.TenantIDOrDefault(r.Context())
	count, err := h.svc.GetUnreadCount(r.Context(), tenantID, userID)
	if err != nil {
		apperrors.HandleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"unreadCount": count,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
