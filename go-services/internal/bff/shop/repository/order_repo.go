package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/shop/model"
)

type OrderRepo struct {
	db *sqlx.DB
}

func NewOrderRepo(db *sqlx.DB) *OrderRepo {
	return &OrderRepo{db: db}
}

func (r *OrderRepo) BeginTx(ctx context.Context) (*sqlx.Tx, error) {
	return r.db.BeginTxx(ctx, nil)
}

func (r *OrderRepo) CreateOrder(ctx context.Context, tx *sqlx.Tx, o *model.Order) error {
	_, err := tx.NamedExecContext(ctx, `
		INSERT INTO orders (
			id, tenant_id, user_id, order_number, payment_type, status,
			subtotal, delivery_fee, total_amount, delivery_address,
			deposit_amount, amount_financed, deposit_status, deposit_payment_method,
			deposit_transaction_ref, bnpl_plan_id, lms_loan_application_id, notes,
			created_at, updated_at
		) VALUES (
			:id, :tenant_id, :user_id, :order_number, :payment_type, :status,
			:subtotal, :delivery_fee, :total_amount, :delivery_address,
			:deposit_amount, :amount_financed, :deposit_status, :deposit_payment_method,
			:deposit_transaction_ref, :bnpl_plan_id, :lms_loan_application_id, :notes,
			NOW(), NOW()
		)`, o)
	return err
}

func (r *OrderRepo) CreateOrderItem(ctx context.Context, tx *sqlx.Tx, item *model.OrderItem) error {
	_, err := tx.NamedExecContext(ctx, `
		INSERT INTO order_items (id, order_id, product_id, product_name, product_image_url, quantity, unit_price, total_price, created_at)
		VALUES (:id, :order_id, :product_id, :product_name, :product_image_url, :quantity, :unit_price, :total_price, NOW())`, item)
	return err
}

func (r *OrderRepo) CreateDeliveryEvent(ctx context.Context, tx *sqlx.Tx, ev *model.DeliveryEvent) error {
	_, err := tx.NamedExecContext(ctx, `
		INSERT INTO delivery_events (id, order_id, event_type, description, location, created_at)
		VALUES (:id, :order_id, :event_type, :description, :location, NOW())`, ev)
	return err
}

func (r *OrderRepo) CreateDeliveryEventNoTx(ctx context.Context, ev *model.DeliveryEvent) error {
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO delivery_events (id, order_id, event_type, description, location, created_at)
		VALUES (:id, :order_id, :event_type, :description, :location, NOW())`, ev)
	return err
}

func (r *OrderRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	var o model.Order
	err := r.db.GetContext(ctx, &o, `SELECT * FROM orders WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *OrderRepo) FindByUser(ctx context.Context, tenantID string, userID uuid.UUID, page, size int) ([]model.Order, int64, error) {
	var total int64
	err := r.db.GetContext(ctx, &total, `
		SELECT COUNT(*) FROM orders WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID)
	if err != nil {
		return nil, 0, err
	}

	var orders []model.Order
	err = r.db.SelectContext(ctx, &orders, `
		SELECT * FROM orders
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`,
		tenantID, userID, size, page*size)
	return orders, total, err
}

func (r *OrderRepo) FindOrderItems(ctx context.Context, orderID uuid.UUID) ([]model.OrderItem, error) {
	var items []model.OrderItem
	err := r.db.SelectContext(ctx, &items, `
		SELECT * FROM order_items WHERE order_id = $1 ORDER BY created_at ASC`, orderID)
	return items, err
}

func (r *OrderRepo) FindDeliveryEvents(ctx context.Context, orderID uuid.UUID) ([]model.DeliveryEvent, error) {
	var events []model.DeliveryEvent
	err := r.db.SelectContext(ctx, &events, `
		SELECT * FROM delivery_events WHERE order_id = $1 ORDER BY created_at ASC`, orderID)
	return events, err
}

func (r *OrderRepo) UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.OrderStatus) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2`, status, orderID)
	return err
}

func (r *OrderRepo) UpdateDepositStatus(ctx context.Context, orderID uuid.UUID, status model.DepositStatus, txRef *string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE orders SET deposit_status = $1, deposit_transaction_ref = $2, updated_at = NOW() WHERE id = $3`,
		status, txRef, orderID)
	return err
}

func (r *OrderRepo) UpdateLoanApplicationID(ctx context.Context, orderID uuid.UUID, loanAppID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE orders SET lms_loan_application_id = $1, updated_at = NOW() WHERE id = $2`,
		loanAppID, orderID)
	return err
}
