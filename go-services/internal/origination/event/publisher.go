package event

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/origination/model"
)

const serviceName = "loan-origination-service"

// Publisher publishes loan origination events.
type Publisher struct {
	pub    *commonEvent.Publisher
	logger *zap.Logger
}

// NewPublisher creates a new origination event publisher.
func NewPublisher(pub *commonEvent.Publisher, logger *zap.Logger) *Publisher {
	return &Publisher{pub: pub, logger: logger}
}

// applicationPayload is the standard event payload for loan application events.
type applicationPayload struct {
	ApplicationID       string           `json:"applicationId"`
	TenantID            string           `json:"tenantId"`
	CustomerID          string           `json:"customerId"`
	ProductID           string           `json:"productId"`
	Status              string           `json:"status"`
	Amount              decimal.Decimal  `json:"amount"`
	Currency            string           `json:"currency"`
	TenorMonths         int              `json:"tenorMonths"`
	InterestRate        *decimal.Decimal `json:"interestRate,omitempty"`
	DisbursementAccount *string          `json:"disbursementAccount,omitempty"`
	DepositAmount       decimal.Decimal  `json:"depositAmount,omitempty"`

	// Extra fields for specific events
	Reason             string  `json:"reason,omitempty"`
	ScheduleType       *string `json:"scheduleType,omitempty"`
	RepaymentFrequency *string `json:"repaymentFrequency,omitempty"`
}

func buildPayload(app *model.LoanApplication) applicationPayload {
	amount := app.RequestedAmount
	if app.ApprovedAmount != nil {
		amount = *app.ApprovedAmount
	}
	return applicationPayload{
		ApplicationID:       app.ID.String(),
		TenantID:            app.TenantID,
		CustomerID:          app.CustomerID,
		ProductID:           app.ProductID.String(),
		Status:              string(app.Status),
		Amount:              amount,
		Currency:            app.Currency,
		TenorMonths:         app.TenorMonths,
		InterestRate:        app.InterestRate,
		DisbursementAccount: app.DisbursementAccount,
		DepositAmount:       app.DepositAmount,
	}
}

// PublishSubmitted publishes a loan.application.submitted event.
func (p *Publisher) PublishSubmitted(ctx context.Context, app *model.LoanApplication) {
	p.publish(ctx, commonEvent.LoanApplicationSubmitted, app, nil)
}

// PublishApproved publishes a loan.application.approved event.
func (p *Publisher) PublishApproved(ctx context.Context, app *model.LoanApplication) {
	p.publish(ctx, commonEvent.LoanApplicationApproved, app, nil)
}

// PublishRejected publishes a loan.application.rejected event.
func (p *Publisher) PublishRejected(ctx context.Context, app *model.LoanApplication, reason string) {
	extra := map[string]string{"reason": reason}
	p.publish(ctx, commonEvent.LoanApplicationRejected, app, extra)
}

// PublishDisbursed publishes a loan.disbursed event with schedule config.
func (p *Publisher) PublishDisbursed(ctx context.Context, app *model.LoanApplication, scheduleType, repaymentFrequency *string) {
	extra := map[string]*string{
		"scheduleType":       scheduleType,
		"repaymentFrequency": repaymentFrequency,
	}
	p.publish(ctx, commonEvent.LoanDisbursed, app, extra)
}

// BuildSubmitted constructs the loan.application.submitted DomainEvent WITHOUT
// publishing it. Used by the transactional-outbox path so the event is persisted
// atomically with the SUBMITTED state change and delivered at-least-once by the relay.
func (p *Publisher) BuildSubmitted(app *model.LoanApplication) (*commonEvent.DomainEvent, error) {
	return p.build(commonEvent.LoanApplicationSubmitted, app, nil)
}

// BuildApproved constructs the loan.application.approved DomainEvent WITHOUT
// publishing it. Used by the transactional-outbox path so the event is persisted
// atomically with the APPROVED state change and delivered at-least-once by the relay.
func (p *Publisher) BuildApproved(app *model.LoanApplication) (*commonEvent.DomainEvent, error) {
	return p.build(commonEvent.LoanApplicationApproved, app, nil)
}

// BuildRejected constructs the loan.application.rejected DomainEvent WITHOUT
// publishing it. Used by the transactional-outbox path so the event is persisted
// atomically with the REJECTED state change and delivered at-least-once by the relay.
func (p *Publisher) BuildRejected(app *model.LoanApplication, reason string) (*commonEvent.DomainEvent, error) {
	return p.build(commonEvent.LoanApplicationRejected, app, map[string]string{"reason": reason})
}

// BuildDisbursed constructs the loan.disbursed DomainEvent WITHOUT publishing it.
// Used by the transactional-outbox path so the event is persisted atomically
// with the disbursement state change and delivered at-least-once by the relay.
func (p *Publisher) BuildDisbursed(app *model.LoanApplication, scheduleType, repaymentFrequency *string) (*commonEvent.DomainEvent, error) {
	return p.build(commonEvent.LoanDisbursed, app, map[string]*string{
		"scheduleType":       scheduleType,
		"repaymentFrequency": repaymentFrequency,
	})
}

// build constructs a DomainEvent from an application and optional extra fields.
func (p *Publisher) build(eventType string, app *model.LoanApplication, extra interface{}) (*commonEvent.DomainEvent, error) {
	payload := buildPayload(app)

	// Merge extra fields
	switch e := extra.(type) {
	case map[string]string:
		if v, ok := e["reason"]; ok {
			payload.Reason = v
		}
	case map[string]*string:
		if v, ok := e["scheduleType"]; ok {
			payload.ScheduleType = v
		}
		if v, ok := e["repaymentFrequency"]; ok {
			payload.RepaymentFrequency = v
		}
	}
	return commonEvent.NewDomainEvent(eventType, serviceName, app.TenantID, app.ID.String(), payload)
}

func (p *Publisher) publish(ctx context.Context, eventType string, app *model.LoanApplication, extra interface{}) {
	event, err := p.build(eventType, app, extra)
	if err != nil {
		p.logger.Error("Failed to create event",
			zap.String("type", eventType),
			zap.Error(err))
		return
	}

	// Use a background context with timeout for publishing so it doesn't fail if the request ctx is done.
	pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.pub.Publish(pubCtx, event); err != nil {
		p.logger.Error("Failed to publish event",
			zap.String("type", eventType),
			zap.String("applicationId", app.ID.String()),
			zap.Error(err))
	} else {
		p.logger.Info("Published event",
			zap.String("type", eventType),
			zap.String("applicationId", app.ID.String()))
	}
}
