package event

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/rabbitmq"
)

// Publisher publishes domain events to the LMS exchange.
//
// It is self-healing: it holds a reference to the connection and lazily
// (re)establishes the channel on each publish. This means a service that
// started before RabbitMQ was reachable — or one whose connection later
// dropped — recovers automatically instead of silently no-op'ing forever.
type Publisher struct {
	conn   *rabbitmq.Connection
	ch     *amqp.Channel
	logger *zap.Logger
	mu     sync.Mutex
}

// NewPublisher creates a new event publisher. It never fails on a missing
// broker: if RabbitMQ is not yet reachable the publisher starts channel-less
// and connects lazily on the first successful Publish.
func NewPublisher(conn *rabbitmq.Connection, logger *zap.Logger) (*Publisher, error) {
	p := &Publisher{conn: conn, logger: logger}
	if conn == nil {
		logger.Warn("Publisher created without a RabbitMQ connection")
		return p, nil
	}
	if err := p.ensureChannel(); err != nil {
		// Non-fatal: HTTP can serve immediately; the channel is opened lazily
		// once the broker becomes reachable.
		logger.Warn("Publisher starting without an open channel; will connect lazily",
			zap.Error(err))
	}
	return p, nil
}

// ensureChannel guarantees a live, confirm-enabled channel, reconnecting the
// underlying connection if necessary. Caller must hold p.mu (or be NewPublisher).
func (p *Publisher) ensureChannel() error {
	if p.ch != nil && !p.ch.IsClosed() {
		return nil
	}
	if p.conn == nil {
		return fmt.Errorf("no RabbitMQ connection configured")
	}
	// Fail fast if the broker is down — do NOT block the caller reconnecting.
	// The connection runs its own background reconnect loop, so the next publish
	// (or the outbox relay's next tick) succeeds once the broker is back. Without
	// this, a synchronous publish on an HTTP path would stall for the full
	// reconnect backoff during an outage.
	if !p.conn.IsConnected() {
		return fmt.Errorf("rabbitmq not connected")
	}
	ch, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("open publisher channel: %w", err)
	}
	// Enable publisher confirms for reliability.
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return fmt.Errorf("enable confirms: %w", err)
	}
	p.ch = ch
	return nil
}

// Publish publishes a DomainEvent to the LMS exchange with its type as routing key.
func (p *Publisher) Publish(ctx context.Context, event *DomainEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureChannel(); err != nil {
		p.logger.Warn("Event not published (RabbitMQ unavailable)",
			zap.String("type", event.Type), zap.String("id", event.ID), zap.Error(err))
		return fmt.Errorf("publish event %s: %w", event.Type, err)
	}

	err = p.ch.PublishWithContext(ctx,
		rabbitmq.LMSExchange, // exchange
		event.Type,           // routing key = event type
		false,                // mandatory
		false,                // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
	if err != nil {
		// Drop the channel so the next publish reopens a fresh one.
		_ = p.ch.Close()
		p.ch = nil
		return fmt.Errorf("publish event %s: %w", event.Type, err)
	}

	p.logger.Debug("Published event",
		zap.String("type", event.Type),
		zap.String("id", event.ID),
		zap.String("source", event.Source),
	)

	return nil
}

// Close closes the publisher channel.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ch != nil {
		return p.ch.Close()
	}
	return nil
}
