package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/notification/model"
	"github.com/athena-lms/go-services/internal/bff/notification/provider"
	"github.com/athena-lms/go-services/internal/bff/notification/repository"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

type NotificationService struct {
	notifRepo    *repository.NotificationRepo
	templateRepo *repository.TemplateRepo
	deliveryRepo *repository.DeliveryLogRepo
	rateLimiter  *RateLimiter
	sms          *provider.SmsProvider
	push         *provider.PushProvider
	email        *provider.EmailProvider
}

func NewNotificationService(
	notifRepo *repository.NotificationRepo,
	templateRepo *repository.TemplateRepo,
	deliveryRepo *repository.DeliveryLogRepo,
	rateLimiter *RateLimiter,
	sms *provider.SmsProvider,
	push *provider.PushProvider,
	email *provider.EmailProvider,
) *NotificationService {
	return &NotificationService{
		notifRepo:    notifRepo,
		templateRepo: templateRepo,
		deliveryRepo: deliveryRepo,
		rateLimiter:  rateLimiter,
		sms:          sms,
		push:         push,
		email:        email,
	}
}

// SendOtpSms sends an OTP SMS with rate limiting.
func (s *NotificationService) SendOtpSms(ctx context.Context, phone, otp string) error {
	limited, err := s.rateLimiter.IsRateLimited(ctx, phone)
	if err != nil {
		return fmt.Errorf("rate limit check: %w", err)
	}
	if limited {
		return apperrors.BadRequest("SMS rate limit exceeded for this phone number")
	}

	msg := fmt.Sprintf("Your Athena verification code is: %s. Do not share this code.", otp)
	extID, err := s.sms.SendSms(phone, msg)
	if err != nil {
		slog.Error("OTP SMS send failed", "phone", phone, "error", err)
		return fmt.Errorf("send otp sms: %w", err)
	}
	slog.Info("OTP SMS sent", "phone", phone, "externalId", extID)
	return nil
}

// SendByTemplate sends a notification using a template.
func (s *NotificationService) SendByTemplate(ctx context.Context, tenantID string, userID uuid.UUID, templateCode string, variables map[string]string, channelStr string, recipientOverride string) error {
	channel := model.Channel(channelStr)

	tmpl, err := s.templateRepo.FindByTenantCodeAndChannel(ctx, tenantID, templateCode, channel)
	if err != nil {
		return fmt.Errorf("find template: %w", err)
	}
	if tmpl == nil {
		return apperrors.BadRequest(fmt.Sprintf("template %s not found for channel %s", templateCode, channelStr))
	}

	body := RenderTemplate(tmpl.BodyTemplate, variables)
	title := ""
	if tmpl.TitleTemplate != nil {
		title = RenderTemplate(*tmpl.TitleTemplate, variables)
	}

	switch channel {
	case model.ChannelInApp:
		return s.createInAppNotification(ctx, tenantID, userID, title, body, tmpl.Category)
	case model.ChannelSMS:
		return s.dispatchSMS(ctx, tenantID, recipientOverride, body, &templateCode)
	case model.ChannelPush:
		return s.dispatchPush(ctx, tenantID, recipientOverride, title, body, &templateCode)
	case model.ChannelEmail:
		return s.dispatchEmail(ctx, tenantID, recipientOverride, title, body, &templateCode)
	default:
		return apperrors.BadRequest("unsupported channel: " + channelStr)
	}
}

// SendDirectPush creates an in-app notification (used by event consumers).
func (s *NotificationService) SendDirectPush(ctx context.Context, tenantID string, userID uuid.UUID, title, body string, category string, actionType string, actionData map[string]any) error {
	actionJSON, _ := json.Marshal(actionData)
	cat := model.Category(category)
	notif := &model.Notification{
		TenantID:   tenantID,
		UserID:     userID,
		Title:      title,
		Body:       body,
		Category:   cat,
		Read:       false,
		ActionType: &actionType,
		ActionData: (*json.RawMessage)(&actionJSON),
	}
	return s.notifRepo.Create(ctx, notif)
}

