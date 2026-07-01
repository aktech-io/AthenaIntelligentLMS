package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/outbox"
)

// TransferService provides fund transfer business logic.
// Port of Java TransferService.java.
type TransferService struct {
	repo              *repository.Repository
	publisher         *event.Publisher
	logger            *zap.Logger
	productServiceURL string
	serviceKey        string
	auditor           *audit.Logger
}

// NewTransferService creates a new TransferService.
func NewTransferService(repo *repository.Repository, publisher *event.Publisher, logger *zap.Logger,
	productServiceURL, serviceKey string) *TransferService {
	return &TransferService{
		repo:              repo,
		publisher:         publisher,
		logger:            logger,
		productServiceURL: productServiceURL,
		serviceKey:        serviceKey,
		auditor:           audit.New(repo, logger),
	}
}

// TransferRequest is the DTO for initiating a transfer.
type TransferRequest struct {
	SourceAccountID          uuid.UUID       `json:"sourceAccountId"`
	DestinationAccountID     *uuid.UUID      `json:"destinationAccountId,omitempty"`
	DestinationAccountNumber *string         `json:"destinationAccountNumber,omitempty"`
	Amount                   decimal.Decimal `json:"amount"`
	TransferType             string          `json:"transferType"`
	Narration                *string         `json:"narration,omitempty"`
	IdempotencyKey           *string         `json:"idempotencyKey,omitempty"`
}

// TransferResponse is the response DTO for transfers.
type TransferResponse struct {
	ID                       uuid.UUID       `json:"id"`
	SourceAccountID          uuid.UUID       `json:"sourceAccountId"`
	SourceAccountNumber      *string         `json:"sourceAccountNumber,omitempty"`
	DestinationAccountID     uuid.UUID       `json:"destinationAccountId"`
	DestinationAccountNumber *string         `json:"destinationAccountNumber,omitempty"`
	Amount                   decimal.Decimal `json:"amount"`
	Currency                 string          `json:"currency"`
	TransferType             string          `json:"transferType"`
	Status                   string          `json:"status"`
	Reference                string          `json:"reference"`
	Narration                *string         `json:"narration,omitempty"`
	ChargeAmount             decimal.Decimal `json:"chargeAmount"`
	InitiatedAt              time.Time       `json:"initiatedAt"`
	CompletedAt              *time.Time      `json:"completedAt,omitempty"`
	FailedReason             *string         `json:"failedReason,omitempty"`
}

func transferResponseFrom(t *model.FundTransfer, srcNum, destNum *string) TransferResponse {
	return TransferResponse{
		ID:                       t.ID,
		SourceAccountID:          t.SourceAccountID,
		SourceAccountNumber:      srcNum,
		DestinationAccountID:     t.DestinationAccountID,
		DestinationAccountNumber: destNum,
		Amount:                   t.Amount,
		Currency:                 t.Currency,
		TransferType:             string(t.TransferType),
		Status:                   string(t.Status),
		Reference:                t.Reference,
		Narration:                t.Narration,
		ChargeAmount:             t.ChargeAmount,
		InitiatedAt:              t.InitiatedAt,
		CompletedAt:              t.CompletedAt,
		FailedReason:             t.FailedReason,
	}
}

