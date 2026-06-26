package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/errors"
	floatEvent "github.com/athena-lms/go-services/internal/float/event"
	"github.com/athena-lms/go-services/internal/float/model"
	"github.com/athena-lms/go-services/internal/float/repository"
)

var nonAlphaNum = regexp.MustCompile(`[^A-Z0-9]`)

// Service implements float business logic.
type Service struct {
	repo      *repository.Repository
	publisher *floatEvent.Publisher
	logger    *zap.Logger
	auditor   *audit.Logger
}

// New creates a new float Service.
func New(repo *repository.Repository, publisher *floatEvent.Publisher, logger *zap.Logger) *Service {
	return &Service{repo: repo, publisher: publisher, logger: logger, auditor: audit.New(repo, logger)}
}

// ListAuditLog returns the float-service audit trail, optionally filtered by
// entityType and entityId.
func (s *Service) ListAuditLog(ctx context.Context, tenantID, entityType, entityID string, limit, offset int) ([]*repository.AuditRecord, error) {
	return s.repo.ListAuditLog(ctx, tenantID, entityType, entityID, limit, offset)
}

// VerifyAuditChain reports whether the tamper-evident audit trail is intact, or
// the seq of the first altered/missing entry.
func (s *Service) VerifyAuditChain(ctx context.Context, tenantID string) (*repository.ChainVerification, error) {
	return s.repo.VerifyAuditChain(ctx, tenantID)
}

// CreateAccount creates a new float account.
func (s *Service) CreateAccount(ctx context.Context, req *model.CreateFloatAccountRequest, tenantID string) (*model.FloatAccountResponse, error) {
	if msg := req.Validate(); msg != "" {
		return nil, errors.BadRequest(msg)
	}

	exists, err := s.repo.ExistsAccountByTenantAndCode(ctx, tenantID, req.AccountCode)
	if err != nil {
		return nil, fmt.Errorf("check account code: %w", err)
	}
	if exists {
		return nil, errors.NewBusinessError("Float account code already exists: " + req.AccountCode)
	}

	currency := req.Currency
	if currency == "" {
		currency = "KES"
	}

	account := &model.FloatAccount{
		TenantID:    tenantID,
		AccountName: req.AccountName,
		AccountCode: req.AccountCode,
		Currency:    currency,
		FloatLimit:  req.FloatLimit,
		DrawnAmount: decimal.Zero,
		Status:      model.FloatAccountStatusActive,
		Description: req.Description,
	}

	saved, err := s.repo.InsertAccount(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("insert account: %w", err)
	}

	s.auditor.Record(ctx, "FLOAT_ACCOUNT_CREATE", "FLOAT_ACCOUNT", saved.ID.String(),
		nil, saved, map[string]any{"accountCode": saved.AccountCode, "floatLimit": saved.FloatLimit})
	s.logger.Info("Created float account",
		zap.String("id", saved.ID.String()),
		zap.String("tenantId", tenantID))

	resp := model.ToAccountResponse(saved)
	return &resp, nil
}

// GetAccount returns a single float account by ID.
func (s *Service) GetAccount(ctx context.Context, id uuid.UUID, tenantID string) (*model.FloatAccountResponse, error) {
	account, err := s.repo.FindAccountByTenantAndID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("find account: %w", err)
	}
	if account == nil {
		return nil, errors.NotFoundResource("Float account", id)
	}
	resp := model.ToAccountResponse(account)
	return &resp, nil
}

// ListAccounts returns all float accounts for a tenant.
func (s *Service) ListAccounts(ctx context.Context, tenantID string) ([]model.FloatAccountResponse, error) {
	accounts, err := s.repo.FindAccountsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	result := make([]model.FloatAccountResponse, 0, len(accounts))
	for i := range accounts {
		result = append(result, model.ToAccountResponse(&accounts[i]))
	}
	return result, nil
}

