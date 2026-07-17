package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/billpay/client"
	"github.com/athena-lms/go-services/internal/bff/billpay/model"
	"github.com/athena-lms/go-services/internal/bff/billpay/publisher"
	"github.com/athena-lms/go-services/internal/bff/billpay/repository"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/event"
)

type SavingsService struct {
	repo       *repository.SavingsRepo
	accountCli *client.AccountClient
	publisher  *publisher.EventPublisher
}

func NewSavingsService(
	repo *repository.SavingsRepo,
	accountCli *client.AccountClient,
	pub *publisher.EventPublisher,
) *SavingsService {
	return &SavingsService{
		repo:       repo,
		accountCli: accountCli,
		publisher:  pub,
	}
}

func (s *SavingsService) ListGoals(ctx context.Context, tenantID string, userID uuid.UUID) ([]model.SavingsGoal, error) {
	return s.repo.FindGoalsByUser(ctx, tenantID, userID)
}

type CreateGoalRequest struct {
	GoalName          string                   `json:"goalName"`
	GoalIcon          *string                  `json:"goalIcon,omitempty"`
	TargetAmount      float64                  `json:"targetAmount"`
	Deadline          *string                  `json:"deadline,omitempty"`
	AutoSaveEnabled   bool                     `json:"autoSaveEnabled"`
	AutoSaveAmount    *float64                 `json:"autoSaveAmount,omitempty"`
	AutoSaveFrequency *model.AutoSaveFrequency `json:"autoSaveFrequency,omitempty"`
	LMSAccountID      *string                  `json:"lmsAccountId,omitempty"`
}

func (s *SavingsService) CreateGoal(ctx context.Context, tenantID string, userID uuid.UUID, req CreateGoalRequest) (*model.SavingsGoal, error) {
	if req.GoalName == "" {
		return nil, apperrors.BadRequest("goalName is required")
	}
	if req.TargetAmount < 100 {
		return nil, apperrors.BadRequest("targetAmount must be at least 100")
	}
	if req.AutoSaveEnabled {
		if req.AutoSaveAmount == nil || *req.AutoSaveAmount < 10 {
			return nil, apperrors.BadRequest("autoSaveAmount must be at least 10 when auto-save is enabled")
		}
		if req.AutoSaveFrequency == nil {
			return nil, apperrors.BadRequest("autoSaveFrequency is required when auto-save is enabled")
		}
	}

	goal := &model.SavingsGoal{
		TenantID:          tenantID,
		UserID:            userID,
		GoalName:          req.GoalName,
		GoalIcon:          req.GoalIcon,
		TargetAmount:      req.TargetAmount,
		CurrentAmount:     0,
		AutoSaveEnabled:   req.AutoSaveEnabled,
		AutoSaveAmount:    req.AutoSaveAmount,
		AutoSaveFrequency: req.AutoSaveFrequency,
		LMSAccountID:      req.LMSAccountID,
		Status:            model.GoalStatusActive,
	}

	// Parse deadline if provided.
	if req.Deadline != nil && *req.Deadline != "" {
		t, err := parseDate(*req.Deadline)
		if err != nil {
			return nil, apperrors.BadRequest("invalid deadline format, expected YYYY-MM-DD")
		}
		goal.Deadline = &t
	}

	if err := s.repo.CreateGoal(ctx, goal); err != nil {
		return nil, fmt.Errorf("create goal: %w", err)
	}

	s.publisher.Publish(event.SavingsGoalCreated, tenantID, map[string]any{
		"goalId":       goal.ID,
		"userId":       userID,
		"goalName":     req.GoalName,
		"targetAmount": req.TargetAmount,
	})

	return goal, nil
}

type DepositRequest struct {
	Amount          float64   `json:"amount"`
	SourceAccountID uuid.UUID `json:"sourceAccountId"`
}

