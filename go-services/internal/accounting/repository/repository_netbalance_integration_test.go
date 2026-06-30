//go:build integration

package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests prove the H-1 fix at the SQL layer: GetNetBalanceForRange bounds
// the net balance to a single fiscal year while GetNetBalance returns the
// lifetime balance (the bug the year-end close used to sweep). They require a
// Postgres reachable at ACCOUNTING_TEST_DSN and are excluded from the default
// build (`go test`) via the `integration` build tag. Run with:
//
//	ACCOUNTING_TEST_DSN=postgres://user:pass@host:port/db go test -tags=integration ./internal/accounting/repository/...

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ACCOUNTING_TEST_DSN")
	if dsn == "" {
		t.Skip("ACCOUNTING_TEST_DSN not set; skipping DB-backed integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))
	return pool
}

// setupNetBalanceSchema creates the minimal slice of schema the net-balance
// queries touch (journal_entries + journal_lines), isolated per test run.
func setupNetBalanceSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS journal_lines;
		DROP TABLE IF EXISTS journal_entries;
		CREATE TABLE journal_entries (
			id           UUID PRIMARY KEY,
			tenant_id    VARCHAR(50)  NOT NULL,
			status       VARCHAR(20)  NOT NULL,
			entry_date   DATE         NOT NULL
		);
		CREATE TABLE journal_lines (
			id            UUID PRIMARY KEY,
			entry_id      UUID NOT NULL REFERENCES journal_entries(id),
			tenant_id     VARCHAR(50) NOT NULL,
			account_id    UUID NOT NULL,
			debit_amount  NUMERIC(18,2) NOT NULL DEFAULT 0,
			credit_amount NUMERIC(18,2) NOT NULL DEFAULT 0
		);`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS journal_lines; DROP TABLE IF EXISTS journal_entries;`)
	})
}

// seedLine inserts a POSTED (or DRAFT) journal entry with a single line for the
// given account on the given date.
func seedLine(t *testing.T, pool *pgxpool.Pool, tenantID, status string, date time.Time, accountID uuid.UUID, debit, credit decimal.Decimal) {
	t.Helper()
	ctx := context.Background()
	entryID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO journal_entries (id, tenant_id, status, entry_date) VALUES ($1,$2,$3,$4)`,
		entryID, tenantID, status, date)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO journal_lines (id, entry_id, tenant_id, account_id, debit_amount, credit_amount)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		uuid.New(), entryID, tenantID, accountID, debit, credit)
	require.NoError(t, err)
}

// TestGetNetBalanceForRange_BoundsByFiscalYear seeds the SAME income account in
// 2023, 2024 and 2025 and asserts the date-bounded query returns only the
// target year's net, while the unbounded query returns the lifetime net.
func TestGetNetBalanceForRange_BoundsByFiscalYear(t *testing.T) {
	pool := newTestPool(t)
	defer pool.Close()
	setupNetBalanceSchema(t, pool)

	repo := New(pool)
	ctx := context.Background()
	tenant := "tenant-h1"
	income := uuid.New()

	// Income account: credit balances (income earned) in three different years.
	seedLine(t, pool, tenant, "POSTED", time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC), income, decimal.Zero, decimal.NewFromInt(3000))
	seedLine(t, pool, tenant, "POSTED", time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), income, decimal.Zero, decimal.NewFromInt(5000))
	seedLine(t, pool, tenant, "POSTED", time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC), income, decimal.Zero, decimal.NewFromInt(3000)) // boundary day
	seedLine(t, pool, tenant, "POSTED", time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC), income, decimal.Zero, decimal.NewFromInt(1000))
	// A DRAFT entry in 2024 must be ignored by both queries.
	seedLine(t, pool, tenant, "DRAFT", time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), income, decimal.Zero, decimal.NewFromInt(9999))

	y2024Start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	y2025Start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	y2023Start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	// 2024 only: -(5000 + 3000) = -8000, including the 31-Dec boundary entry.
	net2024, err := repo.GetNetBalanceForRange(ctx, income, tenant, y2024Start, y2025Start)
	require.NoError(t, err)
	assert.True(t, net2024.Equal(decimal.NewFromInt(-8000)), "2024 net should be -8000, got %s", net2024)

	// 2023 only: -3000.
	net2023, err := repo.GetNetBalanceForRange(ctx, income, tenant, y2023Start, y2024Start)
	require.NoError(t, err)
	assert.True(t, net2023.Equal(decimal.NewFromInt(-3000)), "2023 net should be -3000, got %s", net2023)

	// Lifetime (the old, buggy behaviour the close relied on): -(3000+5000+3000+1000) = -12000.
	lifetime, err := repo.GetNetBalance(ctx, income, tenant)
	require.NoError(t, err)
	assert.True(t, lifetime.Equal(decimal.NewFromInt(-12000)),
		"unbounded net should be lifetime -12000, got %s", lifetime)
}

// TestHasPostedEntriesInRange verifies the prior-year activity probe used by the
// sequential-close guard.
func TestHasPostedEntriesInRange(t *testing.T) {
	pool := newTestPool(t)
	defer pool.Close()
	setupNetBalanceSchema(t, pool)

	repo := New(pool)
	ctx := context.Background()
	tenant := "tenant-h1b"
	acct := uuid.New()

	seedLine(t, pool, tenant, "POSTED", time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), acct, decimal.NewFromInt(100), decimal.Zero)

	has2024, err := repo.HasPostedEntriesInRange(ctx, tenant,
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.True(t, has2024, "2024 has posted activity")

	has2022, err := repo.HasPostedEntriesInRange(ctx, tenant,
		time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.False(t, has2022, "2022 has no posted activity")
}
