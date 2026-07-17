package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"github.com/athena-lms/go-services/internal/common/market"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/overdraft/client"
	ovEvent "github.com/athena-lms/go-services/internal/overdraft/event"
	"github.com/athena-lms/go-services/internal/overdraft/model"
	"github.com/athena-lms/go-services/internal/overdraft/repository"
)

// WalletService manages customer wallets and transactions.
type WalletService struct {
	repo          *repository.Repository
	publisher     *ovEvent.Publisher
	audit         *AuditService
	scoringClient *client.ScoringClient
	logger        *zap.Logger
}

// NewWalletService creates a new WalletService.
func NewWalletService(repo *repository.Repository, publisher *ovEvent.Publisher, audit *AuditService, logger *zap.Logger) *WalletService {
	return &WalletService{repo: repo, publisher: publisher, audit: audit, logger: logger}
}

// SetScoringClient sets the credit scoring client for overdraft applications.
func (s *WalletService) SetScoringClient(sc *client.ScoringClient) {
	s.scoringClient = sc
}

// CreateWallet creates a new customer wallet.
func (s *WalletService) CreateWallet(ctx context.Context, req model.CreateWalletRequest, tenantID string) (*model.WalletResponse, error) {
	if req.CustomerID == "" {
		return nil, errors.BadRequest("customerId is required")
	}

	exists, err := s.repo.WalletExistsByTenantAndCustomer(ctx, tenantID, req.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("check wallet existence: %w", err)
	}
	if exists {
		return nil, errors.NewBusinessError("Wallet already exists for customer: " + req.CustomerID)
	}

	currency := req.Currency
	if currency == "" {
		currency = market.Currency()
	}

	wallet := &model.CustomerWallet{
		TenantID:         tenantID,
		CustomerID:       req.CustomerID,
		AccountNumber:    generateAccountNumber(req.CustomerID),
		Currency:         currency,
		CurrentBalance:   decimal.Zero,
		AvailableBalance: decimal.Zero,
		Status:           "ACTIVE",
	}

	if err := s.repo.CreateWallet(ctx, wallet); err != nil {
		return nil, fmt.Errorf("create wallet: %w", err)
	}

	s.audit.Audit(ctx, tenantID, "WALLET", wallet.ID, "CREATED",
		nil, map[string]interface{}{"customerId": req.CustomerID, "accountNumber": wallet.AccountNumber}, nil)

	s.logger.Info("Created wallet", zap.String("walletId", wallet.ID.String()),
		zap.String("customerId", req.CustomerID), zap.String("tenant", tenantID))

	return toWalletResponse(wallet), nil
}

// GetWalletByCustomer returns the wallet for a given customer.
func (s *WalletService) GetWalletByCustomer(ctx context.Context, customerID, tenantID string) (*model.WalletResponse, error) {
	wallet, err := s.repo.FindWalletByTenantAndCustomer(ctx, tenantID, customerID)
	if err != nil {
		return nil, err
	}
	if wallet == nil {
		return nil, errors.NotFound("Wallet not found for customer: " + customerID)
	}
	return toWalletResponse(wallet), nil
}

// GetWallet returns a wallet by ID.
func (s *WalletService) GetWallet(ctx context.Context, walletID uuid.UUID, tenantID string) (*model.WalletResponse, error) {
	wallet, err := s.repo.FindWalletByTenantAndID(ctx, tenantID, walletID)
	if err != nil {
		return nil, err
	}
	if wallet == nil {
		return nil, errors.NotFound("Wallet not found: " + walletID.String())
	}
	return toWalletResponse(wallet), nil
}

// ListWallets returns all wallets for a tenant.
func (s *WalletService) ListWallets(ctx context.Context, tenantID string) ([]model.WalletResponse, error) {
	wallets, err := s.repo.ListWalletsByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]model.WalletResponse, 0, len(wallets))
	for i := range wallets {
		result = append(result, *toWalletResponse(&wallets[i]))
	}
	return result, nil
}

// checkDepositAllowed enforces wallet status on the credit rail (BLOCKER-7).
// Deposits onto a FROZEN/SUSPENDED wallet stay allowed because they only
// reduce the lender's exposure; only CLOSED wallets reject deposits.
func checkDepositAllowed(status string) error {
	if status == "CLOSED" {
		return errors.NewBusinessError("Wallet is CLOSED; deposits are not allowed")
	}
	return nil
}

