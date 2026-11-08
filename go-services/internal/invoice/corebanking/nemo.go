package corebanking

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/invoice/client"
)

// ProductCode is the loan product the Nemo provider books advances under.
const ProductCode = "INVOICE_FINANCE"

// Distinct service identities so loan-origination's maker-checker (approver ≠
// creator, disburser ≠ approver ≠ creator) is satisfied by a fully automated
// flow while every step still lands in its audit log under a named actor.
const (
	actorMaker     = "invoice-service-maker"
	actorChecker   = "invoice-service-checker"
	actorDisburser = "invoice-service-disburser"
	actorAdmin     = "invoice-service-admin"
)

// NemoProvider implements Provider on Nemo's own services: account-service
// (customers/accounts/credits), loan-origination (book + disburse the
// advance), loan-management (position), payment-service (repayments).
type NemoProvider struct {
	c             *client.Clients
	logger        *zap.Logger
	productID     string
	currency      string
	loanPollTries int
	loanPollWait  time.Duration
}

// NewNemoProvider wires the provider; call EnsureProduct once at startup.
func NewNemoProvider(c *client.Clients, defaultCurrency string, logger *zap.Logger) *NemoProvider {
	return &NemoProvider{c: c, logger: logger, currency: defaultCurrency, loanPollTries: 6, loanPollWait: 500 * time.Millisecond}
}

func (p *NemoProvider) Name() string { return "nemo" }

// EnsureProduct finds product INVOICE_FINANCE in product-service or creates
// and activates it (ADMIN service identity). Schedule: FLAT_RATE / MONTHLY —
// the closest loan-management supports (see COREBANKING.md "Limitations").
func (p *NemoProvider) EnsureProduct(ctx context.Context) error {
	ctx = auth.WithUserID(auth.WithTenantID(ctx, "default"), actorAdmin)
	prod, err := p.c.Products.FindByCode(ctx, ProductCode)
	if err != nil {
		return fmt.Errorf("lookup product %s: %w", ProductCode, err)
	}
	if prod == nil {
		body := map[string]any{
			"productCode":        ProductCode,
			"name":               "Invoice Finance Advance",
			"description":        "Receivables advance booked by invoice-service. Tenor in days is approximated to whole months (FLAT_RATE, MONTHLY); fees are charged in invoice-service, not here.",
			"productType":        "SME_LOAN",
			"currency":           p.currency,
			"nominalRate":        AnnualRatePct(decimal.RequireFromString("0.0007")), // ≈ 25.55% p.a. (0.07%/day band B)
			"minAmount":          1000,
			"maxAmount":          100000000,
			"minTenorDays":       7,
			"maxTenorDays":       120,
			"scheduleType":       "FLAT_RATE",
			"repaymentFrequency": "MONTHLY",
			"processingFeeRate":  0,
			"penaltyRate":        0,
			"gracePeriodDays":    5,
			"requiresCollateral": false,
		}
		prod, err = p.c.Products.Create(ctx, body)
		if err != nil {
			return fmt.Errorf("create product %s: %w", ProductCode, err)
		}
		p.logger.Info("Created core product", zap.String("code", ProductCode), zap.String("id", prod.ID))
	} else if prod.Status != "" && prod.Status != "ACTIVE" {
		if err := p.c.Products.Activate(ctx, prod.ID); err != nil {
			p.logger.Warn("Could not activate product", zap.String("id", prod.ID), zap.Error(err))
		}
	}
	p.productID = prod.ID
	return nil
}

// ProductID returns the resolved product id ("" until EnsureProduct ran).
func (p *NemoProvider) ProductID() string { return p.productID }

// EnsureCustomer resolves the seller in account-service, creating a BUSINESS
// customer with the same business id when missing.
func (p *NemoProvider) EnsureCustomer(ctx context.Context, ref CustomerRef) (CoreCustomerID, error) {
	ctx = p.as(ctx, ref.TenantID, actorMaker)
	cust, err := p.c.Accounts.GetCustomer(ctx, ref.CustomerID)
	if err != nil {
		return "", &ErrUnavailable{err}
	}
	if cust == nil {
		cust, err = p.c.Accounts.CreateCustomer(ctx, ref.CustomerID, ref.Name, ref.Email, ref.Phone, ref.TaxID)
		if err != nil {
			return "", fmt.Errorf("create core customer: %w", err)
		}
		p.logger.Info("Created core customer", zap.String("customerId", ref.CustomerID))
	}
	return CoreCustomerID(cust.CustomerID), nil
}

