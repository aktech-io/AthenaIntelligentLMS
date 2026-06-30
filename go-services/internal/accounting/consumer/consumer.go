package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
)

// accountingService is the subset of *service.AccountingService that the
// consumer depends on. Declaring it as an interface keeps the consumer
// unit-testable (a fake can record postings and dedupe) while the concrete
// service satisfies it unchanged.
type accountingService interface {
	EntryExists(ctx context.Context, sourceEvent, sourceID string) bool
	PostLoanDisbursement(ctx context.Context, tenantID, applicationID string, amount decimal.Decimal) error
	PostRepayment(ctx context.Context, tenantID, paymentID string, amount decimal.Decimal, payload map[string]any) error
	PostPaymentReversal(ctx context.Context, tenantID, paymentID string, amount decimal.Decimal) error
	PostOverdraftDrawn(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error
	PostOverdraftRepaid(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error
	PostOverdraftInterestCharged(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error
	PostOverdraftFeeCharged(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error
	PostFloatDrawn(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error
	PostFloatRepaid(ctx context.Context, tenantID, sourceID string, amount decimal.Decimal) error
}

// AccountingConsumer handles incoming domain events for the accounting service.
type AccountingConsumer struct {
	svc    accountingService
	conn   *rabbitmq.Connection
	logger *zap.Logger
}

// New creates a new accounting event consumer.
func New(svc accountingService, conn *rabbitmq.Connection, logger *zap.Logger) *AccountingConsumer {
	return &AccountingConsumer{svc: svc, conn: conn, logger: logger}
}

// Start begins consuming events from the accounting queue.
// Blocks until ctx is cancelled.
func (c *AccountingConsumer) Start(ctx context.Context) error {
	consumer := event.NewConsumer(c.conn, rabbitmq.AccountingQueue, 3, 5, c.handle, c.logger)
	return consumer.Start(ctx)
}

func (c *AccountingConsumer) handle(ctx context.Context, evt *event.DomainEvent) error {
	eventType := evt.Type
	tenantID := evt.TenantID

	// Try to get event type from payload if not on envelope
	var payload map[string]any
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		c.logger.Error("Failed to unmarshal event payload", zap.Error(err))
		return nil // don't retry malformed payloads
	}

	// Handle raw-map events that have eventType in payload
	if eventType == "" {
		if et, ok := payload["eventType"].(string); ok {
			eventType = et
		} else if et, ok := payload["type"].(string); ok {
			eventType = et
		}
	}

	if eventType == "" {
		c.logger.Debug("Could not resolve event type, skipping")
		return nil
	}

	// Resolve tenant ID from payload if not on envelope
	if tenantID == "" {
		if tid, ok := payload["tenantId"].(string); ok {
			tenantID = tid
		}
	}

	// Resolve nested payload if DomainEvent envelope wraps another payload
	if inner, ok := payload["payload"].(map[string]any); ok {
		if tenantID == "" {
			if tid, ok := inner["tenantId"].(string); ok {
				tenantID = tid
			}
		}
		payload = inner
	}

	c.logger.Info("Accounting processing event", zap.String("eventType", eventType), zap.String("tenantId", tenantID))

	switch eventType {
	case "loan.disbursed":
		return c.handleLoanDisbursed(ctx, payload, tenantID)
	case "payment.completed":
		return c.handlePaymentCompleted(ctx, payload, tenantID)
	case "payment.reversed":
		return c.handlePaymentReversed(ctx, payload, tenantID)
	case "loan.closed":
		c.handleLoanClosed(payload)
		return nil
	case "loan.stage.changed":
		c.handleStageChanged(payload)
		return nil
	case "float.drawn":
		return c.handleFloatDrawn(ctx, payload, tenantID, evt.ID)
	case "float.repaid":
		return c.handleFloatRepaid(ctx, payload, tenantID, evt.ID)
	case "overdraft.drawn":
		return c.handleOverdraftDrawn(ctx, payload, tenantID)
	case "overdraft.repaid":
		return c.handleOverdraftRepaid(ctx, payload, tenantID, evt.ID)
	case "overdraft.interest.charged":
		return c.handleOverdraftInterestCharged(ctx, payload, tenantID, evt.ID)
	case "overdraft.fee.charged":
		return c.handleOverdraftFeeCharged(ctx, payload, tenantID)
	default:
		c.logger.Debug("No accounting handler for event", zap.String("type", eventType))
		return nil
	}
}

func (c *AccountingConsumer) handleLoanDisbursed(ctx context.Context, payload map[string]any, tenantID string) error {
	sourceID := getStr(payload, "applicationId")
	if c.svc.EntryExists(ctx, "loan.disbursed", sourceID) {
		return nil
	}
	amount := getDecimal(payload, "amount")
	return c.svc.PostLoanDisbursement(ctx, tenantID, sourceID, amount)
}

func (c *AccountingConsumer) handlePaymentCompleted(ctx context.Context, payload map[string]any, tenantID string) error {
	sourceID := getStr(payload, "paymentId")
	if sourceID == "" {
		sourceID = getStr(payload, "internalReference")
	}
	if c.svc.EntryExists(ctx, "payment.completed", sourceID) {
		return nil
	}

	amount := getDecimal(payload, "amount")
	paymentType := getStr(payload, "paymentType")

	// Skip disbursements -- already handled by loan.disbursed
	if paymentType == "LOAN_DISBURSEMENT" {
		return nil
	}

	return c.svc.PostRepayment(ctx, tenantID, sourceID, amount, payload)
}

func (c *AccountingConsumer) handlePaymentReversed(ctx context.Context, payload map[string]any, tenantID string) error {
	sourceID := getStr(payload, "paymentId")
	if sourceID == "" {
		return nil
	}
	amount := getDecimal(payload, "amount")
	return c.svc.PostPaymentReversal(ctx, tenantID, sourceID, amount)
}

func (c *AccountingConsumer) handleLoanClosed(payload map[string]any) {
	loanID := getStr(payload, "loanId")
	c.logger.Info("Loan closed -- no accounting entry required at close (balance already zeroed by repayments)", zap.String("loanId", loanID))
}

func (c *AccountingConsumer) handleStageChanged(payload map[string]any) {
	loanID := getStr(payload, "loanId")
	newStage := getStr(payload, "newStage")
	c.logger.Info("Loan stage changed -- provision review may be required", zap.String("loanId", loanID), zap.String("newStage", newStage))
}

func (c *AccountingConsumer) handleOverdraftDrawn(ctx context.Context, payload map[string]any, tenantID string) error {
	walletID := getStr(payload, "walletId")
	sourceID := "OD-DRAW-" + walletID
	if c.svc.EntryExists(ctx, "overdraft.drawn", sourceID) {
		return nil
	}
	amount := getDecimal(payload, "amount")
	return c.svc.PostOverdraftDrawn(ctx, tenantID, sourceID, amount)
}

func (c *AccountingConsumer) handleOverdraftRepaid(ctx context.Context, payload map[string]any, tenantID, eventID string) error {
	walletID := getStr(payload, "walletId")
	// overdraft.repaid carries no per-repayment business key, so derive the
	// idempotency key from the STABLE event envelope ID (constant across
	// RabbitMQ redelivery) rather than a wall-clock timestamp. Using a timestamp
	// made every redelivery a fresh sourceID, defeating EntryExists and
	// double-posting the cash entry (H-3).
	sourceID := fmt.Sprintf("OD-RPMT-%s-%s", walletID, eventID)
	if c.svc.EntryExists(ctx, "overdraft.repaid", sourceID) {
		return nil
	}
	amount := getDecimal(payload, "amount")
	return c.svc.PostOverdraftRepaid(ctx, tenantID, sourceID, amount)
}

func (c *AccountingConsumer) handleOverdraftInterestCharged(ctx context.Context, payload map[string]any, tenantID, eventID string) error {
	walletID := getStr(payload, "walletId")
	// Dedupe on the stable event envelope ID (see handleOverdraftRepaid); the
	// interest-charged event carries no per-charge business key, so a timestamp
	// key would re-post the interest accrual on every redelivery (H-3).
	sourceID := fmt.Sprintf("OD-INT-%s-%s", walletID, eventID)
	if c.svc.EntryExists(ctx, "overdraft.interest.charged", sourceID) {
		return nil
	}
	interest := getDecimal(payload, "interestCharged")
	return c.svc.PostOverdraftInterestCharged(ctx, tenantID, sourceID, interest)
}

func (c *AccountingConsumer) handleOverdraftFeeCharged(ctx context.Context, payload map[string]any, tenantID string) error {
	walletID := getStr(payload, "walletId")
	reference := getStr(payload, "reference")
	sourceID := "OD-FEE-"
	if reference != "" {
		sourceID += reference
	} else {
		sourceID += walletID
	}
	if c.svc.EntryExists(ctx, "overdraft.fee.charged", sourceID) {
		return nil
	}
	amount := getDecimal(payload, "amount")
	return c.svc.PostOverdraftFeeCharged(ctx, tenantID, sourceID, amount)
}

// handleFloatDrawn creates a GL entry when float pool is drawn for loan disbursement.
// DR 2100 Borrowings (Float Liability increases) / CR 1000 Cash (Cash decreases)
func (c *AccountingConsumer) handleFloatDrawn(ctx context.Context, payload map[string]any, tenantID, eventID string) error {
	floatAccountID := getStr(payload, "floatAccountId")
	loanID := getStr(payload, "loanId")
	sourceID := fmt.Sprintf("FLOAT-DRAW-%s-%s", floatAccountID, loanID)
	if sourceID == "FLOAT-DRAW--" {
		// No business key on the event — fall back to the stable event envelope
		// ID so redelivery still dedupes (a timestamp would double-post; H-3).
		sourceID = "FLOAT-DRAW-" + eventID
	}
	if c.svc.EntryExists(ctx, "float.drawn", sourceID) {
		return nil
	}
	amount := getDecimal(payload, "amount")
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	return c.svc.PostFloatDrawn(ctx, tenantID, sourceID, amount)
}

// handleFloatRepaid creates a GL entry when float pool is repaid via collections.
// DR 1000 Cash (Cash increases) / CR 2100 Borrowings (Float Liability decreases)
func (c *AccountingConsumer) handleFloatRepaid(ctx context.Context, payload map[string]any, tenantID, eventID string) error {
	floatAccountID := getStr(payload, "floatAccountId")
	// float.repaid carries no per-repayment business key, so dedupe on the stable
	// event envelope ID (constant across redelivery) instead of a timestamp,
	// which re-posted the cash receipt on every at-least-once redelivery (H-3).
	sourceID := fmt.Sprintf("FLOAT-RPMT-%s-%s", floatAccountID, eventID)
	if c.svc.EntryExists(ctx, "float.repaid", sourceID) {
		return nil
	}
	amount := getDecimal(payload, "amount")
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	return c.svc.PostFloatRepaid(ctx, tenantID, sourceID, amount)
}

// --- helpers ---

func getStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func getDecimal(m map[string]any, key string) decimal.Decimal {
	v, ok := m[key]
	if !ok || v == nil {
		return decimal.Zero
	}
	switch val := v.(type) {
	case float64:
		return decimal.NewFromFloat(val)
	case string:
		d, err := decimal.NewFromString(val)
		if err != nil {
			return decimal.Zero
		}
		return d
	case json.Number:
		d, err := decimal.NewFromString(val.String())
		if err != nil {
			return decimal.Zero
		}
		return d
	default:
		d, err := decimal.NewFromString(fmt.Sprintf("%v", val))
		if err != nil {
			return decimal.Zero
		}
		return d
	}
}