// Draw draws from a float account.
func (s *Service) Draw(ctx context.Context, accountID uuid.UUID, req *model.FloatDrawRequest, tenantID string) (*model.FloatTransactionResponse, error) {
	if msg := req.Validate(); msg != "" {
		return nil, errors.BadRequest(msg)
	}

	account, err := s.repo.FindAccountByTenantAndID(ctx, tenantID, accountID)
	if err != nil {
		return nil, fmt.Errorf("find account: %w", err)
	}
	if account == nil {
		return nil, errors.NotFoundResource("Float account", accountID)
	}

	available := account.FloatLimit.Sub(account.DrawnAmount)
	if available.LessThan(req.Amount) {
		return nil, errors.NewBusinessError(
			fmt.Sprintf("Insufficient float balance. Available: %s, Requested: %s", available.String(), req.Amount.String()))
	}

	balanceBefore := account.DrawnAmount
	newDrawn := account.DrawnAmount.Add(req.Amount)

	if err := s.repo.UpdateAccountDrawnAmount(ctx, accountID, newDrawn); err != nil {
		return nil, fmt.Errorf("update drawn amount: %w", err)
	}

	tx := &model.FloatTransaction{
		TenantID:        tenantID,
		FloatAccountID:  accountID,
		TransactionType: model.FloatTransactionTypeDraw,
		Amount:          req.Amount,
		BalanceBefore:   balanceBefore,
		BalanceAfter:    newDrawn,
		ReferenceID:     req.ReferenceID,
		ReferenceType:   req.ReferenceType,
		Narration:       req.Narration,
	}

	saved, err := s.repo.InsertTransaction(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("insert transaction: %w", err)
	}

	s.publisher.PublishFloatDrawn(ctx, accountID, req.Amount, nil, tenantID)

	s.auditor.Record(ctx, "FLOAT_DRAW", "FLOAT_ACCOUNT", accountID.String(),
		map[string]any{"drawnAmount": balanceBefore}, map[string]any{"drawnAmount": newDrawn},
		map[string]any{"amount": req.Amount, "referenceId": req.ReferenceID, "referenceType": req.ReferenceType})
	resp := model.ToTransactionResponse(saved)
	return &resp, nil
}

// Repay repays a float account.
func (s *Service) Repay(ctx context.Context, accountID uuid.UUID, req *model.FloatRepayRequest, tenantID string) (*model.FloatTransactionResponse, error) {
	if msg := req.Validate(); msg != "" {
		return nil, errors.BadRequest(msg)
	}

	account, err := s.repo.FindAccountByTenantAndID(ctx, tenantID, accountID)
	if err != nil {
		return nil, fmt.Errorf("find account: %w", err)
	}
	if account == nil {
		return nil, errors.NotFoundResource("Float account", accountID)
	}

	if account.DrawnAmount.LessThan(req.Amount) {
		return nil, errors.NewBusinessError(
			fmt.Sprintf("Repayment exceeds drawn amount. Drawn: %s, Repayment: %s",
				account.DrawnAmount.String(), req.Amount.String()))
	}

	balanceBefore := account.DrawnAmount
	newDrawn := account.DrawnAmount.Sub(req.Amount)

	if err := s.repo.UpdateAccountDrawnAmount(ctx, accountID, newDrawn); err != nil {
		return nil, fmt.Errorf("update drawn amount: %w", err)
	}

	tx := &model.FloatTransaction{
		TenantID:        tenantID,
		FloatAccountID:  accountID,
		TransactionType: model.FloatTransactionTypeRepayment,
		Amount:          req.Amount,
		BalanceBefore:   balanceBefore,
		BalanceAfter:    newDrawn,
		ReferenceID:     req.ReferenceID,
		Narration:       req.Narration,
	}

	saved, err := s.repo.InsertTransaction(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("insert transaction: %w", err)
	}

	s.publisher.PublishFloatRepaid(ctx, accountID, req.Amount, nil, tenantID)

	s.auditor.Record(ctx, "FLOAT_REPAYMENT", "FLOAT_ACCOUNT", accountID.String(),
		map[string]any{"drawnAmount": balanceBefore}, map[string]any{"drawnAmount": newDrawn},
		map[string]any{"amount": req.Amount, "referenceId": req.ReferenceID})
	resp := model.ToTransactionResponse(saved)
	return &resp, nil
}