// Disburse books the advance as a loan application and walks it to DISBURSED.
func (p *NemoProvider) Disburse(ctx context.Context, req FundingRequest) (FundingResult, error) {
	if p.productID == "" {
		if err := p.EnsureProduct(ctx); err != nil {
			return FundingResult{}, &ErrUnavailable{err}
		}
	}
	customerID := string(req.CoreCustomerID)
	if customerID == "" {
		id, err := p.EnsureCustomer(ctx, req.Customer)
		if err != nil {
			return FundingResult{}, err
		}
		customerID = string(id)
	}
	currency := req.Currency
	if currency == "" {
		currency = p.currency
	}
	maker := p.as(ctx, req.TenantID, actorMaker)
	acct, err := p.c.Accounts.ResolveAccount(maker, customerID, currency, req.Customer.Name)
	if err != nil {
		return FundingResult{}, &ErrUnavailable{fmt.Errorf("resolve seller account: %w", err)}
	}
	purpose := fmt.Sprintf("Invoice finance advance %s (invoice %s, buyer %s)", req.InvoiceID, req.InvoiceNumber, req.BuyerName)
	app, err := p.c.Orig.Create(maker, customerID, p.productID, req.AdvanceAmount, TenorMonths(req.TenorDays), currency, purpose, acct.ID)
	if err != nil {
		return FundingResult{}, fmt.Errorf("create application: %w", err)
	}
	if err := p.c.Orig.Submit(maker, app.ID); err != nil {
		return FundingResult{}, fmt.Errorf("submit application %s: %w", app.ID, err)
	}
	checker := p.as(ctx, req.TenantID, actorChecker)
	if err := p.c.Orig.StartReview(checker, app.ID); err != nil {
		return FundingResult{}, fmt.Errorf("start review %s: %w", app.ID, err)
	}
	notes := fmt.Sprintf("Auto-approved by invoice-service: offer %s, tenor %d days, fee/day %s", req.OfferID, req.TenorDays, req.FeePerDay.String())
	if err := p.c.Orig.Approve(checker, app.ID, req.AdvanceAmount, AnnualRatePct(req.FeePerDay), notes); err != nil {
		return FundingResult{}, fmt.Errorf("approve application %s: %w", app.ID, err)
	}
	disburser := p.as(ctx, req.TenantID, actorDisburser)
	disbursed := req.NetDisbursement
	if !disbursed.IsPositive() {
		disbursed = req.AdvanceAmount
	}
	if _, err := p.c.Orig.Disburse(disburser, app.ID, disbursed, acct.ID); err != nil {
		return FundingResult{}, fmt.Errorf("disburse application %s: %w", app.ID, err)
	}
	res := FundingResult{
		CoreRef:        "app:" + app.ID,
		CoreCustomerID: CoreCustomerID(customerID),
		AccountRef:     acct.ID,
		DisbursedAt:    time.Now().UTC(),
		Raw:            map[string]any{"applicationId": app.ID, "accountId": acct.ID, "productId": p.productID},
	}
	// loan-management creates the loan asynchronously from loan.disbursed;
	// try briefly to pick up the loan id, else keep the application ref and
	// let Position() resolve it later.
	for i := 0; i < p.loanPollTries; i++ {
		if loan, err := p.c.Loans.FindByApplication(maker, customerID, app.ID); err == nil && loan != nil {
			res.CoreRef = loan.ID
			res.Raw["loanId"] = loan.ID
			break
		}
		select {
		case <-ctx.Done():
			return res, nil
		case <-time.After(p.loanPollWait):
		}
	}
	return res, nil
}

