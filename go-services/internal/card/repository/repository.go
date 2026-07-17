// Package repository persists cards, their audit trail, and outbox events.
// Writes are transactional: the card change, the card_events audit row, and
// the domain event (transactional outbox, F27) commit atomically.
package repository

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athena-lms/go-services/internal/card/model"
	commonerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/outbox"
)

// Repository handles card persistence with pgx. It implements
// service.Repository.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a Repository.
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const cardColumns = `id, tenant_id, customer_id, account_id, processor, processor_ref,
    pan_last4, network, card_type, status, currency, cardholder_name, limits, created_at, updated_at`

func scanCard(row pgx.Row) (*model.Card, error) {
	var c model.Card
	var limits []byte
	err := row.Scan(
		&c.ID, &c.TenantID, &c.CustomerID, &c.AccountID, &c.Processor, &c.ProcessorRef,
		&c.PanLast4, &c.Network, &c.Type, &c.Status, &c.Currency, &c.CardholderName,
		&limits, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(limits, &c.Limits); err != nil {
		return nil, fmt.Errorf("decode card limits: %w", err)
	}
	return &c, nil
}

// InsertCard persists a new card, its audit row, and the outbox event in one
// transaction.
func (r *Repository) InsertCard(ctx context.Context, c *model.Card, audit *model.CardEvent, evt *event.DomainEvent) (*model.Card, error) {
	limits, err := json.Marshal(c.Limits)
	if err != nil {
		return nil, fmt.Errorf("encode card limits: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO cards
			(id, tenant_id, customer_id, account_id, processor, processor_ref,
			 pan_last4, network, card_type, status, currency, cardholder_name, limits)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING %s`, cardColumns),
		c.ID, c.TenantID, c.CustomerID, c.AccountID, c.Processor, c.ProcessorRef,
		c.PanLast4, c.Network, c.Type, c.Status, c.Currency, c.CardholderName, limits,
	)
	created, err := scanCard(row)
	if err != nil {
		// uq_cards_processor_ref: the processor returned a card ref we already
		// hold (e.g. the deterministic sandbox re-issuing the same
		// customer/account) — a duplicate issuance, not an internal error.
		var pgErr *pgconn.PgError
		if stderrors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, commonerrors.Conflict("a card from this processor already exists for this customer/account")
		}
		return nil, fmt.Errorf("insert card: %w", err)
	}
	if err := r.insertCardEvent(ctx, tx, audit); err != nil {
		return nil, err
	}
	if err := outbox.Write(ctx, tx, evt, c.ID.String()); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return created, nil
}

// UpdateCard persists status/limits, the audit row, and the outbox event in
// one transaction. Tenant-scoped: a wrong tenant updates nothing.
func (r *Repository) UpdateCard(ctx context.Context, c *model.Card, audit *model.CardEvent, evt *event.DomainEvent) (*model.Card, error) {
	limits, err := json.Marshal(c.Limits)
	if err != nil {
		return nil, fmt.Errorf("encode card limits: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE cards
		SET status = $3, limits = $4, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2
		RETURNING %s`, cardColumns),
		c.TenantID, c.ID, c.Status, limits,
	)
	updated, err := scanCard(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("update card: not found for tenant")
	}
	if err != nil {
		return nil, fmt.Errorf("update card: %w", err)
	}
	if err := r.insertCardEvent(ctx, tx, audit); err != nil {
		return nil, err
	}
	if err := outbox.Write(ctx, tx, evt, c.ID.String()); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *Repository) insertCardEvent(ctx context.Context, tx pgx.Tx, e *model.CardEvent) error {
	detail, err := json.Marshal(e.Detail)
	if err != nil {
		return fmt.Errorf("encode card event detail: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO card_events (tenant_id, card_id, event_type, actor, detail)
		VALUES ($1, $2, $3, $4, $5)`,
		e.TenantID, e.CardID, e.EventType, e.Actor, detail)
	if err != nil {
		return fmt.Errorf("insert card event: %w", err)
	}
	return nil
}

// FindByTenantAndID returns a card, or nil when not found (tenant-scoped).
func (r *Repository) FindByTenantAndID(ctx context.Context, tenantID string, id uuid.UUID) (*model.Card, error) {
	row := r.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT %s FROM cards WHERE tenant_id = $1 AND id = $2`, cardColumns),
		tenantID, id)
	c, err := scanCard(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find card by tenant and id: %w", err)
	}
	return c, nil
}

// FindByTenant returns the tenant's cards, optionally filtered by customer.
func (r *Repository) FindByTenant(ctx context.Context, tenantID string, customerID *uuid.UUID) ([]model.Card, error) {
	query := fmt.Sprintf(`SELECT %s FROM cards WHERE tenant_id = $1`, cardColumns)
	args := []any{tenantID}
	if customerID != nil {
		query += ` AND customer_id = $2`
		args = append(args, *customerID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find cards by tenant: %w", err)
	}
	defer rows.Close()

	var result []model.Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, fmt.Errorf("scan card: %w", err)
		}
		result = append(result, *c)
	}
	return result, rows.Err()
}

// FindEventsByCard returns the audit trail for a card, newest first.
func (r *Repository) FindEventsByCard(ctx context.Context, tenantID string, cardID uuid.UUID, limit, offset int) ([]model.CardEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, card_id, event_type, actor, detail, created_at
		FROM card_events
		WHERE tenant_id = $1 AND card_id = $2
		ORDER BY id DESC
		LIMIT $3 OFFSET $4`,
		tenantID, cardID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("find card events: %w", err)
	}
	defer rows.Close()

	var result []model.CardEvent
	for rows.Next() {
		var e model.CardEvent
		var detail []byte
		if err := rows.Scan(&e.ID, &e.TenantID, &e.CardID, &e.EventType, &e.Actor, &detail, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan card event: %w", err)
		}
		if len(detail) > 0 {
			if err := json.Unmarshal(detail, &e.Detail); err != nil {
				return nil, fmt.Errorf("decode card event detail: %w", err)
			}
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
