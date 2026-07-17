package db

import (
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/config"
)

// RunMigrations applies database migrations from the given directory.
// migrationsPath should be "file://migrations/service-name".
func RunMigrations(dsn, migrationsPath string, logger *zap.Logger) error {
	m, err := migrate.New(migrationsPath, dsn)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	logger.Info("Migrations applied",
		zap.Uint("version", version),
		zap.Bool("dirty", dirty),
	)

	return nil
}

// MigrateGate applies the service's startup-migration policy (D3):
//   - MIGRATE_ONLY: run migrations, then tell the caller to exit — success
//     means exit 0 (the Helm pre-upgrade Job pattern), failure is fatal so a
//     bad migration blocks the rollout instead of racing the pods.
//   - MIGRATE_ON_STARTUP (legacy default): best-effort migrate, keep serving.
//
// Returns true when the process should exit after this call.
func MigrateGate(cfg *config.Config, migrationsPath string, logger *zap.Logger) bool {
	if cfg.MigrateOnly {
		if err := RunMigrations(cfg.DatabaseDSN(), migrationsPath, logger); err != nil {
			logger.Fatal("Migration failed in migrate-only mode", zap.Error(err))
		}
		logger.Info("Migrations applied; exiting (MIGRATE_ONLY)")
		return true
	}
	if cfg.MigrateOnStartup {
		if err := RunMigrations(cfg.DatabaseDSN(), migrationsPath, logger); err != nil {
			logger.Warn("Migration failed (may be first run)", zap.Error(err))
		}
	}
	return false
}
