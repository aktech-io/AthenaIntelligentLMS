package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/billpay/model"
)

type BillPaymentRepo struct {
	db *sqlx.DB
}

func NewBillPaymentRepo(db *sqlx.DB) *BillPaymentRepo {
	return &BillPaymentRepo{db: db}
}

func (r *BillPaymentRepo) Create(ctx context.Context, p *model.BillPayment) error {
	p.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO bill_payments (id, tenant_id, user_id, biller_id, account_number, amount, fee, total_amount, status, lms_payment_id, biller_reference, failure_reason, created_at, updated_at)
		VALUES (:id, :tenant_id, :user_id, :biller_id, :account_number, :amount, :fee, :total_amount, :status, :lms_payment_id, :biller_reference, :failure_reason, NOW(), NOW())`,
		p)
	return err
}

func (r *BillPaymentRepo) Update(ctx context.Context, p *model.BillPayment) error {
	_, err := r.db.NamedExecContext(ctx, `
		UPDATE bill_payments SET
			status = :status,
			lms_payment_id = :lms_payment_id,
			biller_reference = :biller_reference,
			failure_reason = :failure_reason,
			updated_at = NOW()
		WHERE id = :id`,
		p)
	return err
}

func (r *BillPaymentRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.BillPayment, error) {
	var p model.BillPayment
	err := r.db.GetContext(ctx, &p, `SELECT * FROM bill_payments WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *BillPaymentRepo) FindByUserPaginated(ctx context.Context, tenantID string, userID uuid.UUID, page, size int) ([]model.BillPayment, error) {
	var results []model.BillPayment
	err := r.db.SelectContext(ctx, &results, `
		SELECT * FROM bill_payments
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`,
		tenantID, userID, size, page*size)
	return results, err
}

func (r *BillPaymentRepo) CountByUser(ctx context.Context, tenantID string, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM bill_payments WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID)
	return count, err
}
