package consumer

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	cerrors "github.com/athena-lms/go-services/internal/common/errors"
	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	mgmtevent "github.com/athena-lms/go-services/internal/management/event"
	"github.com/athena-lms/go-services/internal/management/model"
)

// fakeLoanService is an in-memory stand-in for *service.Service recording
// every ActivateLoan/ApplyRepayment call so tests can prove which events do
// (and do not) reach the service layer.
type fakeLoanService struct {
	activations int
	repayments  []repaymentCall
	applyErr    error
}

type repaymentCall struct {
	loanID   uuid.UUID
	tenantID string
	userID   string
	req      *model.RepaymentRequest
}

func (f *fakeLoanService) ActivateLoan(_ context.Context, _ uuid.UUID, _ string, _ uuid.UUID, _ string,
	_, _ decimal.Decimal, _ int, _, _ string) error {
	f.activations++
	return nil
}

func (f *fakeLoanService) ApplyRepayment(_ context.Context, loanID uuid.UUID, req *model.RepaymentRequest,
	tenantID, userID string) (*model.RepaymentResponse, error) {
	if f.applyErr != nil {
		return nil, f.applyErr
	}
	f.repayments = append(f.repayments, repaymentCall{loanID: loanID, tenantID: tenantID, userID: userID, req: req})
	return &model.RepaymentResponse{ID: uuid.New(), Status: "COMPLETED", Amount: req.Amount}, nil
}

func newTestConsumer(svc LoanService) *LoanDisbursedConsumer {
	return &LoanDisbursedConsumer{svc: svc, logger: zap.NewNop()}
}

func paymentCompletedEvent(t *testing.T, source string, payload map[string]any) *commonEvent.DomainEvent {
	t.Helper()
	evt, err := commonEvent.NewDomainEvent(commonEvent.PaymentCompleted, source, "tenant1", "", payload)
	require.NoError(t, err)
	return evt
}

func TestHandlePaymentCompleted_AppliesRepayment(t *testing.T) {
	fake := &fakeLoanService{}
	c := newTestConsumer(fake)
	loanID := uuid.New()

	evt := paymentCompletedEvent(t, "payment-service", map[string]any{
		"paymentId":         uuid.New().String(),
		"customerId":        "CUST-001",
		"loanId":            loanID.String(),
		"paymentType":       "LOAN_REPAYMENT",
		"paymentChannel":    "MPESA",
		"amount":            "2500.00",
		"currency":          "KES",
		"internalReference": "PAY-INT-42",
	})

	require.NoError(t, c.handle(context.Background(), evt))
	require.Len(t, fake.repayments, 1)
	call := fake.repayments[0]
	assert.Equal(t, loanID, call.loanID)
	assert.Equal(t, "tenant1", call.tenantID, "tenant must come from the envelope when absent from the payload")
	assert.Equal(t, "PAY-INT-42", call.req.PaymentReference, "dedup key must be the payment's internal reference")
	assert.Equal(t, "MPESA", call.req.PaymentMethod)
	assert.Equal(t, "KES", call.req.Currency)
	assert.True(t, call.req.Amount.Equal(decimal.NewFromFloat(2500)))
}

func TestHandlePaymentCompleted_FallsBackToPaymentIDReference(t *testing.T) {
	fake := &fakeLoanService{}
	c := newTestConsumer(fake)
	paymentID := uuid.New().String()

	evt := paymentCompletedEvent(t, "payment-service", map[string]any{
		"paymentId":   paymentID,
		"loanId":      uuid.New().String(),
		"paymentType": "LOAN_REPAYMENT",
		"amount":      "100",
	})

	require.NoError(t, c.handle(context.Background(), evt))
	require.Len(t, fake.repayments, 1)
	assert.Equal(t, paymentID, fake.repayments[0].req.PaymentReference)
}

func TestHandlePaymentCompleted_SkipsNonRepaymentTypes(t *testing.T) {
	for _, paymentType := range []string{"LOAN_DISBURSEMENT", "FLOAT_TRANSFER", "OTHER", ""} {
		t.Run(paymentType, func(t *testing.T) {
			fake := &fakeLoanService{}
			c := newTestConsumer(fake)

			evt := paymentCompletedEvent(t, "payment-service", map[string]any{
				"paymentId":   uuid.New().String(),
				"loanId":      uuid.New().String(),
				"paymentType": paymentType,
				"amount":      "100",
			})

			assert.NoError(t, c.handle(context.Background(), evt), "must ack, not requeue")
			assert.Empty(t, fake.repayments)
		})
	}
}

