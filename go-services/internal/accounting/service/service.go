package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/accounting/audit"
	"github.com/athena-lms/go-services/internal/accounting/event"
	"github.com/athena-lms/go-services/internal/accounting/model"
	"github.com/athena-lms/go-services/internal/accounting/repository"
	"github.com/athena-lms/go-services/internal/common/errors"
)

// AccountingService implements the accounting domain logic.
type AccountingService struct {
	repo      *repository.Repository
	publisher *event.Publisher
	audit     *audit.Logger
	logger    *zap.Logger
}

// New creates a new AccountingService.
func New(repo *repository.Repository, publisher *event.Publisher, auditLogger *audit.Logger, logger *zap.Logger) *AccountingService {
	return &AccountingService{repo: repo, publisher: publisher, audit: auditLogger, logger: logger}
}

// --- Chart of Accounts ---

// CreateAccount creates a new GL account.
func (s *AccountingService) CreateAccount(ctx context.Context, req model.CreateAccountRequest, tenantID string) (*model.AccountResponse, error) {
	existing, err := s.repo.FindAccountByTenantAndCode(ctx, tenantID, req.Code)
	if err != nil {
		return nil, fmt.Errorf("check existing account: %w", err)
	}
	if existing != nil {
		return nil, errors.NewBusinessError("Account code already exists: " + req.Code)
	}

	account := &model.ChartOfAccount{
		TenantID:    tenantID,
		Code:        req.Code,
		Name:        req.Name,
		AccountType: req.AccountType,
		BalanceType: req.BalanceType,
		ParentID:    req.ParentID,
		Description: req.Description,
		IsActive:    true,
	}

	if err := s.repo.CreateAccount(ctx, account); err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	s.audit.Log(ctx, "CREATE_ACCOUNT", "ChartOfAccount", account.ID.String(), map[string]any{
		"code": account.Code, "name": account.Name,
	})

	resp := model.ToAccountResponse(account)
	return &resp, nil
}

// ListAccounts lists GL accounts for a tenant (with optional type filter), merging system accounts.
func (s *AccountingService) ListAccounts(ctx context.Context, tenantID string, accountType *model.AccountType) ([]model.AccountResponse, error) {
	var tenantAccounts, systemAccounts []model.ChartOfAccount
	var err error

	if accountType != nil {
		tenantAccounts, err = s.repo.ListActiveAccountsByType(ctx, tenantID, *accountType)
	} else {
		tenantAccounts, err = s.repo.ListActiveAccounts(ctx, tenantID)
	}
	if err != nil {
		return nil, err
	}

	// Also fetch system accounts and merge (fill gaps not covered by tenant)
	if tenantID != "system" {
		if accountType != nil {
			systemAccounts, err = s.repo.ListActiveAccountsByType(ctx, "system", *accountType)
		} else {
			systemAccounts, err = s.repo.ListActiveAccounts(ctx, "system")
		}
		if err != nil {
			return nil, err
		}
	}

	// Build set of tenant account codes to avoid duplicates
	tenantCodes := make(map[string]bool, len(tenantAccounts))
	for _, a := range tenantAccounts {
		tenantCodes[a.Code] = true
	}

	// Merge: tenant accounts first, then system accounts for codes not in tenant
	merged := make([]model.ChartOfAccount, 0, len(tenantAccounts)+len(systemAccounts))
	merged = append(merged, tenantAccounts...)
	for _, sa := range systemAccounts {
		if !tenantCodes[sa.Code] {
			merged = append(merged, sa)
		}
	}

	result := make([]model.AccountResponse, 0, len(merged))
	for i := range merged {
		result = append(result, model.ToAccountResponse(&merged[i]))
	}
	return result, nil
}

// GetAccount returns a single GL account by ID.
func (s *AccountingService) GetAccount(ctx context.Context, id uuid.UUID, tenantID string) (*model.AccountResponse, error) {
	account, err := s.repo.FindAccountByIDAndTenantIn(ctx, id, []string{tenantID, "system"})
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, errors.NotFoundResource("Account", id)
	}
	resp := model.ToAccountResponse(account)
	return &resp, nil
}

// GetAccountByCode returns a GL account by code.
func (s *AccountingService) GetAccountByCode(ctx context.Context, code, tenantID string) (*model.AccountResponse, error) {
	account, err := s.repo.FindAccountByCodeAndTenantIn(ctx, code, []string{tenantID, "system"})
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, errors.NotFoundResource("Account", code)
	}
	resp := model.ToAccountResponse(account)
	return &resp, nil
}

// --- Journal Entries ---