// checkWithdrawAllowed enforces wallet status on the debit rail (BLOCKER-7):
// only ACTIVE wallets may withdraw or draw on the overdraft, so fraud/AML
// freezes (SUSPENDED/FROZEN/CLOSED) are effective on the wallet rail.
func checkWithdrawAllowed(status string) error {
	if status != "ACTIVE" {
		return errors.NewBusinessError("Wallet is " + status + "; withdrawals are not allowed")
	}
	return nil
}

// isUniqueViolation reports whether err is a Postgres unique violation
// (SQLSTATE 23505) — for wallet transactions, the (wallet_id, reference)
// idempotency index added in migration 000004.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return stderrors.As(err, &pgErr) && pgErr.Code == "23505"
}

// depositAllocation is the outcome of applying the repayment waterfall to a deposit.
type depositAllocation struct {
	FeesRepaid      decimal.Decimal
	InterestRepaid  decimal.Decimal
	PrincipalRepaid decimal.Decimal
	BalanceAfter    decimal.Decimal
	// PaidFees are the fees that received a payment in this allocation and
	// must be persisted (AmountPaid/Status/ChargedAt mutated in place).
	PaidFees []*model.OverdraftFee
}

// applyDepositWaterfall allocates a deposit in the order
// fees -> accrued interest -> drawn principal, mutating facility and fees in
// place, and returns the allocation.
//
// Correct accounting (BLOCKER-6): money used to settle fees and accrued
// interest is income to the lender and must NOT raise the customer's balance:
//
//	balanceAfter = balanceBefore + (amount - feesRepaid - interestRepaid)
//
// Principal repayment needs no explicit deduction — the negative balance
// rising toward zero IS the principal being repaid, mirrored by the same
// reduction of facility.DrawnPrincipal.
func applyDepositWaterfall(balanceBefore, amount decimal.Decimal, facility *model.OverdraftFacility, fees []model.OverdraftFee) depositAllocation {
	alloc := depositAllocation{
		FeesRepaid:      decimal.Zero,
		InterestRepaid:  decimal.Zero,
		PrincipalRepaid: decimal.Zero,
	}
	remaining := amount

	// 1) Fees: allocate against the unpaid remainder of each pending fee.
	// Partial payments accumulate in AmountPaid; the fee is only marked
	// CHARGED once fully paid (BLOCKER-6 partial-fee fix).
	for i := range fees {
		if remaining.LessThanOrEqual(decimal.Zero) {
			break
		}
		fee := &fees[i]
		outstanding := fee.Outstanding()
		if outstanding.LessThanOrEqual(decimal.Zero) {
			continue
		}
		payment := decimal.Min(remaining, outstanding)
		fee.AmountPaid = fee.AmountPaid.Add(payment)
		if fee.AmountPaid.GreaterThanOrEqual(fee.Amount) {
			fee.Status = "CHARGED"
			now := time.Now()
			fee.ChargedAt = &now
		}
		alloc.FeesRepaid = alloc.FeesRepaid.Add(payment)
		alloc.PaidFees = append(alloc.PaidFees, fee)
		remaining = remaining.Sub(payment)
	}

	// 2) Accrued interest
	if remaining.GreaterThan(decimal.Zero) && facility.AccruedInterest.GreaterThan(decimal.Zero) {
		payment := decimal.Min(remaining, facility.AccruedInterest)
		alloc.InterestRepaid = payment
		facility.AccruedInterest = facility.AccruedInterest.Sub(payment)
		remaining = remaining.Sub(payment)
	}

	// 3) Drawn principal
	if remaining.GreaterThan(decimal.Zero) && facility.DrawnPrincipal.GreaterThan(decimal.Zero) {
		payment := decimal.Min(remaining, facility.DrawnPrincipal)
		alloc.PrincipalRepaid = payment
		facility.DrawnPrincipal = facility.DrawnPrincipal.Sub(payment)
	}

	facility.RecalculateDrawnAmount()

	alloc.BalanceAfter = balanceBefore.Add(amount).Sub(alloc.FeesRepaid).Sub(alloc.InterestRepaid)
	return alloc
}

