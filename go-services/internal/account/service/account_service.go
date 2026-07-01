// Package service contains the business logic for the account service.
// Port of Java AccountService.java.
package service

import (
	"context"
	"crypto/rand"
	"fmt"
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
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/outbox"
)

// KYC tier limits in KES
var (
	tier0DailyLimit   = decimal.NewFromInt(2600)
	tier1MonthlyLimit = decimal.NewFromInt(65000)
	tier2MonthlyLimit = decimal.NewFromInt(650000)
)

// AccountService provides account business logic.
type AccountService struct {
	repo      *repository.Repository
	publisher *event.Publisher
	logger    *zap.Logger
	auditor   *audit.Logger
}

// NewAccountService creates a new AccountService.
func NewAccountService(repo *repository.Repository, publisher *event.Publisher, logger *zap.Logger) *AccountService {
	return &AccountService{
		repo:      repo,
		publisher: publisher,
		logger:    logger,
		auditor:   audit.New(repo, logger),
	}
}

// CreateAccountRequest is the DTO for account creation.
type CreateAccountRequest struct {
	CustomerID  string `json:"customerId"`
	AccountType string `json:"accountType"`
	Currency    string `json:"currency"`
	KycTier     int    `json:"kycTier"`
	AccountName string `json:"accountName"`
}

// CreateAccount creates a new account with zero balance.
func (s *AccountService) CreateAccount(ctx context.Context, req CreateAccountRequest, tenantID string) (*model.Account, error) {
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

	currency := "KES"
	if req.Currency != "" {
		currency = req.Currency
	}

	var accountName *string
	if req.AccountName != "" {
		accountName = &req.AccountName
	}

	account := &model.Account{
		TenantID:      tenantID,
		AccountNumber: accountNumber,
		CustomerID:    req.CustomerID,
		AccountType:   model.AccountType(strings.ToUpper(req.AccountType)),
		Status:        model.AccountStatusActive,
		Currency:      currency,
		KycTier:       req.KycTier,
		AccountName:   accountName,
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

	balance := &model.AccountBalance{
		AccountID:        account.ID,
		AvailableBalance: decimal.Zero,
		CurrentBalance:   decimal.Zero,
		LedgerBalance:    decimal.Zero,
	}
	if err := s.repo.CreateBalance(ctx, tx, balance); err != nil {
		return nil, fmt.Errorf("create balance: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	account.Balance = balance
	s.publisher.PublishCreated(ctx, account.ID, accountNumber, req.CustomerID, tenantID)
	s.logger.Info("Created account",
		zap.String("accountNumber", accountNumber),
		zap.String("customerId", req.CustomerID),
		zap.String("tenantId", tenantID))

	return account, nil
}

// GetAccount fetches an account with its balance.
func (s *AccountService) GetAccount(ctx context.Context, id uuid.UUID, tenantID string) (*model.Account, error) {
	account, err := s.repo.GetAccountByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Account", id)
		}
		return nil, err
	}
	bal, err := s.repo.GetBalanceByAccountID(ctx, id)
	if err == nil {
		account.Balance = bal
	}
	return account, nil
}

// ListAccounts returns paginated accounts for a tenant, each with its balance
// attached so the directory can show balances without per-row fetches.
func (s *AccountService) ListAccounts(ctx context.Context, tenantID string, page, size int) (dto.PageResponse, error) {
	accounts, total, err := s.repo.ListAccountsByTenant(ctx, tenantID, size, page*size)
	if err != nil {
		return dto.PageResponse{}, err
	}
	ids := make([]uuid.UUID, 0, len(accounts))
	for _, a := range accounts {
		ids = append(ids, a.ID)
	}
	if balances, berr := s.repo.GetBalancesByAccountIDs(ctx, ids); berr == nil {
		for _, a := range accounts {
			if b, ok := balances[a.ID]; ok {
				a.Balance = b
			}
		}
	} else {
		s.logger.Warn("could not batch-fetch account balances for list", zap.Error(berr))
	}
	return dto.NewPageResponse(accounts, page, size, total), nil
}

// GetBalance returns the balance for an account.
func (s *AccountService) GetBalance(ctx context.Context, accountID uuid.UUID, tenantID string) (*model.AccountBalance, error) {
	_, err := s.repo.GetAccountByIDAndTenant(ctx, accountID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Account", accountID)
		}
		return nil, err
	}
	bal, err := s.repo.GetBalanceByAccountID(ctx, accountID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Balance for account", accountID)
		}
		return nil, err
	}
	return bal, nil
}

