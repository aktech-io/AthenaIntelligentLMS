package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/account/repository"
	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/errors"
)

// Maker-checker operation identifiers.
const (
	OpAccountCredit = "ACCOUNT_CREDIT"
	OpAccountDebit  = "ACCOUNT_DEBIT"
	OpTransfer      = "TRANSFER"
	OpAccountClose  = "ACCOUNT_CLOSE"
)

// defaultControl holds the seeded defaults used when a tenant has no explicit
// control_config row. Editable per tenant via the config API.
type controlDefault struct {
	enabled   bool
	threshold decimal.Decimal
}

var controlDefaults = map[string]controlDefault{
	OpAccountCredit: {enabled: true, threshold: decimal.NewFromInt(100000)},
	OpAccountDebit:  {enabled: true, threshold: decimal.NewFromInt(100000)},
	OpTransfer:      {enabled: true, threshold: decimal.NewFromInt(100000)},
	OpAccountClose:  {enabled: true, threshold: decimal.Zero},
}

// effectiveControl resolves the active config for an operation: the tenant's
// explicit row if present, otherwise the seeded default.
func effectiveControl(ctx context.Context, repo *repository.Repository, tenantID, op string) (bool, decimal.Decimal) {
	if cfg, err := repo.GetControlConfig(ctx, tenantID, op); err == nil && cfg != nil {
		return cfg.Enabled, cfg.ThresholdAmount
	}
	d := controlDefaults[op]
	return d.enabled, d.threshold
}

// requiresApproval reports whether an operation must be queued for a second
// authoriser given its amount.
func requiresApproval(ctx context.Context, repo *repository.Repository, tenantID, op string, amount decimal.Decimal) bool {
	enabled, threshold := effectiveControl(ctx, repo, tenantID, op)
	if !enabled {
		return false
	}
	return amount.GreaterThanOrEqual(threshold)
}

// ── Approval bypass marker ──────────────────────────────────────────────────
// When a queued operation is approved, it is re-executed with a context that
// bypasses the maker-checker gate (the gate already ran when it was queued).

type ctxKey string

const bypassKey ctxKey = "mc_bypass"

func withBypass(ctx context.Context) context.Context { return context.WithValue(ctx, bypassKey, true) }

func isBypassed(ctx context.Context) bool {
	v, _ := ctx.Value(bypassKey).(bool)
	return v
}

// isServiceCall reports whether the request was authenticated with the internal
// service key (role SERVICE). System-initiated operations — loan disbursement,
// interest posting, inter-service transfers — are governed by their own
// workflows and bypass human maker-checker dual control.
func isServiceCall(ctx context.Context) bool {
	for _, r := range auth.RolesFromContext(ctx) {
		if r == "SERVICE" {
			return true
		}
	}
	return false
}

// gateOpen reports whether maker-checker should be skipped for this call (either
// an internal re-execution after approval, or an internal service call).
func gateOpen(ctx context.Context) bool {
	return isBypassed(ctx) || isServiceCall(ctx)
}

// ErrPendingApproval signals that an operation was queued for dual control
// instead of executing. Handlers translate it to HTTP 202.
type ErrPendingApproval struct {
	PendingID string
	Operation string
}

func (e *ErrPendingApproval) Error() string {
	return fmt.Sprintf("operation %s queued for approval (id %s)", e.Operation, e.PendingID)
}

// queueApproval records a pending operation and returns ErrPendingApproval.
func queueApproval(ctx context.Context, repo *repository.Repository, tenantID, op, entityType, entityID string,
	amount decimal.Decimal, description string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	p := &repository.PendingApproval{
		TenantID:   tenantID,
		Operation:  op,
		EntityType: &entityType,
		EntityID:   &entityID,
		Amount:     &amount,
		Payload:    raw,
	}
	if description != "" {
		p.Description = &description
	}
	if uid := auth.UserIDFromContext(ctx); uid != "" {
		p.MakerID = &uid
	}
	if roles := auth.RolesFromContext(ctx); len(roles) > 0 {
		p.MakerRole = &roles[0]
	}
	if err := repo.CreatePendingApproval(ctx, p); err != nil {
		return err
	}
	return &ErrPendingApproval{PendingID: p.ID, Operation: op}
}

// ── Approval service ────────────────────────────────────────────────────────

// ApprovalService executes or rejects queued operations and manages config.
type ApprovalService struct {
	repo        *repository.Repository
	accountSvc  *AccountService
	transferSvc *TransferService
	openingSvc  *AccountOpeningService
	auditor     *audit.Logger
	logger      *zap.Logger
}

// NewApprovalService wires the approval service.
func NewApprovalService(repo *repository.Repository, accountSvc *AccountService, transferSvc *TransferService, openingSvc *AccountOpeningService, logger *zap.Logger) *ApprovalService {
	return &ApprovalService{
		repo:        repo,
		accountSvc:  accountSvc,
		transferSvc: transferSvc,
		openingSvc:  openingSvc,
		auditor:     audit.New(repo, logger),
		logger:      logger,
	}
}

