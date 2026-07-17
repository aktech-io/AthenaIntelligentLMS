package model

import (
	"time"

	"github.com/google/uuid"
)

// GoalStatus represents the status of a savings goal.
type GoalStatus string

const (
	GoalStatusActive    GoalStatus = "ACTIVE"
	GoalStatusCompleted GoalStatus = "COMPLETED"
	GoalStatusCancelled GoalStatus = "CANCELLED"
)

// AutoSaveFrequency represents how often auto-save runs.
type AutoSaveFrequency string

const (
	FrequencyDaily   AutoSaveFrequency = "DAILY"
	FrequencyWeekly  AutoSaveFrequency = "WEEKLY"
	FrequencyMonthly AutoSaveFrequency = "MONTHLY"
)

// SavingsGoal represents the savings_goals table.
type SavingsGoal struct {
	ID                uuid.UUID          `db:"id" json:"id"`
	TenantID          string             `db:"tenant_id" json:"tenantId"`
	UserID            uuid.UUID          `db:"user_id" json:"userId"`
	GoalName          string             `db:"goal_name" json:"goalName"`
	GoalIcon          *string            `db:"goal_icon" json:"goalIcon,omitempty"`
	TargetAmount      float64            `db:"target_amount" json:"targetAmount"`
	CurrentAmount     float64            `db:"current_amount" json:"currentAmount"`
	Deadline          *time.Time         `db:"deadline" json:"deadline,omitempty"`
	AutoSaveEnabled   bool               `db:"auto_save_enabled" json:"autoSaveEnabled"`
	AutoSaveAmount    *float64           `db:"auto_save_amount" json:"autoSaveAmount,omitempty"`
	AutoSaveFrequency *AutoSaveFrequency `db:"auto_save_frequency" json:"autoSaveFrequency,omitempty"`
	LMSAccountID      *string            `db:"lms_account_id" json:"lmsAccountId,omitempty"`
	Status            GoalStatus         `db:"status" json:"status"`
	CreatedAt         time.Time          `db:"created_at" json:"createdAt"`
	UpdatedAt         time.Time          `db:"updated_at" json:"updatedAt"`
}

// ProgressPercentage calculates the progress as a percentage capped at 100.
func (g *SavingsGoal) ProgressPercentage() float64 {
	if g.TargetAmount <= 0 {
		return 0
	}
	pct := (g.CurrentAmount / g.TargetAmount) * 100
	if pct > 100 {
		pct = 100
	}
	return pct
}

// SavingsGoalResponse is the API response for a savings goal.
type SavingsGoalResponse struct {
	ID                 uuid.UUID          `json:"id"`
	GoalName           string             `json:"goalName"`
	GoalIcon           *string            `json:"goalIcon,omitempty"`
	TargetAmount       float64            `json:"targetAmount"`
	CurrentAmount      float64            `json:"currentAmount"`
	ProgressPercentage float64            `json:"progressPercentage"`
	Deadline           *time.Time         `json:"deadline,omitempty"`
	AutoSaveEnabled    bool               `json:"autoSaveEnabled"`
	AutoSaveAmount     *float64           `json:"autoSaveAmount,omitempty"`
	AutoSaveFrequency  *AutoSaveFrequency `json:"autoSaveFrequency,omitempty"`
	LMSAccountID       *string            `json:"lmsAccountId,omitempty"`
	Status             GoalStatus         `json:"status"`
	CreatedAt          time.Time          `json:"createdAt"`
}

func (g *SavingsGoal) ToResponse() SavingsGoalResponse {
	return SavingsGoalResponse{
		ID:                 g.ID,
		GoalName:           g.GoalName,
		GoalIcon:           g.GoalIcon,
		TargetAmount:       g.TargetAmount,
		CurrentAmount:      g.CurrentAmount,
		ProgressPercentage: g.ProgressPercentage(),
		Deadline:           g.Deadline,
		AutoSaveEnabled:    g.AutoSaveEnabled,
		AutoSaveAmount:     g.AutoSaveAmount,
		AutoSaveFrequency:  g.AutoSaveFrequency,
		LMSAccountID:       g.LMSAccountID,
		Status:             g.Status,
		CreatedAt:          g.CreatedAt,
	}
}

// TransactionType represents the type of savings transaction.
type TransactionType string

const (
	TransactionTypeDeposit    TransactionType = "DEPOSIT"
	TransactionTypeWithdrawal TransactionType = "WITHDRAWAL"
	TransactionTypeAutoSave   TransactionType = "AUTO_SAVE"
)

// SavingsTransaction represents the savings_transactions table.
type SavingsTransaction struct {
	ID           uuid.UUID       `db:"id" json:"id"`
	TenantID     string          `db:"tenant_id" json:"tenantId"`
	GoalID       uuid.UUID       `db:"goal_id" json:"goalId"`
	Type         TransactionType `db:"type" json:"type"`
	Amount       float64         `db:"amount" json:"amount"`
	BalanceAfter float64         `db:"balance_after" json:"balanceAfter"`
	Reference    *string         `db:"reference" json:"reference,omitempty"`
	CreatedAt    time.Time       `db:"created_at" json:"createdAt"`
}

// SavingsTransactionResponse is the API response for a savings transaction.
type SavingsTransactionResponse struct {
	ID           uuid.UUID       `json:"id"`
	GoalID       uuid.UUID       `json:"goalId"`
	Type         TransactionType `json:"type"`
	Amount       float64         `json:"amount"`
	BalanceAfter float64         `json:"balanceAfter"`
	Reference    *string         `json:"reference,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}

func (t *SavingsTransaction) ToResponse() SavingsTransactionResponse {
	return SavingsTransactionResponse{
		ID:           t.ID,
		GoalID:       t.GoalID,
		Type:         t.Type,
		Amount:       t.Amount,
		BalanceAfter: t.BalanceAfter,
		Reference:    t.Reference,
		CreatedAt:    t.CreatedAt,
	}
}