// Deposit adds funds to a wallet and applies the repayment waterfall if overdraft is drawn.
//
// The whole operation runs in ONE DB transaction with the wallet, facility and
// fee rows locked FOR UPDATE, and is idempotent on (walletId, reference): a
// retried mobile-money callback returns the original transaction instead of
// applying twice (BLOCKER-5).
func (s *WalletService) Deposit(ctx context.Context, walletID uuid.UUID, req model.WalletTransactionRequest, tenantID string) (*model.WalletTransactionResponse, error) {
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return nil, errors.BadRequest("amount must be > 0")
	}
	if req.Reference == "" {
		return nil, errors.BadRequest("reference is required")
	}

	dbTx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer dbTx.Rollback(ctx)

	wallet, err := s.repo.FindWalletForUpdate(ctx, dbTx, tenantID, walletID)
	if err != nil {
		return nil, err
	}
	if wallet == nil {
		return nil, errors.NotFound("Wallet not found: " + walletID.String())
	}
	if err := checkDepositAllowed(wallet.Status); err != nil {
		return nil, err
	}

	// Idempotency pre-check: the wallet row lock serializes concurrent
	// requests on this wallet, so a duplicate reference is reliably visible.
	if existing, err := s.repo.FindTransactionByWalletAndReferenceTx(ctx, dbTx, walletID, req.Reference); err != nil {
		return nil, err
	} else if existing != nil {
		if existing.TransactionType != "DEPOSIT" {
			return nil, errors.Conflict("Reference already used by a " + existing.TransactionType + " transaction: " + req.Reference)
		}
		s.logger.Info("Duplicate deposit reference; returning existing transaction",
			zap.String("walletId", walletID.String()), zap.String("reference", req.Reference))
		return toTxResponse(existing), nil
	}

	facility, err := s.repo.FindLatestFacilityForUpdate(ctx, dbTx, walletID)
	if err != nil {
		return nil, err
	}

	balanceBefore := wallet.CurrentBalance

	// Default when nothing is drawn: the full deposit credits the balance.
	alloc := depositAllocation{
		FeesRepaid:      decimal.Zero,
		InterestRepaid:  decimal.Zero,
		PrincipalRepaid: decimal.Zero,
		BalanceAfter:    balanceBefore.Add(req.Amount),
	}

	if facility != nil && facility.Status == "ACTIVE" && facility.DrawnAmount.GreaterThan(decimal.Zero) {
		pendingFees, err := s.repo.FindPendingFeesForUpdate(ctx, dbTx, facility.ID)
		if err != nil {
			return nil, err
		}

		alloc = applyDepositWaterfall(balanceBefore, req.Amount, facility, pendingFees)

		for _, fee := range alloc.PaidFees {
			if err := s.repo.ApplyFeePaymentTx(ctx, dbTx, fee); err != nil {
				return nil, err
			}
		}
		if err := s.repo.UpdateFacilityTx(ctx, dbTx, facility); err != nil {
			return nil, err
		}
	}

	wallet.CurrentBalance = alloc.BalanceAfter
	if facility != nil && facility.Status == "ACTIVE" {
		overdraftHeadroom := facility.ApprovedLimit.Sub(facility.DrawnAmount)
		wallet.AvailableBalance = alloc.BalanceAfter.Add(overdraftHeadroom)
	} else {
		wallet.AvailableBalance = alloc.BalanceAfter
	}
	if err := s.repo.UpdateWalletTx(ctx, dbTx, wallet); err != nil {
		return nil, err
	}

	ref := req.Reference
	desc := req.Description
	tx := &model.WalletTransaction{
		TenantID:        tenantID,
		WalletID:        wallet.ID,
		TransactionType: "DEPOSIT",
		Amount:          req.Amount,
		BalanceBefore:   balanceBefore,
		BalanceAfter:    alloc.BalanceAfter,
		Reference:       &ref,
		Description:     strPtr(desc),
	}
	if err := s.repo.CreateTransactionTx(ctx, dbTx, tx); err != nil {
		if isUniqueViolation(err) {
			// Lost a duplicate race despite the pre-check: discard our work
			// and return the winner's transaction (idempotent behaviour).
			_ = dbTx.Rollback(ctx)
			existing, ferr := s.repo.FindTransactionByWalletAndReference(ctx, walletID, req.Reference)
			if ferr == nil && existing != nil && existing.TransactionType == "DEPOSIT" {
				return toTxResponse(existing), nil
			}
			return nil, errors.Conflict("Duplicate transaction reference: " + req.Reference)
		}
		return nil, err
	}

	if err := dbTx.Commit(ctx); err != nil {
		return nil, err
	}

	// Post-commit: events and audit describe only durably committed state.
	totalRepaid := alloc.FeesRepaid.Add(alloc.InterestRepaid).Add(alloc.PrincipalRepaid)
	if totalRepaid.GreaterThan(decimal.Zero) {
		s.publisher.PublishOverdraftRepaidDetailed(ctx, walletID, wallet.CustomerID,
			totalRepaid, alloc.InterestRepaid, alloc.PrincipalRepaid, alloc.FeesRepaid, tenantID)
	}

	s.audit.Audit(ctx, tenantID, "WALLET", walletID, "DEPOSIT",
		map[string]interface{}{"balance": balanceBefore},
		map[string]interface{}{"balance": alloc.BalanceAfter, "feesRepaid": alloc.FeesRepaid,
			"interestRepaid": alloc.InterestRepaid, "principalRepaid": alloc.PrincipalRepaid},
		map[string]interface{}{"amount": req.Amount, "reference": req.Reference})

	return toTxResponse(tx), nil
}

