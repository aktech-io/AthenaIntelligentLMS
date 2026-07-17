package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/athena-lms/go-services/internal/common/market"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/account/event"
	"github.com/athena-lms/go-services/internal/account/model"
	"github.com/athena-lms/go-services/internal/account/repository"
	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/errors"
)

// AccountOpeningService handles account opening with deposit product configuration.
type AccountOpeningService struct {
	repo      *repository.Repository
	publisher *event.Publisher
	logger    *zap.Logger
	auditor   *audit.Logger
}

// NewAccountOpeningService creates a new AccountOpeningService.
func NewAccountOpeningService(repo *repository.Repository, publisher *event.Publisher, logger *zap.Logger) *AccountOpeningService {
	return &AccountOpeningService{repo: repo, publisher: publisher, logger: logger, auditor: audit.New(repo, logger)}
}

// OpenAccountRequest is the DTO for opening an account with a deposit product.
type OpenAccountRequest struct {
	CustomerID       string           `json:"customerId"`
	DepositProductID *string          `json:"depositProductId"`
	AccountType      string           `json:"accountType"`
	Currency         string           `json:"currency"`
	KycTier          int              `json:"kycTier"`
	AccountName      string           `json:"accountName"`
	BranchID         *string          `json:"branchId"`
	InitialDeposit   *decimal.Decimal `json:"initialDeposit"`
	// Fixed deposit fields
	TermDays             *int             `json:"termDays"`
	AutoRenew            bool             `json:"autoRenew"`
	InterestRateOverride *decimal.Decimal `json:"interestRateOverride"`
}