// TransactionRequest is the DTO for credit/debit operations.
type TransactionRequest struct {
	Amount         decimal.Decimal `json:"amount"`
	Description    *string         `json:"description,omitempty"`
	Reference      *string         `json:"reference,omitempty"`
	Channel        *string         `json:"channel,omitempty"`
	IdempotencyKey *string         `json:"idempotencyKey,omitempty"`
}

// Credit adds funds to an account.
func (s *AccountService) Credit(ctx context.Context, accountID uuid.UUID, req TransactionRequest, tenantID string) (*model.AccountTransaction, error) {
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return nil, errors.BadRequest("amount must be positive")
	}

	// Maker-checker: queue for a second authoriser when required.
	if !gateOpen(ctx) && requiresApproval(ctx, s.repo, tenantID, OpAccountCredit, req.Amount) {
		desc := ""
		if req.Description != nil {
			desc = *req.Description
		}
		return nil, queueApproval(ctx, s.repo, tenantID, OpAccountCredit, "ACCOUNT", accountID.String(), req.Amount, desc, req)
	}

	// Idempotency check. Only ErrNoRows means "no prior transaction" — any other
	// error must surface, otherwise a flaky lookup would re-execute the credit.
	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		existing, err := s.repo.GetTransactionByIdempotencyKeyAndTenant(ctx, *req.IdempotencyKey, tenantID)
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
		if err == nil {
			return existing, nil
		}
	}

	account, err := s.repo.GetAccountByIDAndTenant(ctx, accountID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Account", accountID)
		}
		return nil, err
	}

	// Allow credits on ACTIVE and DORMANT accounts (dormant reactivation)
	if account.Status != model.AccountStatusActive && account.Status != model.AccountStatusDormant {
		return nil, errors.NewBusinessError(fmt.Sprintf("Account is %s - cannot credit", account.Status))
	}

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Auto-reactivate dormant account on credit
	if account.Status == model.AccountStatusDormant {
		if err := s.repo.ReactivateAccount(ctx, tx, accountID); err != nil {
			return nil, fmt.Errorf("reactivate dormant account: %w", err)
		}
	}

	balance, err := s.repo.GetBalanceForUpdate(ctx, tx, accountID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Balance for account", accountID)
		}
		return nil, err
	}

	newBalance := balance.AvailableBalance.Add(req.Amount)
	balance.AvailableBalance = newBalance
	balance.CurrentBalance = balance.CurrentBalance.Add(req.Amount)
	balance.LedgerBalance = balance.LedgerBalance.Add(req.Amount)
	if err := s.repo.UpdateBalance(ctx, tx, balance); err != nil {
		return nil, err
	}

	channel := "SYSTEM"
	if req.Channel != nil {
		channel = *req.Channel
	}

	txn := &model.AccountTransaction{
		TenantID:        tenantID,
		AccountID:       accountID,
		TransactionType: model.TransactionTypeCredit,
		Amount:          req.Amount,
		BalanceAfter:    &newBalance,
		Reference:       req.Reference,
		Description:     req.Description,
		Channel:         channel,
		IdempotencyKey:  req.IdempotencyKey,
		CreatedBy:       actorPtr(ctx),
	}
	if err := s.repo.CreateTransaction(ctx, tx, txn); err != nil {
		if existing, handled, herr := s.idempotentTxnFromRace(ctx, tx, err, req.IdempotencyKey, tenantID); handled {
			return existing, herr
		}
		return nil, err
	}

	// Track last transaction date for dormancy detection
	if err := s.repo.UpdateAccountLastTransactionDate(ctx, tx, accountID); err != nil {
		return nil, err
	}

	// Emit account.credit.received atomically with the balance change via the
	// transactional outbox so the money-path event can never be lost relative to
	// the committed state change (F27). The relay publishes it at-least-once.
	evt, err := s.publisher.BuildCreditReceived(accountID, req.Amount, tenantID)
	if err != nil {
		return nil, err
	}
	if err := outbox.Write(ctx, tx, evt, accountID.String()); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.auditor.Record(ctx, "ACCOUNT_CREDIT", "ACCOUNT", accountID.String(),
		map[string]any{"availableBalance": balance.AvailableBalance.Sub(req.Amount)},
		map[string]any{"availableBalance": newBalance},
		map[string]any{"amount": req.Amount, "channel": channel, "description": req.Description, "transactionId": txn.ID})

	return txn, nil
}

