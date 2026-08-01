package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/account/model"
	"github.com/athena-lms/go-services/internal/account/service"
	commonEvent "github.com/athena-lms/go-services/internal/common/event"
)

// fakeCustomerSvc records CreateCustomer calls so tests can prove which events
// do (and do not) reach the service layer.
type fakeCustomerSvc struct {
	calls     []customerCall
	createErr error
}

type customerCall struct {
	req      service.CreateCustomerRequest
	tenantID string
}

func (f *fakeCustomerSvc) CreateCustomer(_ context.Context, req service.CreateCustomerRequest, tenantID string) (*model.Customer, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.calls = append(f.calls, customerCall{req: req, tenantID: tenantID})
	return &model.Customer{CustomerID: req.CustomerID, TenantID: tenantID}, nil
}

type fakeAccountSvc struct {
	calls     []accountCall
	createErr error
}

type accountCall struct {
	req      service.CreateAccountRequest
	tenantID string
}

func (f *fakeAccountSvc) CreateAccount(_ context.Context, req service.CreateAccountRequest, tenantID string) (*model.Account, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.calls = append(f.calls, accountCall{req: req, tenantID: tenantID})
	return &model.Account{CustomerID: req.CustomerID, TenantID: tenantID, AccountNumber: "1000000001"}, nil
}

// fakeRepo answers the exists-checks the handler performs before creating.
// existsSequence, when set, is consumed one result per call — used to simulate
// a concurrent creator racing the handler between check and create.
type fakeRepo struct {
	customerExists bool
	existsSequence []bool
	accounts       []*model.Account
	existsErr      error
	accountsErr    error
}

func (f *fakeRepo) CustomerExistsByCustomerIDAndTenant(_ context.Context, _, _ string) (bool, error) {
	if len(f.existsSequence) > 0 {
		next := f.existsSequence[0]
		f.existsSequence = f.existsSequence[1:]
		return next, f.existsErr
	}
	return f.customerExists, f.existsErr
}

func (f *fakeRepo) GetAccountsByCustomer(_ context.Context, _, _ string) ([]*model.Account, error) {
	return f.accounts, f.accountsErr
}

func newTestConsumer(custSvc *fakeCustomerSvc, acctSvc *fakeAccountSvc, repo *fakeRepo) *MobileUserConsumer {
	return &MobileUserConsumer{
		customerSvc: custSvc,
		accountSvc:  acctSvc,
		repo:        repo,
		logger:      zap.NewNop(),
	}
}

func registeredEvent(t *testing.T, tenantID string, payload map[string]any) *commonEvent.DomainEvent {
	t.Helper()
	evt, err := commonEvent.NewDomainEvent(commonEvent.MobileUserRegistered, "bff-gateway", tenantID, "", payload)
	require.NoError(t, err)
	return evt
}

func TestHandleCreatesCustomerAndWalletAccount(t *testing.T) {
	custSvc := &fakeCustomerSvc{}
	acctSvc := &fakeAccountSvc{}
	repo := &fakeRepo{}
	c := newTestConsumer(custSvc, acctSvc, repo)

	evt := registeredEvent(t, "default", map[string]any{
		"userId":      "9f0e7f6a-0000-0000-0000-000000000001",
		"phoneNumber": "+254712345678",
		"customerId":  "CUST-001",
	})

	require.NoError(t, c.handle(context.Background(), evt))

	require.Len(t, custSvc.calls, 1)
	cc := custSvc.calls[0]
	assert.Equal(t, "CUST-001", cc.req.CustomerID)
	assert.Equal(t, "Mobile", cc.req.FirstName)
	assert.Equal(t, "User", cc.req.LastName)
	require.NotNil(t, cc.req.Phone)
	assert.Equal(t, "+254712345678", *cc.req.Phone)
	require.NotNil(t, cc.req.Source)
	assert.Equal(t, "MOBILE", *cc.req.Source)
	assert.Equal(t, "default", cc.tenantID)

	require.Len(t, acctSvc.calls, 1)
	ac := acctSvc.calls[0]
	assert.Equal(t, "CUST-001", ac.req.CustomerID)
	assert.Equal(t, "WALLET", ac.req.AccountType)
	assert.Empty(t, ac.req.Currency, "currency left blank so the market pack default applies")
	assert.Equal(t, 0, ac.req.KycTier)
	assert.Equal(t, "Mobile Wallet - +254712345678", ac.req.AccountName)
	assert.Equal(t, "default", ac.tenantID)
}

func TestHandleUsesPayloadNamesWhenPresent(t *testing.T) {
	custSvc := &fakeCustomerSvc{}
	acctSvc := &fakeAccountSvc{}
	c := newTestConsumer(custSvc, acctSvc, &fakeRepo{})

	evt := registeredEvent(t, "default", map[string]any{
		"customerId":  "CUST-002",
		"phoneNumber": "+254700000001",
		"firstName":   "Wanjiku",
		"lastName":    "Kamau",
	})

	require.NoError(t, c.handle(context.Background(), evt))
	require.Len(t, custSvc.calls, 1)
	assert.Equal(t, "Wanjiku", custSvc.calls[0].req.FirstName)
	assert.Equal(t, "Kamau", custSvc.calls[0].req.LastName)
}

func TestHandleSkipsExistingCustomerStillCreatesAccount(t *testing.T) {
	custSvc := &fakeCustomerSvc{}
	acctSvc := &fakeAccountSvc{}
	repo := &fakeRepo{customerExists: true}
	c := newTestConsumer(custSvc, acctSvc, repo)

	evt := registeredEvent(t, "default", map[string]any{
		"customerId":  "CUST-003",
		"phoneNumber": "+254700000002",
	})

	require.NoError(t, c.handle(context.Background(), evt))
	assert.Empty(t, custSvc.calls, "existing customer must not be recreated")
	require.Len(t, acctSvc.calls, 1)
}