// PostEntry validates and posts a manual journal entry.
func (s *AccountingService) PostEntry(ctx context.Context, req model.PostJournalEntryRequest, tenantID, userID string) (*model.JournalEntryResponse, error) {
	if len(req.Lines) < 2 {
		return nil, errors.NewBusinessError("Journal entry must have at least 2 lines")
	}

	totalDebit := decimal.Zero
	totalCredit := decimal.Zero
	for _, line := range req.Lines {
		totalDebit = totalDebit.Add(line.DebitAmount)
		totalCredit = totalCredit.Add(line.CreditAmount)
	}
	if !totalDebit.Equal(totalCredit) {
		return nil, errors.NewBusinessError(fmt.Sprintf(
			"Journal entry is not balanced: debits=%s credits=%s", totalDebit, totalCredit))
	}

	entryDate := time.Now()
	if req.EntryDate != nil {
		entryDate = *req.EntryDate
	}

	// Check fiscal period is open
	if err := s.checkPeriodOpen(ctx, tenantID, entryDate); err != nil {
		return nil, err
	}

	entry := &model.JournalEntry{
		TenantID:          tenantID,
		Reference:         req.Reference,
		Description:       req.Description,
		EntryDate:         entryDate,
		Status:            model.EntryStatusDraft,
		TotalDebit:        totalDebit,
		TotalCredit:       totalCredit,
		PostedBy:          &userID,
		CreatedBy:         &userID,
		IsSystemGenerated: false,
	}

	for i, lr := range req.Lines {
		currency := lr.Currency
		if currency == "" {
			currency = "KES"
		}
		entry.Lines = append(entry.Lines, model.JournalLine{
			AccountID:    lr.AccountID,
			LineNo:       i + 1,
			Description:  lr.Description,
			DebitAmount:  lr.DebitAmount,
			CreditAmount: lr.CreditAmount,
			Currency:     currency,
		})
	}

	if err := s.repo.CreateJournalEntry(ctx, entry); err != nil {
		return nil, fmt.Errorf("post journal entry: %w", err)
	}

	s.audit.Log(ctx, "CREATE_ENTRY", "JournalEntry", entry.ID.String(), map[string]any{
		"reference": entry.Reference, "status": string(entry.Status),
	})

	resp := model.ToJournalEntryResponse(entry)
	return &resp, nil
}

// ListEntries returns paginated journal entries.
func (s *AccountingService) ListEntries(ctx context.Context, tenantID string, from, to *time.Time, page, size int) ([]model.JournalEntryResponse, int64, error) {
	entries, total, err := s.repo.ListEntries(ctx, tenantID, from, to, page, size)
	if err != nil {
		return nil, 0, err
	}

	result := make([]model.JournalEntryResponse, 0, len(entries))
	for i := range entries {
		result = append(result, model.ToJournalEntryResponse(&entries[i]))
	}
	return result, total, nil
}

// GetEntry returns a single journal entry by ID.
func (s *AccountingService) GetEntry(ctx context.Context, id uuid.UUID, tenantID string) (*model.JournalEntryResponse, error) {
	entry, err := s.repo.FindEntryByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, errors.NotFoundResource("JournalEntry", id)
	}
	resp := model.ToJournalEntryResponse(entry)
	return &resp, nil
}

// --- Balances & Reporting ---

// GetBalance returns the balance for a GL account.
func (s *AccountingService) GetBalance(ctx context.Context, accountID uuid.UUID, tenantID string, year, month int) (*model.BalanceResponse, error) {
	account, err := s.repo.FindAccountByIDAndTenantIn(ctx, accountID, []string{tenantID, "system"})
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, errors.NotFoundResource("Account", accountID)
	}

	net, err := s.repo.GetNetBalance(ctx, accountID, tenantID)
	if err != nil {
		return nil, err
	}

	// For CREDIT-normal accounts, flip sign for display
	if account.BalanceType == model.BalanceTypeCredit {
		net = net.Neg()
	}

	return &model.BalanceResponse{
		AccountID:   accountID,
		AccountCode: account.Code,
		AccountName: account.Name,
		AccountType: string(account.AccountType),
		BalanceType: string(account.BalanceType),
		Balance:     net,
		Currency:    "KES",
		PeriodYear:  year,
		PeriodMonth: month,
	}, nil
}

// GetLedger returns all journal lines for a GL account.
func (s *AccountingService) GetLedger(ctx context.Context, accountID uuid.UUID, tenantID string) ([]model.JournalLineResponse, error) {
	account, err := s.repo.FindAccountByIDAndTenantIn(ctx, accountID, []string{tenantID, "system"})
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, errors.NotFoundResource("Account", accountID)
	}

	lines, err := s.repo.FindLedgerLines(ctx, accountID)
	if err != nil {
		return nil, err
	}

	result := make([]model.JournalLineResponse, 0, len(lines))
	for i := range lines {
		result = append(result, model.ToJournalLineResponse(&lines[i]))
	}
	return result, nil
}

