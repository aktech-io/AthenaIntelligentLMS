package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/notification/model"
)

type NotificationRepo struct {
	db *sqlx.DB
}

func NewNotificationRepo(db *sqlx.DB) *NotificationRepo {
	return &NotificationRepo{db: db}
}

func (r *NotificationRepo) Create(ctx context.Context, n *model.Notification) error {
	n.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO notifications (id, tenant_id, user_id, title, body, category, read, action_type, action_data, created_at, updated_at)
		VALUES (:id, :tenant_id, :user_id, :title, :body, :category, :read, :action_type, :action_data, NOW(), NOW())`,
		n)
	return err
}

func (r *NotificationRepo) FindByTenantAndUser(ctx context.Context, tenantID string, userID uuid.UUID, page, size int) ([]model.Notification, error) {
	var results []model.Notification
	err := r.db.SelectContext(ctx, &results, `
		SELECT * FROM notifications
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`,
		tenantID, userID, size, page*size)
	return results, err
}

func (r *NotificationRepo) FindByTenantUserAndCategory(ctx context.Context, tenantID string, userID uuid.UUID, category string, page, size int) ([]model.Notification, error) {
	var results []model.Notification
	err := r.db.SelectContext(ctx, &results, `
		SELECT * FROM notifications
		WHERE tenant_id = $1 AND user_id = $2 AND category = $3
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5`,
		tenantID, userID, category, size, page*size)
	return results, err
}

func (r *NotificationRepo) CountUnread(ctx context.Context, tenantID string, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM notifications
		WHERE tenant_id = $1 AND user_id = $2 AND read = FALSE`,
		tenantID, userID)
	return count, err
}

func (r *NotificationRepo) CountByTenantAndUser(ctx context.Context, tenantID string, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM notifications WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID)
	return count, err
}

func (r *NotificationRepo) CountByTenantUserAndCategory(ctx context.Context, tenantID string, userID uuid.UUID, category string) (int64, error) {
	var count int64
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM notifications WHERE tenant_id = $1 AND user_id = $2 AND category = $3`,
		tenantID, userID, category)
	return count, err
}

func (r *NotificationRepo) MarkAllRead(ctx context.Context, tenantID string, userID uuid.UUID) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE notifications SET read = TRUE, updated_at = NOW()
		WHERE tenant_id = $1 AND user_id = $2 AND read = FALSE`,
		tenantID, userID)
	if err != nil {
		return 0, fmt.Errorf("mark all read: %w", err)
	}
	return result.RowsAffected()
}