// GetUserNotifications returns paginated notifications for a user.
func (s *NotificationService) GetUserNotifications(ctx context.Context, tenantID string, userID uuid.UUID, category string, page, size int) ([]model.Notification, int64, error) {
	var (
		notifs []model.Notification
		total  int64
		err    error
	)
	if category != "" {
		notifs, err = s.notifRepo.FindByTenantUserAndCategory(ctx, tenantID, userID, category, page, size)
		if err != nil {
			return nil, 0, err
		}
		total, err = s.notifRepo.CountByTenantUserAndCategory(ctx, tenantID, userID, category)
	} else {
		notifs, err = s.notifRepo.FindByTenantAndUser(ctx, tenantID, userID, page, size)
		if err != nil {
			return nil, 0, err
		}
		total, err = s.notifRepo.CountByTenantAndUser(ctx, tenantID, userID)
	}
	return notifs, total, err
}

// GetUnreadCount returns the count of unread notifications for a user.
func (s *NotificationService) GetUnreadCount(ctx context.Context, tenantID string, userID uuid.UUID) (int64, error) {
	return s.notifRepo.CountUnread(ctx, tenantID, userID)
}

// MarkAllRead marks all notifications as read for a user.
func (s *NotificationService) MarkAllRead(ctx context.Context, tenantID string, userID uuid.UUID) (int64, error) {
	return s.notifRepo.MarkAllRead(ctx, tenantID, userID)
}

func (s *NotificationService) createInAppNotification(ctx context.Context, tenantID string, userID uuid.UUID, title, body string, category *string) error {
	cat := model.CategorySystem
	if category != nil {
		cat = model.Category(*category)
	}
	notif := &model.Notification{
		TenantID: tenantID,
		UserID:   userID,
		Title:    title,
		Body:     body,
		Category: cat,
		Read:     false,
	}
	return s.notifRepo.Create(ctx, notif)
}

func (s *NotificationService) dispatchSMS(ctx context.Context, tenantID, recipient, body string, templateCode *string) error {
	extID, err := s.sms.SendSms(recipient, body)
	status := model.StatusSent
	var errMsg *string
	if err != nil {
		status = model.StatusFailed
		e := err.Error()
		errMsg = &e
	}
	logErr := s.deliveryRepo.Create(ctx, &model.NotificationDeliveryLog{
		TenantID:     tenantID,
		Channel:      model.ChannelSMS,
		Recipient:    recipient,
		TemplateCode: templateCode,
		Status:       status,
		ExternalID:   &extID,
		ErrorMessage: errMsg,
	})
	if logErr != nil {
		slog.Error("failed to log delivery", "error", logErr)
	}
	return err
}

func (s *NotificationService) dispatchPush(ctx context.Context, tenantID, recipient, title, body string, templateCode *string) error {
	extID, err := s.push.SendPush(recipient, title, body)
	status := model.StatusSent
	var errMsg *string
	if err != nil {
		status = model.StatusFailed
		e := err.Error()
		errMsg = &e
	}
	logErr := s.deliveryRepo.Create(ctx, &model.NotificationDeliveryLog{
		TenantID:     tenantID,
		Channel:      model.ChannelPush,
		Recipient:    recipient,
		TemplateCode: templateCode,
		Status:       status,
		ExternalID:   &extID,
		ErrorMessage: errMsg,
	})
	if logErr != nil {
		slog.Error("failed to log delivery", "error", logErr)
	}
	return err
}

func (s *NotificationService) dispatchEmail(ctx context.Context, tenantID, recipient, subject, body string, templateCode *string) error {
	extID, err := s.email.SendEmail(recipient, subject, body)
	status := model.StatusSent
	var errMsg *string
	if err != nil {
		status = model.StatusFailed
		e := err.Error()
		errMsg = &e
	}
	logErr := s.deliveryRepo.Create(ctx, &model.NotificationDeliveryLog{
		TenantID:     tenantID,
		Channel:      model.ChannelEmail,
		Recipient:    recipient,
		TemplateCode: templateCode,
		Status:       status,
		ExternalID:   &extID,
		ErrorMessage: errMsg,
	})
	if logErr != nil {
		slog.Error("failed to log delivery", "error", logErr)
	}
	return err
}