// GetTrialBalance returns the trial balance for a tenant.
func (s *AccountingService) GetTrialBalance(ctx context.Context, tenantID string, year, month int) (*model.TrialBalanceResponse, error) {
	accounts, err := s.repo.ListActiveAccounts(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		accounts, err = s.repo.ListActiveAccounts(ctx, "system")
		if err != nil {
			return nil, err
		}
	}

	rows := make([]model.BalanceResponse, 0, len(accounts))
	totalDr := decimal.Zero
	totalCr := decimal.Zero

	for _, acc := range accounts {
		net, err := s.repo.GetNetBalance(ctx, acc.ID, tenantID)
		if err != nil {
			return nil, err
		}

		row := model.BalanceResponse{
			AccountID:   acc.ID,
			AccountCode: acc.Code,
			AccountName: acc.Name,
			AccountType: string(acc.AccountType),
			BalanceType: string(acc.BalanceType),
			Balance:     net.Abs(),
			Currency:    "KES",
			PeriodYear:  year,
			PeriodMonth: month,
		}
		rows = append(rows, row)

		if net.GreaterThanOrEqual(decimal.Zero) {
			totalDr = totalDr.Add(net.Abs())
		} else {
			totalCr = totalCr.Add(net.Abs())
		}
	}

	return &model.TrialBalanceResponse{
		PeriodYear:   year,
		PeriodMonth:  month,
		Accounts:     rows,
		TotalDebits:  totalDr,
		TotalCredits: totalCr,
		Balanced:     totalDr.Equal(totalCr),
	}, nil
}

// --- Event-driven journal posting ---

// EntryExists checks if a journal entry already exists for idempotency.
func (s *AccountingService) EntryExists(ctx context.Context, sourceEvent, sourceID string) bool {
	if sourceID == "" {
		return false
	}
	exists, err := s.repo.EntryExistsBySourceEventAndID(ctx, sourceEvent, sourceID)
	if err != nil {
		s.logger.Error("Failed to check entry existence", zap.Error(err))
		return false
	}
	return exists
}

// PostLoanDisbursement creates a journal entry for a loan disbursement.
// DR Loans Receivable (1100) / CR Cash (1000)
func (s *AccountingService) PostLoanDisbursement(ctx context.Context, tenantID, applicationID string, amount decimal.Decimal) error {
	// Loan disbursed into the borrower's deposit account:
	//   DR Loans Receivable (1100)   — the bank's asset (amount owed by borrower)
	//   CR Customer Deposits (2000)  — the bank's liability (amount it now owes the
	//                                  borrower in their deposit account)
	// (Cash 1000 is only credited for over-the-counter cash-out disbursements.)
	drAccount, err := s.resolveAccountID(ctx, tenantID, "1100")
	if err != nil {
		return err
	}
	crAccount, err := s.resolveAccountID(ctx, tenantID, "2000")
	if err != nil {
		return err
	}

	entry := s.buildSystemEntry(tenantID, "DISB-"+applicationID,
		"Loan disbursement for application "+applicationID,
		"loan.disbursed", applicationID,
		drAccount, crAccount, amount)

	if err := s.repo.CreateJournalEntry(ctx, entry); err != nil {
		return err
	}
	s.publisher.PublishJournalPosted(ctx, entry)
	s.logger.Info("Posted disbursement journal", zap.String("applicationId", applicationID), zap.String("amount", amount.String()))
	return nil
}

