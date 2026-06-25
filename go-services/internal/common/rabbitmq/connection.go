package rabbitmq

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// Connection wraps an AMQP connection with reconnection support.
//
// It is resilient by design: a hard dependency like the broker is never
// permanently "given up" on. The initial dial is bounded so HTTP can start
// immediately, but if it fails a background goroutine keeps retrying with
// capped backoff until connected. Once connected, a close-notification watcher
// transparently re-dials if the broker drops. Callers observe liveness via
// IsConnected() and obtain channels via Channel().
type Connection struct {
	url     string
	logger  *zap.Logger
	onReady func(*Connection) // optional: re-declare topology after each (re)connect

	mu   sync.RWMutex
	conn *amqp.Connection

	startOnce sync.Once
}

// connect backoff bounds.
const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
	// initialDialAttempts bounds the *blocking* startup dial so a service can
	// begin serving HTTP quickly even when the broker is slow to appear.
	initialDialAttempts = 5
)

// NewConnection dials with a bounded retry and fails if the broker never
// appears in that window. Prefer TryConnection for services that should start
// regardless of broker availability.
func NewConnection(url string, logger *zap.Logger) (*Connection, error) {
	c := &Connection{url: url, logger: logger}
	if err := c.dialWithRetry(context.Background(), initialDialAttempts); err != nil {
		return nil, err
	}
	return c, nil
}

// TryConnection attempts a bounded initial dial and returns a Connection
// regardless of outcome. If the initial dial fails, a background goroutine
// keeps retrying forever (capped backoff) so the connection becomes live as
// soon as the broker is reachable — no restart required.
func TryConnection(url string, logger *zap.Logger) *Connection {
	c := &Connection{url: url, logger: logger}
	if err := c.dialWithRetry(context.Background(), initialDialAttempts); err != nil {
		logger.Warn("RabbitMQ not available at startup; retrying in background", zap.Error(err))
		c.StartReconnect(context.Background())
	} else {
		c.watchClose(context.Background())
	}
	return c
}

// OnReady registers a callback invoked after every successful (re)connect —
// use it to (re)declare exchanges/queues so topology survives a broker restart.
// Must be called before the first connect for the initial declare; it is also
// invoked on every subsequent reconnect.
func (c *Connection) OnReady(fn func(*Connection)) {
	c.mu.Lock()
	c.onReady = fn
	c.mu.Unlock()
	if c.IsConnected() {
		fn(c)
	}
}

// IsConnected returns true if the connection is established and open.
func (c *Connection) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil && !c.conn.IsClosed()
}

// Reconnect forces a (bounded) reconnect attempt now. Safe to call on demand
// (e.g. from a publisher) — it is a no-op if already connected.
func (c *Connection) Reconnect() error {
	if c.IsConnected() {
		return nil
	}
	return c.dialWithRetry(context.Background(), initialDialAttempts)
}

// StartReconnect launches (once) a background goroutine that keeps retrying
// forever until connected, then installs a close watcher.
func (c *Connection) StartReconnect(ctx context.Context) {
	c.startOnce.Do(func() {
		go func() {
			if err := c.dialWithRetry(ctx, 0 /* infinite */); err != nil {
				return // only on ctx cancellation
			}
			c.watchClose(ctx)
		}()
	})
}

// dialWithRetry dials with capped exponential backoff. maxAttempts<=0 means
// retry forever (until ctx is cancelled).
func (c *Connection) dialWithRetry(ctx context.Context, maxAttempts int) error {
	backoff := initialBackoff
	for attempt := 1; ; attempt++ {
		conn, err := amqp.Dial(c.url)
		if err == nil {
			c.mu.Lock()
			c.conn = conn
			onReady := c.onReady
			c.mu.Unlock()
			c.logger.Info("Connected to RabbitMQ", zap.Int("attempt", attempt))
			if onReady != nil {
				onReady(c)
			}
			return nil
		}
		if maxAttempts > 0 && attempt >= maxAttempts {
			return fmt.Errorf("failed to connect to RabbitMQ after %d attempts: %w", attempt, err)
		}
		c.logger.Warn("RabbitMQ connection attempt failed, retrying...",
			zap.Int("attempt", attempt), zap.Duration("backoff", backoff), zap.Error(err))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// watchClose blocks (in a goroutine) on the connection's close notification and
// transparently reconnects forever when the broker drops.
func (c *Connection) watchClose(ctx context.Context) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return
	}
	closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))
	go func() {
		select {
		case <-ctx.Done():
			return
		case reason, ok := <-closeCh:
			if !ok {
				return // graceful close
			}
			c.logger.Warn("RabbitMQ connection lost; reconnecting", zap.Error(reason))
			if err := c.dialWithRetry(ctx, 0 /* infinite */); err != nil {
				return
			}
			c.watchClose(ctx) // re-arm for the new connection
		}
	}()
}

// Channel opens a new AMQP channel on the live connection.
func (c *Connection) Channel() (*amqp.Channel, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil || conn.IsClosed() {
		return nil, fmt.Errorf("rabbitmq: no live connection")
	}
	return conn.Channel()
}

// Close closes the connection.
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