func TestHandlePaymentCompleted_SkipsMissingOrInvalidLoanID(t *testing.T) {
	for name, payload := range map[string]map[string]any{
		"missing": {
			"paymentId":   uuid.New().String(),
			"paymentType": "LOAN_REPAYMENT",
			"amount":      "100",
		},
		"null": {
			"paymentId":   uuid.New().String(),
			"loanId":      nil,
			"paymentType": "LOAN_REPAYMENT",
			"amount":      "100",
		},
		"invalid": {
			"paymentId":   uuid.New().String(),
			"loanId":      "not-a-uuid",
			"paymentType": "LOAN_REPAYMENT",
			"amount":      "100",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeLoanService{}
			c := newTestConsumer(fake)

			evt := paymentCompletedEvent(t, "payment-service", payload)

			assert.NoError(t, c.handle(context.Background(), evt), "malformed events must be acked, never requeued")
			assert.Empty(t, fake.repayments)
		})
	}
}

func TestHandlePaymentCompleted_SkipsNonPositiveAmount(t *testing.T) {
	for _, amount := range []string{"0", "-50.00"} {
		t.Run(amount, func(t *testing.T) {
			fake := &fakeLoanService{}
			c := newTestConsumer(fake)

			evt := paymentCompletedEvent(t, "payment-service", map[string]any{
				"paymentId":   uuid.New().String(),
				"loanId":      uuid.New().String(),
				"paymentType": "LOAN_REPAYMENT",
				"amount":      amount,
			})

			assert.NoError(t, c.handle(context.Background(), evt), "malformed events must be acked, never requeued")
			assert.Empty(t, fake.repayments)
		})
	}
}

func TestHandlePaymentCompleted_SkipsSelfPublishedEvents(t *testing.T) {
	// The management service publishes with the payment.completed routing key
	// after applying a repayment; consuming those again would loop forever.
	fake := &fakeLoanService{}
	c := newTestConsumer(fake)

	evt := paymentCompletedEvent(t, mgmtevent.ServiceName, map[string]any{
		"paymentId":   uuid.New().String(),
		"loanId":      uuid.New().String(),
		"paymentType": "LOAN_REPAYMENT",
		"amount":      "100",
	})

	assert.NoError(t, c.handle(context.Background(), evt))
	assert.Empty(t, fake.repayments)
}

func TestHandlePaymentCompleted_BusinessRejectionAcked(t *testing.T) {
	for name, applyErr := range map[string]error{
		"business":  cerrors.NewBusinessError("Loan is not in an active state"),
		"not found": cerrors.NotFoundResource("Loan", uuid.New()),
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeLoanService{applyErr: applyErr}
			c := newTestConsumer(fake)

			evt := paymentCompletedEvent(t, "payment-service", map[string]any{
				"paymentId":   uuid.New().String(),
				"loanId":      uuid.New().String(),
				"paymentType": "LOAN_REPAYMENT",
				"amount":      "100",
			})

			assert.NoError(t, c.handle(context.Background(), evt),
				"business rejections are terminal: ack, don't wedge the queue")
		})
	}
}

func TestHandlePaymentCompleted_InfraErrorRequeued(t *testing.T) {
	fake := &fakeLoanService{applyErr: fmt.Errorf("db connection lost")}
	c := newTestConsumer(fake)

	evt := paymentCompletedEvent(t, "payment-service", map[string]any{
		"paymentId":   uuid.New().String(),
		"loanId":      uuid.New().String(),
		"paymentType": "LOAN_REPAYMENT",
		"amount":      "100",
	})

	assert.Error(t, c.handle(context.Background(), evt),
		"infrastructure failures must be returned so the delivery is nacked and retried")
}

func TestHandle_LoanDisbursedStillActivates(t *testing.T) {
	fake := &fakeLoanService{}
	c := newTestConsumer(fake)

	evt, err := commonEvent.NewDomainEvent(commonEvent.LoanDisbursed, "loan-origination-service", "tenant1", "", map[string]any{
		"applicationId": uuid.New().String(),
		"customerId":    "CUST-001",
		"productId":     uuid.New().String(),
		"amount":        "50000",
		"interestRate":  "14.5",
		"tenorMonths":   6,
	})
	require.NoError(t, err)

	require.NoError(t, c.handle(context.Background(), evt))
	assert.Equal(t, 1, fake.activations)
	assert.Empty(t, fake.repayments)
}

func TestHandle_IgnoresUnknownEventTypes(t *testing.T) {
	fake := &fakeLoanService{}
	c := newTestConsumer(fake)

	evt, err := commonEvent.NewDomainEvent(commonEvent.PaymentReversed, "payment-service", "tenant1", "", map[string]any{
		"paymentId": uuid.New().String(),
	})
	require.NoError(t, err)

	assert.NoError(t, c.handle(context.Background(), evt))
	assert.Empty(t, fake.repayments)
	assert.Zero(t, fake.activations)
}