// Withdraw removes funds from a wallet, potentially drawing on the overdraft.
//
// Runs in ONE DB transaction with the wallet and facility rows locked FOR
// UPDATE so two concurrent withdrawals cannot both pass the available-balance
// check (BLOCKER-5), and is idempotent on (walletId, reference).
func (s *WalletService) Withdraw(ctx context.Context, walletID uuid.UUID, req model.WalletTransactionRequest, tenantID string) (*model.WalletTransactionResponse, error) {
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return nil, errors.BadRequest("amount must be > 0")
	}
	if req.Reference == "" {
		return nil, errors.BadRequest("reference is required")
	}

	dbTx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer dbTx.Rollback(ctx)

	wallet, err := s.repo.FindWalletForUpdate(ctx, dbTx, tenantID, walletID)
	if err != nil {
		return nil, err
	}
	if wallet == nil {
		return nil, errors.NotFound("Wallet not found: " + walletID.String())
	}
	if err := checkWithdrawAllowed(wallet.Status); err != nil {
		return nil, err
	}

	// Idempotency pre-check (serialized by the wallet row lock).
	if existing, err := s.repo.FindTransactionByWalletAndReferenceTx(ctx, dbTx, walletID, req.Reference); err != nil {
		return nil, err
	} else if existing != nil {
		if existing.TransactionType != "WITHDRAWAL" && existing.TransactionType != "OVERDRAFT_DRAW" {
			return nil, errors.Conflict("Reference already used by a " + existing.TransactionType + " transaction: " + req.Reference)
		}
		s.logger.Info("Duplicate withdrawal reference; returning existing transaction",
			zap.String("walletId", walletID.String()), zap.String("reference", req.Reference))
		return toTxResponse(existing), nil
	}

	if wallet.AvailableBalance.LessThan(req.Amount) {
		return nil, errors.NewBusinessError(fmt.Sprintf(
			"Insufficient balance. Available: %s, Requested: %s",
			wallet.AvailableBalance.String(), req.Amount.String()))
	}

	balanceBefore := wallet.CurrentBalance
	balanceAfter := balanceBefore.Sub(req.Amount)
	wallet.CurrentBalance = balanceAfter

	txType := "WITHDRAWAL"
	overdraftDrawn := balanceAfter.LessThan(decimal.Zero)
	if overdraftDrawn {
		txType = "OVERDRAFT_DRAW"
	}

	facility, err := s.repo.FindLatestFacilityForUpdate(ctx, dbTx, walletID)
	if err != nil {
		return nil, err
	}

	if facility != nil && facility.Status == "ACTIVE" {
		if overdraftDrawn {
			previousOverdraft := decimal.Max(balanceBefore.Neg(), decimal.Zero)
			newOverdraft := balanceAfter.Neg()
			additionalDraw := newOverdraft.Sub(previousOverdraft)
			facility.DrawnPrincipal = facility.DrawnPrincipal.Add(additionalDraw)
			facility.RecalculateDrawnAmount()
			if err := s.repo.UpdateFacilityTx(ctx, dbTx, facility); err != nil {
				return nil, err
			}
		}
		overdraftHeadroom := facility.ApprovedLimit.Sub(facility.DrawnAmount)
		wallet.AvailableBalance = balanceAfter.Add(overdraftHeadroom)
	} else {
		wallet.AvailableBalance = balanceAfter
	}

	if err := s.repo.UpdateWalletTx(ctx, dbTx, wallet); err != nil {
		return nil, err
	}

	ref := req.Reference
	desc := req.Description
	tx := &model.WalletTransaction{
		TenantID:        tenantID,
		WalletID:        wallet.ID,
		TransactionType: txType,
		Amount:          req.Amount,
		BalanceBefore:   balanceBefore,
		BalanceAfter:    balanceAfter,
		Reference:       &ref,
		Description:     strPtr(desc),
	}
	if err := s.repo.CreateTransactionTx(ctx, dbTx, tx); err != nil {
		if isUniqueViolation(err) {
			// Lost a duplicate race despite the pre-check: discard our work
			// and return the winner's transaction (idempotent behaviour).
			_ = dbTx.Rollback(ctx)
			existing, ferr := s.repo.FindTransactionByWalletAndReference(ctx, walletID, req.Reference)
			if ferr == nil && existing != nil &&
				(existing.TransactionType == "WITHDRAWAL" || existing.TransactionType == "OVERDRAFT_DRAW") {
				return toTxResponse(existing), nil
			}
			return nil, errors.Conflict("Duplicate transaction reference: " + req.Reference)
		}
		return nil, err
	}

	if err := dbTx.Commit(ctx); err != nil {
		return nil, err
	}

	// Post-commit: events and audit describe only durably committed state.
	if overdraftDrawn {
		previousOverdraft := decimal.Max(balanceBefore.Neg(), decimal.Zero)
		actualDraw := balanceAfter.Neg().Sub(previousOverdraft)
		s.publisher.PublishOverdraftDrawn(ctx, walletID, wallet.CustomerID, actualDraw, tenantID)
	}

	s.audit.Audit(ctx, tenantID, "WALLET", walletID, "WITHDRAWAL",
		map[string]interface{}{"balance": balanceBefore},
		map[string]interface{}{"balance": balanceAfter},
		map[string]interface{}{"amount": req.Amount, "reference": req.Reference, "overdraftDrawn": overdraftDrawn})

	return toTxResponse(tx), nil
}