// ProcessDraw handles a loan.disbursed event — draws float and creates an allocation.
// Does NOT return an error on insufficient float; logs a warning and exits cleanly.
func (s *Service) ProcessDraw(ctx context.Context, loanID uuid.UUID, amount decimal.Decimal, tenantID string) {
	// Idempotency: check if allocation for this loan already exists
	existing, err := s.repo.FindAllocationByLoanID(ctx, loanID)
	if err != nil {
		s.logger.Error("Failed to check allocation idempotency", zap.Error(err))
		return
	}
	if existing != nil {
		s.logger.Info("Float draw already processed for loan, skipping",
			zap.String("loanId", loanID.String()))
		return
	}

	accounts, err := s.repo.FindAccountsByTenant(ctx, tenantID)
	if err != nil {
		s.logger.Error("Failed to find float accounts", zap.Error(err))
		return
	}

	var account *model.FloatAccount
	for i := range accounts {
		if accounts[i].Status == model.FloatAccountStatusActive {
			account = &accounts[i]
			break
		}
	}

	if account == nil {
		s.logger.Info("No active float account for tenant — auto-creating one",
			zap.String("tenantId", tenantID),
			zap.String("loanId", loanID.String()))
		account, err = s.autoCreateFloatAccount(ctx, tenantID)
		if err != nil {
			s.logger.Error("Failed to auto-create float account", zap.Error(err))
			return
		}
	}

	available := account.FloatLimit.Sub(account.DrawnAmount)
	if available.LessThan(amount) {
		s.logger.Warn("Insufficient float — loan not drawn",
			zap.String("tenantId", tenantID),
			zap.String("accountId", account.ID.String()),
			zap.String("available", available.String()),
			zap.String("requested", amount.String()),
			zap.String("loanId", loanID.String()))
		return
	}

	eventID := "loan-disbursed-" + loanID.String()
	eventExists, err := s.repo.ExistsTransactionByEventID(ctx, eventID)
	if err != nil {
		s.logger.Error("Failed to check event idempotency", zap.Error(err))
		return
	}
	if eventExists {
		s.logger.Info("Float draw transaction already recorded for event, skipping",
			zap.String("eventId", eventID))
		return
	}

	balanceBefore := account.DrawnAmount
	newDrawn := account.DrawnAmount.Add(amount)

	if err := s.repo.UpdateAccountDrawnAmount(ctx, account.ID, newDrawn); err != nil {
		s.logger.Error("Failed to update account drawn amount", zap.Error(err))
		return
	}

	tx := &model.FloatTransaction{
		TenantID:        tenantID,
		FloatAccountID:  account.ID,
		TransactionType: model.FloatTransactionTypeDraw,
		Amount:          amount,
		BalanceBefore:   balanceBefore,
		BalanceAfter:    newDrawn,
		ReferenceID:     loanID.String(),
		ReferenceType:   "LOAN_DISBURSEMENT",
		Narration:       "Float draw for loan " + loanID.String(),
		EventID:         eventID,
	}
	if _, err := s.repo.InsertTransaction(ctx, tx); err != nil {
		s.logger.Error("Failed to insert float transaction", zap.Error(err))
		return
	}

	alloc := &model.FloatAllocation{
		TenantID:        tenantID,
		FloatAccountID:  account.ID,
		LoanID:          loanID,
		AllocatedAmount: amount,
		RepaidAmount:    decimal.Zero,
		Status:          model.FloatAllocationStatusActive,
		DisbursedAt:     time.Now(),
	}
	savedAlloc, err := s.repo.InsertAllocation(ctx, alloc)
	if err != nil {
		s.logger.Error("Failed to insert float allocation", zap.Error(err))
		return
	}

	s.auditor.Record(ctx, "FLOAT_ALLOCATION", "FLOAT_ALLOCATION", savedAlloc.ID.String(),
		nil, savedAlloc, map[string]any{"loanId": loanID.String(), "amount": amount, "accountId": account.ID.String()})
	s.publisher.PublishFloatDrawn(ctx, account.ID, amount, &loanID, tenantID)
	s.logger.Info("Float drawn for loan",
		zap.String("amount", amount.String()),
		zap.String("loanId", loanID.String()),
		zap.String("accountId", account.ID.String()),
		zap.String("tenantId", tenantID))
}

// ProcessTopUp handles an account.credit.received event — tops up the float.
func (s *Service) ProcessTopUp(ctx context.Context, referenceAccountID string, amount decimal.Decimal, tenantID string) {
	accounts, err := s.repo.FindAccountsByTenant(ctx, tenantID)
	if err != nil {
		s.logger.Error("Failed to find float accounts for top-up", zap.Error(err))
		return
	}

	var account *model.FloatAccount
	for i := range accounts {
		if accounts[i].Status == model.FloatAccountStatusActive {
			account = &accounts[i]
			break
		}
	}

	if account == nil {
		s.logger.Info("No active float account for tenant — auto-creating one for top-up",
			zap.String("tenantId", tenantID))
		account, err = s.autoCreateFloatAccount(ctx, tenantID)
		if err != nil {
			s.logger.Error("Failed to auto-create float account for top-up", zap.Error(err))
			return
		}
	}

	balanceBefore := account.DrawnAmount
	newDrawn := account.DrawnAmount.Sub(amount)
	if newDrawn.LessThan(decimal.Zero) {
		newDrawn = decimal.Zero
	}

	if err := s.repo.UpdateAccountDrawnAmount(ctx, account.ID, newDrawn); err != nil {
		s.logger.Error("Failed to update account drawn amount for top-up", zap.Error(err))
		return
	}

	tx := &model.FloatTransaction{
		TenantID:        tenantID,
		FloatAccountID:  account.ID,
		TransactionType: model.FloatTransactionTypeTopUp,
		Amount:          amount,
		BalanceBefore:   balanceBefore,
		BalanceAfter:    newDrawn,
		ReferenceID:     referenceAccountID,
		ReferenceType:   "ACCOUNT_CREDIT",
		Narration:       "Float top-up from account credit",
	}
	if _, err := s.repo.InsertTransaction(ctx, tx); err != nil {
		s.logger.Error("Failed to insert float top-up transaction", zap.Error(err))
		return
	}

	s.auditor.Record(ctx, "FLOAT_ADJUSTMENT", "FLOAT_ACCOUNT", account.ID.String(),
		map[string]any{"drawnAmount": balanceBefore}, map[string]any{"drawnAmount": newDrawn},
		map[string]any{"amount": amount, "referenceId": referenceAccountID, "referenceType": "ACCOUNT_CREDIT"})
	s.publisher.PublishFloatRepaid(ctx, account.ID, amount, nil, tenantID)
	s.logger.Info("Float top-up processed",
		zap.String("amount", amount.String()),
		zap.String("tenantId", tenantID),
		zap.String("accountId", account.ID.String()))
}

