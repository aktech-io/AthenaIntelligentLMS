package provider

import (
	"log/slog"
	"os"

	"github.com/google/uuid"
)

type PushProvider struct {
	serviceAccountPath string
}

func NewPushProvider(path string) *PushProvider {
	return &PushProvider{serviceAccountPath: path}
}

func (p *PushProvider) SendPush(deviceToken, title, body string) (string, error) {
	if _, err := os.Stat(p.serviceAccountPath); os.IsNotExist(err) {
		slog.Warn("FCM service account not found, using stub", "path", p.serviceAccountPath)
	}
	slog.Info("push notification (stub)", "token", deviceToken, "title", title)
	return "fcm-" + uuid.New().String(), nil
}
