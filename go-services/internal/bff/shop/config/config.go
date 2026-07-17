// Package config loads bff-shop configuration. Platform-wide settings come
// from internal/common/config; downstream URLs and the BNPL loan product are
// layered on top.
package config

import (
	"strings"

	"github.com/spf13/viper"

	commonconfig "github.com/athena-lms/go-services/internal/common/config"
)

// Config is the bff-shop configuration.
type Config struct {
	*commonconfig.Config

	AccountServiceURL         string
	PaymentServiceURL         string
	LoanOriginationServiceURL string
	AIScoringServiceURL       string

	// BNPLProductID is the LMS loan product used for BNPL checkout.
	BNPLProductID string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	base, err := commonconfig.Load("bff-shop")
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	v.SetDefault("ACCOUNT_SERVICE_URL", "http://localhost:8105/lms")
	v.SetDefault("PAYMENT_SERVICE_URL", "http://localhost:8105/lms")
	v.SetDefault("LOAN_ORIGINATION_SERVICE_URL", "http://localhost:8105/lms")
	v.SetDefault("AI_SCORING_SERVICE_URL", "http://localhost:8105/lms")
	v.SetDefault("BNPL_PRODUCT_ID", "3b299267-f8f7-48fa-b9b7-f35430aa3104")

	return &Config{
		Config:                    base,
		AccountServiceURL:         v.GetString("ACCOUNT_SERVICE_URL"),
		PaymentServiceURL:         v.GetString("PAYMENT_SERVICE_URL"),
		LoanOriginationServiceURL: v.GetString("LOAN_ORIGINATION_SERVICE_URL"),
		AIScoringServiceURL:       v.GetString("AI_SCORING_SERVICE_URL"),
		BNPLProductID:             v.GetString("BNPL_PRODUCT_ID"),
	}, nil
}