// GetTransactions returns paginated transactions for a wallet.
func (s *WalletService) GetTransactions(ctx context.Context, walletID uuid.UUID, tenantID string, page, size int) (dto.PageResponse, error) {
	wallet, err := s.repo.FindWalletByTenantAndID(ctx, tenantID, walletID)
	if err != nil {
		return dto.PageResponse{}, err
	}
	if wallet == nil {
		return dto.PageResponse{}, errors.NotFound("Wallet not found: " + walletID.String())
	}

	txns, total, err := s.repo.ListTransactions(ctx, walletID, tenantID, size, page*size)
	if err != nil {
		return dto.PageResponse{}, err
	}

	responses := make([]model.WalletTransactionResponse, 0, len(txns))
	for i := range txns {
		responses = append(responses, *toTxResponse(&txns[i]))
	}

	return dto.NewPageResponse(responses, page, size, total), nil
}

func generateAccountNumber(customerID string) string {
	cleaned := strings.ToUpper(customerID)
	cleaned = strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, cleaned)
	prefix := cleaned
	if len(prefix) > 6 {
		prefix = prefix[:6]
	}
	suffix := strings.ToUpper(uuid.New().String()[:8])
	return "WLT-" + prefix + "-" + suffix
}

func toWalletResponse(w *model.CustomerWallet) *model.WalletResponse {
	return &model.WalletResponse{
		ID:               w.ID,
		TenantID:         w.TenantID,
		CustomerID:       w.CustomerID,
		AccountNumber:    w.AccountNumber,
		Currency:         w.Currency,
		CurrentBalance:   w.CurrentBalance,
		AvailableBalance: w.AvailableBalance,
		Status:           w.Status,
		CreatedAt:        w.CreatedAt,
		UpdatedAt:        w.UpdatedAt,
	}
}

func toTxResponse(tx *model.WalletTransaction) *model.WalletTransactionResponse {
	return &model.WalletTransactionResponse{
		ID:              tx.ID,
		WalletID:        tx.WalletID,
		TransactionType: tx.TransactionType,
		Amount:          tx.Amount,
		BalanceBefore:   tx.BalanceBefore,
		BalanceAfter:    tx.BalanceAfter,
		Reference:       tx.Reference,
		Description:     tx.Description,
		CreatedAt:       tx.CreatedAt,
	}
}

