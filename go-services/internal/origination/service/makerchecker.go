package service

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/origination/repository"
)

// Loan maker-checker operation identifiers.
const (
	OpLoanApprove  = "LOAN_APPROVE"
	OpLoanDisburse = "LOAN_DISBURSE"
)

type controlDefault struct {
	enabled   bool
	threshold decimal.Decimal
}

// Defaults: segregation of duties is on by default for both approval and
// disbursement, with no amount threshold (always enforced).
var controlDefaults = map[string]controlDefault{
	OpLoanApprove:  {enabled: true, threshold: decimal.Zero},
	OpLoanDisburse: {enabled: true, threshold: decimal.Zero},
}

func effectiveControl(ctx context.Context, repo *repository.Repository, tenantID, op string) (bool, decimal.Decimal) {
	if cfg, err := repo.GetControlConfig(ctx, tenantID, op); err == nil && cfg != nil {
		return cfg.Enabled, cfg.ThresholdAmount
	}
	d := controlDefaults[op]
	return d.enabled, d.threshold
}

// requiresSoD reports whether segregation of duties must be enforced for an
// operation given its amount.
func requiresSoD(ctx context.Context, repo *repository.Repository, tenantID, op string, amount decimal.Decimal) bool {
	enabled, threshold := effectiveControl(ctx, repo, tenantID, op)
	if !enabled {
		return false
	}
	return amount.GreaterThanOrEqual(threshold)
}

// EffectiveControlConfig returns the active config for all loan operations
// (explicit tenant row, else default).
func (s *Service) EffectiveControlConfig(ctx context.Context, tenantID string) []*repository.ControlConfig {
	rows, _ := s.repo.ListControlConfig(ctx, tenantID)
	byOp := map[string]*repository.ControlConfig{}
	for _, r := range rows {
		byOp[r.Operation] = r
	}
	out := []*repository.ControlConfig{}
	for _, op := range []string{OpLoanApprove, OpLoanDisburse} {
		if c, ok := byOp[op]; ok {
			out = append(out, c)
			continue
		}
		d := controlDefaults[op]
		out = append(out, &repository.ControlConfig{TenantID: tenantID, Operation: op, Enabled: d.enabled, ThresholdAmount: d.threshold})
	}
	return out
}

// UpsertControlConfig updates a loan control config row.
func (s *Service) UpsertControlConfig(ctx context.Context, tenantID, operation string, enabled bool, threshold decimal.Decimal) error {
	if _, ok := controlDefaults[operation]; !ok {
		return errors.BadRequest("unknown operation: " + operation)
	}
	c := &repository.ControlConfig{TenantID: tenantID, Operation: operation, Enabled: enabled, ThresholdAmount: threshold}
	if uid := auth.UserIDFromContext(ctx); uid != "" {
		c.UpdatedBy = &uid
	}
	if err := s.repo.UpsertControlConfig(ctx, c); err != nil {
		return err
	}
	s.auditor.Record(ctx, "CONTROL_CONFIG_UPDATE", "CONTROL_CONFIG", operation, nil,
		map[string]any{"enabled": enabled, "threshold": threshold}, nil)
	return nil
}
