package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/billpay/model"
)

type SavingsRepo struct {
	db *sqlx.DB
}

func NewSavingsRepo(db *sqlx.DB) *SavingsRepo {
	return &SavingsRepo{db: db}
}

// --- Goals ---

func (r *SavingsRepo) CreateGoal(ctx context.Context, g *model.SavingsGoal) error {
	g.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO savings_goals (id, tenant_id, user_id, goal_name, goal_icon, target_amount, current_amount, deadline, auto_save_enabled, auto_save_amount, auto_save_frequency, lms_account_id, status, created_at, updated_at)
		VALUES (:id, :tenant_id, :user_id, :goal_name, :goal_icon, :target_amount, :current_amount, :deadline, :auto_save_enabled, :auto_save_amount, :auto_save_frequency, :lms_account_id, :status, NOW(), NOW())`,
		g)
	return err
}

func (r *SavingsRepo) UpdateGoal(ctx context.Context, g *model.SavingsGoal) error {
	_, err := r.db.NamedExecContext(ctx, `
		UPDATE savings_goals SET
			current_amount = :current_amount,
			status = :status,
			updated_at = NOW()
		WHERE id = :id`,
		g)
	return err
}

func (r *SavingsRepo) FindGoalByID(ctx context.Context, id uuid.UUID) (*model.SavingsGoal, error) {
	var g model.SavingsGoal
	err := r.db.GetContext(ctx, &g, `SELECT * FROM savings_goals WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *SavingsRepo) FindGoalsByUser(ctx context.Context, tenantID string, userID uuid.UUID) ([]model.SavingsGoal, error) {
	var results []model.SavingsGoal
	err := r.db.SelectContext(ctx, &results, `
		SELECT * FROM savings_goals
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY created_at DESC`,
		tenantID, userID)
	return results, err
}

func (r *SavingsRepo) FindActiveAutoSaveGoals(ctx context.Context) ([]model.SavingsGoal, error) {
	var results []model.SavingsGoal
	err := r.db.SelectContext(ctx, &results, `
		SELECT * FROM savings_goals
		WHERE auto_save_enabled = TRUE AND status = 'ACTIVE'`)
	return results, err
}

// --- Transactions ---

func (r *SavingsRepo) CreateTransaction(ctx context.Context, t *model.SavingsTransaction) error {
	t.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO savings_transactions (id, tenant_id, goal_id, type, amount, balance_after, reference, created_at)
		VALUES (:id, :tenant_id, :goal_id, :type, :amount, :balance_after, :reference, NOW())`,
		t)
	return err
}

func (r *SavingsRepo) FindTransactionsByGoal(ctx context.Context, goalID uuid.UUID, page, size int) ([]model.SavingsTransaction, error) {
	var results []model.SavingsTransaction
	err := r.db.SelectContext(ctx, &results, `
		SELECT * FROM savings_transactions
		WHERE goal_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		goalID, size, page*size)
	return results, err
}

func (r *SavingsRepo) CountTransactionsByGoal(ctx context.Context, goalID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM savings_transactions WHERE goal_id = $1`, goalID)
	return count, err
}