// OpenAccount creates a new account linked to a deposit product.
func (s *AccountOpeningService) OpenAccount(ctx context.Context, req OpenAccountRequest, tenantID, openedBy string) (*model.Account, error) {
	if req.CustomerID == "" {
		return nil, errors.BadRequest("customerId is required")
	}
	if !model.ValidAccountType(strings.ToUpper(req.AccountType)) {
		return nil, errors.BadRequest("Invalid account type: " + req.AccountType)
	}

	accountNumber, err := s.generateAccountNumber(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("generate account number: %w", err)
	}

	currency := market.Currency()
	if req.Currency != "" {
		currency = req.Currency
	}

	var accountName *string
	if req.AccountName != "" {
		accountName = &req.AccountName
	}

	accountType := model.AccountType(strings.ToUpper(req.AccountType))
	status := model.AccountStatusActive

	// Fixed deposits start as ACTIVE but locked
	var maturityDate *time.Time
	var termDays *int
	var lockedAmount *decimal.Decimal
	if accountType == model.AccountTypeFixedDeposit {
		if req.TermDays == nil || *req.TermDays <= 0 {
			return nil, errors.BadRequest("termDays is required for fixed deposits")
		}
		termDays = req.TermDays
		mat := time.Now().AddDate(0, 0, *req.TermDays)
		maturityDate = &mat
		if req.InitialDeposit != nil {
			lockedAmount = req.InitialDeposit
		}
	}

	var depositProductID *uuid.UUID
	if req.DepositProductID != nil {
		dpID, err := uuid.Parse(*req.DepositProductID)
		if err != nil {
			return nil, errors.BadRequest("Invalid depositProductId")
		}
		depositProductID = &dpID
	}

	account := &model.Account{
		TenantID:             tenantID,
		AccountNumber:        accountNumber,
		CustomerID:           req.CustomerID,
		AccountType:          accountType,
		Status:               status,
		Currency:             currency,
		KycTier:              req.KycTier,
		AccountName:          accountName,
		DepositProductID:     depositProductID,
		BranchID:             req.BranchID,
		OpenedBy:             &openedBy,
		MaturityDate:         maturityDate,
		TermDays:             termDays,
		LockedAmount:         lockedAmount,
		AutoRenew:            req.AutoRenew,
		InterestRateOverride: req.InterestRateOverride,
	}

	applyKycLimits(account, req.KycTier)

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.repo.CreateAccount(ctx, tx, account); err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	initialBalance := decimal.Zero
	if req.InitialDeposit != nil && req.InitialDeposit.GreaterThan(decimal.Zero) {
		initialBalance = *req.InitialDeposit
	}

	balance := &model.AccountBalance{
		AccountID:        account.ID,
		AvailableBalance: initialBalance,
		CurrentBalance:   initialBalance,
		LedgerBalance:    initialBalance,
	}
	if err := s.repo.CreateBalance(ctx, tx, balance); err != nil {
		return nil, fmt.Errorf("create balance: %w", err)
	}

	// Record initial deposit as a transaction
	if initialBalance.GreaterThan(decimal.Zero) {
		desc := "Initial deposit"
		txn := &model.AccountTransaction{
			TenantID:        tenantID,
			AccountID:       account.ID,
			TransactionType: model.TransactionTypeCredit,
			Amount:          initialBalance,
			BalanceAfter:    &initialBalance,
			Description:     &desc,
			Channel:         "BRANCH",
		}
		if err := s.repo.CreateTransaction(ctx, tx, txn); err != nil {
			return nil, fmt.Errorf("create initial transaction: %w", err)
		}
		if err := s.repo.UpdateAccountLastTransactionDate(ctx, tx, account.ID); err != nil {
			return nil, fmt.Errorf("update last txn date: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	account.Balance = balance
	s.publisher.PublishCreated(ctx, account.ID, accountNumber, req.CustomerID, tenantID)
	s.logger.Info("Account opened",
		zap.String("accountNumber", accountNumber),
		zap.String("type", string(accountType)),
		zap.String("customerId", req.CustomerID))

	return account, nil
}

// ApproveAccount approves a pending account.
func (s *AccountOpeningService) ApproveAccount(ctx context.Context, accountID uuid.UUID, tenantID string) (*model.Account, error) {
	account, err := s.repo.GetAccountByIDAndTenant(ctx, accountID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Account", accountID)
		}
		return nil, err
	}

	if account.Status != model.AccountStatusPendingApproval {
		return nil, errors.NewBusinessError("Account is not pending approval")
	}

	if err := s.repo.UpdateAccountStatus(ctx, accountID, model.AccountStatusActive); err != nil {
		return nil, err
	}
	account.Status = model.AccountStatusActive
	return account, nil
}

// CloseAccount closes an account with a reason.
func (s *AccountOpeningService) CloseAccount(ctx context.Context, accountID uuid.UUID, reason, tenantID string) (*model.Account, error) {
	account, err := s.repo.GetAccountByIDAndTenant(ctx, accountID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Account", accountID)
		}
		return nil, err
	}

	if account.Status == model.AccountStatusClosed {
		return nil, errors.NewBusinessError("Account is already closed")
	}

	// Check for remaining balance
	bal, err := s.repo.GetBalanceByAccountID(ctx, accountID)
	if err == nil && bal.AvailableBalance.GreaterThan(decimal.Zero) {
		return nil, errors.NewBusinessError(
			fmt.Sprintf("Account has remaining balance of %s %s — withdraw or transfer before closing",
				bal.AvailableBalance.String(), account.Currency))
	}

	// Maker-checker: account closure requires a second authoriser when enabled.
	if !gateOpen(ctx) && requiresApproval(ctx, s.repo, tenantID, OpAccountClose, decimal.Zero) {
		return nil, queueApproval(ctx, s.repo, tenantID, OpAccountClose, "ACCOUNT", accountID.String(),
			decimal.Zero, "Account closure: "+reason, map[string]any{"reason": reason})
	}

	if err := s.repo.CloseAccount(ctx, accountID, reason); err != nil {
		return nil, err
	}
	account.Status = model.AccountStatusClosed

	s.auditor.Record(ctx, "ACCOUNT_CLOSE", "ACCOUNT", accountID.String(),
		map[string]any{"status": "ACTIVE"}, map[string]any{"status": model.AccountStatusClosed},
		map[string]any{"reason": reason})

	return account, nil
}

func (s *AccountOpeningService) generateAccountNumber(ctx context.Context, tenantID string) (string, error) {
	prefix := strings.ToUpper(tenantID)
	if len(prefix) > 3 {
		prefix = prefix[:3]
	}

	for i := 0; i < 10; i++ {
		n, err := generateRandomInt()
		if err != nil {
			return "", err
		}
		candidate := fmt.Sprintf("ACC-%s-%08d", prefix, n)
		exists, err := s.repo.AccountNumberExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique account number after 10 attempts")
}

// generateRandomInt returns a random int64 up to 100_000_000.
func generateRandomInt() (int64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(100_000_000))
	if err != nil {
		return 0, err
	}
	return n.Int64(), nil
}
