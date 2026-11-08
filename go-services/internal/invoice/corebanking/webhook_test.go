package corebanking

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const secret = "bank-shared-secret"

// fakeCore is a minimal bank-side implementation of COREBANKING.md.
func fakeCore(t *testing.T, seen *[]string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*seen = append(*seen, r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NotEmpty(t, r.Header.Get("X-Idempotency-Key"))
		if !VerifySignature([]byte(secret), r.Header.Get("X-Timestamp"), body, r.Header.Get("X-Signature")) {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(webhookError{Error: "bad signature", Code: "SIG"})
			return
		}
		switch r.URL.Path {
		case "/customer":
			var ref CustomerRef
			require.NoError(t, json.Unmarshal(body, &ref))
			_ = json.NewEncoder(w).Encode(map[string]string{"coreCustomerId": "CORE-" + ref.CustomerID})
		case "/disburse":
			var fr FundingRequest
			require.NoError(t, json.Unmarshal(body, &fr))
			assert.True(t, fr.AdvanceAmount.Equal(decimal.NewFromInt(80000)))
			assert.Equal(t, 45, fr.TenorDays)
			_ = json.NewEncoder(w).Encode(FundingResult{CoreRef: "LN-1", CoreCustomerID: "CORE-S1", AccountRef: "ACC-9"})
		case "/repayment":
			var rr RepaymentRequest
			require.NoError(t, json.Unmarshal(body, &rr))
			if rr.Amount.GreaterThan(decimal.NewFromInt(1000000)) {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_ = json.NewEncoder(w).Encode(webhookError{Error: "exceeds outstanding", Code: "OVERPAY"})
				return
			}
			_ = json.NewEncoder(w).Encode(RepaymentResult{TxnRef: "TX-1", Outstanding: decimal.NewFromInt(500), Closed: false})
		case "/credit":
			_ = json.NewEncoder(w).Encode(map[string]string{"txnRef": "TX-CR-1"})
		case "/position":
			var in map[string]string
			require.NoError(t, json.Unmarshal(body, &in))
			_ = json.NewEncoder(w).Encode(Position{CoreRef: in["coreRef"], Status: "ACTIVE", Outstanding: decimal.NewFromInt(500), DPD: 0})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func newProvider(t *testing.T, url string) *WebhookProvider {
	t.Helper()
	p := NewWebhookProvider(url, secret, 5*time.Second, zap.NewNop())
	p.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	return p
}

func TestWebhookRoundTrip(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(fakeCore(t, &seen))
	defer srv.Close()
	p := newProvider(t, srv.URL)
	ctx := context.Background()

	id, err := p.EnsureCustomer(ctx, CustomerRef{TenantID: "default", CustomerID: "S1", Name: "Seller One"})
	require.NoError(t, err)
	assert.Equal(t, CoreCustomerID("CORE-S1"), id)

	fr, err := p.Disburse(ctx, FundingRequest{
		TenantID: "default", InvoiceID: "inv-1", OfferID: "off-1", IdempotencyKey: "fund:inv-1",
		CoreCustomerID: id, Currency: "KES", AdvanceAmount: decimal.NewFromInt(80000),
		NetDisbursement: decimal.NewFromInt(76000), TenorDays: 45, DueDate: time.Now().AddDate(0, 0, 45),
	})
	require.NoError(t, err)
	assert.Equal(t, "LN-1", fr.CoreRef)
	assert.False(t, fr.DisbursedAt.IsZero(), "DisbursedAt defaulted when bank omits it")

	rr, err := p.RecordRepayment(ctx, RepaymentRequest{TenantID: "default", InvoiceID: "inv-1", CoreRef: "LN-1", Amount: decimal.NewFromInt(50000), Reference: "MPESA1", IdempotencyKey: "rep:1"})
	require.NoError(t, err)
	assert.Equal(t, TxnRef("TX-1"), rr.TxnRef)
	assert.True(t, rr.Outstanding.Equal(decimal.NewFromInt(500)))

	tx, err := p.Credit(ctx, CreditRequest{TenantID: "default", InvoiceID: "inv-1", Amount: decimal.NewFromInt(20000), Reference: "RES-1", IdempotencyKey: "res:1"})
	require.NoError(t, err)
	assert.Equal(t, TxnRef("TX-CR-1"), tx)

	pos, err := p.Position(ctx, "LN-1")
	require.NoError(t, err)
	assert.Equal(t, "ACTIVE", pos.Status)
	assert.Equal(t, "LN-1", pos.CoreRef)

	assert.Equal(t, []string{"/customer", "/disburse", "/repayment", "/credit", "/position"}, seen)
}

func TestWebhookRejectsBadSecret(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(fakeCore(t, &seen))
	defer srv.Close()
	p := NewWebhookProvider(srv.URL, "wrong", time.Second, zap.NewNop())
	_, err := p.Position(context.Background(), "LN-1")
	require.Error(t, err)
	var unavailable *ErrUnavailable
	assert.False(t, errors.As(err, &unavailable), "4xx is a rejection, not an outage")
	assert.Contains(t, err.Error(), "SIG")
}

func TestWebhookBusinessRejectionIsNotRetryable(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(fakeCore(t, &seen))
	defer srv.Close()
	p := newProvider(t, srv.URL)
	_, err := p.RecordRepayment(context.Background(), RepaymentRequest{CoreRef: "LN-1", Amount: decimal.NewFromInt(5000000), IdempotencyKey: "x"})
	require.Error(t, err)
	var unavailable *ErrUnavailable
	assert.False(t, errors.As(err, &unavailable))
	assert.Contains(t, err.Error(), "OVERPAY")
}

func TestWebhook5xxIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	p := newProvider(t, srv.URL)
	_, err := p.Disburse(context.Background(), FundingRequest{IdempotencyKey: "k"})
	var unavailable *ErrUnavailable
	assert.True(t, errors.As(err, &unavailable))

	srv.Close()
	_, err = p.Credit(context.Background(), CreditRequest{IdempotencyKey: "k"})
	assert.True(t, errors.As(err, &unavailable), "connection refused is an outage")
}

func TestSignatureHelpers(t *testing.T) {
	body := []byte(`{"a":1}`)
	sig := Sign([]byte(secret), "123", body)
	assert.True(t, VerifySignature([]byte(secret), "123", body, sig))
	assert.False(t, VerifySignature([]byte(secret), "124", body, sig))
	assert.False(t, VerifySignature([]byte(secret), "123", []byte(`{"a":2}`), sig))
}

func TestTenorAndRateHelpers(t *testing.T) {
	assert.Equal(t, 1, TenorMonths(7))
	assert.Equal(t, 1, TenorMonths(30))
	assert.Equal(t, 2, TenorMonths(31))
	assert.Equal(t, 2, TenorMonths(60))
	assert.Equal(t, 4, TenorMonths(120))
	assert.Equal(t, "25.55", AnnualRatePct(decimal.RequireFromString("0.0007")).String())
	_, err := Select("bogus", nil, nil)
	assert.Error(t, err)
	_, err = Select("webhook", nil, nil)
	assert.Error(t, err)
}
