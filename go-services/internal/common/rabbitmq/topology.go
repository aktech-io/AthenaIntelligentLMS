package rabbitmq

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// Topology constants matching Java LmsRabbitMQConfig.java exactly.
const (
	// Exchange
	LMSExchange = "athena.lms.exchange"

	// Queues
	AccountingQueue      = "athena.lms.accounting.queue"
	CollectionsQueue     = "athena.lms.collections.queue"
	ComplianceQueue      = "athena.lms.compliance.queue"
	NotificationQueue    = "athena.lms.notification.queue"
	LoanMgmtQueue        = "athena.lms.loan.mgmt.queue"
	ReportingQueue       = "athena.lms.reporting.queue"
	FloatQueue           = "athena.lms.float.queue"
	AccountMobileQueue   = "athena.lms.account.mobile.queue"
	OverdraftMobileQueue = "athena.lms.overdraft.mobile.queue"

	// Routing key patterns
	LoanRoutingPattern     = "loan.#"
	PaymentRoutingPattern  = "payment.#"
	FloatRoutingPattern    = "float.#"
	AccountRoutingPattern  = "account.#"
	DPDRoutingPattern      = "loan.dpd.#"
	StageRoutingPattern    = "loan.stage.#"
	AMLRoutingPattern      = "aml.#"
	KYCRoutingPattern      = "customer.kyc.#"
	WildcardPattern        = "#"
	PaymentCompletedKey    = "payment.completed"
	PaymentReversedKey     = "payment.reversed"
	LoanDisbursedKey       = "loan.disbursed"
	LoanSubmittedKey       = "loan.application.submitted"
	AccountCreditKey       = "account.credit.received"
	TransferRoutingPattern = "transfer.#"
	CustomerRoutingPattern = "customer.#"

	// Mobile wallet routing patterns
	MobileRoutingPattern   = "mobile.#"
	BillRoutingPattern     = "bill.#"
	SavingsRoutingPattern  = "savings.#"
	ShopRoutingPattern     = "shop.#"
	OverdraftRoutingPattern = "overdraft.#"
	FraudRoutingPattern    = "fraud.#"

	// Collections-specific routing keys
	LoanClosedKey            = "loan.closed"
	LoanWrittenOffKey        = "loan.written.off"
	LoanRepaymentReceivedKey = "loan.repayment.received"
	WriteOffApprovedKey      = "collection.writeoff.approved"
)

// Binding represents a queue-to-exchange binding.
type Binding struct {
	Queue      string
	RoutingKey string
}

// AllBindings returns all queue-exchange bindings matching Java config.
var AllBindings = []Binding{
	// Accounting bindings
	{AccountingQueue, LoanRoutingPattern},
	{AccountingQueue, PaymentRoutingPattern},
	{AccountingQueue, FloatRoutingPattern},
	{AccountingQueue, AccountRoutingPattern},
	{AccountingQueue, TransferRoutingPattern},
	{AccountingQueue, OverdraftRoutingPattern},

	// Collections bindings
	{CollectionsQueue, DPDRoutingPattern},
	{CollectionsQueue, StageRoutingPattern},
	{CollectionsQueue, OverdraftRoutingPattern},
	{CollectionsQueue, LoanClosedKey},
	{CollectionsQueue, LoanWrittenOffKey},
	{CollectionsQueue, LoanRepaymentReceivedKey},

	// Compliance bindings
	{ComplianceQueue, AMLRoutingPattern},
	{ComplianceQueue, KYCRoutingPattern},
	{ComplianceQueue, CustomerRoutingPattern},

	// Notification bindings (wildcard — receives everything)
	{NotificationQueue, WildcardPattern},

	// Loan management bindings
	{LoanMgmtQueue, PaymentCompletedKey},
	{LoanMgmtQueue, PaymentReversedKey},
	{LoanMgmtQueue, LoanDisbursedKey},
	{LoanMgmtQueue, WriteOffApprovedKey},

	// Reporting bindings (wildcard — receives everything)
	{ReportingQueue, WildcardPattern},

	// Float bindings
	{FloatQueue, AccountCreditKey},

	// Account mobile bindings
	{AccountMobileQueue, MobileRoutingPattern},

	// Overdraft mobile bindings
	{OverdraftMobileQueue, MobileRoutingPattern},
}

// DeclareTopology declares the exchange, all queues, and all bindings.
// Safe to call multiple times (idempotent).
func DeclareTopology(ch *amqp.Channel, logger *zap.Logger) error {
	// Declare topic exchange (durable, not auto-deleted)
	if err := ch.ExchangeDeclare(
		LMSExchange, "topic", true, false, false, false, nil,
	); err != nil {
		return err
	}
	logger.Info("Declared exchange", zap.String("exchange", LMSExchange))

	// Declare all queues (durable)
	queues := []string{
		AccountingQueue, CollectionsQueue, ComplianceQueue,
		NotificationQueue, LoanMgmtQueue, ReportingQueue,
		FloatQueue, AccountMobileQueue, OverdraftMobileQueue,
	}
	for _, q := range queues {
		if _, err := ch.QueueDeclare(q, true, false, false, false, nil); err != nil {
			return err
		}
	}
	logger.Info("Declared queues", zap.Int("count", len(queues)))

	// Declare all bindings
	for _, b := range AllBindings {
		if err := ch.QueueBind(b.Queue, b.RoutingKey, LMSExchange, false, nil); err != nil {
			return err
		}
	}
	logger.Info("Declared bindings", zap.Int("count", len(AllBindings)))

	return nil
}