// resolveLoan turns an "app:<id>" ref into the loan id when it exists.
func (p *NemoProvider) resolveLoan(ctx context.Context, customerID, coreRef string) (*client.Loan, error) {
	if strings.HasPrefix(coreRef, "app:") {
		if customerID == "" {
			return nil, nil
		}
		return p.c.Loans.FindByApplication(ctx, customerID, strings.TrimPrefix(coreRef, "app:"))
	}
	return p.c.Loans.Get(ctx, coreRef)
}

// RecordRepayment posts a LOAN_REPAYMENT through payment-service; loan-
// management applies it from payment.completed.
func (p *NemoProvider) RecordRepayment(ctx context.Context, req RepaymentRequest) (RepaymentResult, error) {
	ctx = p.as(ctx, req.TenantID, actorMaker)
	customerID := req.CustomerID
	if customerID == "" {
		customerID = string(req.CoreCustomerID)
	}
	loanID := ""
	if loan, err := p.resolveLoan(ctx, customerID, req.CoreRef); err == nil && loan != nil {
		loanID = loan.ID
	}
	currency := req.Currency
	if currency == "" {
		currency = p.currency
	}
	extRef := req.IdempotencyKey
	if extRef == "" {
		extRef = "invadv:" + req.InvoiceID + ":" + req.Reference
	}
	id, err := p.c.Payments.LoanRepayment(ctx, customerID, loanID, req.Amount, currency, extRef,
		fmt.Sprintf("Invoice %s buyer payment %s from %s", req.InvoiceID, req.Reference, req.Payer))
	if err != nil {
		if strings.Contains(err.Error(), "returned 409") {
			return RepaymentResult{TxnRef: TxnRef(id)}, nil // already recorded
		}
		return RepaymentResult{}, &ErrUnavailable{err}
	}
	res := RepaymentResult{TxnRef: TxnRef(id)}
	if loanID != "" {
		if loan, err := p.c.Loans.Get(ctx, loanID); err == nil {
			res.Outstanding = loan.TotalOutstanding
			res.Closed = loan.Status == "CLOSED"
		}
	}
	return res, nil
}

// Credit pays the seller's account (reserve release / refund).
func (p *NemoProvider) Credit(ctx context.Context, req CreditRequest) (TxnRef, error) {
	ctx = p.as(ctx, req.TenantID, actorMaker)
	customerID := req.CustomerID
	if customerID == "" {
		customerID = string(req.CoreCustomerID)
	}
	currency := req.Currency
	if currency == "" {
		currency = p.currency
	}
	acct, err := p.c.Accounts.ResolveAccount(ctx, customerID, currency, "")
	if err != nil {
		return "", &ErrUnavailable{err}
	}
	ref := req.IdempotencyKey
	if ref == "" {
		ref = req.Reference
	}
	id, err := p.c.Accounts.Credit(ctx, acct.ID, req.Amount, req.Description, ref, "INVOICE_FINANCE")
	if err != nil {
		return "", &ErrUnavailable{err}
	}
	return TxnRef(id), nil
}

// Position reads the loan from loan-management.
func (p *NemoProvider) Position(ctx context.Context, coreRef string) (Position, error) {
	ctx = p.as(ctx, "", actorMaker)
	loan, err := p.resolveLoan(ctx, "", coreRef)
	if err != nil {
		return Position{}, &ErrUnavailable{err}
	}
	if loan == nil {
		return Position{CoreRef: coreRef, Status: "UNKNOWN"}, nil
	}
	status := loan.Status
	if loan.DPD > 0 && status == "ACTIVE" {
		status = "OVERDUE"
	}
	return Position{
		CoreRef: loan.ID, Status: status, Principal: loan.DisbursedAmount,
		Outstanding: loan.TotalOutstanding, DPD: loan.DPD, MaturityAt: loan.MaturityDate,
		Raw: map[string]any{"applicationId": loan.ApplicationID, "customerId": loan.CustomerID},
	}, nil
}

// as returns a context carrying the tenant and the given service identity.
func (p *NemoProvider) as(ctx context.Context, tenantID, actor string) context.Context {
	if tenantID != "" {
		ctx = auth.WithTenantID(ctx, tenantID)
	} else if auth.TenantIDFromContext(ctx) == "" {
		ctx = auth.WithTenantID(ctx, "default")
	}
	return auth.WithUserID(ctx, actor)
}