// Debit removes funds from an account.
func (s *AccountService) Debit(ctx context.Context, accountID uuid.UUID, req TransactionRequest, tenantID string) (*model.AccountTransaction, error) {
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return nil, errors.BadRequest("amount must be positive")
	}

	// Maker-checker: queue for a second authoriser when required.
	if !gateOpen(ctx) && requiresApproval(ctx, s.repo, tenantID, OpAccountDebit, req.Amount) {
		desc := ""
		if req.Description != nil {
			desc = *req.Description
		}
		return nil, queueApproval(ctx, s.repo, tenantID, OpAccountDebit, "ACCOUNT", accountID.String(), req.Amount, desc, req)
	}

	// Idempotency check. Only ErrNoRows means "no prior transaction" — any other
	// error must surface, otherwise a flaky lookup would re-execute the debit.
	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		existing, err := s.repo.GetTransactionByIdempotencyKeyAndTenant(ctx, *req.IdempotencyKey, tenantID)
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
		if err == nil {
			return existing, nil
		}
	}

	account, err := s.repo.GetAccountByIDAndTenant(ctx, accountID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Account", accountID)
		}
		return nil, err
	}

	if account.Status != model.AccountStatusActive {
		return nil, errors.NewBusinessError(fmt.Sprintf("Account is %s - cannot debit", account.Status))
	}

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	balance, err := s.repo.GetBalanceForUpdate(ctx, tx, accountID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Balance for account", accountID)
		}
		return nil, err
	}

	if balance.AvailableBalance.LessThan(req.Amount) {
		return nil, errors.NewBusinessError("Insufficient funds")
	}

	// KYC limit enforcement
	if err := s.enforceKycLimits(ctx, account, req.Amount, accountID); err != nil {
		return nil, err
	}

	newBalance := balance.AvailableBalance.Sub(req.Amount)
	balance.AvailableBalance = newBalance
	balance.CurrentBalance = balance.CurrentBalance.Sub(req.Amount)
	balance.LedgerBalance = balance.LedgerBalance.Sub(req.Amount)
	if err := s.repo.UpdateBalance(ctx, tx, balance); err != nil {
		return nil, err
	}

	channel := "SYSTEM"
	if req.Channel != nil {
		channel = *req.Channel
	}

	txn := &model.AccountTransaction{
		TenantID:        tenantID,
		AccountID:       accountID,
		TransactionType: model.TransactionTypeDebit,
		Amount:          req.Amount,
		BalanceAfter:    &newBalance,
		Reference:       req.Reference,
		Description:     req.Description,
		Channel:         channel,
		IdempotencyKey:  req.IdempotencyKey,
		CreatedBy:       actorPtr(ctx),
	}
	if err := s.repo.CreateTransaction(ctx, tx, txn); err != nil {
		if existing, handled, herr := s.idempotentTxnFromRace(ctx, tx, err, req.IdempotencyKey, tenantID); handled {
			return existing, herr
		}
		return nil, err
	}

	// Track last transaction date for dormancy detection
	if err := s.repo.UpdateAccountLastTransactionDate(ctx, tx, accountID); err != nil {
		return nil, err
	}

	// Emit account.debit.processed atomically with the balance change via the
	// transactional outbox so the money-path event can never be lost relative to
	// the committed state change (F27). The relay publishes it at-least-once.
	evt, err := s.publisher.BuildDebitProcessed(accountID, req.Amount, tenantID)
	if err != nil {
		return nil, err
	}
	if err := outbox.Write(ctx, tx, evt, accountID.String()); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.auditor.Record(ctx, "ACCOUNT_DEBIT", "ACCOUNT", accountID.String(),
		map[string]any{"availableBalance": balance.AvailableBalance.Add(req.Amount)},
		map[string]any{"availableBalance": newBalance},
		map[string]any{"amount": req.Amount, "channel": channel, "description": req.Description, "transactionId": txn.ID})

	return txn, nil
}