// PostRepayment creates a journal entry for a loan repayment with breakdown.
func (s *AccountingService) PostRepayment(ctx context.Context, tenantID, paymentID string, amount decimal.Decimal, payload map[string]any) error {
	principal := getDecimalFromPayload(payload, "principalApplied")
	interest := getDecimalFromPayload(payload, "interestApplied")
	fees := getDecimalFromPayload(payload, "feeApplied")
	penalties := getDecimalFromPayload(payload, "penaltyApplied")

	// Fallback: if breakdown doesn't sum to amount, treat full amount as principal
	breakdownTotal := principal.Add(interest).Add(fees).Add(penalties)
	if breakdownTotal.IsZero() || !breakdownTotal.Equal(amount) {
		principal = amount
		interest = decimal.Zero
		fees = decimal.Zero
		penalties = decimal.Zero
	}

	cashAccount, err := s.resolveAccountID(ctx, tenantID, "1000")
	if err != nil {
		return err
	}
	loansAccount, err := s.resolveAccountID(ctx, tenantID, "1100")
	if err != nil {
		return err
	}

	sourceEvent := "payment.completed"
	description := "Loan repayment payment " + paymentID
	postedBy := "system"

	entry := &model.JournalEntry{
		TenantID:          tenantID,
		Reference:         "RPMT-" + paymentID,
		Description:       &description,
		EntryDate:         time.Now(),
		Status:            model.EntryStatusPosted,
		SourceEvent:       &sourceEvent,
		SourceID:          &paymentID,
		TotalDebit:        amount,
		TotalCredit:       amount,
		PostedBy:          &postedBy,
		CreatedBy:         &postedBy,
		IsSystemGenerated: true,
	}

	lineNo := 1
	// Line 1: DR Cash - total received
	entry.Lines = append(entry.Lines, model.JournalLine{
		AccountID: cashAccount, LineNo: lineNo, DebitAmount: amount, CreditAmount: decimal.Zero, Currency: "KES",
	})
	lineNo++

	// Line 2: CR Loans Receivable - principal portion
	if principal.GreaterThan(decimal.Zero) {
		entry.Lines = append(entry.Lines, model.JournalLine{
			AccountID: loansAccount, LineNo: lineNo, DebitAmount: decimal.Zero, CreditAmount: principal, Currency: "KES",
		})
		lineNo++
	}

	// Line 3: CR Interest Income (4000)
	if interest.GreaterThan(decimal.Zero) {
		interestAccount, err := s.resolveAccountID(ctx, tenantID, "4000")
		if err != nil {
			return err
		}
		entry.Lines = append(entry.Lines, model.JournalLine{
			AccountID: interestAccount, LineNo: lineNo, DebitAmount: decimal.Zero, CreditAmount: interest, Currency: "KES",
		})
		lineNo++
	}

	// Line 4: CR Fee Income (4100)
	if fees.GreaterThan(decimal.Zero) {
		feeAccount, err := s.resolveAccountID(ctx, tenantID, "4100")
		if err != nil {
			return err
		}
		entry.Lines = append(entry.Lines, model.JournalLine{
			AccountID: feeAccount, LineNo: lineNo, DebitAmount: decimal.Zero, CreditAmount: fees, Currency: "KES",
		})
		lineNo++
	}

	// Line 5: CR Penalty Income (4200)
	if penalties.GreaterThan(decimal.Zero) {
		penaltyAccount, err := s.resolveAccountID(ctx, tenantID, "4200")
		if err != nil {
			return err
		}
		entry.Lines = append(entry.Lines, model.JournalLine{
			AccountID: penaltyAccount, LineNo: lineNo, DebitAmount: decimal.Zero, CreditAmount: penalties, Currency: "KES",
		})
	}

	if err := s.repo.CreateJournalEntry(ctx, entry); err != nil {
		return err
	}
	s.publisher.PublishJournalPosted(ctx, entry)
	s.logger.Info("Posted repayment journal",
		zap.String("paymentId", paymentID), zap.String("amount", amount.String()),
		zap.String("principal", principal.String()), zap.String("interest", interest.String()),
		zap.String("fees", fees.String()), zap.String("penalties", penalties.String()))
	return nil
}

// PostPaymentReversal creates a journal entry for a payment reversal.
// DR Loans Receivable (1100) / CR Cash (1000)
func (s *AccountingService) PostPaymentReversal(ctx context.Context, tenantID, paymentID string, amount decimal.Decimal) error {
	drAccount, err := s.resolveAccountID(ctx, tenantID, "1100")
	if err != nil {
		return err
	}
	crAccount, err := s.resolveAccountID(ctx, tenantID, "1000")
	if err != nil {
		return err
	}

	entry := s.buildSystemEntry(tenantID, "REV-"+paymentID,
		"Payment reversal for "+paymentID,
		"payment.reversed", paymentID,
		drAccount, crAccount, amount)

	if err := s.repo.CreateJournalEntry(ctx, entry); err != nil {
		return err
	}
	s.publisher.PublishJournalPosted(ctx, entry)
	return nil
}