func (s *SavingsService) Deposit(ctx context.Context, tenantID string, userID, goalID uuid.UUID, req DepositRequest) (*model.SavingsGoal, error) {
	if req.Amount < 1 {
		return nil, apperrors.BadRequest("amount must be at least 1")
	}

	goal, err := s.repo.FindGoalByID(ctx, goalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.NotFoundResource("SavingsGoal", goalID.String())
		}
		return nil, fmt.Errorf("find goal: %w", err)
	}
	if goal.UserID != userID {
		return nil, apperrors.Forbidden("not authorized to access this goal")
	}
	if goal.Status != model.GoalStatusActive {
		return nil, apperrors.BadRequest("goal is not active")
	}

	// Debit source account.
	ref := fmt.Sprintf("SAV-DEP-%s", goalID.String()[:8])
	desc := fmt.Sprintf("Savings deposit to %s", goal.GoalName)
	if err := s.accountCli.Debit(ctx, req.SourceAccountID, req.Amount, ref, desc); err != nil {
		return nil, apperrors.BadRequest("failed to debit account: " + err.Error())
	}

	// Update goal balance.
	goal.CurrentAmount += req.Amount
	if goal.CurrentAmount >= goal.TargetAmount {
		goal.Status = model.GoalStatusCompleted
	}
	if err := s.repo.UpdateGoal(ctx, goal); err != nil {
		return nil, fmt.Errorf("update goal: %w", err)
	}

	// Record transaction.
	tx := &model.SavingsTransaction{
		TenantID:     tenantID,
		GoalID:       goalID,
		Type:         model.TransactionTypeDeposit,
		Amount:       req.Amount,
		BalanceAfter: goal.CurrentAmount,
		Reference:    &ref,
	}
	if err := s.repo.CreateTransaction(ctx, tx); err != nil {
		slog.Error("failed to record savings transaction", "goalId", goalID, "error", err)
	}

	s.publisher.Publish(event.SavingsDeposit, tenantID, map[string]any{
		"goalId":       goalID,
		"userId":       userID,
		"amount":       req.Amount,
		"balanceAfter": goal.CurrentAmount,
	})

	return goal, nil
}

type WithdrawRequest struct {
	Amount               float64   `json:"amount"`
	DestinationAccountID uuid.UUID `json:"destinationAccountId"`
}

func (s *SavingsService) Withdraw(ctx context.Context, tenantID string, userID, goalID uuid.UUID, req WithdrawRequest) (*model.SavingsGoal, error) {
	if req.Amount < 1 {
		return nil, apperrors.BadRequest("amount must be at least 1")
	}

	goal, err := s.repo.FindGoalByID(ctx, goalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.NotFoundResource("SavingsGoal", goalID.String())
		}
		return nil, fmt.Errorf("find goal: %w", err)
	}
	if goal.UserID != userID {
		return nil, apperrors.Forbidden("not authorized to access this goal")
	}
	if goal.Status == model.GoalStatusCancelled {
		return nil, apperrors.BadRequest("cannot withdraw from a cancelled goal")
	}
	if req.Amount > goal.CurrentAmount {
		return nil, apperrors.BadRequest("insufficient balance in savings goal")
	}

	// Credit destination account.
	ref := fmt.Sprintf("SAV-WDR-%s", goalID.String()[:8])
	desc := fmt.Sprintf("Savings withdrawal from %s", goal.GoalName)
	if err := s.accountCli.Credit(ctx, req.DestinationAccountID, req.Amount, ref, desc); err != nil {
		return nil, apperrors.BadRequest("failed to credit account: " + err.Error())
	}

	// Update goal balance.
	wasCompleted := goal.Status == model.GoalStatusCompleted
	goal.CurrentAmount -= req.Amount
	if wasCompleted && goal.CurrentAmount < goal.TargetAmount {
		goal.Status = model.GoalStatusActive
	}
	if err := s.repo.UpdateGoal(ctx, goal); err != nil {
		return nil, fmt.Errorf("update goal: %w", err)
	}

	// Record transaction.
	tx := &model.SavingsTransaction{
		TenantID:     tenantID,
		GoalID:       goalID,
		Type:         model.TransactionTypeWithdrawal,
		Amount:       req.Amount,
		BalanceAfter: goal.CurrentAmount,
		Reference:    &ref,
	}
	if err := s.repo.CreateTransaction(ctx, tx); err != nil {
		slog.Error("failed to record savings transaction", "goalId", goalID, "error", err)
	}

	s.publisher.Publish(event.SavingsWithdrawal, tenantID, map[string]any{
		"goalId":       goalID,
		"userId":       userID,
		"amount":       req.Amount,
		"balanceAfter": goal.CurrentAmount,
	})

	return goal, nil
}

