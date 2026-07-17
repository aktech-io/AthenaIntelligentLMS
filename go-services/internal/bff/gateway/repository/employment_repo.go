package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/gateway/model"
)

type EmploymentRepo struct {
	db *sqlx.DB
}

func NewEmploymentRepo(db *sqlx.DB) *EmploymentRepo {
	return &EmploymentRepo{db: db}
}

func (r *EmploymentRepo) FindByUserID(ctx context.Context, userID uuid.UUID) (*model.UserEmployment, error) {
	var e model.UserEmployment
	err := r.db.GetContext(ctx, &e, `
		SELECT * FROM user_employment WHERE user_id = $1`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &e, err
}

func (r *EmploymentRepo) Upsert(ctx context.Context, e *model.UserEmployment) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO user_employment (id, tenant_id, user_id, employer_name, job_title, monthly_income, employment_status, created_at, updated_at)
		VALUES (:id, :tenant_id, :user_id, :employer_name, :job_title, :monthly_income, :employment_status, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			employer_name = EXCLUDED.employer_name,
			job_title = EXCLUDED.job_title,
			monthly_income = EXCLUDED.monthly_income,
			employment_status = EXCLUDED.employment_status,
			updated_at = NOW()`, e)
	return err
}
