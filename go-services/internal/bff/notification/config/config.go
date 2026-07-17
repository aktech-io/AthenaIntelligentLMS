// Package config loads bff-notification configuration. Platform-wide settings
// come from internal/common/config; provider credentials are layered on top.
package config

import (
	"strings"

	"github.com/spf13/viper"

	commonconfig "github.com/athena-lms/go-services/internal/common/config"
)

// Config is the bff-notification configuration.
type Config struct {
	*commonconfig.Config

	ATApiKey              string
	ATUsername            string
	ATSenderID            string
	FCMServiceAccountPath string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	base, err := commonconfig.Load("bff-notification")
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	v.SetDefault("AFRICASTALKING_API_KEY", "sandbox")
	v.SetDefault("AFRICASTALKING_USERNAME", "sandbox")
	v.SetDefault("AFRICASTALKING_SENDER_ID", "ATHENA")
	v.SetDefault("FCM_SERVICE_ACCOUNT_PATH", "config/firebase-service-account.json")

	return &Config{
		Config:                base,
		ATApiKey:              v.GetString("AFRICASTALKING_API_KEY"),
		ATUsername:            v.GetString("AFRICASTALKING_USERNAME"),
		ATSenderID:            v.GetString("AFRICASTALKING_SENDER_ID"),
		FCMServiceAccountPath: v.GetString("FCM_SERVICE_ACCOUNT_PATH"),
	}, nil
}
