package provider

import (
	"log/slog"

	"github.com/google/uuid"
)

type EmailProvider struct{}

func NewEmailProvider() *EmailProvider { return &EmailProvider{} }

func (p *EmailProvider) SendEmail(to, subject, body string) (string, error) {
	slog.Info("email notification (stub)", "to", to, "subject", subject)
	return "email-" + uuid.New().String(), nil
}
