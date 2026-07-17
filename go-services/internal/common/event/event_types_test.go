package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEventTypeValues verifies constants match Java EventTypes.java exactly.
// These strings are RabbitMQ routing keys — any mismatch breaks event routing.
func TestEventTypeValues(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		// Account
		{"AccountCreated", AccountCreated, "account.created"},
		{"AccountCreditReceived", AccountCreditReceived, "account.credit.received"},
		{"AccountDebitProcessed", AccountDebitProcessed, "account.debit.processed"},
		{"AccountFrozen", AccountFrozen, "account.frozen"},
		{"AccountUnfrozen", AccountUnfrozen, "account.unfrozen"},
		{"AccountClosed", AccountClosed, "account.closed"},

		// Loan origination
		{"LoanApplicationSubmitted", LoanApplicationSubmitted, "loan.application.submitted"},
		{"LoanApplicationApproved", LoanApplicationApproved, "loan.application.approved"},
		{"LoanApplicationRejected", LoanApplicationRejected, "loan.application.rejected"},
		{"LoanDocumentsVerified", LoanDocumentsVerified, "loan.documents.verified"},
		{"LoanCreditAssessed", LoanCreditAssessed, "loan.credit.assessed"},

		// Loan management
		{"LoanDisbursed", LoanDisbursed, "loan.disbursed"},
		{"LoanRepaymentReceived", LoanRepaymentReceived, "loan.repayment.received"},
		{"LoanDPDUpdated", LoanDPDUpdated, "loan.dpd.updated"},
		{"LoanStageChanged", LoanStageChanged, "loan.stage.changed"},
		{"LoanClosed", LoanClosed, "loan.closed"},
		{"LoanWrittenOff", LoanWrittenOff, "loan.written.off"},
		{"LoanModified", LoanModified, "loan.modified"},

		// Payment
		{"PaymentInitiated", PaymentInitiated, "payment.initiated"},
		{"PaymentCompleted", PaymentCompleted, "payment.completed"},
		{"PaymentFailed", PaymentFailed, "payment.failed"},
		{"PaymentReversed", PaymentReversed, "payment.reversed"},

		// Float
		{"FloatDrawn", FloatDrawn, "float.drawn"},
		{"FloatRepaid", FloatRepaid, "float.repaid"},
		{"FloatFeeCharged", FloatFeeCharged, "float.fee.charged"},
		{"FloatRestrictionApplied", FloatRestrictionApplied, "float.restriction.applied"},
		{"FloatLimitChanged", FloatLimitChanged, "float.limit.changed"},

		// Card issuing (Nemo B1)
		{"CardIssued", CardIssued, "card.issued"},
		{"CardActivated", CardActivated, "card.activated"},
		{"CardFrozen", CardFrozen, "card.frozen"},
		{"CardUnfrozen", CardUnfrozen, "card.unfrozen"},
		{"CardBlocked", CardBlocked, "card.blocked"},
		{"CardClosed", CardClosed, "card.closed"},
		{"CardLimitsChanged", CardLimitsChanged, "card.limits.changed"},

		// AML / compliance
		{"AMLAlertRaised", AMLAlertRaised, "aml.alert.raised"},
		{"AMLSARFiled", AMLSARFiled, "aml.sar.filed"},
		{"CustomerKYCPassed", CustomerKYCPassed, "customer.kyc.passed"},
		{"CustomerKYCFailed", CustomerKYCFailed, "customer.kyc.failed"},

		// Customer
		{"CustomerCreated", CustomerCreated, "customer.created"},
		{"CustomerUpdated", CustomerUpdated, "customer.updated"},

		// Transfer
		{"TransferInitiated", TransferInitiated, "transfer.initiated"},
		{"TransferCompleted", TransferCompleted, "transfer.completed"},
		{"TransferFailed", TransferFailed, "transfer.failed"},

		// Mobile
		{"MobileUserRegistered", MobileUserRegistered, "mobile.user.registered"},
		{"MobileTransferCompleted", MobileTransferCompleted, "mobile.transfer.completed"},
		{"MobileTransferFailed", MobileTransferFailed, "mobile.transfer.failed"},

		// Bill
		{"BillPaymentCompleted", BillPaymentCompleted, "bill.payment.completed"},
		{"BillPaymentFailed", BillPaymentFailed, "bill.payment.failed"},

		// Savings
		{"SavingsGoalCreated", SavingsGoalCreated, "savings.goal.created"},
		{"SavingsDeposit", SavingsDeposit, "savings.deposit"},
		{"SavingsWithdrawal", SavingsWithdrawal, "savings.withdrawal"},
		{"SavingsAutoSaveExecuted", SavingsAutoSaveExecuted, "savings.auto.save.executed"},

		// Shop/BNPL
		{"ShopOrderPlaced", ShopOrderPlaced, "shop.order.placed"},
		{"ShopOrderShipped", ShopOrderShipped, "shop.order.shipped"},
		{"ShopOrderDelivered", ShopOrderDelivered, "shop.order.delivered"},
		{"ShopBNPLApproved", ShopBNPLApproved, "shop.bnpl.approved"},

		// Fraud
		{"FraudAlertRaised", FraudAlertRaised, "fraud.alert.raised"},
		{"FraudBlockAccount", FraudBlockAccount, "fraud.block.account"},

		// Overdraft
		{"OverdraftApplied", OverdraftApplied, "overdraft.applied"},
		{"OverdraftDrawn", OverdraftDrawn, "overdraft.drawn"},
		{"OverdraftRepaid", OverdraftRepaid, "overdraft.repaid"},
		{"OverdraftSuspended", OverdraftSuspended, "overdraft.suspended"},
		{"OverdraftInterestCharged", OverdraftInterestCharged, "overdraft.interest.charged"},
		{"OverdraftFeeCharged", OverdraftFeeCharged, "overdraft.fee.charged"},
		{"OverdraftDPDUpdated", OverdraftDPDUpdated, "overdraft.dpd.updated"},
		{"OverdraftStageChanged", OverdraftStageChanged, "overdraft.stage.changed"},
		{"OverdraftBillingStatement", OverdraftBillingStatement, "overdraft.billing.statement"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.got)
		})
	}
}
