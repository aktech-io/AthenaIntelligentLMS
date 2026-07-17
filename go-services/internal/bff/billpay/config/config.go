// Package config loads bff-billpay-savings configuration. Platform-wide
// settings come from internal/common/config; downstream URLs are layered on top.
package config

import (
	"strings"

	"github.com/spf13/viper"

	commonconfig "github.com/athena-lms/go-services/internal/common/config"
)

// Config is the bff-billpay-savings configuration.
type Config struct {
	*commonconfig.Config

	AccountServiceURL string
	PaymentServiceURL string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	base, err := commonconfig.Load("bff-billpay-savings")
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	v.SetDefault("ACCOUNT_SERVICE_URL", "http://localhost:8105/lms")
	v.SetDefault("PAYMENT_SERVICE_URL", "http://localhost:8105/lms")

	return &Config{
		Config:            base,
		AccountServiceURL: v.GetString("ACCOUNT_SERVICE_URL"),
		PaymentServiceURL: v.GetString("PAYMENT_SERVICE_URL"),
	}, nil
}