// PostOverdraftDrawn creates a journal entry for an overdraft draw.
// DR 1250 Overdraft Receivable / CR 1000 Cash
func (s *AccountingService) PostOverdraftDrawn(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error {
	drAccount, err := s.resolveAccountID(ctx, tenantID, "1250")
	if err != nil {
		return err
	}
	crAccount, err := s.resolveAccountID(ctx, tenantID, "1000")
	if err != nil {
		return err
	}

	entry := s.buildSystemEntry(tenantID, "OD-DRAW-"+sourceID,
		"Overdraft drawn "+sourceID, "overdraft.drawn", sourceID,
		drAccount, crAccount, amount)

	if err := s.repo.CreateJournalEntry(ctx, entry); err != nil {
		return err
	}
	s.publisher.PublishJournalPosted(ctx, entry)
	s.logger.Info("Posted overdraft drawn journal", zap.String("sourceId", sourceID), zap.String("amount", amount.String()))
	return nil
}

// PostOverdraftRepaid creates a journal entry for an overdraft repayment.
// DR 1000 Cash / CR 1250 Overdraft Receivable
func (s *AccountingService) PostOverdraftRepaid(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error {
	drAccount, err := s.resolveAccountID(ctx, tenantID, "1000")
	if err != nil {
		return err
	}
	crAccount, err := s.resolveAccountID(ctx, tenantID, "1250")
	if err != nil {
		return err
	}

	entry := s.buildSystemEntry(tenantID, "OD-RPMT-"+sourceID,
		"Overdraft repayment "+sourceID, "overdraft.repaid", sourceID,
		drAccount, crAccount, amount)

	if err := s.repo.CreateJournalEntry(ctx, entry); err != nil {
		return err
	}
	s.publisher.PublishJournalPosted(ctx, entry)
	s.logger.Info("Posted overdraft repayment journal", zap.String("sourceId", sourceID), zap.String("amount", amount.String()))
	return nil
}

// PostOverdraftInterestCharged creates a journal entry for overdraft interest.
// DR 1250 Overdraft Receivable / CR 4300 Overdraft Interest Income
func (s *AccountingService) PostOverdraftInterestCharged(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error {
	drAccount, err := s.resolveAccountID(ctx, tenantID, "1250")
	if err != nil {
		return err
	}
	crAccount, err := s.resolveAccountID(ctx, tenantID, "4300")
	if err != nil {
		return err
	}

	entry := s.buildSystemEntry(tenantID, "OD-INT-"+sourceID,
		"Overdraft interest charged "+sourceID, "overdraft.interest.charged", sourceID,
		drAccount, crAccount, amount)

	if err := s.repo.CreateJournalEntry(ctx, entry); err != nil {
		return err
	}
	s.publisher.PublishJournalPosted(ctx, entry)
	s.logger.Info("Posted overdraft interest journal", zap.String("sourceId", sourceID), zap.String("amount", amount.String()))
	return nil
}

// PostOverdraftFeeCharged creates a journal entry for an overdraft fee.
// DR 1250 Overdraft Receivable / CR 4100 Fee Income
func (s *AccountingService) PostOverdraftFeeCharged(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error {
	drAccount, err := s.resolveAccountID(ctx, tenantID, "1250")
	if err != nil {
		return err
	}
	crAccount, err := s.resolveAccountID(ctx, tenantID, "4100")
	if err != nil {
		return err
	}

	entry := s.buildSystemEntry(tenantID, "OD-FEE-"+sourceID,
		"Overdraft fee charged "+sourceID, "overdraft.fee.charged", sourceID,
		drAccount, crAccount, amount)

	if err := s.repo.CreateJournalEntry(ctx, entry); err != nil {
		return err
	}
	s.publisher.PublishJournalPosted(ctx, entry)
	s.logger.Info("Posted overdraft fee journal", zap.String("sourceId", sourceID), zap.String("amount", amount.String()))
	return nil
}

// PostFloatDrawn creates a journal entry for a float pool draw (loan disbursement funded by float).
// DR 2100 Borrowings (Float Liability) / CR 1000 Cash
func (s *AccountingService) PostFloatDrawn(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error {
	drAccount, err := s.resolveAccountID(ctx, tenantID, "2100")
	if err != nil {
		return err
	}
	crAccount, err := s.resolveAccountID(ctx, tenantID, "1000")
	if err != nil {
		return err
	}

	entry := s.buildSystemEntry(tenantID, "FLOAT-DRAW-"+sourceID,
		"Float drawn for loan disbursement "+sourceID, "float.drawn", sourceID,
		drAccount, crAccount, amount)

	if err := s.repo.CreateJournalEntry(ctx, entry); err != nil {
		return err
	}
	s.publisher.PublishJournalPosted(ctx, entry)
	s.logger.Info("Posted float drawn journal", zap.String("sourceId", sourceID), zap.String("amount", amount.String()))
	return nil
}

// PostFloatRepaid creates a journal entry for a float pool repayment (collections reducing float liability).
// DR 1000 Cash / CR 2100 Borrowings (Float Liability)
func (s *AccountingService) PostFloatRepaid(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error {
	drAccount, err := s.resolveAccountID(ctx, tenantID, "1000")
	if err != nil {
		return err
	}
	crAccount, err := s.resolveAccountID(ctx, tenantID, "2100")
	if err != nil {
		return err
	}

	entry := s.buildSystemEntry(tenantID, "FLOAT-RPMT-"+sourceID,
		"Float repayment from collections "+sourceID, "float.repaid", sourceID,
		drAccount, crAccount, amount)

	if err := s.repo.CreateJournalEntry(ctx, entry); err != nil {
		return err
	}
	s.publisher.PublishJournalPosted(ctx, entry)
	s.logger.Info("Posted float repayment journal", zap.String("sourceId", sourceID), zap.String("amount", amount.String()))
	return nil
}

// --- Entry Workflow ---

// SubmitForApproval transitions a DRAFT entry to PENDING_APPROVAL.
func (s *AccountingService) SubmitForApproval(ctx context.Context, id uuid.UUID, tenantID string) (*model.JournalEntryResponse, error) {
	entry, err := s.repo.FindEntryByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, errors.NotFoundResource("JournalEntry", id)
	}
	if entry.Status != model.EntryStatusDraft {
		return nil, errors.NewBusinessError("Only DRAFT entries can be submitted for approval")
	}

	// Check fiscal period
	if err := s.checkPeriodOpen(ctx, tenantID, entry.EntryDate); err != nil {
		return nil, err
	}

	entry.Status = model.EntryStatusPendingApproval
	if err := s.repo.UpdateEntryStatus(ctx, entry); err != nil {
		return nil, fmt.Errorf("submit for approval: %w", err)
	}

	s.audit.Log(ctx, "SUBMIT_FOR_APPROVAL", "JournalEntry", id.String(), map[string]any{
		"reference": entry.Reference,
	})

	resp := model.ToJournalEntryResponse(entry)
	return &resp, nil
}

// ApproveEntry approves a PENDING_APPROVAL entry and posts it.
func (s *AccountingService) ApproveEntry(ctx context.Context, id uuid.UUID, tenantID, approverID string) (*model.JournalEntryResponse, error) {
	entry, err := s.repo.FindEntryByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, errors.NotFoundResource("JournalEntry", id)
	}
	if entry.Status != model.EntryStatusPendingApproval {
		return nil, errors.NewBusinessError("Only PENDING_APPROVAL entries can be approved")
	}

	// Segregation of duties: approver must differ from creator
	if entry.CreatedBy != nil && *entry.CreatedBy == approverID {
		return nil, errors.Forbidden("Cannot approve your own journal entry (segregation of duties)")
	}

	// Check fiscal period
	if err := s.checkPeriodOpen(ctx, tenantID, entry.EntryDate); err != nil {
		return nil, err
	}

	now := time.Now()
	entry.Status = model.EntryStatusPosted
	entry.ApprovedBy = &approverID
	entry.ApprovedAt = &now
	entry.PostedBy = &approverID

	if err := s.repo.UpdateEntryStatus(ctx, entry); err != nil {
		return nil, fmt.Errorf("approve entry: %w", err)
	}

	// Now apply balances
	if err := s.repo.ApplyEntryToBalances(ctx, entry); err != nil {
		return nil, fmt.Errorf("apply balances: %w", err)
	}

	s.publisher.PublishJournalPosted(ctx, entry)
	s.audit.Log(ctx, "APPROVE_ENTRY", "JournalEntry", id.String(), map[string]any{
		"reference": entry.Reference, "approvedBy": approverID,
	})

	resp := model.ToJournalEntryResponse(entry)
	return &resp, nil
}

// RejectEntry rejects a PENDING_APPROVAL entry.
func (s *AccountingService) RejectEntry(ctx context.Context, id uuid.UUID, tenantID, reason string) (*model.JournalEntryResponse, error) {
	entry, err := s.repo.FindEntryByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, errors.NotFoundResource("JournalEntry", id)
	}
	if entry.Status != model.EntryStatusPendingApproval {
		return nil, errors.NewBusinessError("Only PENDING_APPROVAL entries can be rejected")
	}

	entry.Status = model.EntryStatusRejected
	entry.RejectionReason = &reason

	if err := s.repo.UpdateEntryStatus(ctx, entry); err != nil {
		return nil, fmt.Errorf("reject entry: %w", err)
	}

	s.audit.Log(ctx, "REJECT_ENTRY", "JournalEntry", id.String(), map[string]any{
		"reference": entry.Reference, "reason": reason,
	})

	resp := model.ToJournalEntryResponse(entry)
	return &resp, nil
}

// ReverseEntry reverses a POSTED entry by creating a mirror entry.
func (s *AccountingService) ReverseEntry(ctx context.Context, id uuid.UUID, tenantID, userID, reason string) (*model.JournalEntryResponse, error) {
	entry, err := s.repo.FindEntryByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, errors.NotFoundResource("JournalEntry", id)
	}
	if entry.Status != model.EntryStatusPosted {
		return nil, errors.NewBusinessError("Only POSTED entries can be reversed")
	}

	// Check fiscal period for reversal date
	if err := s.checkPeriodOpen(ctx, tenantID, time.Now()); err != nil {
		return nil, err
	}

	// Mark original as REVERSED
	now := time.Now()
	entry.Status = model.EntryStatusReversed
	entry.ReversedBy = &userID
	entry.ReversedAt = &now
	entry.ReversalReason = &reason
	if err := s.repo.UpdateEntryStatus(ctx, entry); err != nil {
		return nil, fmt.Errorf("mark entry reversed: %w", err)
	}

	// Create mirror entry with flipped debits/credits
	desc := fmt.Sprintf("Reversal of %s: %s", entry.Reference, reason)
	reversal := &model.JournalEntry{
		TenantID:          tenantID,
		Reference:         "REV-" + entry.Reference,
		Description:       &desc,
		EntryDate:         now,
		Status:            model.EntryStatusPosted,
		TotalDebit:        entry.TotalCredit,
		TotalCredit:       entry.TotalDebit,
		PostedBy:          &userID,
		CreatedBy:         &userID,
		OriginalEntryID:   &entry.ID,
		IsSystemGenerated: false,
	}
	for i, line := range entry.Lines {
		reversal.Lines = append(reversal.Lines, model.JournalLine{
			AccountID:    line.AccountID,
			LineNo:       i + 1,
			Description:  line.Description,
			DebitAmount:  line.CreditAmount,
			CreditAmount: line.DebitAmount,
			Currency:     line.Currency,
		})
	}

	if err := s.repo.CreateJournalEntry(ctx, reversal); err != nil {
		return nil, fmt.Errorf("create reversal entry: %w", err)
	}

	s.publisher.PublishJournalPosted(ctx, reversal)
	s.audit.Log(ctx, "REVERSE_ENTRY", "JournalEntry", id.String(), map[string]any{
		"reference": entry.Reference, "reversalId": reversal.ID.String(), "reason": reason,
	})

	resp := model.ToJournalEntryResponse(reversal)
	return &resp, nil
}

// --- Fiscal Periods ---

// ListPeriods returns all fiscal periods for a tenant.
func (s *AccountingService) ListPeriods(ctx context.Context, tenantID string) ([]model.FiscalPeriod, error) {
	return s.repo.ListPeriods(ctx, tenantID)
}

// ClosePeriod closes a fiscal period.
func (s *AccountingService) ClosePeriod(ctx context.Context, tenantID string, year, month int, userID string) (*model.FiscalPeriod, error) {
	existing, err := s.repo.FindPeriod(ctx, tenantID, year, month)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if existing != nil {
		if existing.Status == model.PeriodStatusClosed {
			return nil, errors.NewBusinessError(fmt.Sprintf("Period %d/%02d is already closed", year, month))
		}
		existing.Status = model.PeriodStatusClosed
		existing.ClosedBy = &userID
		existing.ClosedAt = &now
		if err := s.repo.UpsertPeriod(ctx, existing); err != nil {
			return nil, err
		}
		s.audit.Log(ctx, "CLOSE_PERIOD", "FiscalPeriod", fmt.Sprintf("%d-%02d", year, month), map[string]any{
			"closedBy": userID,
		})
		return existing, nil
	}

	period := &model.FiscalPeriod{
		TenantID:    tenantID,
		PeriodYear:  year,
		PeriodMonth: month,
		Status:      model.PeriodStatusClosed,
		ClosedBy:    &userID,
		ClosedAt:    &now,
	}
	if err := s.repo.UpsertPeriod(ctx, period); err != nil {
		return nil, err
	}
	s.audit.Log(ctx, "CLOSE_PERIOD", "FiscalPeriod", fmt.Sprintf("%d-%02d", year, month), map[string]any{
		"closedBy": userID,
	})
	return period, nil
}

// ReopenPeriod reopens a closed fiscal period.
func (s *AccountingService) ReopenPeriod(ctx context.Context, tenantID string, year, month int, userID, reason string) (*model.FiscalPeriod, error) {
	existing, err := s.repo.FindPeriod(ctx, tenantID, year, month)
	if err != nil {
		return nil, err
	}
	if existing == nil || existing.Status == model.PeriodStatusOpen {
		return nil, errors.NewBusinessError(fmt.Sprintf("Period %d/%02d is not closed", year, month))
	}

	existing.Status = model.PeriodStatusOpen
	existing.ReopenedBy = &userID
	existing.ReopenReason = &reason
	if err := s.repo.UpsertPeriod(ctx, existing); err != nil {
		return nil, err
	}
	s.audit.Log(ctx, "REOPEN_PERIOD", "FiscalPeriod", fmt.Sprintf("%d-%02d", year, month), map[string]any{
		"reopenedBy": userID, "reason": reason,
	})
	return existing, nil
}

// --- Audit Log ---

// ListAuditLogs returns audit logs with filters.
func (s *AccountingService) ListAuditLogs(ctx context.Context, tenantID string, entityType, userID *string, from, to *time.Time, page, size int) ([]model.FinancialAuditLog, int64, error) {
	return s.repo.ListAuditLogs(ctx, tenantID, entityType, userID, from, to, page, size)
}

// GetEntityAuditTrail returns the audit trail for a specific entity.
func (s *AccountingService) GetEntityAuditTrail(ctx context.Context, tenantID, entityType, entityID string) ([]model.FinancialAuditLog, error) {
	return s.repo.FindAuditLogsByEntity(ctx, tenantID, entityType, entityID)
}

// --- Cash Flow ---

// GetCashFlow generates a cash flow statement for a period.
func (s *AccountingService) GetCashFlow(ctx context.Context, tenantID string, year, month int) (*model.CashFlowResponse, error) {
	cashAccount, err := s.repo.FindAccountByCodeAndTenantIn(ctx, "1000", []string{tenantID, "system"})
	if err != nil {
		return nil, err
	}
	if cashAccount == nil {
		return nil, errors.NewBusinessError("Cash account (1000) not found")
	}

	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1)

	lines, err := s.repo.GetCashFlowLines(ctx, cashAccount.ID, tenantID, from, to)
	if err != nil {
		return nil, err
	}

	resp := &model.CashFlowResponse{
		PeriodYear:  year,
		PeriodMonth: month,
	}

	for _, line := range lines {
		item := model.CashFlowItem{
			Description: line.CounterAccountName,
			Amount:      line.NetAmount.Neg(), // Cash perspective is opposite of counter-account
		}

		switch {
		case isOperatingAccount(line.CounterAccountCode):
			resp.OperatingItems = append(resp.OperatingItems, item)
			resp.TotalOperating = resp.TotalOperating.Add(item.Amount)
		case isInvestingAccount(line.CounterAccountCode):
			resp.InvestingItems = append(resp.InvestingItems, item)
			resp.TotalInvesting = resp.TotalInvesting.Add(item.Amount)
		default: // Financing
			resp.FinancingItems = append(resp.FinancingItems, item)
			resp.TotalFinancing = resp.TotalFinancing.Add(item.Amount)
		}
	}

	resp.NetCashFlow = resp.TotalOperating.Add(resp.TotalInvesting).Add(resp.TotalFinancing)

	// Get opening cash balance
	openingNet, err := s.repo.GetNetBalance(ctx, cashAccount.ID, tenantID)
	if err != nil {
		return nil, err
	}
	resp.ClosingCash = openingNet
	resp.OpeningCash = openingNet.Sub(resp.NetCashFlow)

	return resp, nil
}

// checkPeriodOpen verifies the fiscal period for the given date is not closed.
func (s *AccountingService) checkPeriodOpen(ctx context.Context, tenantID string, entryDate time.Time) error {
	year := entryDate.Year()
	month := int(entryDate.Month())
	period, err := s.repo.FindPeriod(ctx, tenantID, year, month)
	if err != nil {
		return err
	}
	if period != nil && period.Status == model.PeriodStatusClosed {
		return errors.NewBusinessError(fmt.Sprintf("Fiscal period %d/%02d is closed", year, month))
	}
	return nil
}

// isOperatingAccount classifies an account code as operating activity.
func isOperatingAccount(code string) bool {
	// Loans (1100, 1200, 1300), income (4xxx), expenses (5xxx, 6xxx), customer deposits (2000)
	if len(code) >= 1 {
		switch code[0] {
		case '4', '5', '6':
			return true
		}
	}
	switch code {
	case "1100", "1200", "1250", "1300", "1400", "1410", "1150", "2000":
		return true
	}
	return false
}

// isInvestingAccount classifies an account code as investing activity.
func isInvestingAccount(code string) bool {
	switch code {
	case "1600", "1610":
		return true
	}
	return false
}

// --- Private helpers ---

func (s *AccountingService) resolveAccountID(ctx context.Context, tenantID, code string) (uuid.UUID, error) {
	account, err := s.repo.FindAccountByCodeAndTenantIn(ctx, code, []string{tenantID, "system"})
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve account %s: %w", code, err)
	}
	if account == nil {
		return uuid.Nil, errors.NewBusinessError("GL account not found: " + code)
	}
	return account.ID, nil
}

