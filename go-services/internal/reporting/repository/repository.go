package repository

import (
	"context"
	"fmt"
	"github.com/athena-lms/go-services/internal/common/market"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/reporting/model"
)

// Repository provides data access for reporting entities.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new Repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ---------- ReportEvent ----------

// InsertEvent inserts a new report event.
func (r *Repository) InsertEvent(ctx context.Context, e *model.ReportEvent) error {
	e.ID = uuid.New()
	now := time.Now().UTC()
	if e.OccurredAt.IsZero() {
		e.OccurredAt = now
	}
	e.CreatedAt = now
	if e.Currency == "" {
		e.Currency = market.Currency()
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO report_events
			(id, tenant_id, event_id, event_type, event_category, source_service,
			 subject_id, customer_id, amount, currency, payload, occurred_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		e.ID, e.TenantID, e.EventID, e.EventType, e.EventCategory, e.SourceService,
		e.SubjectID, e.CustomerID, e.Amount, e.Currency, e.Payload, e.OccurredAt, e.CreatedAt,
	)
	return err
}

// FindEventsByTenant returns paginated events for a tenant ordered by occurred_at DESC.
func (r *Repository) FindEventsByTenant(ctx context.Context, tenantID string, offset, limit int) ([]*model.ReportEvent, int64, error) {
	total, err := r.countQuery(ctx, `SELECT COUNT(*) FROM report_events WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, event_id, event_type, event_category, source_service,
		        subject_id, customer_id, amount, currency, payload, occurred_at, created_at
		 FROM report_events WHERE tenant_id = $1
		 ORDER BY occurred_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	events, err := scanEvents(rows)
	return events, total, err
}

// FindEventsByTenantAndType returns paginated events filtered by event type.
func (r *Repository) FindEventsByTenantAndType(ctx context.Context, tenantID, eventType string, offset, limit int) ([]*model.ReportEvent, int64, error) {
	total, err := r.countQuery(ctx,
		`SELECT COUNT(*) FROM report_events WHERE tenant_id = $1 AND event_type = $2`,
		tenantID, eventType,
	)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, event_id, event_type, event_category, source_service,
		        subject_id, customer_id, amount, currency, payload, occurred_at, created_at
		 FROM report_events WHERE tenant_id = $1 AND event_type = $2
		 ORDER BY occurred_at DESC LIMIT $3 OFFSET $4`,
		tenantID, eventType, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	events, err := scanEvents(rows)
	return events, total, err
}

// FindEventsByTenantAndTimeRange returns paginated events within a time range.
func (r *Repository) FindEventsByTenantAndTimeRange(ctx context.Context, tenantID string, from, to time.Time, offset, limit int) ([]*model.ReportEvent, int64, error) {
	total, err := r.countQuery(ctx,
		`SELECT COUNT(*) FROM report_events WHERE tenant_id = $1 AND occurred_at BETWEEN $2 AND $3`,
		tenantID, from, to,
	)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, event_id, event_type, event_category, source_service,
		        subject_id, customer_id, amount, currency, payload, occurred_at, created_at
		 FROM report_events WHERE tenant_id = $1 AND occurred_at BETWEEN $2 AND $3
		 ORDER BY occurred_at DESC LIMIT $4 OFFSET $5`,
		tenantID, from, to, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	events, err := scanEvents(rows)
	return events, total, err
}

func scanEvents(rows pgx.Rows) ([]*model.ReportEvent, error) {
	defer rows.Close()
	var result []*model.ReportEvent
	for rows.Next() {
		e := &model.ReportEvent{}
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.EventID, &e.EventType, &e.EventCategory,
			&e.SourceService, &e.SubjectID, &e.CustomerID, &e.Amount,
			&e.Currency, &e.Payload, &e.OccurredAt, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan report event: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// ---------- PortfolioSnapshot ----------

// UpsertSnapshot inserts or updates a portfolio snapshot for the given tenant/date.
func (r *Repository) UpsertSnapshot(ctx context.Context, s *model.PortfolioSnapshot) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	if s.Period == "" {
		s.Period = string(model.SnapshotPeriodDaily)
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO portfolio_snapshots
			(id, tenant_id, snapshot_date, period, total_loans, active_loans, closed_loans,
			 defaulted_loans, total_disbursed, total_outstanding, total_collected,
			 watch_loans, substandard_loans, doubtful_loans, loss_loans, par30, par90, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		 ON CONFLICT (tenant_id, snapshot_date, period) DO UPDATE SET
			total_loans = EXCLUDED.total_loans,
			active_loans = EXCLUDED.active_loans,
			closed_loans = EXCLUDED.closed_loans,
			defaulted_loans = EXCLUDED.defaulted_loans,
			total_disbursed = EXCLUDED.total_disbursed,
			total_outstanding = EXCLUDED.total_outstanding,
			total_collected = EXCLUDED.total_collected,
			watch_loans = EXCLUDED.watch_loans,
			substandard_loans = EXCLUDED.substandard_loans,
			doubtful_loans = EXCLUDED.doubtful_loans,
			loss_loans = EXCLUDED.loss_loans,
			par30 = EXCLUDED.par30,
			par90 = EXCLUDED.par90`,
		s.ID, s.TenantID, s.SnapshotDate, s.Period, s.TotalLoans, s.ActiveLoans,
		s.ClosedLoans, s.DefaultedLoans, s.TotalDisbursed, s.TotalOutstanding,
		s.TotalCollected, s.WatchLoans, s.SubstandardLoans, s.DoubtfulLoans,
		s.LossLoans, s.Par30, s.Par90, s.CreatedAt,
	)
	return err
}

// FindSnapshotsByTenant returns paginated snapshots ordered by date DESC.
func (r *Repository) FindSnapshotsByTenant(ctx context.Context, tenantID string, offset, limit int) ([]*model.PortfolioSnapshot, int64, error) {
	total, err := r.countQuery(ctx, `SELECT COUNT(*) FROM portfolio_snapshots WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, snapshot_date, period, total_loans, active_loans, closed_loans,
		        defaulted_loans, total_disbursed, total_outstanding, total_collected,
		        watch_loans, substandard_loans, doubtful_loans, loss_loans, par30, par90, created_at
		 FROM portfolio_snapshots WHERE tenant_id = $1
		 ORDER BY snapshot_date DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	snapshots, err := scanSnapshots(rows)
	return snapshots, total, err
}

// FindLatestSnapshot returns the most recent snapshot for a tenant.
func (r *Repository) FindLatestSnapshot(ctx context.Context, tenantID string) (*model.PortfolioSnapshot, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, snapshot_date, period, total_loans, active_loans, closed_loans,
		        defaulted_loans, total_disbursed, total_outstanding, total_collected,
		        watch_loans, substandard_loans, doubtful_loans, loss_loans, par30, par90, created_at
		 FROM portfolio_snapshots WHERE tenant_id = $1
		 ORDER BY snapshot_date DESC LIMIT 1`,
		tenantID,
	)

	s := &model.PortfolioSnapshot{}
	err := row.Scan(
		&s.ID, &s.TenantID, &s.SnapshotDate, &s.Period, &s.TotalLoans, &s.ActiveLoans,
		&s.ClosedLoans, &s.DefaultedLoans, &s.TotalDisbursed, &s.TotalOutstanding,
		&s.TotalCollected, &s.WatchLoans, &s.SubstandardLoans, &s.DoubtfulLoans,
		&s.LossLoans, &s.Par30, &s.Par90, &s.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan latest snapshot: %w", err)
	}
	return s, nil
}

// FindLatestSnapshotByDate returns the latest snapshot for a tenant on a specific date.
func (r *Repository) FindLatestSnapshotByDate(ctx context.Context, tenantID string, date time.Time) (*model.PortfolioSnapshot, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, snapshot_date, period, total_loans, active_loans, closed_loans,
		        defaulted_loans, total_disbursed, total_outstanding, total_collected,
		        watch_loans, substandard_loans, doubtful_loans, loss_loans, par30, par90, created_at
		 FROM portfolio_snapshots WHERE tenant_id = $1 AND snapshot_date = $2
		 ORDER BY created_at DESC LIMIT 1`,
		tenantID, date,
	)

	s := &model.PortfolioSnapshot{}
	err := row.Scan(
		&s.ID, &s.TenantID, &s.SnapshotDate, &s.Period, &s.TotalLoans, &s.ActiveLoans,
		&s.ClosedLoans, &s.DefaultedLoans, &s.TotalDisbursed, &s.TotalOutstanding,
		&s.TotalCollected, &s.WatchLoans, &s.SubstandardLoans, &s.DoubtfulLoans,
		&s.LossLoans, &s.Par30, &s.Par90, &s.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan snapshot by date: %w", err)
	}
	return s, nil
}

func scanSnapshots(rows pgx.Rows) ([]*model.PortfolioSnapshot, error) {
	defer rows.Close()
	var result []*model.PortfolioSnapshot
	for rows.Next() {
		s := &model.PortfolioSnapshot{}
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.SnapshotDate, &s.Period, &s.TotalLoans, &s.ActiveLoans,
			&s.ClosedLoans, &s.DefaultedLoans, &s.TotalDisbursed, &s.TotalOutstanding,
			&s.TotalCollected, &s.WatchLoans, &s.SubstandardLoans, &s.DoubtfulLoans,
			&s.LossLoans, &s.Par30, &s.Par90, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan portfolio snapshot: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// ---------- EventMetric ----------

// UpsertMetric increments the event count and adds to total amount for
// the given tenant/date/eventType combination.
func (r *Repository) UpsertMetric(ctx context.Context, tenantID string, metricDate time.Time, eventType string, amount decimal.Decimal) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO event_metrics (id, tenant_id, metric_date, event_type, event_count, total_amount, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 1, $5, $6, $6)
		 ON CONFLICT (tenant_id, metric_date, event_type) DO UPDATE SET
			event_count = event_metrics.event_count + 1,
			total_amount = event_metrics.total_amount + $5,
			updated_at = $6`,
		uuid.New(), tenantID, metricDate, eventType, amount, now,
	)
	return err
}