// ApplyOverdraft applies for an overdraft facility on a wallet.
// Fetches credit score, determines band, looks up band config, creates facility,
// charges arrangement fee, updates wallet available balance, publishes events.
func (s *WalletService) ApplyOverdraft(ctx context.Context, walletID uuid.UUID, tenantID string) (*model.OverdraftFacilityResponse, error) {
	// HIGH-6: fail closed. An overdraft is a real credit decision, so a
	// missing scoring dependency must reject the application — never fabricate
	// a score and approve a facility on a misconfigured deployment.
	if s.scoringClient == nil {
		return nil, errors.NewBusinessError("Credit scoring is unavailable; cannot approve an overdraft facility")
	}

	wallet, err := s.repo.FindWalletByTenantAndID(ctx, tenantID, walletID)
	if err != nil || wallet == nil {
		return nil, errors.NotFoundResource("Wallet", walletID)
	}

	// Check if active facility already exists
	existing, err := s.repo.FindLatestFacilityByWallet(ctx, walletID)
	if err != nil {
		return nil, fmt.Errorf("check existing facility: %w", err)
	}
	if existing != nil && existing.Status == "ACTIVE" {
		return nil, errors.NewBusinessError("Active overdraft facility already exists for this wallet")
	}

	// Get credit score from the AI scoring service (fail closed on any error).
	scoreResult, err := s.scoringClient.GetLatestScore(ctx, wallet.CustomerID)
	if err != nil {
		s.logger.Error("Credit scoring lookup failed; rejecting overdraft application",
			zap.String("customerId", wallet.CustomerID), zap.Error(err))
		return nil, errors.NewBusinessError("Credit scoring is unavailable; cannot approve an overdraft facility")
	}

	// Look up band configuration for this tenant
	bandConfig, err := s.repo.FindBandConfigByTenantBandStatus(ctx, tenantID, scoreResult.Band, "ACTIVE")
	if err != nil {
		return nil, fmt.Errorf("lookup band config: %w", err)
	}
	// Fallback to system tenant config
	if bandConfig == nil {
		bandConfig, err = s.repo.FindBandConfigByTenantBandStatus(ctx, "system", scoreResult.Band, "ACTIVE")
		if err != nil || bandConfig == nil {
			return nil, errors.NewBusinessError("No credit band configuration found for band: " + scoreResult.Band)
		}
	}

	// Create the facility
	now := time.Now()
	firstBilling := now.AddDate(0, 1, 0)
	expiryDate := now.AddDate(1, 0, 0) // 1 year validity
	facility := &model.OverdraftFacility{
		TenantID:        tenantID,
		WalletID:        walletID,
		CustomerID:      wallet.CustomerID,
		CreditScore:     scoreResult.Score,
		CreditBand:      scoreResult.Band,
		ApprovedLimit:   bandConfig.ApprovedLimit,
		DrawnAmount:     decimal.Zero,
		DrawnPrincipal:  decimal.Zero,
		AccruedInterest: decimal.Zero,
		InterestRate:    bandConfig.InterestRate,
		Status:          "ACTIVE",
		DPD:             0,
		NPLStage:        "PERFORMING",
		NextBillingDate: &firstBilling,
		ExpiryDate:      &expiryDate,
	}

	if err := s.repo.CreateFacility(ctx, facility); err != nil {
		return nil, fmt.Errorf("create facility: %w", err)
	}

	// Charge arrangement fee if applicable
	if bandConfig.ArrangementFee.GreaterThan(decimal.Zero) {
		ref := fmt.Sprintf("ARR-FEE-%s", facility.ID.String()[:8])
		fee := &model.OverdraftFee{
			TenantID:   tenantID,
			FacilityID: facility.ID,
			FeeType:    "ARRANGEMENT",
			Amount:     bandConfig.ArrangementFee,
			Reference:  &ref,
			Status:     "PENDING",
		}
		if err := s.repo.CreateFee(ctx, fee); err != nil {
			s.logger.Warn("Failed to create arrangement fee", zap.Error(err))
		} else {
			s.publisher.PublishFeeCharged(ctx, walletID, wallet.CustomerID, "ARRANGEMENT", bandConfig.ArrangementFee, ref, tenantID)
		}
	}

	// Update wallet available balance with new overdraft headroom
	wallet.AvailableBalance = wallet.CurrentBalance.Add(facility.ApprovedLimit)
	if err := s.repo.UpdateWallet(ctx, wallet); err != nil {
		s.logger.Warn("Failed to update wallet available balance", zap.Error(err))
	}

	// Publish event
	s.publisher.PublishOverdraftApplied(ctx, walletID, wallet.CustomerID, scoreResult.Band, bandConfig.ApprovedLimit, tenantID)

	// Audit trail
	s.audit.Audit(ctx, tenantID, "FACILITY", facility.ID, "FACILITY_APPROVED",
		nil,
		map[string]interface{}{
			"creditScore":   scoreResult.Score,
			"creditBand":    scoreResult.Band,
			"approvedLimit": bandConfig.ApprovedLimit.String(),
			"interestRate":  bandConfig.InterestRate.String(),
		},
		map[string]interface{}{"walletId": walletID.String(), "customerId": wallet.CustomerID})

	s.logger.Info("Overdraft facility approved",
		zap.String("facilityId", facility.ID.String()),
		zap.String("walletId", walletID.String()),
		zap.Int("score", scoreResult.Score),
		zap.String("band", scoreResult.Band),
		zap.String("limit", bandConfig.ApprovedLimit.String()))

	return &model.OverdraftFacilityResponse{
		ID:                 facility.ID,
		TenantID:           tenantID,
		WalletID:           walletID,
		CustomerID:         wallet.CustomerID,
		CreditScore:        scoreResult.Score,
		CreditBand:         scoreResult.Band,
		ApprovedLimit:      bandConfig.ApprovedLimit,
		DrawnAmount:        decimal.Zero,
		AvailableOverdraft: bandConfig.ApprovedLimit,
		InterestRate:       bandConfig.InterestRate,
		DrawnPrincipal:     decimal.Zero,
		AccruedInterest:    decimal.Zero,
		Status:             "ACTIVE",
		DPD:                0,
		NPLStage:           "PERFORMING",
		AppliedAt:          facility.AppliedAt,
		ApprovedAt:         facility.ApprovedAt,
		CreatedAt:          facility.CreatedAt,
	}, nil
}

