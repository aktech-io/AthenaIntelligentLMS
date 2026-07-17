package db

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// NewSQLX wraps an existing pgx pool in an sqlx handle (database/sql
// compatible, named-parameter and struct-scan support). Added for the mobile
// BFF services folded in from the wallet repo, whose repositories are written
// against sqlx; both interfaces share the one pgx pool, so health checks,
// metrics and repositories all observe the same connections.
func NewSQLX(pool *pgxpool.Pool) *sqlx.DB {
	return sqlx.NewDb(stdlib.OpenDBFromPool(pool), "pgx")
}
