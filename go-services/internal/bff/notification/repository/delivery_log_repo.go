package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/notification/model"
)

type DeliveryLogRepo struct {
	db *sqlx.DB
}

func NewDeliveryLogRepo(db *sqlx.DB) *DeliveryLogRepo {
	return &DeliveryLogRepo{db: db}
}

func (r *DeliveryLogRepo) Create(ctx context.Context, log *model.NotificationDeliveryLog) error {
	log.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO notification_delivery_logs (id, tenant_id, notification_id, channel, recipient, template_code, status, external_id, error_message, created_at)
		VALUES (:id, :tenant_id, :notification_id, :channel, :recipient, :template_code, :status, :external_id, :error_message, NOW())`,
		log)
	return err
}
