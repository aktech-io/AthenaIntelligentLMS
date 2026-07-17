// Package service implements the card-issuing business logic (Nemo B1).
//
// Lifecycle state machine (enforced here, acknowledged by the processor):
//
//	REQUESTED ──activate──▶ ACTIVE ◀──unfreeze── FROZEN
//	                          │  ╲──freeze──────▶ FROZEN
//	                          │
//	   ACTIVE|FROZEN ──block──▶ BLOCKED   (terminal: immutable)
//	   ACTIVE|FROZEN ──close──▶ CLOSED    (terminal; close deferred to v2 API)
//
// Every state change is (a) pushed to the processor first, (b) persisted
// atomically with a card_events audit row and a transactional-outbox domain
// event (card.issued / card.frozen / ...), per repo convention (F27).
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/card/model"
	"github.com/athena-lms/go-services/internal/card/processor"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/event"
)

const serviceName = "card-service"

// Repository is the persistence surface the service needs. The pgx
// implementation (internal/card/repository) writes the card change, the
// card_events audit row, and the outbox event in ONE transaction; tests use
// an in-memory fake.
type Repository interface {
	// InsertCard persists a new card + audit row + outbox event atomically.
	InsertCard(ctx context.Context, c *model.Card, audit *model.CardEvent, evt *event.DomainEvent) (*model.Card, error)
	// UpdateCard persists status/limits changes + audit row + outbox event atomically.
	UpdateCard(ctx context.Context, c *model.Card, audit *model.CardEvent, evt *event.DomainEvent) (*model.Card, error)
	FindByTenantAndID(ctx context.Context, tenantID string, id uuid.UUID) (*model.Card, error)
	FindByTenant(ctx context.Context, tenantID string, customerID *uuid.UUID) ([]model.Card, error)
	FindEventsByCard(ctx context.Context, tenantID string, cardID uuid.UUID, limit, offset int) ([]model.CardEvent, error)
}

// Service implements card issuance and lifecycle.
type Service struct {
	repo   Repository
	proc   processor.Processor
	logger *zap.Logger
}

// New creates a Service.
func New(repo Repository, proc processor.Processor, logger *zap.Logger) *Service {
	return &Service{repo: repo, proc: proc, logger: logger}
}

func actor(ctx context.Context) string {
	if a := auth.UserIDFromContext(ctx); a != "" {
		return a
	}
	return "system"
}

// IssueCard issues a card via the configured processor and persists it.
// Sandbox flow works end-to-end; Paymentology fails fast until credentials
// arrive (adapter stub). Virtual cards issue straight to ACTIVE; physical to
// REQUESTED until an activation webhook/flow lands.
func (s *Service) IssueCard(ctx context.Context, req *model.IssueCardRequest, tenantID string) (*model.Card, error) {
	req.Normalize()
	if msg := req.Validate(); msg != "" {
		return nil, errors.BadRequest(msg)
	}

	res, err := s.proc.IssueCard(ctx, processor.IssueRequest{
		TenantID:       tenantID,
		CustomerID:     req.CustomerID.String(),
		AccountID:      req.AccountID.String(),
		Type:           req.Type,
		Network:        req.Network,
		Currency:       req.Currency,
		CardholderName: req.CardholderName,
		Limits:         *req.Limits,
	})
	if err != nil {
		s.logger.Warn("Processor issuance failed",
			zap.String("processor", s.proc.Name()), zap.Error(err))
		return nil, errors.NewBusinessError(fmt.Sprintf("card issuance failed: %v", err))
	}

	status := model.CardStatusRequested
	if res.Active {
		status = model.CardStatusActive
	}
	card := &model.Card{
		ID:             uuid.New(),
		TenantID:       tenantID,
		CustomerID:     req.CustomerID,
		AccountID:      req.AccountID,
		Processor:      s.proc.Name(),
		ProcessorRef:   res.ProcessorRef,
		PanLast4:       res.PanLast4, // last4 only — PCI posture, see model package comment
		Network:        res.Network,
		Type:           req.Type,
		Status:         status,
		Currency:       req.Currency,
		CardholderName: req.CardholderName,
		Limits:         *req.Limits,
	}

	evt, err := s.buildEvent(event.CardIssued, card, map[string]any{"type": card.Type, "network": card.Network})
	if err != nil {
		return nil, err
	}
	audit := s.auditRow(ctx, card, event.CardIssued, map[string]any{
		"processor": card.Processor, "type": string(card.Type), "status": string(card.Status),
	})
	created, err := s.repo.InsertCard(ctx, card, audit, evt)
	if err != nil {
		return nil, err
	}
	s.logger.Info("Card issued",
		zap.String("cardId", created.ID.String()),
		zap.String("processor", created.Processor),
		zap.String("status", string(created.Status)))
	return created, nil
}

