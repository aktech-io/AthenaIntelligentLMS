package corebanking

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// WebhookProvider lets a bank plug its own core in: every operation is a
// POST of signed JSON to COREBANKING_WEBHOOK_URL/{customer|disburse|repayment|
// credit|position} expecting a synchronous JSON reply. Contract in
// docs/nemo/invoice-finance/COREBANKING.md.
//
// Headers: Content-Type: application/json, X-Timestamp (unix seconds),
// X-Idempotency-Key, X-Signature: sha256=<hex hmac(secret, timestamp + "." + body)>.
type WebhookProvider struct {
	baseURL string
	secret  []byte
	http    *http.Client
	logger  *zap.Logger
	now     func() time.Time
}

// NewWebhookProvider builds the adapter. timeout bounds each call.
func NewWebhookProvider(baseURL, secret string, timeout time.Duration, logger *zap.Logger) *WebhookProvider {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &WebhookProvider{
		baseURL: strings.TrimRight(baseURL, "/"), secret: []byte(secret),
		http: &http.Client{Timeout: timeout}, logger: logger, now: time.Now,
	}
}

func (w *WebhookProvider) Name() string { return "webhook" }

// Sign computes the X-Signature value for a timestamp + body.
func Sign(secret []byte, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(timestamp))
	m.Write([]byte("."))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// VerifySignature checks a signature header against timestamp + body (for
// banks implementing the receiving side, and for tests).
func VerifySignature(secret []byte, timestamp string, body []byte, signature string) bool {
	return hmac.Equal([]byte(Sign(secret, timestamp, body)), []byte(signature))
}

// webhookError is the standard error reply: {"error": "...", "code": "..."}.
type webhookError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func (w *WebhookProvider) post(ctx context.Context, op, idempotencyKey string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(w.now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.baseURL+"/"+op, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Idempotency-Key", idempotencyKey)
	req.Header.Set("X-Signature", Sign(w.secret, ts, body))
	req.Header.Set("User-Agent", "nemo-invoice-service/1")

	resp, err := w.http.Do(req)
	if err != nil {
		return &ErrUnavailable{fmt.Errorf("webhook %s: %w", op, err)}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return &ErrUnavailable{fmt.Errorf("webhook %s returned %d: %s", op, resp.StatusCode, string(raw))}
	}
	if resp.StatusCode >= 400 {
		var we webhookError
		_ = json.Unmarshal(raw, &we)
		if we.Error == "" {
			we.Error = string(raw)
		}
		return fmt.Errorf("webhook %s rejected (%d %s): %s", op, resp.StatusCode, we.Code, we.Error)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("webhook %s: bad reply: %w", op, err)
		}
	}
	return nil
}

// EnsureCustomer → POST /customer {customer: CustomerRef} → {"coreCustomerId": "..."}.
func (w *WebhookProvider) EnsureCustomer(ctx context.Context, ref CustomerRef) (CoreCustomerID, error) {
	var out struct {
		CoreCustomerID string `json:"coreCustomerId"`
	}
	if err := w.post(ctx, "customer", "cust:"+ref.TenantID+":"+ref.CustomerID, ref, &out); err != nil {
		return "", err
	}
	if out.CoreCustomerID == "" {
		return "", fmt.Errorf("webhook customer: reply missing coreCustomerId")
	}
	return CoreCustomerID(out.CoreCustomerID), nil
}

// Disburse → POST /disburse FundingRequest → FundingResult.
func (w *WebhookProvider) Disburse(ctx context.Context, req FundingRequest) (FundingResult, error) {
	var out FundingResult
	if err := w.post(ctx, "disburse", req.IdempotencyKey, req, &out); err != nil {
		return FundingResult{}, err
	}
	if out.CoreRef == "" {
		return FundingResult{}, fmt.Errorf("webhook disburse: reply missing coreRef")
	}
	if out.DisbursedAt.IsZero() {
		out.DisbursedAt = w.now().UTC()
	}
	return out, nil
}

// RecordRepayment → POST /repayment RepaymentRequest → RepaymentResult.
func (w *WebhookProvider) RecordRepayment(ctx context.Context, req RepaymentRequest) (RepaymentResult, error) {
	var out RepaymentResult
	if err := w.post(ctx, "repayment", req.IdempotencyKey, req, &out); err != nil {
		return RepaymentResult{}, err
	}
	return out, nil
}

// Credit → POST /credit CreditRequest → {"txnRef": "..."}.
func (w *WebhookProvider) Credit(ctx context.Context, req CreditRequest) (TxnRef, error) {
	var out struct {
		TxnRef string `json:"txnRef"`
	}
	if err := w.post(ctx, "credit", req.IdempotencyKey, req, &out); err != nil {
		return "", err
	}
	return TxnRef(out.TxnRef), nil
}

// Position → POST /position {"coreRef": "..."} → Position.
func (w *WebhookProvider) Position(ctx context.Context, coreRef string) (Position, error) {
	var out Position
	if err := w.post(ctx, "position", "pos:"+coreRef, map[string]string{"coreRef": coreRef}, &out); err != nil {
		return Position{}, err
	}
	if out.CoreRef == "" {
		out.CoreRef = coreRef
	}
	return out, nil
}