// GetOverdraftFacility returns the overdraft facility for a wallet.
func (s *WalletService) GetOverdraftFacility(ctx context.Context, walletID uuid.UUID, tenantID string) (map[string]any, error) {
	facility, err := s.repo.FindLatestFacilityByWallet(ctx, walletID)
	if err != nil || facility == nil {
		return map[string]any{
			"walletId":  walletID,
			"hasOD":     false,
			"limit":     decimal.Zero,
			"drawn":     decimal.Zero,
			"available": decimal.Zero,
		}, nil
	}
	available := facility.ApprovedLimit.Sub(facility.DrawnAmount)
	if available.LessThan(decimal.Zero) {
		available = decimal.Zero
	}
	return map[string]any{
		"id":              facility.ID,
		"walletId":        facility.WalletID,
		"customerId":      facility.CustomerID,
		"hasOD":           true,
		"creditScore":     facility.CreditScore,
		"creditBand":      facility.CreditBand,
		"limit":           facility.ApprovedLimit,
		"drawn":           facility.DrawnAmount,
		"drawnPrincipal":  facility.DrawnPrincipal,
		"accruedInterest": facility.AccruedInterest,
		"available":       available,
		"interestRate":    facility.InterestRate,
		"status":          facility.Status,
		"dpd":             facility.DPD,
		"nplStage":        facility.NPLStage,
	}, nil
}

// SuspendOverdraft suspends an overdraft facility.
// Recalculates wallet available balance, publishes events, creates audit log.
func (s *WalletService) SuspendOverdraft(ctx context.Context, walletID uuid.UUID, tenantID string) (*model.OverdraftFacilityResponse, error) {
	wallet, err := s.repo.FindWalletByTenantAndID(ctx, tenantID, walletID)
	if err != nil || wallet == nil {
		return nil, errors.NotFoundResource("Wallet", walletID)
	}

	facility, err := s.repo.FindLatestFacilityByWallet(ctx, walletID)
	if err != nil {
		return nil, fmt.Errorf("lookup facility: %w", err)
	}
	if facility == nil {
		return nil, errors.NewBusinessError("No overdraft facility found for this wallet")
	}
	if facility.Status != "ACTIVE" {
		return nil, errors.NewBusinessError("Facility is not active, current status: " + facility.Status)
	}

	previousStatus := facility.Status
	facility.Status = "SUSPENDED"
	if err := s.repo.UpdateFacility(ctx, facility); err != nil {
		return nil, fmt.Errorf("update facility: %w", err)
	}

	// Recalculate wallet available balance (no overdraft headroom when suspended)
	wallet.AvailableBalance = wallet.CurrentBalance
	if wallet.AvailableBalance.LessThan(decimal.Zero) {
		wallet.AvailableBalance = decimal.Zero
	}
	if err := s.repo.UpdateWallet(ctx, wallet); err != nil {
		s.logger.Warn("Failed to update wallet balance", zap.Error(err))
	}

	// Publish event
	s.publisher.PublishOverdraftSuspended(ctx, walletID, wallet.CustomerID, tenantID)

	// Audit trail
	s.audit.Audit(ctx, tenantID, "FACILITY", facility.ID, "SUSPENDED",
		map[string]interface{}{"status": previousStatus},
		map[string]interface{}{"status": "SUSPENDED"},
		map[string]interface{}{"walletId": walletID.String()})

	s.logger.Info("Overdraft facility suspended",
		zap.String("facilityId", facility.ID.String()),
		zap.String("walletId", walletID.String()))

	return &model.OverdraftFacilityResponse{
		ID:                 facility.ID,
		TenantID:           tenantID,
		WalletID:           walletID,
		CustomerID:         wallet.CustomerID,
		CreditScore:        facility.CreditScore,
		CreditBand:         facility.CreditBand,
		ApprovedLimit:      facility.ApprovedLimit,
		DrawnAmount:        facility.DrawnAmount,
		AvailableOverdraft: decimal.Zero,
		InterestRate:       facility.InterestRate,
		DrawnPrincipal:     facility.DrawnPrincipal,
		AccruedInterest:    facility.AccruedInterest,
		Status:             "SUSPENDED",
		DPD:                facility.DPD,
		NPLStage:           facility.NPLStage,
		AppliedAt:          facility.AppliedAt,
		ApprovedAt:         facility.ApprovedAt,
		CreatedAt:          facility.CreatedAt,
	}, nil
}