// InitiateTransfer executes a fund transfer between two accounts.
func (s *TransferService) InitiateTransfer(ctx context.Context, req TransferRequest, tenantID, initiatedBy string) (*TransferResponse, error) {
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return nil, errors.BadRequest("amount must be > 0")
	}

	// Idempotency check. Only ErrNoRows means "no prior transfer" — any other
	// error must surface, otherwise a flaky lookup would re-execute the transfer.
	// The fund_transfers.reference UNIQUE constraint backstops the race below.
	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		existing, err := s.repo.GetTransferByReferenceAndTenant(ctx, *req.IdempotencyKey, tenantID)
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
		if err == nil {
			srcNum := s.getAccountNumber(ctx, existing.SourceAccountID)
			destNum := s.getAccountNumber(ctx, existing.DestinationAccountID)
			resp := transferResponseFrom(existing, srcNum, destNum)
			return &resp, nil
		}
	}

	// Validate transfer type
	upper := strings.ToUpper(req.TransferType)
	if !model.ValidTransferType(upper) {
		return nil, errors.BadRequest("Invalid transfer type: " + req.TransferType)
	}
	transferType := model.TransferType(upper)

	// Maker-checker: queue for a second authoriser when required.
	if !gateOpen(ctx) && requiresApproval(ctx, s.repo, tenantID, OpTransfer, req.Amount) {
		narration := ""
		if req.Narration != nil {
			narration = *req.Narration
		}
		return nil, queueApproval(ctx, s.repo, tenantID, OpTransfer, "ACCOUNT", req.SourceAccountID.String(),
			req.Amount, narration, req)
	}

	// Resolve source account
	sourceAccount, err := s.repo.GetAccountByIDAndTenant(ctx, req.SourceAccountID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Source account", req.SourceAccountID)
		}
		return nil, err
	}
	if sourceAccount.Status != model.AccountStatusActive {
		return nil, errors.NewBusinessError("Source account is " + string(sourceAccount.Status))
	}

	// Resolve destination account. Transfers are intra-tenant only, so both
	// lookups are tenant-scoped — a cross-tenant destination is indistinguishable
	// from a missing one (no account probing across tenants).
	var destAccount *model.Account
	if req.DestinationAccountID != nil {
		destAccount, err = s.repo.GetAccountByIDAndTenant(ctx, *req.DestinationAccountID, tenantID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, errors.NotFoundResource("Destination account", *req.DestinationAccountID)
			}
			return nil, err
		}
	} else if req.DestinationAccountNumber != nil {
		destAccount, err = s.repo.GetAccountByNumberAndTenant(ctx, *req.DestinationAccountNumber, tenantID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, errors.BadRequest("Destination account not found: " + *req.DestinationAccountNumber)
			}
			return nil, err
		}
	} else {
		return nil, errors.BadRequest("Either destinationAccountId or destinationAccountNumber is required")
	}

	if destAccount.Status != model.AccountStatusActive {
		return nil, errors.NewBusinessError("Destination account is " + string(destAccount.Status))
	}

	// Same account check
	if sourceAccount.ID == destAccount.ID {
		return nil, errors.BadRequest("Cannot transfer to the same account")
	}

	// Currency check
	if sourceAccount.Currency != destAccount.Currency {
		return nil, errors.BadRequest(fmt.Sprintf("Currency mismatch: %s vs %s", sourceAccount.Currency, destAccount.Currency))
	}

	// For INTERNAL transfers, verify same customer
	if transferType == model.TransferTypeInternal && sourceAccount.CustomerID != destAccount.CustomerID {
		return nil, errors.BadRequest("INTERNAL transfers require same customer; use THIRD_PARTY for different customers")
	}

	// Calculate charge (fail-open: 0 if product-service unreachable)
	chargeAmount := s.calculateCharge(string(transferType), req.Amount, tenantID)
	totalDebit := req.Amount.Add(chargeAmount)

	// Generate reference
	reference := fmt.Sprintf("TXF-%s", strings.ToUpper(uuid.New().String()[:12]))
	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		reference = *req.IdempotencyKey
	}

	// Begin transaction
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Lock balances in UUID order to prevent deadlocks
	first, second := sourceAccount.ID, destAccount.ID
	if first.String() > second.String() {
		first, second = second, first
	}

	firstBal, err := s.repo.GetBalanceForUpdate(ctx, tx, first)
	if err != nil {
		return nil, errors.NotFoundResource("Balance", first)
	}
	secondBal, err := s.repo.GetBalanceForUpdate(ctx, tx, second)
	if err != nil {
		return nil, errors.NotFoundResource("Balance", second)
	}

	var sourceBal, destBal *model.AccountBalance
	if first == sourceAccount.ID {
		sourceBal, destBal = firstBal, secondBal
	} else {
		sourceBal, destBal = secondBal, firstBal
	}

	// Sufficient funds check
	if sourceBal.AvailableBalance.LessThan(totalDebit) {
		return nil, errors.NewBusinessError(
			fmt.Sprintf("Insufficient funds. Required: %s %s (transfer: %s + charge: %s)",
				totalDebit.String(), sourceAccount.Currency, req.Amount.String(), chargeAmount.String()))
	}

	// Debit source
	sourceBal.AvailableBalance = sourceBal.AvailableBalance.Sub(totalDebit)
	sourceBal.CurrentBalance = sourceBal.CurrentBalance.Sub(totalDebit)
	sourceBal.LedgerBalance = sourceBal.LedgerBalance.Sub(totalDebit)
	if err := s.repo.UpdateBalance(ctx, tx, sourceBal); err != nil {
		return nil, err
	}

	// Credit destination
	destBal.AvailableBalance = destBal.AvailableBalance.Add(req.Amount)
	destBal.CurrentBalance = destBal.CurrentBalance.Add(req.Amount)
	destBal.LedgerBalance = destBal.LedgerBalance.Add(req.Amount)
	if err := s.repo.UpdateBalance(ctx, tx, destBal); err != nil {
		return nil, err
	}

	// Create transaction records
	debitDesc := "Transfer to " + destAccount.AccountNumber
	if req.Narration != nil {
		debitDesc += " - " + *req.Narration
	}
	transferChannel := "TRANSFER"
	debitTxn := &model.AccountTransaction{
		TenantID:        tenantID,
		AccountID:       sourceAccount.ID,
		TransactionType: model.TransactionTypeDebit,
		Amount:          totalDebit,
		BalanceAfter:    &sourceBal.AvailableBalance,
		Reference:       &reference,
		Description:     &debitDesc,
		Channel:         transferChannel,
		CreatedBy:       actorPtr(ctx),
	}
	if err := s.repo.CreateTransaction(ctx, tx, debitTxn); err != nil {
		return nil, err
	}

	creditDesc := "Transfer from " + sourceAccount.AccountNumber
	if req.Narration != nil {
		creditDesc += " - " + *req.Narration
	}
	creditTxn := &model.AccountTransaction{
		TenantID:        tenantID,
		AccountID:       destAccount.ID,
		TransactionType: model.TransactionTypeCredit,
		Amount:          req.Amount,
		BalanceAfter:    &destBal.AvailableBalance,
		Reference:       &reference,
		Description:     &creditDesc,
		Channel:         transferChannel,
		CreatedBy:       actorPtr(ctx),
	}
	if err := s.repo.CreateTransaction(ctx, tx, creditTxn); err != nil {
		return nil, err
	}

	// Save transfer record
	now := time.Now()
	initiator := initiatedBy
	transfer := &model.FundTransfer{
		TenantID:             tenantID,
		SourceAccountID:      sourceAccount.ID,
		DestinationAccountID: destAccount.ID,
		Amount:               req.Amount,
		Currency:             sourceAccount.Currency,
		TransferType:         transferType,
		Status:               model.TransferStatusCompleted,
		Reference:            reference,
		Narration:            req.Narration,
		ChargeAmount:         chargeAmount,
		InitiatedBy:          &initiator,
		CompletedAt:          &now,
	}
	if err := s.repo.CreateTransfer(ctx, tx, transfer); err != nil {
		// A concurrent request with the same idempotency key won the race on the
		// fund_transfers.reference UNIQUE constraint. Roll back this attempt (the
		// tx rollback undoes our balance updates and transaction rows — nothing
		// was committed) and return the winner's transfer as the idempotent
		// response.
		if repository.IsUniqueViolation(err) {
			_ = tx.Rollback(ctx)
			winner, ferr := s.repo.GetTransferByReferenceAndTenant(ctx, reference, tenantID)
			if ferr != nil {
				if ferr == pgx.ErrNoRows {
					// Reference exists but not in this tenant (cross-tenant key
					// collision) — reject rather than leak the other row.
					return nil, errors.Conflict("Transfer reference already in use: " + reference)
				}
				return nil, ferr
			}
			s.logger.Info("Lost transfer idempotency race, returning existing transfer",
				zap.String("reference", reference),
				zap.String("transferId", winner.ID.String()))
			srcNum := s.getAccountNumber(ctx, winner.SourceAccountID)
			destNum := s.getAccountNumber(ctx, winner.DestinationAccountID)
			resp := transferResponseFrom(winner, srcNum, destNum)
			return &resp, nil
		}
		return nil, err
	}

	// Emit transfer.completed atomically with the balance moves via the
	// transactional outbox so the money-path event can never be lost relative to
	// the committed state change (F27). The relay publishes it at-least-once.
	evt, err := s.publisher.BuildTransferCompleted(transfer.ID, sourceAccount.ID, destAccount.ID, req.Amount, tenantID)
	if err != nil {
		return nil, err
	}
	if err := outbox.Write(ctx, tx, evt, transfer.ID.String()); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.auditor.Record(ctx, "TRANSFER", "TRANSFER", transfer.ID.String(), nil, nil,
		map[string]any{
			"sourceAccount":      sourceAccount.AccountNumber,
			"destinationAccount": destAccount.AccountNumber,
			"amount":             req.Amount,
			"charge":             chargeAmount,
			"transferType":       transferType,
			"reference":          reference,
		})

	s.logger.Info("Transfer completed",
		zap.String("reference", reference),
		zap.String("amount", req.Amount.String()),
		zap.String("currency", sourceAccount.Currency),
		zap.String("from", sourceAccount.AccountNumber),
		zap.String("to", destAccount.AccountNumber),
		zap.String("charge", chargeAmount.String()))

	srcNum := sourceAccount.AccountNumber
	destNum := destAccount.AccountNumber
	resp := transferResponseFrom(transfer, &srcNum, &destNum)
	return &resp, nil
}

