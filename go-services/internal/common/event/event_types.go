package event

// Canonical event type constants for all LMS domain events.
// These strings are used as RabbitMQ routing keys.
// Must match Java EventTypes.java exactly.
const (
	// Account events
	AccountCreated        = "account.created"
	AccountCreditReceived = "account.credit.received"
	AccountDebitProcessed = "account.debit.processed"
	AccountFrozen         = "account.frozen"
	AccountUnfrozen       = "account.unfrozen"
	AccountClosed         = "account.closed"

	// Loan origination events
	LoanApplicationSubmitted = "loan.application.submitted"
	LoanApplicationApproved  = "loan.application.approved"
	LoanApplicationRejected  = "loan.application.rejected"
	LoanDocumentsVerified    = "loan.documents.verified"
	LoanCreditAssessed       = "loan.credit.assessed"

	// Loan management events
	LoanDisbursed         = "loan.disbursed"
	LoanRepaymentReceived = "loan.repayment.received"
	LoanDPDUpdated        = "loan.dpd.updated"
	LoanStageChanged      = "loan.stage.changed"
	LoanClosed            = "loan.closed"
	LoanWrittenOff        = "loan.written.off"
	LoanModified          = "loan.modified"
	LoanPenaltyAccrued    = "loan.penalty.accrued"
	LoanFeeCharged        = "loan.fee.charged"

	// Collections events
	WriteOffApproved = "collection.writeoff.approved"

	// Payment events
	PaymentInitiated = "payment.initiated"
	PaymentCompleted = "payment.completed"
	PaymentFailed    = "payment.failed"
	PaymentReversed  = "payment.reversed"

	// Float events
	FloatDrawn              = "float.drawn"
	FloatRepaid             = "float.repaid"
	FloatFeeCharged         = "float.fee.charged"
	FloatRestrictionApplied = "float.restriction.applied"
	FloatLimitChanged       = "float.limit.changed"

	// AML / compliance events
	AMLAlertRaised    = "aml.alert.raised"
	AMLSARFiled       = "aml.sar.filed"
	CustomerKYCPassed = "customer.kyc.passed"
	CustomerKYCFailed = "customer.kyc.failed"

	// Customer events
	CustomerCreated = "customer.created"
	CustomerUpdated = "customer.updated"

	// Fund transfer events
	TransferInitiated = "transfer.initiated"
	TransferCompleted = "transfer.completed"
	TransferFailed    = "transfer.failed"

	// Mobile wallet events
	MobileUserRegistered    = "mobile.user.registered"
	MobileTransferCompleted = "mobile.transfer.completed"
	MobileTransferFailed    = "mobile.transfer.failed"

	// Bill pay events
	BillPaymentCompleted = "bill.payment.completed"
	BillPaymentFailed    = "bill.payment.failed"

	// Savings events
	SavingsGoalCreated      = "savings.goal.created"
	SavingsDeposit          = "savings.deposit"
	SavingsWithdrawal       = "savings.withdrawal"
	SavingsAutoSaveExecuted = "savings.auto.save.executed"

	// Shop / BNPL events
	ShopOrderPlaced    = "shop.order.placed"
	ShopOrderShipped   = "shop.order.shipped"
	ShopOrderDelivered = "shop.order.delivered"
	ShopBNPLApproved   = "shop.bnpl.approved"

	// Fraud detection events
	FraudAlertRaised  = "fraud.alert.raised"
	FraudBlockAccount = "fraud.block.account"

	// Tenant lifecycle events (Nemo C1 tenant provisioning)
	TenantProvisioned = "tenant.provisioned"
	TenantActivated   = "tenant.activated"
	TenantSuspended   = "tenant.suspended"

	// Decision spine events (Nemo E1). Every automated (and recorded human)
	// decision is emitted as decision.recorded through the producer's
	// transactional outbox and projected into decision_log by
	// decision-service. Payload contract: internal/common/decision.Record.
	DecisionRecorded = "decision.recorded"

	// Overdraft events
	OverdraftApplied          = "overdraft.applied"
	OverdraftDrawn            = "overdraft.drawn"
	OverdraftRepaid           = "overdraft.repaid"
	OverdraftSuspended        = "overdraft.suspended"
	OverdraftInterestCharged  = "overdraft.interest.charged"
	OverdraftFeeCharged       = "overdraft.fee.charged"
	OverdraftDPDUpdated       = "overdraft.dpd.updated"
	OverdraftStageChanged     = "overdraft.stage.changed"
	OverdraftBillingStatement = "overdraft.billing.statement"
)