// ListPending returns queued approvals (status filter optional).
func (s *ApprovalService) ListPending(ctx context.Context, tenantID, status string, limit, offset int) ([]*repository.PendingApproval, error) {
	return s.repo.ListPendingApprovals(ctx, tenantID, status, limit, offset)
}

// EffectiveConfig returns the active config for all operations (explicit or default).
func (s *ApprovalService) EffectiveConfig(ctx context.Context, tenantID string) []*repository.ControlConfig {
	rows, _ := s.repo.ListControlConfig(ctx, tenantID)
	byOp := map[string]*repository.ControlConfig{}
	for _, r := range rows {
		byOp[r.Operation] = r
	}
	out := []*repository.ControlConfig{}
	for _, op := range []string{OpAccountCredit, OpAccountDebit, OpTransfer, OpAccountClose} {
		if c, ok := byOp[op]; ok {
			out = append(out, c)
			continue
		}
		d := controlDefaults[op]
		out = append(out, &repository.ControlConfig{TenantID: tenantID, Operation: op, Enabled: d.enabled, ThresholdAmount: d.threshold})
	}
	return out
}

// UpsertConfig updates a control config row.
func (s *ApprovalService) UpsertConfig(ctx context.Context, tenantID, operation string, enabled bool, threshold decimal.Decimal) error {
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

// Reject declines a queued operation.
func (s *ApprovalService) Reject(ctx context.Context, tenantID, id, reason string) error {
	p, err := s.repo.GetPendingApproval(ctx, id, tenantID)
	if err != nil {
		return errors.NotFoundResource("PendingApproval", id)
	}
	if p.Status != "PENDING" {
		return errors.NewBusinessError("approval already " + p.Status)
	}
	checker := auth.UserIDFromContext(ctx)
	role := firstRole(ctx)
	if err := s.repo.DecidePendingApproval(ctx, id, "REJECTED", checker, role, reason, nil); err != nil {
		return err
	}
	s.auditor.Record(ctx, "APPROVAL_REJECT", "PENDING_APPROVAL", id, nil, nil,
		map[string]any{"operation": p.Operation, "reason": reason})
	return nil
}

// Approve enforces checker != maker, then executes the queued operation.
func (s *ApprovalService) Approve(ctx context.Context, tenantID, id string) (*repository.PendingApproval, error) {
	p, err := s.repo.GetPendingApproval(ctx, id, tenantID)
	if err != nil {
		return nil, errors.NotFoundResource("PendingApproval", id)
	}
	if p.Status != "PENDING" {
		return nil, errors.NewBusinessError("approval already " + p.Status)
	}
	checker := auth.UserIDFromContext(ctx)
	if p.MakerID != nil && checker != "" && *p.MakerID == checker {
		return nil, errors.NewBusinessError("maker-checker violation: the approver must differ from the maker")
	}

	result, execErr := s.execute(withBypass(ctx), p)
	if execErr != nil {
		return nil, execErr
	}
	resultJSON, _ := json.Marshal(result)
	if err := s.repo.DecidePendingApproval(ctx, id, "APPROVED", checker, firstRole(ctx), "", resultJSON); err != nil {
		return nil, err
	}
	s.auditor.Record(ctx, "APPROVAL_APPROVE", "PENDING_APPROVAL", id, nil, nil,
		map[string]any{"operation": p.Operation, "maker": p.MakerID, "checker": checker})
	p.Status = "APPROVED"
	return p, nil
}

// execute dispatches an approved operation to its underlying service.
func (s *ApprovalService) execute(ctx context.Context, p *repository.PendingApproval) (any, error) {
	switch p.Operation {
	case OpAccountCredit, OpAccountDebit:
		var req TransactionRequest
		if err := json.Unmarshal(p.Payload, &req); err != nil {
			return nil, err
		}
		accID, err := uuid.Parse(deref(p.EntityID))
		if err != nil {
			return nil, errors.BadRequest("invalid account id in payload")
		}
		if p.Operation == OpAccountCredit {
			return s.accountSvc.Credit(ctx, accID, req, p.TenantID)
		}
		return s.accountSvc.Debit(ctx, accID, req, p.TenantID)
	case OpAccountClose:
		accID, err := uuid.Parse(deref(p.EntityID))
		if err != nil {
			return nil, errors.BadRequest("invalid account id in payload")
		}
		var cp struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(p.Payload, &cp)
		return s.openingSvc.CloseAccount(ctx, accID, cp.Reason, p.TenantID)
	case OpTransfer:
		var req TransferRequest
		if err := json.Unmarshal(p.Payload, &req); err != nil {
			return nil, err
		}
		initiator := ""
		if p.MakerID != nil {
			initiator = *p.MakerID
		}
		return s.transferSvc.InitiateTransfer(ctx, req, p.TenantID, initiator)
	default:
		return nil, errors.BadRequest("unknown operation: " + p.Operation)
	}
}

func firstRole(ctx context.Context) string {
	if roles := auth.RolesFromContext(ctx); len(roles) > 0 {
		return roles[0]
	}
	return ""
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
