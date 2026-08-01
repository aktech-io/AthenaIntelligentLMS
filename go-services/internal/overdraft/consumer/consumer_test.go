package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/overdraft/model"
)

type walletCall struct {
	req      model.CreateWalletRequest
	tenantID string
}

type fakeWalletSvc struct {
	calls     []walletCall
	createErr error
}

func (f *fakeWalletSvc) CreateWallet(_ context.Context, req model.CreateWalletRequest, tenantID string) (*model.WalletResponse, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.calls = append(f.calls, walletCall{req: req, tenantID: tenantID})
	return &model.WalletResponse{CustomerID: req.CustomerID}, nil
}

// fakeRepo answers the wallet exists-check. existsSequence, when set, is
// consumed one result per call — used to simulate a concurrent creator racing
// the handler between check and create.
type fakeRepo struct {
	exists         bool
	existsSequence []bool
	existsErr      error
}

func (f *fakeRepo) WalletExistsByTenantAndCustomer(_ context.Context, _, _ string) (bool, error) {
	if len(f.existsSequence) > 0 {
		next := f.existsSequence[0]
		f.existsSequence = f.existsSequence[1:]
		return next, f.existsErr
	}
	return f.exists, f.existsErr
}

func newTestConsumer(svc *fakeWalletSvc, repo *fakeRepo) *MobileUserConsumer {
	return &MobileUserConsumer{walletSvc: svc, repo: repo, logger: zap.NewNop()}
}

func registeredEvent(t *testing.T, tenantID string, payload map[string]any) *commonEvent.DomainEvent {
	t.Helper()
	evt, err := commonEvent.NewDomainEvent(commonEvent.MobileUserRegistered, "bff-gateway", tenantID, "", payload)
	require.NoError(t, err)
	return evt
}

func TestHandleCreatesWallet(t *testing.T) {
	svc := &fakeWalletSvc{}
	c := newTestConsumer(svc, &fakeRepo{})

	evt := registeredEvent(t, "default", map[string]any{
		"userId":      "9f0e7f6a-0000-0000-0000-000000000001",
		"phoneNumber": "+254712345678",
		"customerId":  "CUST-001",
	})

	require.NoError(t, c.handle(context.Background(), evt))
	require.Len(t, svc.calls, 1)
	assert.Equal(t, "CUST-001", svc.calls[0].req.CustomerID)
	assert.Empty(t, svc.calls[0].req.Currency, "currency left blank so the market pack default applies")
	assert.Equal(t, "default", svc.calls[0].tenantID)
}

func TestHandleRedeliverySkipsExistingWallet(t *testing.T) {
	svc := &fakeWalletSvc{}
	c := newTestConsumer(svc, &fakeRepo{exists: true})

	evt := registeredEvent(t, "default", map[string]any{"customerId": "CUST-002"})

	require.NoError(t, c.handle(context.Background(), evt))
	assert.Empty(t, svc.calls, "existing wallet must not be duplicated on redelivery")
}

func TestHandleIgnoresOtherMobileEvents(t *testing.T) {
	svc := &fakeWalletSvc{}
	c := newTestConsumer(svc, &fakeRepo{})

	evt, err := commonEvent.NewDomainEvent(commonEvent.MobileTransferCompleted, "bff-gateway", "default", "",
		map[string]any{"transferId": "T-1"})
	require.NoError(t, err)

	require.NoError(t, c.handle(context.Background(), evt))
	assert.Empty(t, svc.calls)
}

func TestHandleAcksMalformedPayload(t *testing.T) {
	svc := &fakeWalletSvc{}
	c := newTestConsumer(svc, &fakeRepo{})

	evt := &commonEvent.DomainEvent{
		ID:       "evt-1",
		Type:     commonEvent.MobileUserRegistered,
		TenantID: "default",
		Payload:  json.RawMessage(`42`),
	}

	require.NoError(t, c.handle(context.Background(), evt), "malformed payloads must be acked, not requeued")
	assert.Empty(t, svc.calls)
}

func TestHandleAcksMissingCustomerID(t *testing.T) {
	svc := &fakeWalletSvc{}
	c := newTestConsumer(svc, &fakeRepo{})

	evt := registeredEvent(t, "default", map[string]any{"phoneNumber": "+254700000004"})

	require.NoError(t, c.handle(context.Background(), evt))
	assert.Empty(t, svc.calls)
}

func TestHandleFallsBackToPayloadTenant(t *testing.T) {
	svc := &fakeWalletSvc{}
	c := newTestConsumer(svc, &fakeRepo{})

	evt := registeredEvent(t, "", map[string]any{
		"customerId": "CUST-003",
		"tenantId":   "acme",
	})

	require.NoError(t, c.handle(context.Background(), evt))
	require.Len(t, svc.calls, 1)
	assert.Equal(t, "acme", svc.calls[0].tenantID)
}

func TestHandleRequeuesOnCreateFailure(t *testing.T) {
	svc := &fakeWalletSvc{createErr: fmt.Errorf("db down")}
	c := newTestConsumer(svc, &fakeRepo{})

	evt := registeredEvent(t, "default", map[string]any{"customerId": "CUST-004"})

	require.Error(t, c.handle(context.Background(), evt), "transient failures must be requeued")
}

func TestHandleTreatsRacedCreateAsSuccess(t *testing.T) {
	// CreateWallet fails (concurrent creator won the race), but the follow-up
	// exists-check confirms the wallet is in place — ack, don't requeue.
	repo := &fakeRepo{existsSequence: []bool{false, true}}
	svc := &fakeWalletSvc{createErr: fmt.Errorf("wallet already exists for customer")}
	c := newTestConsumer(svc, repo)

	evt := registeredEvent(t, "default", map[string]any{"customerId": "CUST-005"})

	require.NoError(t, c.handle(context.Background(), evt))
}