func (s *SavingsService) GetTransactions(ctx context.Context, userID, goalID uuid.UUID, page, size int) ([]model.SavingsTransaction, int64, error) {
	// Verify ownership.
	goal, err := s.repo.FindGoalByID(ctx, goalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, apperrors.NotFoundResource("SavingsGoal", goalID.String())
		}
		return nil, 0, fmt.Errorf("find goal: %w", err)
	}
	if goal.UserID != userID {
		return nil, 0, apperrors.Forbidden("not authorized to access this goal")
	}

	txns, err := s.repo.FindTransactionsByGoal(ctx, goalID, page, size)
	if err != nil {
		return nil, 0, fmt.Errorf("find transactions: %w", err)
	}
	total, err := s.repo.CountTransactionsByGoal(ctx, goalID)
	if err != nil {
		return nil, 0, fmt.Errorf("count transactions: %w", err)
	}
	return txns, total, nil
}

// ProcessAutoSave processes a single auto-save for a goal.
// Used by the scheduler.
func (s *SavingsService) ProcessAutoSave(ctx context.Context, goal *model.SavingsGoal) error {
	if goal.AutoSaveAmount == nil || *goal.AutoSaveAmount <= 0 {
		return nil
	}
	amount := *goal.AutoSaveAmount

	// Attempt to debit via LMS account if set; otherwise skip.
	if goal.LMSAccountID == nil || *goal.LMSAccountID == "" {
		slog.Warn("auto-save skipped: no lms_account_id", "goalId", goal.ID)
		return nil
	}

	lmsAccID, err := uuid.Parse(*goal.LMSAccountID)
	if err != nil {
		slog.Error("auto-save: invalid lms_account_id", "goalId", goal.ID, "error", err)
		return err
	}

	ref := fmt.Sprintf("AUTO-SAV-%s", goal.ID.String()[:8])
	desc := fmt.Sprintf("Auto-save to %s", goal.GoalName)
	if err := s.accountCli.Debit(ctx, lmsAccID, amount, ref, desc); err != nil {
		slog.Error("auto-save debit failed", "goalId", goal.ID, "error", err)
		return err
	}

	goal.CurrentAmount += amount
	if goal.CurrentAmount >= goal.TargetAmount {
		goal.Status = model.GoalStatusCompleted
	}
	if err := s.repo.UpdateGoal(ctx, goal); err != nil {
		return fmt.Errorf("update goal: %w", err)
	}

	tx := &model.SavingsTransaction{
		TenantID:     goal.TenantID,
		GoalID:       goal.ID,
		Type:         model.TransactionTypeAutoSave,
		Amount:       amount,
		BalanceAfter: goal.CurrentAmount,
		Reference:    &ref,
	}
	if err := s.repo.CreateTransaction(ctx, tx); err != nil {
		slog.Error("failed to record auto-save transaction", "goalId", goal.ID, "error", err)
	}

	s.publisher.Publish(event.SavingsAutoSaveExecuted, goal.TenantID, map[string]any{
		"goalId":       goal.ID,
		"userId":       goal.UserID,
		"amount":       amount,
		"balanceAfter": goal.CurrentAmount,
	})

	return nil
}

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}
