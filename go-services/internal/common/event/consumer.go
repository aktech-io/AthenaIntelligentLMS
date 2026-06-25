package event

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/rabbitmq"
)

// Handler processes a domain event. Return nil to ack, error to nack+requeue.
type Handler func(ctx context.Context, event *DomainEvent) error

// Consumer reads domain events from a queue using a worker pool.
// Matches Java's concurrency="3-5" by running N goroutines with prefetchCount.
type Consumer struct {
	conn          *rabbitmq.Connection
	queue         string
	workers       int
	prefetchCount int
	handler       Handler
	logger        *zap.Logger
}

// NewConsumer creates a new event consumer.
// workers: number of goroutines processing messages (equivalent to Java concurrency min).
// prefetchCount: AMQP prefetch (equivalent to Java concurrency max).
func NewConsumer(conn *rabbitmq.Connection, queue string, workers, prefetchCount int, handler Handler, logger *zap.Logger) *Consumer {
	return &Consumer{
		conn:          conn,
		queue:         queue,
		workers:       workers,
		prefetchCount: prefetchCount,
		handler:       handler,
		logger:        logger,
	}
}

// Start consumes messages, automatically re-subscribing if the broker drops or
// is not yet available. Blocks until ctx is cancelled. The underlying connection
// reconnects on its own (see rabbitmq.Connection); Start re-opens the channel and
// re-subscribes on top of it, so a broker restart never permanently stops
// consumption without a pod restart.
func (c *Consumer) Start(ctx context.Context) error {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		subscribed, err := c.consume(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if subscribed {
			backoff = time.Second // ran fine for a while; retry quickly
		}
		c.logger.Warn("Consumer subscription ended; will resubscribe",
			zap.String("queue", c.queue), zap.Duration("backoff", backoff), zap.Error(err))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// consume runs one subscription until ctx is cancelled or the channel drops. It
// returns whether it successfully subscribed (used to reset backoff) and the
// terminating error, if any.
func (c *Consumer) consume(ctx context.Context) (bool, error) {
	ch, err := c.conn.Channel()
	if err != nil {
		return false, fmt.Errorf("open consumer channel: %w", err)
	}
	defer ch.Close()

	if err := ch.Qos(c.prefetchCount, 0, false); err != nil {
		return false, fmt.Errorf("set qos: %w", err)
	}

	deliveries, err := ch.Consume(
		c.queue,
		"",    // consumer tag (auto-generated)
		false, // autoAck
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // args
	)
	if err != nil {
		return false, fmt.Errorf("consume queue %s: %w", c.queue, err)
	}

	c.logger.Info("Consumer started",
		zap.String("queue", c.queue),
		zap.Int("workers", c.workers),
		zap.Int("prefetch", c.prefetchCount),
	)

	// Fan out deliveries to worker goroutines
	work := make(chan amqp.Delivery, c.prefetchCount)

	for i := 0; i < c.workers; i++ {
		go c.worker(ctx, i, work)
	}

	for {
		select {
		case <-ctx.Done():
			close(work)
			c.logger.Info("Consumer stopping", zap.String("queue", c.queue))
			return true, nil
		case d, ok := <-deliveries:
			if !ok {
				close(work)
				return true, fmt.Errorf("delivery channel closed for queue %s", c.queue)
			}
			work <- d
		}
	}
}

func (c *Consumer) worker(ctx context.Context, id int, work <-chan amqp.Delivery) {
	for d := range work {
		var event DomainEvent
		if err := json.Unmarshal(d.Body, &event); err != nil {
			c.logger.Error("Failed to unmarshal event",
				zap.Int("worker", id),
				zap.Error(err),
			)
			d.Nack(false, false) // don't requeue malformed messages
			continue
		}

		if err := c.handler(ctx, &event); err != nil {
			c.logger.Error("Failed to handle event",
				zap.String("type", event.Type),
				zap.String("id", event.ID),
				zap.Int("worker", id),
				zap.Error(err),
			)
			d.Nack(false, true) // requeue for retry
			continue
		}

		d.Ack(false)
	}
}