// GetSummary returns overdraft summary for the tenant with full facility aggregation.
func (s *WalletService) GetSummary(ctx context.Context, tenantID string) (*model.OverdraftSummaryResponse, error) {
	facilities, err := s.repo.ListFacilitiesByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	summary := &model.OverdraftSummaryResponse{
		FacilitiesByBand: make(map[string]int64),
		DrawnByBand:      make(map[string]decimal.Decimal),
	}

	summary.TotalFacilities = int64(len(facilities))
	summary.TotalApprovedLimit = decimal.Zero
	summary.TotalDrawnAmount = decimal.Zero
	summary.TotalAvailableOverdraft = decimal.Zero

	for _, f := range facilities {
		summary.TotalApprovedLimit = summary.TotalApprovedLimit.Add(f.ApprovedLimit)
		summary.TotalDrawnAmount = summary.TotalDrawnAmount.Add(f.DrawnAmount)
		available := f.ApprovedLimit.Sub(f.DrawnAmount)
		if available.LessThan(decimal.Zero) {
			available = decimal.Zero
		}
		summary.TotalAvailableOverdraft = summary.TotalAvailableOverdraft.Add(available)

		if f.Status == "ACTIVE" {
			summary.ActiveFacilities++
		}

		summary.FacilitiesByBand[f.CreditBand]++
		if _, ok := summary.DrawnByBand[f.CreditBand]; !ok {
			summary.DrawnByBand[f.CreditBand] = decimal.Zero
		}
		summary.DrawnByBand[f.CreditBand] = summary.DrawnByBand[f.CreditBand].Add(f.DrawnAmount)
	}

	return summary, nil
}

// GetInterestCharges returns interest charges for the wallet's overdraft facility.
func (s *WalletService) GetInterestCharges(ctx context.Context, walletID uuid.UUID, tenantID string) ([]model.InterestChargeResponse, error) {
	facility, err := s.repo.FindLatestFacilityByWallet(ctx, walletID)
	if err != nil || facility == nil {
		return []model.InterestChargeResponse{}, nil
	}

	charges, err := s.repo.ListInterestCharges(ctx, facility.ID)
	if err != nil {
		return nil, err
	}

	result := make([]model.InterestChargeResponse, 0, len(charges))
	for _, c := range charges {
		result = append(result, model.InterestChargeResponse{
			ID:              c.ID,
			FacilityID:      c.FacilityID,
			ChargeDate:      c.ChargeDate,
			DrawnAmount:     c.DrawnAmount,
			DailyRate:       c.DailyRate,
			InterestCharged: c.InterestCharged,
			Reference:       c.Reference,
			CreatedAt:       c.CreatedAt,
		})
	}
	return result, nil
}

// GetBillingStatements returns billing statements for the wallet's overdraft facility.
func (s *WalletService) GetBillingStatements(ctx context.Context, walletID uuid.UUID, tenantID string) ([]model.BillingStatementResponse, error) {
	facility, err := s.repo.FindLatestFacilityByWallet(ctx, walletID)
	if err != nil || facility == nil {
		return []model.BillingStatementResponse{}, nil
	}

	stmts, err := s.repo.ListBillingStatementsByFacility(ctx, facility.ID)
	if err != nil {
		return nil, err
	}

	result := make([]model.BillingStatementResponse, 0, len(stmts))
	for _, st := range stmts {
		result = append(result, model.BillingStatementResponse{
			ID:                st.ID,
			FacilityID:        st.FacilityID,
			BillingDate:       st.BillingDate,
			PeriodStart:       st.PeriodStart,
			PeriodEnd:         st.PeriodEnd,
			OpeningBalance:    st.OpeningBalance,
			InterestAccrued:   st.InterestAccrued,
			FeesCharged:       st.FeesCharged,
			PaymentsReceived:  st.PaymentsReceived,
			ClosingBalance:    st.ClosingBalance,
			MinimumPaymentDue: st.MinimumPaymentDue,
			DueDate:           st.DueDate,
			Status:            st.Status,
			CreatedAt:         st.CreatedAt,
		})
	}
	return result, nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