// FindMetricsByTenantAndDate returns metrics for a tenant on a specific date.
func (r *Repository) FindMetricsByTenantAndDate(ctx context.Context, tenantID string, date time.Time) ([]*model.EventMetric, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, metric_date, event_type, event_count, total_amount, created_at, updated_at
		 FROM event_metrics WHERE tenant_id = $1 AND metric_date = $2`,
		tenantID, date,
	)
	if err != nil {
		return nil, err
	}
	return scanMetrics(rows)
}

// FindMetricsByTenantAndDateRange returns metrics for a tenant within a date range.
func (r *Repository) FindMetricsByTenantAndDateRange(ctx context.Context, tenantID string, from, to time.Time) ([]*model.EventMetric, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, metric_date, event_type, event_count, total_amount, created_at, updated_at
		 FROM event_metrics WHERE tenant_id = $1 AND metric_date BETWEEN $2 AND $3`,
		tenantID, from, to,
	)
	if err != nil {
		return nil, err
	}
	return scanMetrics(rows)
}

// FindMetricByKey returns a single metric by the unique key.
func (r *Repository) FindMetricByKey(ctx context.Context, tenantID string, metricDate time.Time, eventType string) (*model.EventMetric, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, metric_date, event_type, event_count, total_amount, created_at, updated_at
		 FROM event_metrics WHERE tenant_id = $1 AND metric_date = $2 AND event_type = $3`,
		tenantID, metricDate, eventType,
	)
	m := &model.EventMetric{}
	err := row.Scan(&m.ID, &m.TenantID, &m.MetricDate, &m.EventType, &m.EventCount, &m.TotalAmount, &m.CreatedAt, &m.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan event metric: %w", err)
	}
	return m, nil
}

func scanMetrics(rows pgx.Rows) ([]*model.EventMetric, error) {
	defer rows.Close()
	var result []*model.EventMetric
	for rows.Next() {
		m := &model.EventMetric{}
		if err := rows.Scan(
			&m.ID, &m.TenantID, &m.MetricDate, &m.EventType,
			&m.EventCount, &m.TotalAmount, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan event metric: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// ---------- Helpers ----------

func (r *Repository) countQuery(ctx context.Context, query string, args ...any) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	return count, err
}
