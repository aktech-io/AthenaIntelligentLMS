package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/fraud/model"
)

// Repository provides data access for all fraud-detection entities.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new Repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ─── Fraud Alert ────────────────────────────────────────────────────────────

func (r *Repository) CreateAlert(ctx context.Context, a *model.FraudAlert) error {
	a.ID = uuid.New()
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now

	explanationBytes, _ := json.Marshal(a.Explanation)
	if a.Explanation == nil {
		explanationBytes = nil
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO fraud_alerts (id, tenant_id, alert_type, severity, status, source,
			rule_code, customer_id, subject_type, subject_id, description,
			trigger_event, trigger_amount, risk_score, model_version, explanation,
			escalated, escalated_to_compliance, compliance_alert_id,
			assigned_to, resolved_by, resolved_at, resolution, resolution_notes,
			created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)`,
		a.ID, a.TenantID, a.AlertType, a.Severity, a.Status, a.Source,
		a.RuleCode, a.CustomerID, a.SubjectType, a.SubjectID, a.Description,
		a.TriggerEvent, a.TriggerAmount, a.RiskScore, a.ModelVersion, explanationBytes,
		a.Escalated, a.EscalatedToCompliance, a.ComplianceAlertID,
		a.AssignedTo, a.ResolvedBy, a.ResolvedAt, a.Resolution, a.ResolutionNotes,
		a.CreatedAt, a.UpdatedAt)
	return err
}

func (r *Repository) UpdateAlert(ctx context.Context, a *model.FraudAlert) error {
	a.UpdatedAt = time.Now()
	explanationBytes, _ := json.Marshal(a.Explanation)
	if a.Explanation == nil {
		explanationBytes = nil
	}

	_, err := r.pool.Exec(ctx, `
		UPDATE fraud_alerts SET severity=$1, status=$2, risk_score=$3, model_version=$4,
			explanation=$5, escalated=$6, escalated_to_compliance=$7, compliance_alert_id=$8,
			assigned_to=$9, resolved_by=$10, resolved_at=$11, resolution=$12, resolution_notes=$13,
			updated_at=$14
		WHERE id=$15`,
		a.Severity, a.Status, a.RiskScore, a.ModelVersion,
		explanationBytes, a.Escalated, a.EscalatedToCompliance, a.ComplianceAlertID,
		a.AssignedTo, a.ResolvedBy, a.ResolvedAt, a.Resolution, a.ResolutionNotes,
		a.UpdatedAt, a.ID)
	return err
}

func (r *Repository) GetAlert(ctx context.Context, id uuid.UUID) (*model.FraudAlert, error) {
	a := &model.FraudAlert{}
	var explanation []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, alert_type, severity, status, source,
			rule_code, customer_id, subject_type, subject_id, description,
			trigger_event, trigger_amount, risk_score, model_version, explanation,
			escalated, escalated_to_compliance, compliance_alert_id,
			assigned_to, resolved_by, resolved_at, resolution, resolution_notes,
			created_at, updated_at
		FROM fraud_alerts WHERE id=$1`, id).Scan(
		&a.ID, &a.TenantID, &a.AlertType, &a.Severity, &a.Status, &a.Source,
		&a.RuleCode, &a.CustomerID, &a.SubjectType, &a.SubjectID, &a.Description,
		&a.TriggerEvent, &a.TriggerAmount, &a.RiskScore, &a.ModelVersion, &explanation,
		&a.Escalated, &a.EscalatedToCompliance, &a.ComplianceAlertID,
		&a.AssignedTo, &a.ResolvedBy, &a.ResolvedAt, &a.Resolution, &a.ResolutionNotes,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.Explanation = explanation
	return a, nil
}

func (r *Repository) ListAlerts(ctx context.Context, tenantID string, status *model.AlertStatus, page, size int) ([]*model.FraudAlert, int64, error) {
	var countQuery, query string
	var args []interface{}
	argIdx := 1

	if status != nil {
		countQuery = fmt.Sprintf("SELECT COUNT(*) FROM fraud_alerts WHERE tenant_id=$%d AND status=$%d", argIdx, argIdx+1)
		query = fmt.Sprintf(`SELECT id, tenant_id, alert_type, severity, status, source,
			rule_code, customer_id, subject_type, subject_id, description,
			trigger_event, trigger_amount, risk_score, model_version, explanation,
			escalated, escalated_to_compliance, compliance_alert_id,
			assigned_to, resolved_by, resolved_at, resolution, resolution_notes,
			created_at, updated_at
		FROM fraud_alerts WHERE tenant_id=$%d AND status=$%d ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
			argIdx, argIdx+1, argIdx+2, argIdx+3)
		args = []interface{}{tenantID, *status, size, page * size}
	} else {
		countQuery = fmt.Sprintf("SELECT COUNT(*) FROM fraud_alerts WHERE tenant_id=$%d", argIdx)
		query = fmt.Sprintf(`SELECT id, tenant_id, alert_type, severity, status, source,
			rule_code, customer_id, subject_type, subject_id, description,
			trigger_event, trigger_amount, risk_score, model_version, explanation,
			escalated, escalated_to_compliance, compliance_alert_id,
			assigned_to, resolved_by, resolved_at, resolution, resolution_notes,
			created_at, updated_at
		FROM fraud_alerts WHERE tenant_id=$%d ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
			argIdx, argIdx+1, argIdx+2)
		args = []interface{}{tenantID, size, page * size}
	}

	var total int64
	if status != nil {
		r.pool.QueryRow(ctx, countQuery, tenantID, *status).Scan(&total)
	} else {
		r.pool.QueryRow(ctx, countQuery, tenantID).Scan(&total)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return scanAlerts(rows, total)
}

func (r *Repository) ListCustomerAlerts(ctx context.Context, tenantID, customerID string, page, size int) ([]*model.FraudAlert, int64, error) {
	var total int64
	r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM fraud_alerts WHERE tenant_id=$1 AND customer_id=$2", tenantID, customerID).Scan(&total)

	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, alert_type, severity, status, source,
			rule_code, customer_id, subject_type, subject_id, description,
			trigger_event, trigger_amount, risk_score, model_version, explanation,
			escalated, escalated_to_compliance, compliance_alert_id,
			assigned_to, resolved_by, resolved_at, resolution, resolution_notes,
			created_at, updated_at
		FROM fraud_alerts WHERE tenant_id=$1 AND customer_id=$2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`,
		tenantID, customerID, size, page*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return scanAlerts(rows, total)
}

func (r *Repository) CountAlertsByStatus(ctx context.Context, tenantID string, status model.AlertStatus) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM fraud_alerts WHERE tenant_id=$1 AND status=$2", tenantID, status).Scan(&count)
	return count, err
}

func (r *Repository) CountAlertsBySeverityAndStatus(ctx context.Context, tenantID string, severity model.AlertSeverity, status model.AlertStatus) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM fraud_alerts WHERE tenant_id=$1 AND severity=$2 AND status=$3", tenantID, severity, status).Scan(&count)
	return count, err
}

func (r *Repository) CountOpenAlertsByCustomer(ctx context.Context, tenantID, customerID string) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM fraud_alerts WHERE tenant_id=$1 AND customer_id=$2 AND status='OPEN'", tenantID, customerID).Scan(&count)
	return count, err
}

func (r *Repository) CountRecentAlertsByRule(ctx context.Context, tenantID, customerID, ruleCode string, since time.Time) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM fraud_alerts WHERE tenant_id=$1 AND customer_id=$2 AND rule_code=$3 AND created_at > $4`,
		tenantID, customerID, ruleCode, since).Scan(&count)
	return count, err
}

func (r *Repository) CountAlerts(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM fraud_alerts WHERE tenant_id=$1", tenantID).Scan(&count)
	return count, err
}

func (r *Repository) CountResolved(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM fraud_alerts WHERE tenant_id=$1 AND status IN ('CONFIRMED_FRAUD','FALSE_POSITIVE')", tenantID).Scan(&count)
	return count, err
}

type RuleCount struct {
	RuleCode string
	Count    int64
}

func (r *Repository) CountByRule(ctx context.Context, tenantID string) ([]RuleCount, error) {
	rows, err := r.pool.Query(ctx, `SELECT rule_code, COUNT(*) FROM fraud_alerts WHERE tenant_id=$1 AND rule_code IS NOT NULL GROUP BY rule_code ORDER BY COUNT(*) DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RuleCount
	for rows.Next() {
		var rc RuleCount
		if err := rows.Scan(&rc.RuleCode, &rc.Count); err != nil {
			return nil, err
		}
		result = append(result, rc)
	}
	return result, nil
}

func (r *Repository) CountConfirmedByRule(ctx context.Context, tenantID string) ([]RuleCount, error) {
	rows, err := r.pool.Query(ctx, `SELECT rule_code, COUNT(*) FROM fraud_alerts WHERE tenant_id=$1 AND status='CONFIRMED_FRAUD' AND rule_code IS NOT NULL GROUP BY rule_code`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RuleCount
	for rows.Next() {
		var rc RuleCount
		if err := rows.Scan(&rc.RuleCode, &rc.Count); err != nil {
			return nil, err
		}
		result = append(result, rc)
	}
	return result, nil
}

func (r *Repository) CountFalsePositiveByRule(ctx context.Context, tenantID string) ([]RuleCount, error) {
	rows, err := r.pool.Query(ctx, `SELECT rule_code, COUNT(*) FROM fraud_alerts WHERE tenant_id=$1 AND status='FALSE_POSITIVE' AND rule_code IS NOT NULL GROUP BY rule_code`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RuleCount
	for rows.Next() {
		var rc RuleCount
		if err := rows.Scan(&rc.RuleCode, &rc.Count); err != nil {
			return nil, err
		}
		result = append(result, rc)
	}
	return result, nil
}

func (r *Repository) CountByDay(ctx context.Context, tenantID string, since time.Time) ([]model.DailyAlertCount, error) {
	rows, err := r.pool.Query(ctx, `SELECT CAST(created_at AS date), COUNT(*) FROM fraud_alerts WHERE tenant_id=$1 AND created_at > $2 GROUP BY CAST(created_at AS date) ORDER BY CAST(created_at AS date)`, tenantID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.DailyAlertCount
	for rows.Next() {
		var d model.DailyAlertCount
		var dt time.Time
		if err := rows.Scan(&dt, &d.Count); err != nil {
			return nil, err
		}
		d.Date = dt.Format("2006-01-02")
		result = append(result, d)
	}
	return result, nil
}

func (r *Repository) CountByAlertType(ctx context.Context, tenantID string, since time.Time) ([]model.TypeCount, error) {
	rows, err := r.pool.Query(ctx, `SELECT alert_type, COUNT(*) FROM fraud_alerts WHERE tenant_id=$1 AND created_at > $2 GROUP BY alert_type ORDER BY COUNT(*) DESC`, tenantID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.TypeCount
	for rows.Next() {
		var tc model.TypeCount
		if err := rows.Scan(&tc.Type, &tc.Count); err != nil {
			return nil, err
		}
		result = append(result, tc)
	}
	return result, nil
}

func scanAlerts(rows pgx.Rows, total int64) ([]*model.FraudAlert, int64, error) {
	var alerts []*model.FraudAlert
	for rows.Next() {
		a := &model.FraudAlert{}
		var explanation []byte
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.AlertType, &a.Severity, &a.Status, &a.Source,
			&a.RuleCode, &a.CustomerID, &a.SubjectType, &a.SubjectID, &a.Description,
			&a.TriggerEvent, &a.TriggerAmount, &a.RiskScore, &a.ModelVersion, &explanation,
			&a.Escalated, &a.EscalatedToCompliance, &a.ComplianceAlertID,
			&a.AssignedTo, &a.ResolvedBy, &a.ResolvedAt, &a.Resolution, &a.ResolutionNotes,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		a.Explanation = explanation
		alerts = append(alerts, a)
	}
	return alerts, total, nil
}

// ─── Fraud Event ────────────────────────────────────────────────────────────

func (r *Repository) CreateEvent(ctx context.Context, e *model.FraudEvent) error {
	e.ID = uuid.New()
	if e.ProcessedAt.IsZero() {
		e.ProcessedAt = time.Now()
	}
	payloadBytes, _ := json.Marshal(e.Payload)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO fraud_events (id, tenant_id, event_type, source_service, customer_id,
			subject_id, amount, risk_score, rules_triggered, payload, processed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		e.ID, e.TenantID, e.EventType, e.SourceService, e.CustomerID,
		e.SubjectID, e.Amount, e.RiskScore, e.RulesTriggered, payloadBytes, e.ProcessedAt)
	return err
}

func (r *Repository) ListRecentEvents(ctx context.Context, tenantID string, page, size int) ([]*model.FraudEvent, int64, error) {
	var total int64
	r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM fraud_events WHERE tenant_id=$1", tenantID).Scan(&total)

	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, event_type, source_service, customer_id, subject_id, amount, risk_score, rules_triggered, payload, processed_at
		FROM fraud_events WHERE tenant_id=$1 ORDER BY processed_at DESC LIMIT $2 OFFSET $3`, tenantID, size, page*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var events []*model.FraudEvent
	for rows.Next() {
		e := &model.FraudEvent{}
		var payload []byte
		if err := rows.Scan(&e.ID, &e.TenantID, &e.EventType, &e.SourceService, &e.CustomerID,
			&e.SubjectID, &e.Amount, &e.RiskScore, &e.RulesTriggered, &payload, &e.ProcessedAt); err != nil {
			return nil, 0, err
		}
		e.Payload = payload
		events = append(events, e)
	}
	return events, total, nil
}

// ─── Fraud Rule ─────────────────────────────────────────────────────────────

func (r *Repository) FindActiveRules(ctx context.Context, tenantID string) ([]*model.FraudRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, rule_code, rule_name, description, category, severity, event_types, enabled, parameters, created_at, updated_at
		FROM fraud_rules WHERE (tenant_id=$1 OR tenant_id='*') AND enabled=true ORDER BY category, rule_code`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRules(rows)
}

func (r *Repository) FindAllRules(ctx context.Context, tenantID string) ([]*model.FraudRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, rule_code, rule_name, description, category, severity, event_types, enabled, parameters, created_at, updated_at
		FROM fraud_rules WHERE tenant_id=$1 OR tenant_id='*' ORDER BY category, rule_code`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRules(rows)
}

func (r *Repository) GetRule(ctx context.Context, id uuid.UUID) (*model.FraudRule, error) {
	rule := &model.FraudRule{}
	var params []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, rule_code, rule_name, description, category, severity, event_types, enabled, parameters, created_at, updated_at
		FROM fraud_rules WHERE id=$1`, id).Scan(
		&rule.ID, &rule.TenantID, &rule.RuleCode, &rule.RuleName, &rule.Description,
		&rule.Category, &rule.Severity, &rule.EventTypes, &rule.Enabled, &params,
		&rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if params != nil {
		json.Unmarshal(params, &rule.Parameters)
	}
	return rule, nil
}

func (r *Repository) UpdateRule(ctx context.Context, rule *model.FraudRule) error {
	rule.UpdatedAt = time.Now()
	paramsBytes, _ := json.Marshal(rule.Parameters)
	_, err := r.pool.Exec(ctx, `
		UPDATE fraud_rules SET severity=$1, enabled=$2, parameters=$3, updated_at=$4 WHERE id=$5`,
		rule.Severity, rule.Enabled, paramsBytes, rule.UpdatedAt, rule.ID)
	return err
}

func scanRules(rows pgx.Rows) ([]*model.FraudRule, error) {
	var rules []*model.FraudRule
	for rows.Next() {
		rule := &model.FraudRule{}
		var params []byte
		if err := rows.Scan(&rule.ID, &rule.TenantID, &rule.RuleCode, &rule.RuleName, &rule.Description,
			&rule.Category, &rule.Severity, &rule.EventTypes, &rule.Enabled, &params,
			&rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		if params != nil {
			json.Unmarshal(params, &rule.Parameters)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// ─── Velocity Counter ───────────────────────────────────────────────────────

func (r *Repository) FindVelocityCounter(ctx context.Context, tenantID, customerID, counterType string, windowStart time.Time) (*model.VelocityCounter, error) {
	c := &model.VelocityCounter{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, customer_id, counter_type, window_start, window_end, count, total_amount, created_at, updated_at
		FROM velocity_counters WHERE tenant_id=$1 AND customer_id=$2 AND counter_type=$3 AND window_start=$4`,
		tenantID, customerID, counterType, windowStart).Scan(
		&c.ID, &c.TenantID, &c.CustomerID, &c.CounterType, &c.WindowStart, &c.WindowEnd, &c.Count, &c.TotalAmount, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repository) UpsertVelocityCounter(ctx context.Context, c *model.VelocityCounter) error {
	now := time.Now()
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
		c.CreatedAt = now
	}
	c.UpdatedAt = now

	_, err := r.pool.Exec(ctx, `
		INSERT INTO velocity_counters (id, tenant_id, customer_id, counter_type, window_start, window_end, count, total_amount, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (tenant_id, customer_id, counter_type, window_start)
		DO UPDATE SET count=$7, total_amount=$8, updated_at=$10`,
		c.ID, c.TenantID, c.CustomerID, c.CounterType, c.WindowStart, c.WindowEnd, c.Count, c.TotalAmount, c.CreatedAt, c.UpdatedAt)
	return err
}

func (r *Repository) SumCountSince(ctx context.Context, tenantID, customerID, counterType string, since time.Time) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(count), 0) FROM velocity_counters
		WHERE tenant_id=$1 AND customer_id=$2 AND counter_type=$3 AND window_end > $4`,
		tenantID, customerID, counterType, since).Scan(&count)
	return count, err
}

func (r *Repository) SumAmountSince(ctx context.Context, tenantID, customerID, counterType string, since time.Time) (decimal.Decimal, error) {
	var amount decimal.Decimal
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_amount), 0) FROM velocity_counters
		WHERE tenant_id=$1 AND customer_id=$2 AND counter_type=$3 AND window_end > $4`,
		tenantID, customerID, counterType, since).Scan(&amount)
	return amount, err
}