func TestHandleRedeliveryDoesNotDuplicateAccount(t *testing.T) {
	custSvc := &fakeCustomerSvc{}
	acctSvc := &fakeAccountSvc{}
	repo := &fakeRepo{
		customerExists: true,
		accounts:       []*model.Account{{CustomerID: "CUST-004"}},
	}
	c := newTestConsumer(custSvc, acctSvc, repo)

	evt := registeredEvent(t, "default", map[string]any{
		"customerId":  "CUST-004",
		"phoneNumber": "+254700000003",
	})

	require.NoError(t, c.handle(context.Background(), evt))
	assert.Empty(t, custSvc.calls)
	assert.Empty(t, acctSvc.calls, "existing account must not be duplicated on redelivery")
}

func TestHandleIgnoresOtherMobileEvents(t *testing.T) {
	custSvc := &fakeCustomerSvc{}
	acctSvc := &fakeAccountSvc{}
	c := newTestConsumer(custSvc, acctSvc, &fakeRepo{})

	evt, err := commonEvent.NewDomainEvent(commonEvent.MobileTransferCompleted, "bff-gateway", "default", "",
		map[string]any{"transferId": "T-1"})
	require.NoError(t, err)

	require.NoError(t, c.handle(context.Background(), evt))
	assert.Empty(t, custSvc.calls)
	assert.Empty(t, acctSvc.calls)
}

func TestHandleAcksMalformedPayload(t *testing.T) {
	custSvc := &fakeCustomerSvc{}
	acctSvc := &fakeAccountSvc{}
	c := newTestConsumer(custSvc, acctSvc, &fakeRepo{})

	evt := &commonEvent.DomainEvent{
		ID:       "evt-1",
		Type:     commonEvent.MobileUserRegistered,
		TenantID: "default",
		Payload:  json.RawMessage(`"not an object"`),
	}

	require.NoError(t, c.handle(context.Background(), evt), "malformed payloads must be acked, not requeued")
	assert.Empty(t, custSvc.calls)
	assert.Empty(t, acctSvc.calls)
}

func TestHandleAcksMissingCustomerID(t *testing.T) {
	custSvc := &fakeCustomerSvc{}
	acctSvc := &fakeAccountSvc{}
	c := newTestConsumer(custSvc, acctSvc, &fakeRepo{})

	evt := registeredEvent(t, "default", map[string]any{"phoneNumber": "+254700000004"})

	require.NoError(t, c.handle(context.Background(), evt))
	assert.Empty(t, custSvc.calls)
	assert.Empty(t, acctSvc.calls)
}

func TestHandleAcksMissingTenant(t *testing.T) {
	custSvc := &fakeCustomerSvc{}
	acctSvc := &fakeAccountSvc{}
	c := newTestConsumer(custSvc, acctSvc, &fakeRepo{})

	evt := registeredEvent(t, "", map[string]any{"customerId": "CUST-005"})

	require.NoError(t, c.handle(context.Background(), evt))
	assert.Empty(t, custSvc.calls)
	assert.Empty(t, acctSvc.calls)
}

func TestHandleFallsBackToPayloadTenant(t *testing.T) {
	custSvc := &fakeCustomerSvc{}
	acctSvc := &fakeAccountSvc{}
	c := newTestConsumer(custSvc, acctSvc, &fakeRepo{})

	evt := registeredEvent(t, "", map[string]any{
		"customerId": "CUST-006",
		"tenantId":   "acme",
	})

	require.NoError(t, c.handle(context.Background(), evt))
	require.Len(t, custSvc.calls, 1)
	assert.Equal(t, "acme", custSvc.calls[0].tenantID)
	require.Len(t, acctSvc.calls, 1)
	assert.Equal(t, "acme", acctSvc.calls[0].tenantID)
}

func TestHandleRequeuesOnCustomerCreateFailure(t *testing.T) {
	custSvc := &fakeCustomerSvc{createErr: fmt.Errorf("db down")}
	acctSvc := &fakeAccountSvc{}
	c := newTestConsumer(custSvc, acctSvc, &fakeRepo{})

	evt := registeredEvent(t, "default", map[string]any{"customerId": "CUST-007"})

	require.Error(t, c.handle(context.Background(), evt), "transient failures must be requeued")
	assert.Empty(t, acctSvc.calls, "account must not be created when customer creation failed")
}

func TestHandleTreatsRacedCustomerCreateAsSuccess(t *testing.T) {
	// CreateCustomer fails (unique violation from a concurrent creator), but
	// the follow-up exists-check confirms the customer is in place — the
	// handler must proceed to account provisioning instead of requeueing.
	// Simulate the race: not there on the first check, there on the re-check.
	repo := &fakeRepo{existsSequence: []bool{false, true}}
	custSvc := &fakeCustomerSvc{createErr: fmt.Errorf("duplicate key value violates unique constraint")}
	acctSvc := &fakeAccountSvc{}
	c := newTestConsumer(custSvc, acctSvc, repo)

	evt := registeredEvent(t, "default", map[string]any{"customerId": "CUST-008"})

	require.NoError(t, c.handle(context.Background(), evt))
	require.Len(t, acctSvc.calls, 1)
}

func TestHandleRequeuesOnAccountCreateFailure(t *testing.T) {
	custSvc := &fakeCustomerSvc{}
	acctSvc := &fakeAccountSvc{createErr: fmt.Errorf("db down")}
	c := newTestConsumer(custSvc, acctSvc, &fakeRepo{})

	evt := registeredEvent(t, "default", map[string]any{"customerId": "CUST-009"})

	require.Error(t, c.handle(context.Background(), evt))
}