func (s *AccountingService) buildSystemEntry(tenantID, reference, description, sourceEvent, sourceID string, drAccountID, crAccountID uuid.UUID, amount decimal.Decimal) *model.JournalEntry {
	postedBy := "system"
	entry := &model.JournalEntry{
		TenantID:          tenantID,
		Reference:         reference,
		Description:       &description,
		EntryDate:         time.Now(),
		Status:            model.EntryStatusPosted,
		SourceEvent:       &sourceEvent,
		SourceID:          &sourceID,
		TotalDebit:        amount,
		TotalCredit:       amount,
		PostedBy:          &postedBy,
		CreatedBy:         &postedBy,
		IsSystemGenerated: true,
		Lines: []model.JournalLine{
			{
				AccountID: drAccountID, LineNo: 1, DebitAmount: amount, CreditAmount: decimal.Zero, Currency: "KES",
			},
			{
				AccountID: crAccountID, LineNo: 2, DebitAmount: decimal.Zero, CreditAmount: amount, Currency: "KES",
			},
		},
	}
	return entry
}

func getDecimalFromPayload(m map[string]any, key string) decimal.Decimal {
	if m == nil {
		return decimal.Zero
	}
	v, ok := m[key]
	if !ok || v == nil {
		return decimal.Zero
	}
	switch val := v.(type) {
	case float64:
		return decimal.NewFromFloat(val)
	case string:
		d, err := decimal.NewFromString(val)
		if err != nil {
			return decimal.Zero
		}
		return d
	case json.Number:
		d, err := decimal.NewFromString(val.String())
		if err != nil {
			return decimal.Zero
		}
		return d
	default:
		d, err := decimal.NewFromString(fmt.Sprintf("%v", val))
		if err != nil {
			return decimal.Zero
		}
		return d
	}
}
