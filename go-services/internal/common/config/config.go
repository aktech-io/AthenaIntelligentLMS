package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configuration for a service.
type Config struct {
	// Service
	ServiceName string `mapstructure:"SERVICE_NAME"`
	Port        int    `mapstructure:"PORT"`

	// Database
	DBHost     string `mapstructure:"DB_HOST"`
	DBPort     int    `mapstructure:"DB_PORT"`
	DBUser     string `mapstructure:"DB_USER"`
	DBPassword string `mapstructure:"DB_PASSWORD"`
	DBName     string `mapstructure:"DB_NAME"`
	DBSSLMode  string `mapstructure:"DB_SSL_MODE"`

	// RabbitMQ
	RabbitMQHost     string `mapstructure:"RABBITMQ_HOST"`
	RabbitMQPort     int    `mapstructure:"RABBITMQ_PORT"`
	RabbitMQUser     string `mapstructure:"RABBITMQ_USER"`
	RabbitMQPassword string `mapstructure:"RABBITMQ_PASSWORD"`
	RabbitMQVHost    string `mapstructure:"RABBITMQ_VHOST"`

	// RabbitMQ consumer toggle for strangler fig migration
	RabbitMQConsumeEnabled bool `mapstructure:"RABBITMQ_CONSUME_ENABLED"`

	// JWT
	JWTSecret string `mapstructure:"JWT_SECRET"`

	// Internal service key for service-to-service auth
	InternalServiceKey string `mapstructure:"LMS_INTERNAL_SERVICE_KEY"`

	// Migration
	MigrateOnStartup bool `mapstructure:"MIGRATE_ON_STARTUP"`
	// MigrateOnly makes the process run migrations and exit (D3: the Helm
	// pre-upgrade migration Job); a migration failure is then fatal instead
	// of a warning, so a bad migration blocks the rollout, not the pods.
	MigrateOnly bool `mapstructure:"MIGRATE_ONLY"`
}

// Load reads configuration from environment variables with sensible defaults.
func Load(serviceName string) (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Defaults
	v.SetDefault("SERVICE_NAME", serviceName)
	v.SetDefault("PORT", 8080)
	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", 5432)
	v.SetDefault("DB_USER", "athena")
	v.SetDefault("DB_PASSWORD", "athena")
	v.SetDefault("DB_NAME", "athena_accounts")
	v.SetDefault("DB_SSL_MODE", "disable")
	v.SetDefault("RABBITMQ_HOST", "localhost")
	v.SetDefault("RABBITMQ_PORT", 5672)
	v.SetDefault("RABBITMQ_USER", "guest")
	v.SetDefault("RABBITMQ_PASSWORD", "guest")
	v.SetDefault("RABBITMQ_VHOST", "/")
	v.SetDefault("RABBITMQ_CONSUME_ENABLED", true)
	v.SetDefault("MIGRATE_ON_STARTUP", true)
	v.SetDefault("MIGRATE_ONLY", false)
	v.SetDefault("JWT_SECRET", "")
	v.SetDefault("LMS_INTERNAL_SERVICE_KEY", "")

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return cfg, nil
}

// DatabaseDSN returns the PostgreSQL connection string.
func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

// RabbitMQURL returns the AMQP connection URL.
func (c *Config) RabbitMQURL() string {
	vhost := c.RabbitMQVHost
	if vhost == "/" {
		vhost = "%2F"
	}
	return fmt.Sprintf(
		"amqp://%s:%s@%s:%d/%s",
		c.RabbitMQUser, c.RabbitMQPassword, c.RabbitMQHost, c.RabbitMQPort, vhost,
	)
}
