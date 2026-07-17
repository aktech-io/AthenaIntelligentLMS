package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/athena-lms/go-services/internal/common/market"
	"net/smtp"
	"strings"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/notification/model"
	"github.com/athena-lms/go-services/internal/notification/repository"
)

// Service contains the core notification business logic.
type Service struct {
	repo   *repository.Repository
	logger *zap.Logger
}

// New creates a new notification Service.
func New(repo *repository.Repository, logger *zap.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

// SendEmail sends an email via SMTP using the stored EMAIL config.
// It always logs the attempt regardless of outcome.
func (s *Service) SendEmail(ctx context.Context, serviceName, to, subject, body string) error {
	s.logger.Info("Sending email",
		zap.String("service", serviceName),
		zap.String("to", to),
		zap.String("subject", subject),
	)

	status := "FAILED"
	var errorMsg string

	defer func() {
		logEntry := &model.NotificationLog{
			ServiceName:  serviceName,
			Type:         "EMAIL",
			Recipient:    to,
			Subject:      subject,
			Body:         body,
			Status:       status,
			ErrorMessage: errorMsg,
		}
		if err := s.repo.InsertLog(ctx, logEntry); err != nil {
			s.logger.Error("Failed to insert notification log", zap.Error(err))
		}
	}()

	config, err := s.repo.FindConfigByType(ctx, "EMAIL")
	if err != nil {
		errorMsg = err.Error()
		return fmt.Errorf("email config lookup: %w", err)
	}
	if config == nil {
		errorMsg = "Email configuration not found"
		return fmt.Errorf(errorMsg)
	}

	if !config.Enabled {
		s.logger.Warn("Email notifications are DISABLED. Skipping send", zap.String("to", to))
		status = "SKIPPED"
		return nil
	}

	// Build the email message
	msg := buildEmailMessage(ptrStr(config.FromAddress), to, subject, body)

	addr := fmt.Sprintf("%s:%d", ptrStr(config.Host), ptrInt(config.Port))
	hasCredentials := strings.TrimSpace(ptrStr(config.Username)) != "" && strings.TrimSpace(ptrStr(config.Password)) != ""

	var sendErr error
	if ptrInt(config.Port) == 465 {
		// Implicit TLS (SMTPS)
		sendErr = sendSMTPS(addr, config, hasCredentials, to, msg)
	} else {
		// Plain or STARTTLS
		sendErr = sendSMTPWithStartTLS(addr, config, hasCredentials, to, msg)
	}

	if sendErr != nil {
		errorMsg = sendErr.Error()
		return fmt.Errorf("failed to send email: %w", sendErr)
	}

	s.logger.Info("Email sent successfully", zap.String("to", to))
	status = "SENT"
	return nil
}

// sendSMTPS handles port 465 implicit TLS connections.
func sendSMTPS(addr string, config *model.NotificationConfig, hasCredentials bool, to string, msg []byte) error {
	host := ptrStr(config.Host)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // matches Java ssl.trust=*
		ServerName:         host,
	}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()

	if hasCredentials {
		auth := smtp.PlainAuth("", ptrStr(config.Username), ptrStr(config.Password), host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	return sendMessage(c, ptrStr(config.FromAddress), to, msg)
}

// sendSMTPWithStartTLS handles port 587 STARTTLS or plain SMTP connections.
func sendSMTPWithStartTLS(addr string, config *model.NotificationConfig, hasCredentials bool, to string, msg []byte) error {
	host := ptrStr(config.Host)
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer c.Close()

	// Try STARTTLS if credentials are present
	if hasCredentials {
		if ok, _ := c.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // matches Java ssl.trust=*
				ServerName:         host,
			}
			if err := c.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		}
		auth := smtp.PlainAuth("", ptrStr(config.Username), ptrStr(config.Password), host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	return sendMessage(c, ptrStr(config.FromAddress), to, msg)
}

// ptrStr safely dereferences a *string, returning "" if nil.
func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ptrInt safely dereferences a *int, returning 0 if nil.
func ptrInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

func sendMessage(c *smtp.Client, from, to string, msg []byte) error {
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}
	return c.Quit()
}

func buildEmailMessage(from, to, subject, body string) []byte {
	return []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body,
	))
}

// --- Template methods matching the Java service ---

// SendDisputeAcknowledgement sends the credit dispute acknowledgement email.
func (s *Service) SendDisputeAcknowledgement(ctx context.Context, to, disputeID string) error {
	subject := "Dispute Received — Athena Credit Score"
	body := fmt.Sprintf(
		"Dear Valued Customer,\n\n"+
			"We have received your credit report dispute (Ref: %s).\n\n"+
			"Our team will review your dispute and respond within 5 working days in accordance "+
			"with the Credit Reference Bureau Regulations, 2013.\n\n"+
			"You can track the status of your dispute by logging into the Athena Customer Portal.\n\n"+
			"Regards,\nAthena Credit Score Team\n%s",
		disputeID, market.Current().Support.Email)
	return s.SendEmail(ctx, "customer-service", to, subject, body)
}

// SendScoreUpdateNotification sends a credit score update email.
func (s *Service) SendScoreUpdateNotification(ctx context.Context, to string, score any) error {
	subject := "Your Credit Score Has Been Updated — Athena"
	body := fmt.Sprintf(
		"Dear Valued Customer,\n\n"+
			"Your Athena Credit Score has been updated.\n\n"+
			"New Score: %v / 850\n\n"+
			"Log in to the Athena Customer Portal to view your full credit report "+
			"and understand what factors influenced your score.\n\n"+
			"Regards,\nAthena Credit Score Team\n%s",
		score, market.Current().Support.Email)
	return s.SendEmail(ctx, "scoring-service", to, subject, body)
}

// SendConsentGrantedNotification sends a data consent confirmation email.
func (s *Service) SendConsentGrantedNotification(ctx context.Context, to string, partnerID any) error {
	subject := "Data Access Consent Confirmed — Athena"
	support := market.Current().Support
	body := fmt.Sprintf(
		"Dear Valued Customer,\n\n"+
			"You have successfully granted data access consent to partner: %v.\n\n"+
			"If you did not authorise this, please contact us immediately at %s "+
			"or call %s.\n\n"+
			"You can revoke consent at any time from the Athena Customer Portal.\n\n"+
			"Regards,\nAthena Credit Score Team",
		partnerID, support.Email, support.Phone)
	return s.SendEmail(ctx, "customer-service", to, subject, body)
}

// --- Config management ---

// GetConfig retrieves the notification config for the given type.
func (s *Service) GetConfig(ctx context.Context, configType string) (*model.NotificationConfig, error) {
	return s.repo.FindConfigByType(ctx, configType)
}

// UpdateConfig creates or updates a notification config.
func (s *Service) UpdateConfig(ctx context.Context, config *model.NotificationConfig) (*model.NotificationConfig, error) {
	return s.repo.UpsertConfig(ctx, config)
}

// ListLogs returns paginated notification logs.
func (s *Service) ListLogs(ctx context.Context, page, size int) ([]model.NotificationLog, int64, error) {
	return s.repo.ListLogs(ctx, page, size)
}
