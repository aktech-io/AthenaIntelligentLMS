package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestIsUniqueViolation(t *testing.T) {
	t.Run("unique violation code", func(t *testing.T) {
		err := &pgconn.PgError{Code: "23505", ConstraintName: "uq_payments_tenant_ext_ref"}
		assert.True(t, IsUniqueViolation(err))
	})

	t.Run("wrapped unique violation", func(t *testing.T) {
		err := fmt.Errorf("insert payment: %w", &pgconn.PgError{Code: "23505"})
		assert.True(t, IsUniqueViolation(err))
	})

	t.Run("other pg error code", func(t *testing.T) {
		err := &pgconn.PgError{Code: "23503"} // foreign key violation
		assert.False(t, IsUniqueViolation(err))
	})

	t.Run("non-pg error", func(t *testing.T) {
		assert.False(t, IsUniqueViolation(errors.New("connection refused")))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsUniqueViolation(nil))
	})
}
