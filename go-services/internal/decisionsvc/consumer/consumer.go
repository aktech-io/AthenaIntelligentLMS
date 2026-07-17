// Package consumer projects decision.recorded events into the append-only
// decision_log (design §2.1/§2.4). Delivery is at-least-once (producer outbox
// → RabbitMQ), so the projection is idempotent twice over: the standard
// processed_events guard, and ON CONFLICT (id, decided_at) DO NOTHING on the
// log itself.
package consumer

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/idempotency"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
	"github.com/athena-lms/go-services/internal/decisionsvc/model"
	"github.com/athena-lms/go-services/internal/decisionsvc/repository"
)

// recordPayload mirrors internal/common/decision.Record on the wire, keeping
// the nested JSON documents raw: the projection stores what the producer
// recorded, verbatim, and never reinterprets it.
type recordPayload struct {
	DecisionType     string          `json:"decisionType"`
	TenantID         string          `json:"tenantId"`
	SubjectType      string          `json:"subjectType"`
	SubjectID        string          `json:"subjectId"`
	CustomerID       string          `json:"customerId"`
	ActorType        string          `json:"actorType"`
	ActorID          string          `json:"actorId"`
	PolicyID         string          `json:"policyId"`
	PolicyVersion    int             `json:"policyVersion"`
	PolicyHash       string          `json:"policyHash"`
	Inputs           json.RawMessage `json:"inputs"`
	Outcome          string          `json:"outcome"`
	OutcomeDetail    json.RawMessage `json:"outcomeDetail"`
	Reasons          json.RawMessage `json:"reasons"`
	Models           json.RawMessage `json:"models"`
	Variant          string          `json:"variant"`
	ParentDecisionID string          `json:"parentDecisionId"`
	LatencyMS        float64         `json:"latencyMs"`
	DecidedAt        time.Time       `json:"decidedAt"`
}

// Consumer subscribes to the decision queue and projects records.
type Consumer struct {
	inner *commonEvent.Consumer
}

// New wires the projection consumer on athena.lms.decision.queue.
func New(conn *rabbitmq.Connection, pool *pgxpool.Pool, repo *repository.Repository, logger *zap.Logger) *Consumer {
	h := Handler(repo, logger)
	guarded := idempotency.Wrap(pool, logger, h)
	return &Consumer{
		inner: commonEvent.NewConsumer(conn, rabbitmq.DecisionQueue, 3, 5, guarded, logger),
	}
}

// Start blocks until ctx is cancelled.
func (c *Consumer) Start(ctx context.Context) error { return c.inner.Start(ctx) }

// Handler returns the projection handler (exported for tests). Contract:
//   - nil          → ack. Includes permanently unprocessable events (bad
//     payload): they are logged loudly and skipped, never poison the queue.
//   - error        → nack + requeue (transient failures: DB down, DDL race).
func Handler(repo *repository.Repository, logger *zap.Logger) commonEvent.Handler {
	return func(ctx context.Context, evt *commonEvent.DomainEvent) error {
		if evt.Type != commonEvent.DecisionRecorded {
			// decision.# may carry future event types; they are not ours to fail.
			logger.Debug("Ignoring non-decision.recorded event", zap.String("type", evt.Type))
			return nil
		}
		d, err := Project(evt)
		if err != nil {
			// Permanently malformed: skipping is deliberate — the producer's
			// outbox row is the durable source for backfill/repair.
			logger.Error("Unprocessable decision.recorded event; skipping",
				zap.String("id", evt.ID), zap.Error(err))
			return nil
		}
		if err := repo.InsertDecision(ctx, d); err != nil {
			return err
		}
		logger.Debug("Projected decision",
			zap.String("id", d.ID), zap.String("type", d.DecisionType),
			zap.String("outcome", d.Outcome), zap.String("variant", d.Variant))
		return nil
	}
}

// Project maps one decision.recorded event to a decision_log row, validating
// the invariants the schema relies on.
func Project(evt *commonEvent.DomainEvent) (*model.Decision, error) {
	if _, err := uuid.Parse(evt.ID); err != nil {
		return nil, errBad("event id is not a uuid: " + evt.ID)
	}
	var p recordPayload
	if err := json.Unmarshal(evt.Payload, &p); err != nil {
		return nil, err
	}
	if p.DecisionType == "" || p.Outcome == "" || p.SubjectID == "" || p.DecidedAt.IsZero() {
		return nil, errBad("missing decisionType/outcome/subjectId/decidedAt")
	}
	tenant := p.TenantID
	if tenant == "" {
		tenant = evt.TenantID
	}
	if tenant == "" {
		return nil, errBad("no tenant on payload or envelope")
	}
	if p.Variant == "" {
		p.Variant = "champion"
	}

	d := &model.Decision{
		ID:            evt.ID,
		TenantID:      tenant,
		DecisionType:  p.DecisionType,
		SubjectType:   p.SubjectType,
		SubjectID:     p.SubjectID,
		ActorType:     p.ActorType,
		ActorID:       p.ActorID,
		PolicyID:      p.PolicyID,
		PolicyVersion: p.PolicyVersion,
		PolicyHash:    p.PolicyHash,
		Inputs:        orJSON(p.Inputs, "{}"),
		Outcome:       p.Outcome,
		OutcomeDetail: p.OutcomeDetail,
		Reasons:       orJSON(p.Reasons, "[]"),
		Models:        orJSON(p.Models, "[]"),
		Variant:       p.Variant,
		LatencyMS:     &p.LatencyMS,
		DecidedAt:     p.DecidedAt,
	}
	if p.CustomerID != "" {
		d.CustomerID = &p.CustomerID
	}
	if p.ParentDecisionID != "" {
		if _, err := uuid.Parse(p.ParentDecisionID); err != nil {
			return nil, errBad("parentDecisionId is not a uuid: " + p.ParentDecisionID)
		}
		d.ParentDecisionID = &p.ParentDecisionID
	}
	if evt.CorrelationID != "" {
		d.CorrelationID = &evt.CorrelationID
	}
	return d, nil
}

func orJSON(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(fallback)
	}
	return raw
}

type errBad string

func (e errBad) Error() string { return string(e) }