// idempotentTxnFromRace resolves a lost idempotency race on credit/debit: if
// err is a unique violation (uq_txn_idempotency), a concurrent request with the
// same idempotency key won. Roll back this attempt (undoing its balance change)
// and return the winner's transaction as the idempotent response. handled is
// false when err is not an idempotency-key unique violation.
func (s *AccountService) idempotentTxnFromRace(ctx context.Context, tx pgx.Tx, err error,
	idempotencyKey *string, tenantID string) (existing *model.AccountTransaction, handled bool, herr error) {
	if idempotencyKey == nil || *idempotencyKey == "" || !repository.IsUniqueViolation(err) {
		return nil, false, nil
	}
	_ = tx.Rollback(ctx)
	winner, ferr := s.repo.GetTransactionByIdempotencyKeyAndTenant(ctx, *idempotencyKey, tenantID)
	if ferr != nil {
		if ferr == pgx.ErrNoRows {
			// Key exists but under another tenant — reject rather than leak it.
			return nil, true, errors.Conflict("idempotencyKey already in use: " + *idempotencyKey)
		}
		return nil, true, ferr
	}
	s.logger.Info("Lost transaction idempotency race, returning existing transaction",
		zap.String("idempotencyKey", *idempotencyKey),
		zap.String("transactionId", winner.ID.String()))
	return winner, true, nil
}

// actorPtr returns the acting user from context as a pointer, or nil if absent.
func actorPtr(ctx context.Context) *string {
	if uid := auth.UserIDFromContext(ctx); uid != "" {
		return &uid
	}
	return nil
}

// GetTransactionHistory returns paginated transactions.
func (s *AccountService) GetTransactionHistory(ctx context.Context, accountID uuid.UUID, tenantID string, page, size int) (dto.PageResponse, error) {
	_, err := s.repo.GetAccountByIDAndTenant(ctx, accountID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return dto.PageResponse{}, errors.NotFoundResource("Account", accountID)
		}
		return dto.PageResponse{}, err
	}

	txns, total, err := s.repo.ListTransactions(ctx, accountID, size, page*size)
	if err != nil {
		return dto.PageResponse{}, err
	}
	return dto.NewPageResponse(txns, page, size, total), nil
}

// GetMiniStatement returns the last N transactions.
func (s *AccountService) GetMiniStatement(ctx context.Context, accountID uuid.UUID, tenantID string, count int) ([]*model.AccountTransaction, error) {
	_, err := s.repo.GetAccountByIDAndTenant(ctx, accountID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Account", accountID)
		}
		return nil, err
	}
	limit := count
	if limit > 50 {
		limit = 50
	}
	return s.repo.GetMiniStatement(ctx, accountID, limit)
}

// SearchAccounts searches accounts by number or name.
func (s *AccountService) SearchAccounts(ctx context.Context, q, tenantID string) ([]*model.Account, error) {
	return s.repo.SearchAccounts(ctx, tenantID, q)
}

// GetByCustomerID returns accounts for a customer.
func (s *AccountService) GetByCustomerID(ctx context.Context, customerID, tenantID string) ([]*model.Account, error) {
	return s.repo.GetAccountsByCustomer(ctx, customerID, tenantID)
}

// UpdateStatus changes the status of an account.
func (s *AccountService) UpdateStatus(ctx context.Context, accountID uuid.UUID, status, tenantID string) (*model.Account, error) {
	account, err := s.repo.GetAccountByIDAndTenant(ctx, accountID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Account", accountID)
		}
		return nil, err
	}

	upper := strings.ToUpper(status)
	if !model.ValidAccountStatus(upper) {
		return nil, errors.BadRequest("Invalid account status: " + status)
	}
	newStatus := model.AccountStatus(upper)
	prevStatus := account.Status
	if err := s.repo.UpdateAccountStatus(ctx, accountID, newStatus); err != nil {
		return nil, err
	}
	account.Status = newStatus

	action := "ACCOUNT_STATUS_CHANGE"
	switch newStatus {
	case model.AccountStatusFrozen:
		action = "ACCOUNT_FREEZE"
	case model.AccountStatusClosed:
		action = "ACCOUNT_CLOSE"
	case model.AccountStatusActive:
		action = "ACCOUNT_REACTIVATE"
	}
	s.auditor.Record(ctx, action, "ACCOUNT", accountID.String(),
		map[string]any{"status": prevStatus},
		map[string]any{"status": newStatus}, nil)

	return account, nil
}

// StatementResponse holds statement data.
type StatementResponse struct {
	AccountNumber  string           `json:"accountNumber"`
	CustomerName   string           `json:"customerName"`
	Currency       string           `json:"currency"`
	OpeningBalance decimal.Decimal  `json:"openingBalance"`
	ClosingBalance decimal.Decimal  `json:"closingBalance"`
	PeriodFrom     string           `json:"periodFrom"`
	PeriodTo       string           `json:"periodTo"`
	Transactions   dto.PageResponse `json:"transactions"`
}