// ─── Customer Risk Profile ──────────────────────────────────────────────────

func (r *Repository) GetRiskProfile(ctx context.Context, tenantID, customerID string) (*model.CustomerRiskProfile, error) {
	p := &model.CustomerRiskProfile{}
	var factors []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, customer_id, risk_score, risk_level, total_alerts, open_alerts,
			confirmed_fraud, false_positives, avg_transaction_amount, transaction_count_30d,
			last_alert_at, last_scored_at, factors, created_at, updated_at
		FROM customer_risk_profiles WHERE tenant_id=$1 AND customer_id=$2`,
		tenantID, customerID).Scan(
		&p.ID, &p.TenantID, &p.CustomerID, &p.RiskScore, &p.RiskLevel,
		&p.TotalAlerts, &p.OpenAlerts, &p.ConfirmedFraud, &p.FalsePositives,
		&p.AvgTransactionAmount, &p.TransactionCount30d, &p.LastAlertAt, &p.LastScoredAt,
		&factors, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if factors != nil {
		json.Unmarshal(factors, &p.Factors)
	}
	return p, nil
}

func (r *Repository) UpsertRiskProfile(ctx context.Context, p *model.CustomerRiskProfile) error {
	now := time.Now()
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	factorsBytes, _ := json.Marshal(p.Factors)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO customer_risk_profiles (id, tenant_id, customer_id, risk_score, risk_level,
			total_alerts, open_alerts, confirmed_fraud, false_positives,
			avg_transaction_amount, transaction_count_30d, last_alert_at, last_scored_at,
			factors, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (tenant_id, customer_id) DO UPDATE SET
			risk_score=$4, risk_level=$5, total_alerts=$6, open_alerts=$7,
			confirmed_fraud=$8, false_positives=$9, avg_transaction_amount=$10,
			transaction_count_30d=$11, last_alert_at=$12, last_scored_at=$13,
			factors=$14, updated_at=$16`,
		p.ID, p.TenantID, p.CustomerID, p.RiskScore, p.RiskLevel,
		p.TotalAlerts, p.OpenAlerts, p.ConfirmedFraud, p.FalsePositives,
		p.AvgTransactionAmount, p.TransactionCount30d, p.LastAlertAt, p.LastScoredAt,
		factorsBytes, p.CreatedAt, p.UpdatedAt)
	return err
}