// GetSummary returns a summary of float accounts and allocations for a tenant.
func (s *Service) GetSummary(ctx context.Context, tenantID string) (*model.FloatSummaryResponse, error) {
	accounts, err := s.repo.FindAccountsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list accounts for summary: %w", err)
	}

	allocations, err := s.repo.FindAllocationsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list allocations for summary: %w", err)
	}

	totalLimit := decimal.Zero
	totalDrawn := decimal.Zero
	activeAccounts := 0
	for _, a := range accounts {
		totalLimit = totalLimit.Add(a.FloatLimit)
		totalDrawn = totalDrawn.Add(a.DrawnAmount)
		if a.Status == model.FloatAccountStatusActive {
			activeAccounts++
		}
	}

	activeAllocations := 0
	for _, a := range allocations {
		if a.Status == model.FloatAllocationStatusActive {
			activeAllocations++
		}
	}

	return &model.FloatSummaryResponse{
		TenantID:          tenantID,
		TotalLimit:        totalLimit,
		TotalDrawn:        totalDrawn,
		TotalAvailable:    totalLimit.Sub(totalDrawn),
		ActiveAccounts:    activeAccounts,
		ActiveAllocations: activeAllocations,
	}, nil
}

// GetTransactions returns paginated transactions for a float account.
func (s *Service) GetTransactions(ctx context.Context, accountID uuid.UUID, tenantID string, page, size int) (*dto.PageResponse, error) {
	// Verify account belongs to tenant
	account, err := s.repo.FindAccountByTenantAndID(ctx, tenantID, accountID)
	if err != nil {
		return nil, fmt.Errorf("find account: %w", err)
	}
	if account == nil {
		return nil, errors.NotFoundResource("Float account", accountID)
	}

	txs, total, err := s.repo.FindTransactionsByAccount(ctx, accountID, tenantID, page, size)
	if err != nil {
		return nil, fmt.Errorf("find transactions: %w", err)
	}

	responses := make([]model.FloatTransactionResponse, 0, len(txs))
	for i := range txs {
		responses = append(responses, model.ToTransactionResponse(&txs[i]))
	}

	resp := dto.NewPageResponse(responses, page, size, total)
	return &resp, nil
}

// autoCreateFloatAccount creates a default float account for a tenant.
func (s *Service) autoCreateFloatAccount(ctx context.Context, tenantID string) (*model.FloatAccount, error) {
	accountCode := "FLOAT-AUTO-" + nonAlphaNum.ReplaceAllString(strings.ToUpper(tenantID), "")
	defaultLimit, _ := decimal.NewFromString("10000000") // 10M KES default

	account := &model.FloatAccount{
		TenantID:    tenantID,
		AccountName: "Auto-created Float Account",
		AccountCode: accountCode,
		Currency:    "KES",
		FloatLimit:  defaultLimit,
		DrawnAmount: decimal.Zero,
		Status:      model.FloatAccountStatusActive,
		Description: "Auto-created float account for tenant " + tenantID,
	}

	saved, err := s.repo.InsertAccount(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("auto-create float account: %w", err)
	}

	s.logger.Info("Auto-created float account",
		zap.String("id", saved.ID.String()),
		zap.String("code", accountCode),
		zap.String("tenantId", tenantID),
		zap.String("limit", "10000000"))

	return saved, nil
}