// GetTransfer fetches a transfer by ID.
func (s *TransferService) GetTransfer(ctx context.Context, id uuid.UUID, tenantID string) (*TransferResponse, error) {
	t, err := s.repo.GetTransferByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFoundResource("Transfer", id)
		}
		return nil, err
	}
	srcNum := s.getAccountNumber(ctx, t.SourceAccountID)
	destNum := s.getAccountNumber(ctx, t.DestinationAccountID)
	resp := transferResponseFrom(t, srcNum, destNum)
	return &resp, nil
}

// GetTransfersByAccount returns paginated transfers for an account.
func (s *TransferService) GetTransfersByAccount(ctx context.Context, accountID uuid.UUID, tenantID string, page, size int) (dto.PageResponse, error) {
	transfers, total, err := s.repo.ListTransfersByAccount(ctx, tenantID, accountID, size, page*size)
	if err != nil {
		return dto.PageResponse{}, err
	}

	var responses []TransferResponse
	for _, t := range transfers {
		responses = append(responses, transferResponseFrom(t, nil, nil))
	}
	return dto.NewPageResponse(responses, page, size, total), nil
}

func (s *TransferService) getAccountNumber(ctx context.Context, accountID uuid.UUID) *string {
	a, err := s.repo.GetAccountByID(ctx, accountID)
	if err != nil {
		return nil
	}
	return &a.AccountNumber
}

func (s *TransferService) calculateCharge(transferType string, amount decimal.Decimal, tenantID string) decimal.Decimal {
	if s.productServiceURL == "" {
		return decimal.Zero
	}

	chargeType := "TRANSFER_" + transferType
	url := fmt.Sprintf("%s/api/v1/charges/calculate?transactionType=%s&amount=%s",
		s.productServiceURL, chargeType, amount.String())

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return decimal.Zero
	}
	req.Header.Set("X-Service-Key", s.serviceKey)
	req.Header.Set("X-Service-Tenant", tenantID)
	req.Header.Set("X-Service-User", "account-service")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.logger.Warn("Could not fetch charge from product-service, using 0 charge", zap.Error(err))
		return decimal.Zero
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
			if charge, ok := body["chargeAmount"]; ok && charge != nil {
				d, err := decimal.NewFromString(fmt.Sprintf("%v", charge))
				if err == nil {
					return d
				}
			}
		}
	}
	return decimal.Zero
}