func (r *Repository) CountRiskProfilesByLevel(ctx context.Context, tenantID string, level model.RiskLevel) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM customer_risk_profiles WHERE tenant_id=$1 AND risk_level=$2", tenantID, level).Scan(&count)
	return count, err
}

func (r *Repository) ListHighRiskCustomers(ctx context.Context, tenantID string, page, size int) ([]*model.CustomerRiskProfile, int64, error) {
	var total int64
	r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM customer_risk_profiles WHERE tenant_id=$1 AND risk_level='HIGH'", tenantID).Scan(&total)

	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, customer_id, risk_score, risk_level, total_alerts, open_alerts,
			confirmed_fraud, false_positives, avg_transaction_amount, transaction_count_30d,
			last_alert_at, last_scored_at, factors, created_at, updated_at
		FROM customer_risk_profiles WHERE tenant_id=$1 AND risk_level='HIGH'
		ORDER BY risk_score DESC LIMIT $2 OFFSET $3`, tenantID, size, page*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var profiles []*model.CustomerRiskProfile
	for rows.Next() {
		p := &model.CustomerRiskProfile{}
		var factors []byte
		if err := rows.Scan(&p.ID, &p.TenantID, &p.CustomerID, &p.RiskScore, &p.RiskLevel,
			&p.TotalAlerts, &p.OpenAlerts, &p.ConfirmedFraud, &p.FalsePositives,
			&p.AvgTransactionAmount, &p.TransactionCount30d, &p.LastAlertAt, &p.LastScoredAt,
			&factors, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if factors != nil {
			json.Unmarshal(factors, &p.Factors)
		}
		profiles = append(profiles, p)
	}
	return profiles, total, nil
}

func (r *Repository) ListAllRiskProfiles(ctx context.Context, tenantID string) ([]*model.CustomerRiskProfile, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, customer_id, risk_score, risk_level, total_alerts, open_alerts,
			confirmed_fraud, false_positives, avg_transaction_amount, transaction_count_30d,
			last_alert_at, last_scored_at, factors, created_at, updated_at
		FROM customer_risk_profiles WHERE tenant_id=$1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []*model.CustomerRiskProfile
	for rows.Next() {
		p := &model.CustomerRiskProfile{}
		var factors []byte
		if err := rows.Scan(&p.ID, &p.TenantID, &p.CustomerID, &p.RiskScore, &p.RiskLevel,
			&p.TotalAlerts, &p.OpenAlerts, &p.ConfirmedFraud, &p.FalsePositives,
			&p.AvgTransactionAmount, &p.TransactionCount30d, &p.LastAlertAt, &p.LastScoredAt,
			&factors, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if factors != nil {
			json.Unmarshal(factors, &p.Factors)
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// ─── Watchlist ──────────────────────────────────────────────────────────────

func (r *Repository) CreateWatchlistEntry(ctx context.Context, e *model.WatchlistEntry) error {
	e.ID = uuid.New()
	now := time.Now()
	e.CreatedAt = now
	e.UpdatedAt = now
	_, err := r.pool.Exec(ctx, `
		INSERT INTO watchlist_entries (id, tenant_id, list_type, entry_type, name, national_id, phone, reason, source, active, expires_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		e.ID, e.TenantID, e.ListType, e.EntryType, e.Name, e.NationalID, e.Phone, e.Reason, e.Source, e.Active, e.ExpiresAt, e.CreatedAt, e.UpdatedAt)
	return err
}