// GetStatement returns an account statement for a date range.
func (s *AccountService) GetStatement(ctx context.Context, accountID uuid.UUID, tenantID string,
	from, to time.Time, page, size int) (*StatementResponse, error) {

	account, err := s.repo.GetAccountByIDAndTenant(ctx, accountID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Account", accountID)
		}
		return nil, err
	}

	fromDt := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toDt := time.Date(to.Year(), to.Month(), to.Day()+1, 0, 0, 0, 0, time.UTC)

	openingBalance, err := s.repo.SumNetBalanceChangeBefore(ctx, accountID, fromDt)
	if err != nil {
		return nil, err
	}
	closingBalance, err := s.repo.SumNetBalanceChangeBefore(ctx, accountID, toDt)
	if err != nil {
		return nil, err
	}

	txns, total, err := s.repo.ListTransactionsByPeriod(ctx, accountID, fromDt, toDt, size, page*size)
	if err != nil {
		return nil, err
	}

	// Prefer the customer's real name; fall back to the account name, then the
	// raw customer id.
	customerName := account.CustomerID
	if account.AccountName != nil {
		customerName = *account.AccountName
	}
	// account.CustomerID may hold the internal UUID (accounts opened via the
	// wizard) or the business customer_id — resolve either way.
	var cust *model.Customer
	if cid, perr := uuid.Parse(account.CustomerID); perr == nil {
		cust, _ = s.repo.GetCustomerByIDAndTenant(ctx, cid, tenantID)
	}
	if cust == nil {
		cust, _ = s.repo.GetCustomerByCustomerIDAndTenant(ctx, account.CustomerID, tenantID)
	}
	if cust != nil {
		full := strings.TrimSpace(cust.FirstName + " " + cust.LastName)
		if full != "" {
			customerName = full
		}
	}

	return &StatementResponse{
		AccountNumber:  account.AccountNumber,
		CustomerName:   customerName,
		Currency:       account.Currency,
		OpeningBalance: openingBalance,
		ClosingBalance: closingBalance,
		PeriodFrom:     from.Format("2006-01-02"),
		PeriodTo:       to.Format("2006-01-02"),
		Transactions:   dto.NewPageResponse(txns, page, size, total),
	}, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (s *AccountService) generateAccountNumber(ctx context.Context, tenantID string) (string, error) {
	prefix := strings.ToUpper(tenantID)
	if len(prefix) > 3 {
		prefix = prefix[:3]
	}

	for i := 0; i < 10; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(100_000_000))
		if err != nil {
			return "", err
		}
		candidate := fmt.Sprintf("ACC-%s-%08d", prefix, n.Int64())
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

func applyKycLimits(account *model.Account, kycTier int) {
	switch kycTier {
	case 0:
		d := tier0DailyLimit
		account.DailyTransactionLimit = &d
	case 1:
		m := tier1MonthlyLimit
		account.MonthlyTransactionLimit = &m
	case 2:
		m := tier2MonthlyLimit
		account.MonthlyTransactionLimit = &m
	case 3:
		// unlimited
	default:
		d := tier0DailyLimit
		account.DailyTransactionLimit = &d
	}
}

func (s *AccountService) enforceKycLimits(ctx context.Context, account *model.Account, amount decimal.Decimal, accountID uuid.UUID) error {
	tier := account.KycTier
	if tier == 3 {
		return nil
	}

	if tier == 0 && account.DailyTransactionLimit != nil {
		dailyUsed, err := s.repo.SumDailyDebits(ctx, accountID)
		if err != nil {
			return err
		}
		if dailyUsed.Add(amount).GreaterThan(*account.DailyTransactionLimit) {
			return errors.NewBusinessError(
				fmt.Sprintf("KYC Tier 0 daily limit exceeded. Limit: %s KES", account.DailyTransactionLimit.String()))
		}
	}

	if (tier == 1 || tier == 2) && account.MonthlyTransactionLimit != nil {
		monthlyUsed, err := s.repo.SumMonthlyDebits(ctx, accountID)
		if err != nil {
			return err
		}
		if monthlyUsed.Add(amount).GreaterThan(*account.MonthlyTransactionLimit) {
			return errors.NewBusinessError(
				fmt.Sprintf("KYC Tier %d monthly limit exceeded. Limit: %s KES", tier, account.MonthlyTransactionLimit.String()))
		}
	}

	return nil
}
