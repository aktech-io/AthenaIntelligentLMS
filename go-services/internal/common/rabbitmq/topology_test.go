package rabbitmq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTopologyConstants verifies constants match Java LmsRabbitMQConfig.java exactly.
func TestTopologyConstants(t *testing.T) {
	// Exchange
	assert.Equal(t, "athena.lms.exchange", LMSExchange)

	// Queues
	assert.Equal(t, "athena.lms.accounting.queue", AccountingQueue)
	assert.Equal(t, "athena.lms.collections.queue", CollectionsQueue)
	assert.Equal(t, "athena.lms.compliance.queue", ComplianceQueue)
	assert.Equal(t, "athena.lms.notification.queue", NotificationQueue)
	assert.Equal(t, "athena.lms.loan.mgmt.queue", LoanMgmtQueue)
	assert.Equal(t, "athena.lms.reporting.queue", ReportingQueue)
	assert.Equal(t, "athena.lms.float.queue", FloatQueue)
	assert.Equal(t, "athena.lms.account.mobile.queue", AccountMobileQueue)
	assert.Equal(t, "athena.lms.overdraft.mobile.queue", OverdraftMobileQueue)
	assert.Equal(t, "athena.lms.decision.queue", DecisionQueue)

	// Routing keys
	assert.Equal(t, "loan.#", LoanRoutingPattern)
	assert.Equal(t, "payment.#", PaymentRoutingPattern)
	assert.Equal(t, "float.#", FloatRoutingPattern)
	assert.Equal(t, "account.#", AccountRoutingPattern)
	assert.Equal(t, "loan.dpd.#", DPDRoutingPattern)
	assert.Equal(t, "loan.stage.#", StageRoutingPattern)
	assert.Equal(t, "aml.#", AMLRoutingPattern)
	assert.Equal(t, "customer.kyc.#", KYCRoutingPattern)
	assert.Equal(t, "#", WildcardPattern)
	assert.Equal(t, "payment.completed", PaymentCompletedKey)
	assert.Equal(t, "payment.reversed", PaymentReversedKey)
	assert.Equal(t, "loan.disbursed", LoanDisbursedKey)
	assert.Equal(t, "loan.application.submitted", LoanSubmittedKey)
	assert.Equal(t, "account.credit.received", AccountCreditKey)
	assert.Equal(t, "decision.#", DecisionRoutingPattern)
}

// TestAllBindings pins the canonical Go topology so binding changes are
// deliberate. (Originally a parity check against the archived Java
// LmsRabbitMQConfig.java; the Go topology has intentionally grown beyond it —
// collections loan-lifecycle keys and the loan-mgmt write-off key.)
func TestAllBindings(t *testing.T) {
	expected := map[string][]string{
		AccountingQueue: {
			LoanRoutingPattern, PaymentRoutingPattern, FloatRoutingPattern,
			AccountRoutingPattern, TransferRoutingPattern, OverdraftRoutingPattern,
		},
		CollectionsQueue: {
			DPDRoutingPattern, StageRoutingPattern, OverdraftRoutingPattern,
			LoanClosedKey, LoanWrittenOffKey, LoanRepaymentReceivedKey,
		},
		ComplianceQueue: {
			AMLRoutingPattern, KYCRoutingPattern, CustomerRoutingPattern,
		},
		NotificationQueue: {WildcardPattern},
		LoanMgmtQueue: {
			PaymentCompletedKey, PaymentReversedKey, LoanDisbursedKey,
			WriteOffApprovedKey,
		},
		ReportingQueue:       {WildcardPattern},
		FloatQueue:           {AccountCreditKey},
		AccountMobileQueue:   {MobileRoutingPattern},
		OverdraftMobileQueue: {MobileRoutingPattern},
		// Decision spine (Nemo E1): decision.recorded projection feed.
		DecisionQueue: {DecisionRoutingPattern},
	}

	// Build actual bindings map from AllBindings
	actual := make(map[string][]string)
	for _, b := range AllBindings {
		actual[b.Queue] = append(actual[b.Queue], b.RoutingKey)
	}

	for queue, expectedKeys := range expected {
		t.Run(queue, func(t *testing.T) {
			assert.ElementsMatch(t, expectedKeys, actual[queue],
				"Bindings for queue %s don't match the canonical topology", queue)
		})
	}

	// Verify no extra queues
	assert.Equal(t, len(expected), len(actual), "Number of queues with bindings should match")
}