func (r *Repository) UpdateWatchlistEntry(ctx context.Context, e *model.WatchlistEntry) error {
	e.UpdatedAt = time.Now()
	_, err := r.pool.Exec(ctx, `UPDATE watchlist_entries SET active=$1, updated_at=$2 WHERE id=$3`, e.Active, e.UpdatedAt, e.ID)
	return err
}

func (r *Repository) GetWatchlistEntry(ctx context.Context, id uuid.UUID) (*model.WatchlistEntry, error) {
	e := &model.WatchlistEntry{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, list_type, entry_type, name, national_id, phone, reason, source, active, expires_at, created_at, updated_at
		FROM watchlist_entries WHERE id=$1`, id).Scan(
		&e.ID, &e.TenantID, &e.ListType, &e.EntryType, &e.Name, &e.NationalID, &e.Phone, &e.Reason, &e.Source, &e.Active, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

func (r *Repository) ListWatchlistEntries(ctx context.Context, tenantID string, active *bool, page, size int) ([]*model.WatchlistEntry, int64, error) {
	isActive := true
	if active != nil {
		isActive = *active
	}
	var total int64
	r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM watchlist_entries WHERE tenant_id=$1 AND active=$2", tenantID, isActive).Scan(&total)

	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, list_type, entry_type, name, national_id, phone, reason, source, active, expires_at, created_at, updated_at
		FROM watchlist_entries WHERE tenant_id=$1 AND active=$2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`,
		tenantID, isActive, size, page*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []*model.WatchlistEntry
	for rows.Next() {
		e := &model.WatchlistEntry{}
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ListType, &e.EntryType, &e.Name, &e.NationalID, &e.Phone, &e.Reason, &e.Source, &e.Active, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	return entries, total, nil
}

func (r *Repository) FindAllActiveWatchlistEntries(ctx context.Context, tenantID string) ([]*model.WatchlistEntry, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, tenant_id, list_type, entry_type, name, national_id, phone, reason, source, active, expires_at, created_at, updated_at FROM watchlist_entries WHERE tenant_id=$1 AND active=true`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []*model.WatchlistEntry
	for rows.Next() {
		e := &model.WatchlistEntry{}
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ListType, &e.EntryType, &e.Name, &e.NationalID, &e.Phone, &e.Reason, &e.Source, &e.Active, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (r *Repository) FindWatchlistMatches(ctx context.Context, tenantID, nationalID, name, phone string) ([]*model.WatchlistEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, list_type, entry_type, name, national_id, phone, reason, source, active, expires_at, created_at, updated_at
		FROM watchlist_entries
		WHERE (tenant_id=$1 OR tenant_id='*') AND active=true AND (expires_at IS NULL OR expires_at > NOW())
		AND (national_id=$2 OR name=$3 OR phone=$4)`,
		tenantID, nationalID, name, phone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []*model.WatchlistEntry
	for rows.Next() {
		e := &model.WatchlistEntry{}
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ListType, &e.EntryType, &e.Name, &e.NationalID, &e.Phone, &e.Reason, &e.Source, &e.Active, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (r *Repository) FindExpiredWatchlistEntries(ctx context.Context, now time.Time) ([]*model.WatchlistEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, list_type, entry_type, name, national_id, phone, reason, source, active, expires_at, created_at, updated_at
		FROM watchlist_entries WHERE active=true AND expires_at IS NOT NULL AND expires_at < $1`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []*model.WatchlistEntry
	for rows.Next() {
		e := &model.WatchlistEntry{}
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ListType, &e.EntryType, &e.Name, &e.NationalID, &e.Phone, &e.Reason, &e.Source, &e.Active, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// ─── Fraud Case ─────────────────────────────────────────────────────────────

func (r *Repository) CreateCase(ctx context.Context, c *model.FraudCase) error {
	c.ID = uuid.New()
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now

	tagsBytes, _ := json.Marshal(c.Tags)
	if c.Tags == nil {
		tagsBytes = []byte("[]")
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO fraud_cases (id, tenant_id, case_number, title, description, status, priority,
			customer_id, assigned_to, total_exposure, confirmed_loss, tags,
			sla_deadline, sla_breached, closed_at, closed_by, outcome, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		c.ID, c.TenantID, c.CaseNumber, c.Title, c.Description, c.Status, c.Priority,
		c.CustomerID, c.AssignedTo, c.TotalExposure, c.ConfirmedLoss, tagsBytes,
		c.SLADeadline, c.SLABreached, c.ClosedAt, c.ClosedBy, c.Outcome, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return err
	}

	// Insert alert IDs
	for _, alertID := range c.AlertIDs {
		r.pool.Exec(ctx, "INSERT INTO fraud_case_alert_ids (case_id, alert_id) VALUES ($1,$2) ON CONFLICT DO NOTHING", c.ID, alertID)
	}
	return nil
}

func (r *Repository) UpdateCase(ctx context.Context, c *model.FraudCase) error {
	c.UpdatedAt = time.Now()
	tagsBytes, _ := json.Marshal(c.Tags)
	if c.Tags == nil {
		tagsBytes = []byte("[]")
	}

	_, err := r.pool.Exec(ctx, `
		UPDATE fraud_cases SET status=$1, priority=$2, assigned_to=$3, total_exposure=$4,
			confirmed_loss=$5, tags=$6, sla_deadline=$7, sla_breached=$8,
			closed_at=$9, closed_by=$10, outcome=$11, updated_at=$12
		WHERE id=$13`,
		c.Status, c.Priority, c.AssignedTo, c.TotalExposure,
		c.ConfirmedLoss, tagsBytes, c.SLADeadline, c.SLABreached,
		c.ClosedAt, c.ClosedBy, c.Outcome, c.UpdatedAt, c.ID)
	return err
}

func (r *Repository) GetCase(ctx context.Context, id uuid.UUID) (*model.FraudCase, error) {
	c := &model.FraudCase{}
	var tags []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, case_number, title, description, status, priority,
			customer_id, assigned_to, total_exposure, confirmed_loss, tags,
			sla_deadline, sla_breached, closed_at, closed_by, outcome, created_at, updated_at
		FROM fraud_cases WHERE id=$1`, id).Scan(
		&c.ID, &c.TenantID, &c.CaseNumber, &c.Title, &c.Description, &c.Status, &c.Priority,
		&c.CustomerID, &c.AssignedTo, &c.TotalExposure, &c.ConfirmedLoss, &tags,
		&c.SLADeadline, &c.SLABreached, &c.ClosedAt, &c.ClosedBy, &c.Outcome, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.Tags = tags
	c.AlertIDs, _ = r.getCaseAlertIDs(ctx, id)
	return c, nil
}

func (r *Repository) ListCases(ctx context.Context, tenantID string, status *model.CaseStatus, page, size int) ([]*model.FraudCase, int64, error) {
	var total int64
	var rows pgx.Rows
	var err error

	if status != nil {
		r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM fraud_cases WHERE tenant_id=$1 AND status=$2", tenantID, *status).Scan(&total)
		rows, err = r.pool.Query(ctx, `
			SELECT id, tenant_id, case_number, title, description, status, priority,
				customer_id, assigned_to, total_exposure, confirmed_loss, tags,
				sla_deadline, sla_breached, closed_at, closed_by, outcome, created_at, updated_at
			FROM fraud_cases WHERE tenant_id=$1 AND status=$2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`,
			tenantID, *status, size, page*size)
	} else {
		r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM fraud_cases WHERE tenant_id=$1", tenantID).Scan(&total)
		rows, err = r.pool.Query(ctx, `
			SELECT id, tenant_id, case_number, title, description, status, priority,
				customer_id, assigned_to, total_exposure, confirmed_loss, tags,
				sla_deadline, sla_breached, closed_at, closed_by, outcome, created_at, updated_at
			FROM fraud_cases WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			tenantID, size, page*size)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var cases []*model.FraudCase
	for rows.Next() {
		c := &model.FraudCase{}
		var tags []byte
		if err := rows.Scan(&c.ID, &c.TenantID, &c.CaseNumber, &c.Title, &c.Description, &c.Status, &c.Priority,
			&c.CustomerID, &c.AssignedTo, &c.TotalExposure, &c.ConfirmedLoss, &tags,
			&c.SLADeadline, &c.SLABreached, &c.ClosedAt, &c.ClosedBy, &c.Outcome, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, err
		}
		c.Tags = tags
		c.AlertIDs, _ = r.getCaseAlertIDs(ctx, c.ID)
		cases = append(cases, c)
	}
	return cases, total, nil
}

func (r *Repository) FindMaxCaseNumber(ctx context.Context, tenantID string) (int, error) {
	var maxNum int
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(MAX(CAST(SUBSTRING(case_number FROM 5) AS INTEGER)), 0) FROM fraud_cases WHERE tenant_id=$1`, tenantID).Scan(&maxNum)
	return maxNum, err
}

func (r *Repository) CountActiveCases(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM fraud_cases WHERE tenant_id=$1 AND status NOT LIKE 'CLOSED%'`, tenantID).Scan(&count)
	return count, err
}

func (r *Repository) FindOverdueCases(ctx context.Context, now time.Time) ([]*model.FraudCase, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, case_number, title, description, status, priority,
			customer_id, assigned_to, total_exposure, confirmed_loss, tags,
			sla_deadline, sla_breached, closed_at, closed_by, outcome, created_at, updated_at
		FROM fraud_cases WHERE sla_deadline < $1 AND sla_breached=false AND status NOT LIKE 'CLOSED%'`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cases []*model.FraudCase
	for rows.Next() {
		c := &model.FraudCase{}
		var tags []byte
		if err := rows.Scan(&c.ID, &c.TenantID, &c.CaseNumber, &c.Title, &c.Description, &c.Status, &c.Priority,
			&c.CustomerID, &c.AssignedTo, &c.TotalExposure, &c.ConfirmedLoss, &tags,
			&c.SLADeadline, &c.SLABreached, &c.ClosedAt, &c.ClosedBy, &c.Outcome, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Tags = tags
		cases = append(cases, c)
	}
	return cases, nil
}

func (r *Repository) getCaseAlertIDs(ctx context.Context, caseID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, "SELECT alert_id FROM fraud_case_alert_ids WHERE case_id=$1", caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, nil
}

// ─── Case Note ──────────────────────────────────────────────────────────────

func (r *Repository) CreateCaseNote(ctx context.Context, n *model.CaseNote) error {
	n.ID = uuid.New()
	n.CreatedAt = time.Now()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO fraud_case_notes (id, case_id, tenant_id, author, content, note_type, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, n.ID, n.CaseID, n.TenantID, n.Author, n.Content, n.NoteType, n.CreatedAt)
	return err
}

func (r *Repository) ListCaseNotes(ctx context.Context, caseID uuid.UUID, tenantID string) ([]*model.CaseNote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, case_id, tenant_id, author, content, note_type, created_at
		FROM fraud_case_notes WHERE case_id=$1 AND tenant_id=$2 ORDER BY created_at DESC`, caseID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notes []*model.CaseNote
	for rows.Next() {
		n := &model.CaseNote{}
		if err := rows.Scan(&n.ID, &n.CaseID, &n.TenantID, &n.Author, &n.Content, &n.NoteType, &n.CreatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, nil
}

// ─── Audit Log ──────────────────────────────────────────────────────────────

func (r *Repository) CreateAuditLog(ctx context.Context, a *model.AuditLog) error {
	a.ID = uuid.New()
	a.CreatedAt = time.Now()
	changesBytes, _ := json.Marshal(a.Changes)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO fraud_audit_log (id, tenant_id, action, entity_type, entity_id, performed_by, description, changes, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		a.ID, a.TenantID, a.Action, a.EntityType, a.EntityID, a.PerformedBy, a.Description, changesBytes, a.CreatedAt)
	return err
}

func (r *Repository) ListAuditLog(ctx context.Context, tenantID string, entityType *string, entityID *uuid.UUID, page, size int) ([]*model.AuditLog, int64, error) {
	var total int64
	var rows pgx.Rows
	var err error

	if entityType != nil && entityID != nil {
		r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM fraud_audit_log WHERE tenant_id=$1 AND entity_type=$2 AND entity_id=$3", tenantID, *entityType, *entityID).Scan(&total)
		rows, err = r.pool.Query(ctx, `
			SELECT id, tenant_id, action, entity_type, entity_id, performed_by, description, changes, created_at
			FROM fraud_audit_log WHERE tenant_id=$1 AND entity_type=$2 AND entity_id=$3
			ORDER BY created_at DESC LIMIT $4 OFFSET $5`,
			tenantID, *entityType, *entityID, size, page*size)
	} else {
		r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM fraud_audit_log WHERE tenant_id=$1", tenantID).Scan(&total)
		rows, err = r.pool.Query(ctx, `
			SELECT id, tenant_id, action, entity_type, entity_id, performed_by, description, changes, created_at
			FROM fraud_audit_log WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			tenantID, size, page*size)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*model.AuditLog
	for rows.Next() {
		a := &model.AuditLog{}
		var changes []byte
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Action, &a.EntityType, &a.EntityID, &a.PerformedBy, &a.Description, &changes, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		if changes != nil {
			json.Unmarshal(changes, &a.Changes)
		}
		logs = append(logs, a)
	}
	return logs, total, nil
}

// ChainVerification reports the integrity of the tamper-evident audit chain.
type ChainVerification struct {
	Intact    bool   `json:"intact"`
	BrokenSeq *int64 `json:"brokenSeq,omitempty"`
	Total     int64  `json:"total"`
}

// VerifyAuditChain walks the hash chain for the tenant and reports whether it is
// intact or the seq of the first tampered/missing entry (see migration 000007).
func (r *Repository) VerifyAuditChain(ctx context.Context, tenantID string) (*ChainVerification, error) {
	v := &ChainVerification{}
	err := r.pool.QueryRow(ctx,
		`SELECT intact, broken_seq, total FROM audit_verify($1)`, tenantID,
	).Scan(&v.Intact, &v.BrokenSeq, &v.Total)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (r *Repository) ListAuditLogForEntityAsc(ctx context.Context, tenantID, entityType string, entityID uuid.UUID) ([]*model.AuditLog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, action, entity_type, entity_id, performed_by, description, changes, created_at
		FROM fraud_audit_log WHERE tenant_id=$1 AND entity_type=$2 AND entity_id=$3
		ORDER BY created_at ASC`, tenantID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []*model.AuditLog
	for rows.Next() {
		a := &model.AuditLog{}
		var changes []byte
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Action, &a.EntityType, &a.EntityID, &a.PerformedBy, &a.Description, &changes, &a.CreatedAt); err != nil {
			return nil, err
		}
		if changes != nil {
			json.Unmarshal(changes, &a.Changes)
		}
		logs = append(logs, a)
	}
	return logs, nil
}

// ─── Network Link ───────────────────────────────────────────────────────────

func (r *Repository) CreateNetworkLink(ctx context.Context, l *model.NetworkLink) error {
	l.ID = uuid.New()
	l.CreatedAt = time.Now()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO fraud_network_links (id, tenant_id, customer_id_a, customer_id_b, link_type, link_value, strength, flagged, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (tenant_id, customer_id_a, customer_id_b, link_type) DO UPDATE SET strength=fraud_network_links.strength+1`,
		l.ID, l.TenantID, l.CustomerIDA, l.CustomerIDB, l.LinkType, l.LinkValue, l.Strength, l.Flagged, l.CreatedAt)
	return err
}

func (r *Repository) FindLinksByCustomer(ctx context.Context, tenantID, customerID string) ([]*model.NetworkLink, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, customer_id_a, customer_id_b, link_type, link_value, strength, flagged, created_at
		FROM fraud_network_links WHERE tenant_id=$1 AND (customer_id_a=$2 OR customer_id_b=$2)`, tenantID, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (r *Repository) FindLinksByValue(ctx context.Context, tenantID, linkType, linkValue string) ([]*model.NetworkLink, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, customer_id_a, customer_id_b, link_type, link_value, strength, flagged, created_at
		FROM fraud_network_links WHERE tenant_id=$1 AND link_type=$2 AND link_value=$3`, tenantID, linkType, linkValue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (r *Repository) FindFlaggedLinks(ctx context.Context, tenantID string) ([]*model.NetworkLink, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, customer_id_a, customer_id_b, link_type, link_value, strength, flagged, created_at
		FROM fraud_network_links WHERE tenant_id=$1 AND flagged=true`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (r *Repository) GetNetworkLink(ctx context.Context, id uuid.UUID) (*model.NetworkLink, error) {
	l := &model.NetworkLink{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, customer_id_a, customer_id_b, link_type, link_value, strength, flagged, created_at
		FROM fraud_network_links WHERE id=$1`, id).Scan(
		&l.ID, &l.TenantID, &l.CustomerIDA, &l.CustomerIDB, &l.LinkType, &l.LinkValue, &l.Strength, &l.Flagged, &l.CreatedAt)
	return l, err
}

func (r *Repository) FlagLink(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "UPDATE fraud_network_links SET flagged=true WHERE id=$1", id)
	return err
}

func (r *Repository) LinkExists(ctx context.Context, tenantID, customerA, customerB, linkType string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fraud_network_links WHERE tenant_id=$1 AND customer_id_a=$2 AND customer_id_b=$3 AND link_type=$4)`,
		tenantID, customerA, customerB, linkType).Scan(&exists)
	return exists, err
}

func scanLinks(rows pgx.Rows) ([]*model.NetworkLink, error) {
	var links []*model.NetworkLink
	for rows.Next() {
		l := &model.NetworkLink{}
		if err := rows.Scan(&l.ID, &l.TenantID, &l.CustomerIDA, &l.CustomerIDB, &l.LinkType, &l.LinkValue, &l.Strength, &l.Flagged, &l.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, nil
}

// ─── SAR Report ─────────────────────────────────────────────────────────────

func (r *Repository) CreateSarReport(ctx context.Context, s *model.SarReport) error {
	s.ID = uuid.New()
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
	metadata, _ := json.Marshal(s.Metadata)
	if s.Metadata == nil {
		metadata = nil
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO sar_reports (id, tenant_id, report_number, report_type, status,
			subject_customer_id, subject_name, subject_national_id, narrative,
			suspicious_amount, activity_start_date, activity_end_date,
			case_id, prepared_by, reviewed_by, filed_by, filed_at, filing_reference,
			regulator, filing_deadline, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`,
		s.ID, s.TenantID, s.ReportNumber, s.ReportType, s.Status,
		s.SubjectCustomerID, s.SubjectName, s.SubjectNationalID, s.Narrative,
		s.SuspiciousAmount, s.ActivityStartDate, s.ActivityEndDate,
		s.CaseID, s.PreparedBy, s.ReviewedBy, s.FiledBy, s.FiledAt, s.FilingReference,
		s.Regulator, s.FilingDeadline, metadata, s.CreatedAt, s.UpdatedAt)
	if err != nil {
		return err
	}

	for _, alertID := range s.AlertIDs {
		r.pool.Exec(ctx, "INSERT INTO sar_report_alert_ids (report_id, alert_id) VALUES ($1,$2) ON CONFLICT DO NOTHING", s.ID, alertID)
	}
	return nil
}

func (r *Repository) UpdateSarReport(ctx context.Context, s *model.SarReport) error {
	s.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(s.Metadata)
	if s.Metadata == nil {
		metadata = nil
	}

	_, err := r.pool.Exec(ctx, `
		UPDATE sar_reports SET status=$1, narrative=$2, suspicious_amount=$3,
			activity_start_date=$4, activity_end_date=$5, reviewed_by=$6,
			filed_by=$7, filed_at=$8, filing_reference=$9, metadata=$10, updated_at=$11
		WHERE id=$12`,
		s.Status, s.Narrative, s.SuspiciousAmount,
		s.ActivityStartDate, s.ActivityEndDate, s.ReviewedBy,
		s.FiledBy, s.FiledAt, s.FilingReference, metadata, s.UpdatedAt, s.ID)
	return err
}

func (r *Repository) GetSarReport(ctx context.Context, id uuid.UUID) (*model.SarReport, error) {
	s := &model.SarReport{}
	var metadata []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, report_number, report_type, status,
			subject_customer_id, subject_name, subject_national_id, narrative,
			suspicious_amount, activity_start_date, activity_end_date,
			case_id, prepared_by, reviewed_by, filed_by, filed_at, filing_reference,
			regulator, filing_deadline, metadata, created_at, updated_at
		FROM sar_reports WHERE id=$1`, id).Scan(
		&s.ID, &s.TenantID, &s.ReportNumber, &s.ReportType, &s.Status,
		&s.SubjectCustomerID, &s.SubjectName, &s.SubjectNationalID, &s.Narrative,
		&s.SuspiciousAmount, &s.ActivityStartDate, &s.ActivityEndDate,
		&s.CaseID, &s.PreparedBy, &s.ReviewedBy, &s.FiledBy, &s.FiledAt, &s.FilingReference,
		&s.Regulator, &s.FilingDeadline, &metadata, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.Metadata = metadata
	s.AlertIDs, _ = r.getSarAlertIDs(ctx, id)
	return s, nil
}

func (r *Repository) ListSarReports(ctx context.Context, tenantID string, status *model.SarStatus, reportType *model.SarReportType, page, size int) ([]*model.SarReport, int64, error) {
	var total int64
	var rows pgx.Rows
	var err error

	baseCount := "SELECT COUNT(*) FROM sar_reports WHERE tenant_id=$1"
	baseQuery := `SELECT id, tenant_id, report_number, report_type, status,
		subject_customer_id, subject_name, subject_national_id, narrative,
		suspicious_amount, activity_start_date, activity_end_date,
		case_id, prepared_by, reviewed_by, filed_by, filed_at, filing_reference,
		regulator, filing_deadline, metadata, created_at, updated_at
		FROM sar_reports WHERE tenant_id=$1`

	if status != nil {
		r.pool.QueryRow(ctx, baseCount+" AND status=$2", tenantID, *status).Scan(&total)
		rows, err = r.pool.Query(ctx, baseQuery+" AND status=$2 ORDER BY created_at DESC LIMIT $3 OFFSET $4", tenantID, *status, size, page*size)
	} else if reportType != nil {
		r.pool.QueryRow(ctx, baseCount+" AND report_type=$2", tenantID, *reportType).Scan(&total)
		rows, err = r.pool.Query(ctx, baseQuery+" AND report_type=$2 ORDER BY created_at DESC LIMIT $3 OFFSET $4", tenantID, *reportType, size, page*size)
	} else {
		r.pool.QueryRow(ctx, baseCount, tenantID).Scan(&total)
		rows, err = r.pool.Query(ctx, baseQuery+" ORDER BY created_at DESC LIMIT $2 OFFSET $3", tenantID, size, page*size)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reports []*model.SarReport
	for rows.Next() {
		s := &model.SarReport{}
		var metadata []byte
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.ReportNumber, &s.ReportType, &s.Status,
			&s.SubjectCustomerID, &s.SubjectName, &s.SubjectNationalID, &s.Narrative,
			&s.SuspiciousAmount, &s.ActivityStartDate, &s.ActivityEndDate,
			&s.CaseID, &s.PreparedBy, &s.ReviewedBy, &s.FiledBy, &s.FiledAt, &s.FilingReference,
			&s.Regulator, &s.FilingDeadline, &metadata, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, err
		}
		s.Metadata = metadata
		s.AlertIDs, _ = r.getSarAlertIDs(ctx, s.ID)
		reports = append(reports, s)
	}
	return reports, total, nil
}

func (r *Repository) FindMaxReportNumber(ctx context.Context, tenantID string) (int, error) {
	var maxNum int
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(MAX(CAST(SUBSTRING(report_number FROM 5) AS INTEGER)), 0) FROM sar_reports WHERE tenant_id=$1`, tenantID).Scan(&maxNum)
	return maxNum, err
}

func (r *Repository) FindOverdueSarReports(ctx context.Context, now time.Time) ([]*model.SarReport, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, report_number, report_type, status,
			subject_customer_id, subject_name, subject_national_id, narrative,
			suspicious_amount, activity_start_date, activity_end_date,
			case_id, prepared_by, reviewed_by, filed_by, filed_at, filing_reference,
			regulator, filing_deadline, metadata, created_at, updated_at
		FROM sar_reports WHERE status IN ('DRAFT','PENDING_REVIEW','APPROVED') AND filing_deadline IS NOT NULL AND filing_deadline < $1`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []*model.SarReport
	for rows.Next() {
		s := &model.SarReport{}
		var metadata []byte
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.ReportNumber, &s.ReportType, &s.Status,
			&s.SubjectCustomerID, &s.SubjectName, &s.SubjectNationalID, &s.Narrative,
			&s.SuspiciousAmount, &s.ActivityStartDate, &s.ActivityEndDate,
			&s.CaseID, &s.PreparedBy, &s.ReviewedBy, &s.FiledBy, &s.FiledAt, &s.FilingReference,
			&s.Regulator, &s.FilingDeadline, &metadata, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Metadata = metadata
		reports = append(reports, s)
	}
	return reports, nil
}

func (r *Repository) getSarAlertIDs(ctx context.Context, reportID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, "SELECT alert_id FROM sar_report_alert_ids WHERE report_id=$1", reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, nil
}

// ─── Scoring History ────────────────────────────────────────────────────────

func (r *Repository) CreateScoringHistory(ctx context.Context, h *model.ScoringHistory) error {
	h.ID = uuid.New()
	if h.CreatedAt.IsZero() {
		h.CreatedAt = time.Now()
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO scoring_history (id, tenant_id, customer_id, event_type, amount,
			ml_score, risk_level, model_available, latency_ms, rule_score, anomaly_score, lgbm_score, model_details, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		h.ID, h.TenantID, h.CustomerID, h.EventType, h.Amount,
		h.MLScore, h.RiskLevel, h.ModelAvailable, h.LatencyMs, h.RuleScore, h.AnomalyScore, h.LGBMScore, h.ModelDetails, h.CreatedAt)
	return err
}

func (r *Repository) ListScoringHistory(ctx context.Context, tenantID, customerID string, page, size int) ([]*model.ScoringHistory, int64, error) {
	var total int64
	r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM scoring_history WHERE tenant_id=$1 AND customer_id=$2", tenantID, customerID).Scan(&total)

	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, customer_id, event_type, amount, ml_score, risk_level, model_available,
			latency_ms, rule_score, anomaly_score, lgbm_score, model_details, created_at
		FROM scoring_history WHERE tenant_id=$1 AND customer_id=$2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`,
		tenantID, customerID, size, page*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var histories []*model.ScoringHistory
	for rows.Next() {
		h := &model.ScoringHistory{}
		if err := rows.Scan(&h.ID, &h.TenantID, &h.CustomerID, &h.EventType, &h.Amount,
			&h.MLScore, &h.RiskLevel, &h.ModelAvailable, &h.LatencyMs,
			&h.RuleScore, &h.AnomalyScore, &h.LGBMScore, &h.ModelDetails, &h.CreatedAt); err != nil {
			return nil, 0, err
		}
		histories = append(histories, h)
	}
	return histories, total, nil
}

func (r *Repository) CountScoringByRiskLevel(ctx context.Context, tenantID, riskLevel string) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM scoring_history WHERE tenant_id=$1 AND risk_level=$2", tenantID, riskLevel).Scan(&count)
	return count, err
}

