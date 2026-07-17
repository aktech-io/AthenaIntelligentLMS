package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/gateway/model"
)

type TokenRepo struct {
	db *sqlx.DB
}

func NewTokenRepo(db *sqlx.DB) *TokenRepo {
	return &TokenRepo{db: db}
}

func (r *TokenRepo) Create(ctx context.Context, t *model.RefreshToken) error {
	t.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO refresh_tokens (id, user_id, device_id, token_hash, expires_at, revoked, created_at)
		VALUES (:id, :user_id, :device_id, :token_hash, :expires_at, :revoked, NOW())`, t)
	return err
}

func (r *TokenRepo) FindByTokenHash(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	err := r.db.GetContext(ctx, &t, `
		SELECT * FROM refresh_tokens WHERE token_hash = $1 AND revoked = FALSE`, tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &t, err
}

func (r *TokenRepo) FindActiveByUserID(ctx context.Context, userID uuid.UUID) ([]model.RefreshToken, error) {
	var tokens []model.RefreshToken
	err := r.db.SelectContext(ctx, &tokens, `
		SELECT * FROM refresh_tokens WHERE user_id = $1 AND revoked = FALSE AND expires_at > NOW()`, userID)
	return tokens, err
}

func (r *TokenRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE refresh_tokens SET revoked = TRUE WHERE id = $1`, id)
	return err
}

func (r *TokenRepo) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE refresh_tokens SET revoked = TRUE WHERE user_id = $1`, userID)
	return err
}
