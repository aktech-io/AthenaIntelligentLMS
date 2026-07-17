package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/gateway/model"
)

type ContactRepo struct {
	db *sqlx.DB
}

func NewContactRepo(db *sqlx.DB) *ContactRepo {
	return &ContactRepo{db: db}
}

func (r *ContactRepo) Create(ctx context.Context, c *model.UserContact) error {
	c.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO user_contacts (id, tenant_id, user_id, contact_name, phone_number, is_athena_user, is_favorite, last_transacted_at, created_at, updated_at)
		VALUES (:id, :tenant_id, :user_id, :contact_name, :phone_number, :is_athena_user, :is_favorite, :last_transacted_at, NOW(), NOW())`, c)
	return err
}

func (r *ContactRepo) FindRecent(ctx context.Context, tenantID string, userID uuid.UUID, page, size int) ([]model.UserContact, error) {
	var contacts []model.UserContact
	err := r.db.SelectContext(ctx, &contacts, `
		SELECT * FROM user_contacts
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY last_transacted_at DESC NULLS LAST, created_at DESC
		LIMIT $3 OFFSET $4`, tenantID, userID, size, page*size)
	return contacts, err
}

func (r *ContactRepo) CountByUser(ctx context.Context, tenantID string, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM user_contacts WHERE tenant_id = $1 AND user_id = $2`, tenantID, userID)
	return count, err
}

func (r *ContactRepo) Search(ctx context.Context, tenantID string, userID uuid.UUID, query string, page, size int) ([]model.UserContact, error) {
	var contacts []model.UserContact
	q := "%" + query + "%"
	err := r.db.SelectContext(ctx, &contacts, `
		SELECT * FROM user_contacts
		WHERE tenant_id = $1 AND user_id = $2 AND (contact_name ILIKE $3 OR phone_number ILIKE $3)
		ORDER BY is_favorite DESC, contact_name ASC
		LIMIT $4 OFFSET $5`, tenantID, userID, q, size, page*size)
	return contacts, err
}

func (r *ContactRepo) CountSearch(ctx context.Context, tenantID string, userID uuid.UUID, query string) (int64, error) {
	var count int64
	q := "%" + query + "%"
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM user_contacts
		WHERE tenant_id = $1 AND user_id = $2 AND (contact_name ILIKE $3 OR phone_number ILIKE $3)`, tenantID, userID, q)
	return count, err
}

func (r *ContactRepo) UpdateLastTransacted(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE user_contacts SET last_transacted_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	return err
}