type RiskLevelAvg struct {
	RiskLevel string
	AvgScore  float64
}

func (r *Repository) AverageScoreByRiskLevel(ctx context.Context, tenantID string) ([]RiskLevelAvg, error) {
	rows, err := r.pool.Query(ctx, "SELECT risk_level, AVG(ml_score) FROM scoring_history WHERE tenant_id=$1 GROUP BY risk_level", tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RiskLevelAvg
	for rows.Next() {
		var rla RiskLevelAvg
		if err := rows.Scan(&rla.RiskLevel, &rla.AvgScore); err != nil {
			return nil, err
		}
		result = append(result, rla)
	}
	return result, nil
}

type DayVolume struct {
	Date   string
	Volume int64
}

func (r *Repository) ScoringVolumePerDay(ctx context.Context, tenantID string, days int) ([]DayVolume, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT CAST(created_at AS DATE) AS day, COUNT(*) AS volume
		FROM scoring_history WHERE tenant_id=$1 AND created_at >= CURRENT_DATE - CAST($2 || ' days' AS INTERVAL)
		GROUP BY CAST(created_at AS DATE) ORDER BY day`,
		tenantID, fmt.Sprintf("%d", days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []DayVolume
	for rows.Next() {
		var dv DayVolume
		var dt time.Time
		if err := rows.Scan(&dt, &dv.Volume); err != nil {
			return nil, err
		}
		dv.Date = dt.Format("2006-01-02")
		result = append(result, dv)
	}
	return result, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// unused import guard
var _ = strings.Contains