// GetCard returns one card, tenant-scoped.
func (s *Service) GetCard(ctx context.Context, tenantID string, id uuid.UUID) (*model.Card, error) {
	card, err := s.repo.FindByTenantAndID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if card == nil {
		return nil, errors.NotFoundResource("Card", id)
	}
	return card, nil
}

// ListCards returns the tenant's cards, optionally filtered by customer.
func (s *Service) ListCards(ctx context.Context, tenantID string, customerID *uuid.UUID) ([]model.Card, error) {
	return s.repo.FindByTenant(ctx, tenantID, customerID)
}

// ListCardEvents returns the audit trail for a card, tenant-scoped.
func (s *Service) ListCardEvents(ctx context.Context, tenantID string, cardID uuid.UUID, limit, offset int) ([]model.CardEvent, error) {
	if _, err := s.GetCard(ctx, tenantID, cardID); err != nil {
		return nil, err
	}
	return s.repo.FindEventsByCard(ctx, tenantID, cardID, limit, offset)
}

// FreezeCard places a reversible hold. Idempotent: freezing a FROZEN card is
// a no-op success (no duplicate event). Blocked/closed cards are immutable.
func (s *Service) FreezeCard(ctx context.Context, tenantID string, id uuid.UUID) (*model.Card, error) {
	card, err := s.GetCard(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if card.Status == model.CardStatusFrozen {
		return card, nil // idempotent
	}
	if card.Status != model.CardStatusActive {
		return nil, terminalOrInvalid(card, "freeze")
	}
	if err := s.proc.FreezeCard(ctx, card.ProcessorRef); err != nil {
		return nil, errors.NewBusinessError(fmt.Sprintf("processor freeze failed: %v", err))
	}
	return s.transition(ctx, card, model.CardStatusFrozen, event.CardFrozen, nil)
}

// UnfreezeCard lifts a hold. Idempotent on ACTIVE. Blocked/closed immutable.
func (s *Service) UnfreezeCard(ctx context.Context, tenantID string, id uuid.UUID) (*model.Card, error) {
	card, err := s.GetCard(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if card.Status == model.CardStatusActive {
		return card, nil // idempotent
	}
	if card.Status != model.CardStatusFrozen {
		return nil, terminalOrInvalid(card, "unfreeze")
	}
	if err := s.proc.UnfreezeCard(ctx, card.ProcessorRef); err != nil {
		return nil, errors.NewBusinessError(fmt.Sprintf("processor unfreeze failed: %v", err))
	}
	return s.transition(ctx, card, model.CardStatusActive, event.CardUnfrozen, nil)
}

// BlockCard permanently blocks a card (lost/stolen/fraud). Terminal and
// idempotent: blocking a BLOCKED card is a no-op success.
func (s *Service) BlockCard(ctx context.Context, tenantID string, id uuid.UUID, reason string) (*model.Card, error) {
	card, err := s.GetCard(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if card.Status == model.CardStatusBlocked {
		return card, nil // idempotent
	}
	if card.Status == model.CardStatusClosed {
		return nil, errors.Conflict("card is CLOSED and cannot be blocked")
	}
	if err := s.proc.BlockCard(ctx, card.ProcessorRef, reason); err != nil {
		return nil, errors.NewBusinessError(fmt.Sprintf("processor block failed: %v", err))
	}
	detail := map[string]any{}
	if reason != "" {
		detail["reason"] = reason
	}
	return s.transition(ctx, card, model.CardStatusBlocked, event.CardBlocked, detail)
}

// SetLimits replaces the card's spending controls. Not allowed on terminal
// states (blocked-card immutability).
func (s *Service) SetLimits(ctx context.Context, tenantID string, id uuid.UUID, limits model.SpendingLimits) (*model.Card, error) {
	if msg := limits.Validate(); msg != "" {
		return nil, errors.BadRequest(msg)
	}
	card, err := s.GetCard(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if card.Status == model.CardStatusBlocked || card.Status == model.CardStatusClosed {
		return nil, errors.Conflict(fmt.Sprintf("card is %s and cannot be modified", card.Status))
	}
	if err := s.proc.SetLimits(ctx, card.ProcessorRef, limits); err != nil {
		return nil, errors.NewBusinessError(fmt.Sprintf("processor limit update failed: %v", err))
	}

	card.Limits = limits
	evt, err := s.buildEvent(event.CardLimitsChanged, card, map[string]any{"limits": limits})
	if err != nil {
		return nil, err
	}
	audit := s.auditRow(ctx, card, event.CardLimitsChanged, map[string]any{"limits": limits})
	return s.repo.UpdateCard(ctx, card, audit, evt)
}

// transition persists a status change with its audit row + outbox event.
func (s *Service) transition(ctx context.Context, card *model.Card, to model.CardStatus, eventType string, detail map[string]any) (*model.Card, error) {
	from := card.Status
	card.Status = to
	if detail == nil {
		detail = map[string]any{}
	}
	detail["from"] = string(from)
	detail["to"] = string(to)

	evt, err := s.buildEvent(eventType, card, detail)
	if err != nil {
		return nil, err
	}
	audit := s.auditRow(ctx, card, eventType, detail)
	updated, err := s.repo.UpdateCard(ctx, card, audit, evt)
	if err != nil {
		return nil, err
	}
	s.logger.Info("Card state changed",
		zap.String("cardId", card.ID.String()),
		zap.String("from", string(from)), zap.String("to", string(to)))
	return updated, nil
}

func terminalOrInvalid(card *model.Card, op string) error {
	switch card.Status {
	case model.CardStatusBlocked, model.CardStatusClosed:
		return errors.Conflict(fmt.Sprintf("card is %s and cannot be modified", card.Status))
	default:
		return errors.Conflict(fmt.Sprintf("cannot %s a card in status %s", op, card.Status))
	}
}

// buildEvent creates the outbox domain event. Payloads carry processorRef +
// panLast4 only — never PAN/CVV (PCI posture).
func (s *Service) buildEvent(eventType string, card *model.Card, extra map[string]any) (*event.DomainEvent, error) {
	payload := map[string]any{
		"cardId":       card.ID.String(),
		"customerId":   card.CustomerID.String(),
		"accountId":    card.AccountID.String(),
		"processor":    card.Processor,
		"processorRef": card.ProcessorRef,
		"panLast4":     card.PanLast4,
		"status":       string(card.Status),
		"tenantId":     card.TenantID,
	}
	for k, v := range extra {
		payload[k] = v
	}
	evt, err := event.NewDomainEvent(eventType, serviceName, card.TenantID, "", payload)
	if err != nil {
		return nil, fmt.Errorf("build %s event: %w", eventType, err)
	}
	return evt, nil
}

func (s *Service) auditRow(ctx context.Context, card *model.Card, eventType string, detail map[string]any) *model.CardEvent {
	return &model.CardEvent{
		TenantID:  card.TenantID,
		CardID:    card.ID,
		EventType: eventType,
		Actor:     actor(ctx),
		Detail:    detail,
	}
}
