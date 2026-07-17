// Package config loads bff-gateway configuration.
//
// Platform-wide settings (DB, RabbitMQ, JWT secret, internal service key,
// port) come from internal/common/config — the same env var contract as the
// other monorepo services (DB_HOST, DB_NAME, PORT, JWT_SECRET,
// LMS_INTERNAL_SERVICE_KEY, ...). BFF-specific knobs (OTP policy, token
// expiries, downstream LMS URLs) are layered on top via Viper.
package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"

	commonconfig "github.com/athena-lms/go-services/internal/common/config"
)

// Config is the bff-gateway configuration. It embeds the shared platform
// config so cfg.Port, cfg.JWTSecret, cfg.InternalServiceKey etc. resolve the
// same way as in every other service.
type Config struct {
	*commonconfig.Config

	JWTAccessExpiry  time.Duration
	JWTRefreshExpiry time.Duration

	OTPLength      int
	OTPExpiry      time.Duration
	OTPMaxAttempts int

	AccountServiceURL         string
	OverdraftServiceURL       string
	PaymentServiceURL         string
	ProductServiceURL         string
	LoanOriginationServiceURL string
	LoanManagementServiceURL  string
	AIScoringServiceURL       string
	ComplianceServiceURL      string
	MediaServiceURL           string
	NotificationServiceURL    string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	base, err := commonconfig.Load("bff-gateway")
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	v.SetDefault("JWT_ACCESS_EXPIRY_MS", 900000)      // 15 min
	v.SetDefault("JWT_REFRESH_EXPIRY_MS", 2592000000) // 30 days
	v.SetDefault("OTP_LENGTH", 6)
	v.SetDefault("OTP_EXPIRY_SECONDS", 300)
	v.SetDefault("OTP_MAX_ATTEMPTS", 3)

	// All LMS traffic goes through the lms-api-gateway (/lms prefix); the
	// notification URL points at the bff-notification sibling service.
	v.SetDefault("ACCOUNT_SERVICE_URL", "http://localhost:8105/lms")
	v.SetDefault("OVERDRAFT_SERVICE_URL", "http://localhost:8105/lms")
	v.SetDefault("PAYMENT_SERVICE_URL", "http://localhost:8105/lms")
	v.SetDefault("PRODUCT_SERVICE_URL", "http://localhost:8105/lms")
	v.SetDefault("LOAN_ORIGINATION_SERVICE_URL", "http://localhost:8105/lms")
	v.SetDefault("LOAN_MANAGEMENT_SERVICE_URL", "http://localhost:8105/lms")
	v.SetDefault("AI_SCORING_SERVICE_URL", "http://localhost:8105/lms")
	// Compliance + media are called DIRECTLY (not via the lms-api-gateway):
	// the public gateway strips X-Service-Key/-Tenant headers (CRIT-1), so
	// service-key traffic through it would 401. Compose/Helm point these at
	// the in-cluster services; the localhost defaults use their host ports.
	v.SetDefault("COMPLIANCE_SERVICE_URL", "http://localhost:28094")
	v.SetDefault("MEDIA_SERVICE_URL", "http://localhost:28098")
	v.SetDefault("NOTIFICATION_SERVICE_URL", "http://localhost:8111")

	return &Config{
		Config:           base,
		JWTAccessExpiry:  time.Duration(v.GetInt64("JWT_ACCESS_EXPIRY_MS")) * time.Millisecond,
		JWTRefreshExpiry: time.Duration(v.GetInt64("JWT_REFRESH_EXPIRY_MS")) * time.Millisecond,
		OTPLength:        v.GetInt("OTP_LENGTH"),
		OTPExpiry:        time.Duration(v.GetInt("OTP_EXPIRY_SECONDS")) * time.Second,
		OTPMaxAttempts:   v.GetInt("OTP_MAX_ATTEMPTS"),

		AccountServiceURL:         v.GetString("ACCOUNT_SERVICE_URL"),
		OverdraftServiceURL:       v.GetString("OVERDRAFT_SERVICE_URL"),
		PaymentServiceURL:         v.GetString("PAYMENT_SERVICE_URL"),
		ProductServiceURL:         v.GetString("PRODUCT_SERVICE_URL"),
		LoanOriginationServiceURL: v.GetString("LOAN_ORIGINATION_SERVICE_URL"),
		LoanManagementServiceURL:  v.GetString("LOAN_MANAGEMENT_SERVICE_URL"),
		AIScoringServiceURL:       v.GetString("AI_SCORING_SERVICE_URL"),
		ComplianceServiceURL:      v.GetString("COMPLIANCE_SERVICE_URL"),
		MediaServiceURL:           v.GetString("MEDIA_SERVICE_URL"),
		NotificationServiceURL:    v.GetString("NOTIFICATION_SERVICE_URL"),
	}, nil
}
