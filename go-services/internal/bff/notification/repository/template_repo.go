package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/notification/model"
)

type TemplateRepo struct {
	db *sqlx.DB
}

func NewTemplateRepo(db *sqlx.DB) *TemplateRepo {
	return &TemplateRepo{db: db}
}

func (r *TemplateRepo) FindByTenantCodeAndChannel(ctx context.Context, tenantID, code string, channel model.Channel) (*model.NotificationTemplate, error) {
	var t model.NotificationTemplate
	err := r.db.GetContext(ctx, &t, `
		SELECT * FROM notification_templates
		WHERE tenant_id = $1 AND template_code = $2 AND channel = $3 AND active = TRUE`,
		tenantID, code, channel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &t, err
}
