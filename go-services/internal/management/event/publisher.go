package event

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/management/model"
)

// ServiceName is stamped as the Source on every event this service publishes.
// Exported because the management consumer uses it to skip self-published
// payment.completed events (it must only apply payment-service payments).
const ServiceName = "loan-management-service"

// ManagementPublisher publishes loan management domain events.
type ManagementPublisher struct {
	pub    *commonEvent.Publisher
	logger *zap.Logger
}

// NewManagementPublisher creates a new ManagementPublisher.
func NewManagementPublisher(pub *commonEvent.Publisher, logger *zap.Logger) *ManagementPublisher {
	return &ManagementPublisher{pub: pub, logger: logger}
}

// basePayload returns the common fields for all loan events.
func basePayload(loan *model.Loan) map[string]any {
	return map[string]any{
		"loanId":     loan.ID.String(),
		"tenantId":   loan.TenantID,
		"customerId": loan.CustomerID,
		"status":     string(loan.Status),
		"stage":      string(loan.Stage),
		"dpd":        loan.DPD,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}
}

// PublishStageChanged publishes a loan.stage.changed event.
func (p *ManagementPublisher) PublishStageChanged(ctx context.Context, loan *model.Loan, previousStage string) {
	payload := basePayload(loan)
	payload["previousStage"] = previousStage
	payload["newStage"] = string(loan.Stage)
	p.publish(ctx, commonEvent.LoanStageChanged, loan.TenantID, loan.ID.String(), payload)
}

// PublishDpdUpdated publishes a loan.dpd.updated event.
func (p *ManagementPublisher) PublishDpdUpdated(ctx context.Context, loan *model.Loan) {
	payload := basePayload(loan)
	payload["dpd"] = loan.DPD
	payload["stage"] = string(loan.Stage)
	p.publish(ctx, commonEvent.LoanDPDUpdated, loan.TenantID, loan.ID.String(), payload)
}

// PublishLoanClosed publishes a loan.closed event.
func (p *ManagementPublisher) PublishLoanClosed(ctx context.Context, loan *model.Loan) {
	payload := basePayload(loan)
	if loan.ClosedAt != nil {
		payload["closedAt"] = loan.ClosedAt.Format(time.RFC3339)
	}
	p.publish(ctx, commonEvent.LoanClosed, loan.TenantID, loan.ID.String(), payload)
}

// PublishRepaymentCompleted publishes a payment.completed event.
func (p *ManagementPublisher) PublishRepaymentCompleted(ctx context.Context, loan *model.Loan, repayment *model.LoanRepayment) {
	payload := basePayload(loan)
	payload["eventType"] = "payment.completed"
	payload["paymentId"] = repayment.ID.String()
	payload["amount"] = repayment.Amount.String()
	payload["currency"] = repayment.Currency
	payload["principalApplied"] = repayment.PrincipalApplied.String()
	payload["interestApplied"] = repayment.InterestApplied.String()
	payload["feeApplied"] = repayment.FeeApplied.String()
	payload["penaltyApplied"] = repayment.PenaltyApplied.String()
	payload["paymentReference"] = repayment.PaymentReference.String
	payload["paymentMethod"] = repayment.PaymentMethod.String
	payload["paymentType"] = "LOAN_REPAYMENT"
	payload["loanId"] = loan.ID.String()

	outstandingBalance := loan.OutstandingPrincipal.
		Add(loan.OutstandingInterest).
		Add(loan.OutstandingFees).
		Add(loan.OutstandingPenalty)
	payload["outstandingBalance"] = outstandingBalance.String()

	p.publish(ctx, commonEvent.PaymentCompleted, loan.TenantID, loan.ID.String(), payload)
}

// PublishPenaltyAccrued publishes a loan.penalty.accrued event for one loan's
// daily penalty accrual. The payload is a fixed contract consumed by
// accounting — exactly {loanId, customerId, amount, currency, accrualDate,
// tenantId}, with amount as a decimal string and accrualDate as YYYY-MM-DD.
// Do not add fields via basePayload.
func (p *ManagementPublisher) PublishPenaltyAccrued(ctx context.Context, loan *model.Loan, amount decimal.Decimal, accrualDate string) {
	payload := map[string]any{
		"loanId":      loan.ID.String(),
		"customerId":  loan.CustomerID,
		"amount":      amount.String(),
		"currency":    loan.Currency,
		"accrualDate": accrualDate,
		"tenantId":    loan.TenantID,
	}
	p.publish(ctx, commonEvent.LoanPenaltyAccrued, loan.TenantID, loan.ID.String(), payload)
}

// PublishLoanWrittenOff publishes a loan.written.off event after the loan's
// WRITTEN_OFF transition. The payload is a fixed contract consumed by
// accounting (posts the write-off against the ECL allowance) and collections
// (closes the case) — amounts are the loan's outstanding buckets at write-off
// time, as decimal strings. Do not add fields via basePayload.
func (p *ManagementPublisher) PublishLoanWrittenOff(ctx context.Context, loan *model.Loan, caseID string) {
	total := loan.OutstandingPrincipal.
		Add(loan.OutstandingInterest).
		Add(loan.OutstandingFees).
		Add(loan.OutstandingPenalty)
	payload := map[string]any{
		"loanId":              loan.ID.String(),
		"customerId":          loan.CustomerID,
		"principalWrittenOff": loan.OutstandingPrincipal.String(),
		"interestWrittenOff":  loan.OutstandingInterest.String(),
		"feesWrittenOff":      loan.OutstandingFees.String(),
		"penaltyWrittenOff":   loan.OutstandingPenalty.String(),
		"totalWrittenOff":     total.String(),
		"currency":            loan.Currency,
		"caseId":              caseID,
		"tenantId":            loan.TenantID,
	}
	p.publish(ctx, commonEvent.LoanWrittenOff, loan.TenantID, loan.ID.String(), payload)
}

func (p *ManagementPublisher) publish(ctx context.Context, eventType, tenantID, correlationID string, payload map[string]any) {
	if p.pub == nil {
		// No broker wired (unit tests): events are best-effort, drop silently.
		p.logger.Debug("Event publisher not configured; dropping event",
			zap.String("type", eventType))
		return
	}

	evt, err := commonEvent.NewDomainEvent(eventType, ServiceName, tenantID, correlationID, payload)
	if err != nil {
		p.logger.Error("Failed to create domain event",
			zap.String("type", eventType),
			zap.Error(err),
		)
		return
	}

	if err := p.pub.Publish(ctx, evt); err != nil {
		p.logger.Error("Failed to publish event",
			zap.String("type", eventType),
			zap.Error(err),
		)
		return
	}

	p.logger.Info("Published event",
		zap.String("type", eventType),
		zap.String("loanId", correlationID),
	)
}

// Ensure decimal is used (compile-time check for import)
var _ = decimal.Zero
