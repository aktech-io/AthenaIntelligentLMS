package repository

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"
)

// PortfolioStats is a live aggregate of the loan book for a tenant.
type PortfolioStats struct {
	TotalLoans       int             `json:"totalLoans"`
	ActiveLoans      int             `json:"activeLoans"`
	ClosedLoans      int             `json:"closedLoans"`
	DefaultedLoans   int             `json:"defaultedLoans"`
	TotalDisbursed   decimal.Decimal `json:"totalDisbursed"`
	TotalOutstanding decimal.Decimal `json:"totalOutstanding"`
}

// GetPortfolioStats computes live portfolio totals grouped by loan status.
func (r *Repository) GetPortfolioStats(ctx context.Context, tenantID string) (*PortfolioStats, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT status, COUNT(*),
		        COALESCE(SUM(disbursed_amount), 0),
		        COALESCE(SUM(outstanding_principal), 0)
		 FROM loans WHERE tenant_id = $1 GROUP BY status`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := &PortfolioStats{TotalDisbursed: decimal.Zero, TotalOutstanding: decimal.Zero}
	for rows.Next() {
		var status string
		var count int
		var disbursed, outstanding decimal.Decimal
		if err := rows.Scan(&status, &count, &disbursed, &outstanding); err != nil {
			return nil, err
		}
		stats.TotalLoans += count
		stats.TotalDisbursed = stats.TotalDisbursed.Add(disbursed)
		stats.TotalOutstanding = stats.TotalOutstanding.Add(outstanding)
		switch strings.ToUpper(status) {
		case "ACTIVE", "RESTRUCTURED":
			stats.ActiveLoans += count
		case "CLOSED", "PAID_OFF", "SETTLED":
			stats.ClosedLoans += count
		case "WRITTEN_OFF", "DEFAULTED":
			stats.DefaultedLoans += count
		}
	}
	return stats, rows.Err()
}
