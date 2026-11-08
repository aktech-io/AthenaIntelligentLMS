// Package client holds thin HTTP clients for the Nemo services invoice-service
// talks to. Every call is bounded by the caller's context; callers wrap with
// a per-step timeout. Service-to-service auth is the X-Service-Key header
// (common/httputil.ServiceClient), with X-Service-User overridable per call
// where the upstream enforces maker-checker.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/httputil"
	scoringmodel "github.com/athena-lms/go-services/internal/scoring/model"
)

// Clients bundles every upstream.
type Clients struct {
	Extract  *ExtractClient
	Media    *MediaClient
	Notify   *NotifyClient
	Scoring  *ScoringClient
	Fraud    *FraudClient
	Accounts *AccountClient
	Orig     *OriginationClient
	Loans    *LoanClient
	Payments *PaymentClient
	Products *ProductClient
}

// URLs are the upstream base URLs (env-driven; see cmd/invoice-service).
type URLs struct {
	EkycML, Media, Notification, Scoring, Fraud, Account, Origination, LoanMgmt, Payment, Product string
}

// New builds all clients sharing one service key.
func New(u URLs, serviceKey string, logger *zap.Logger) *Clients {
	sc := httputil.NewServiceClient(serviceKey)
	raw := &http.Client{Timeout: 60 * time.Second, Transport: &auth.ServiceKeyTransport{ServiceKey: serviceKey, ServiceName: "invoice-service"}}
	return &Clients{
		Extract:  &ExtractClient{base: strings.TrimRight(u.EkycML, "/"), http: &http.Client{Timeout: 90 * time.Second}, logger: logger},
		Media:    &MediaClient{base: strings.TrimRight(u.Media, "/"), http: raw, logger: logger},
		Notify:   &NotifyClient{base: strings.TrimRight(u.Notification, "/"), sc: sc, logger: logger},
		Scoring:  &ScoringClient{base: strings.TrimRight(u.Scoring, "/"), sc: sc, logger: logger},
		Fraud:    &FraudClient{base: strings.TrimRight(u.Fraud, "/"), sc: sc, logger: logger},
		Accounts: &AccountClient{base: strings.TrimRight(u.Account, "/"), sc: sc, logger: logger},
		Orig:     &OriginationClient{base: strings.TrimRight(u.Origination, "/"), sc: sc, logger: logger},
		Loans:    &LoanClient{base: strings.TrimRight(u.LoanMgmt, "/"), sc: sc, logger: logger},
		Payments: &PaymentClient{base: strings.TrimRight(u.Payment, "/"), sc: sc, logger: logger},
		Products: &ProductClient{base: strings.TrimRight(u.Product, "/"), sc: sc, logger: logger},
	}
}

// ─── ekyc-ml invoice extraction ──────────────────────────────────────────────

// ExtractField is one OCR field with confidence.
type ExtractField struct {
	Value      json.RawMessage `json:"value"`
	Confidence float64         `json:"confidence"`
}

// Str returns the field value as a string (numbers rendered plainly).
func (f ExtractField) Str() string {
	if len(f.Value) == 0 || string(f.Value) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(f.Value, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(f.Value))
}

// Decimal parses the field as a decimal (tolerates thousands separators and currency noise).
func (f ExtractField) Decimal() (decimal.Decimal, bool) {
	s := f.Str()
	if s == "" {
		return decimal.Zero, false
	}
	clean := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' {
			return r
		}
		return -1
	}, s)
	d, err := decimal.NewFromString(clean)
	return d, err == nil
}

// ExtractLineItem mirrors ekyc-ml lineItems[].
type ExtractLineItem struct {
	Description string  `json:"description"`
	Qty         float64 `json:"qty"`
	UnitPrice   float64 `json:"unitPrice"`
	Amount      float64 `json:"amount"`
	Confidence  float64 `json:"confidence"`
}

// ExtractResult is POST /v1/invoice/extract's response.
type ExtractResult struct {
	Engine    string                  `json:"engine"`
	Fields    map[string]ExtractField `json:"fields"`
	LineItems []ExtractLineItem       `json:"lineItems"`
	WordCount int                     `json:"wordCount"`
}

// ExtractClient calls ekyc-ml-service.
type ExtractClient struct {
	base   string
	http   *http.Client
	logger *zap.Logger
}

// Extract uploads the PDF bytes as multipart `file`.
func (c *ExtractClient) Extract(ctx context.Context, fileName string, data []byte) (*ExtractResult, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", fileName)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(data); err != nil {
		return nil, err
	}
	w.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/invoice/extract", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ekyc-ml extract: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("ekyc-ml extract returned %d: %s", resp.StatusCode, string(b))
	}
	var out ExtractResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ekyc-ml extract decode: %w", err)
	}
	return &out, nil
}

// ─── media-service ───────────────────────────────────────────────────────────

// MediaClient uploads/downloads files.
type MediaClient struct {
	base   string
	http   *http.Client
	logger *zap.Logger
}

// Upload stores the invoice PDF (category FINANCIAL, mediaType INVOICE) and returns the media id.
func (c *MediaClient) Upload(ctx context.Context, fileName string, data []byte, referenceID string) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, _ := w.CreateFormFile("file", fileName)
	if _, err := fw.Write(data); err != nil {
		return "", err
	}
	_ = w.WriteField("category", "FINANCIAL")
	_ = w.WriteField("mediaType", "INVOICE")
	_ = w.WriteField("referenceId", referenceID)
	_ = w.WriteField("serviceName", "invoice-service")
	_ = w.WriteField("description", "Invoice finance source document")
	w.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1/media/upload", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	setServiceContext(ctx, req)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("media upload: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("media upload returned %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// Download fetches the stored bytes (used by re-extraction / public verify).
func (c *MediaClient) Download(ctx context.Context, mediaID string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/media/download/"+mediaID, nil)
	if err != nil {
		return nil, err
	}
	setServiceContext(ctx, req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("media download returned %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

func setServiceContext(ctx context.Context, req *http.Request) {
	if t := auth.TenantIDFromContext(ctx); t != "" {
		req.Header.Set("X-Service-Tenant", t)
	}
	if u := auth.UserIDFromContext(ctx); u != "" {
		req.Header.Set("X-Service-User", u)
	}
}

// ─── notification-service ────────────────────────────────────────────────────

// NotifyClient sends email/SMS through notification-service.
type NotifyClient struct {
	base   string
	sc     *httputil.ServiceClient
	logger *zap.Logger
}

// Send posts POST /api/v1/notifications/send. typ is EMAIL or SMS.
func (c *NotifyClient) Send(ctx context.Context, typ, recipient, subject, message string, metadata map[string]any) error {
	if recipient == "" {
		return fmt.Errorf("notification: empty recipient")
	}
	body := map[string]any{
		"serviceName": "invoice-service",
		"type":        typ,
		"recipient":   recipient,
		"subject":     subject,
		"message":     message,
		"metadata":    metadata,
	}
	return c.sc.Post(ctx, c.base+"/api/v1/notifications/send", body, nil)
}

// ─── ai-scoring ──────────────────────────────────────────────────────────────

// Score is the normalised seller credit score.
type Score struct {
	FinalScore decimal.Decimal
	ScoreBand  string // upstream band: EXCELLENT..POOR
	Band       string // A-D
	Provider   string
	Model      string
	ScoredAt   *time.Time
}

// Trusted reports whether the score came from a real model (not "mock").
func (s Score) Trusted() bool { return !strings.EqualFold(s.Provider, "mock") }

// BandFromScoreBand maps the ai-scoring band vocabulary to A-D.
//
// athena-python-service bands (pdo_transformer._BANDS): Excellent ≥780,
// Very Good ≥720, Good ≥680, Fair ≥640, Marginal ≥600, Poor <600. The
// invoice.advance table has four seller bands, so Fair and Marginal both
// land in C (priced, not declined) and only Poor is D. Labels may arrive
// with spaces ("Very Good") or underscores.
func BandFromScoreBand(sb string) string {
	norm := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(sb), " ", "_"))
	switch norm {
	case "EXCELLENT", "VERY_GOOD", "A", "A_PLUS":
		return "A"
	case "GOOD", "B", "B_PLUS":
		return "B"
	case "FAIR", "MARGINAL", "C":
		return "C"
	case "POOR", "VERY_POOR", "D", "E":
		return "D"
	}
	return ""
}

// ScoringClient calls ai-scoring-service.
type ScoringClient struct {
	base   string
	sc     *httputil.ServiceClient
	logger *zap.Logger
}

// Latest returns the latest score for a customer (nil,nil when none).
func (c *ScoringClient) Latest(ctx context.Context, customerID string) (*Score, error) {
	var resp struct {
		FinalScore  *decimal.Decimal `json:"finalScore"`
		ScoreBand   string           `json:"scoreBand"`
		LlmProvider string           `json:"llmProvider"`
		LlmModel    string           `json:"llmModel"`
		ScoredAt    *time.Time       `json:"scoredAt"`
	}
	err := c.sc.Get(ctx, fmt.Sprintf("%s/api/v1/scoring/customers/%s/latest", c.base, customerID), &resp)
	if err != nil {
		if strings.Contains(err.Error(), "returned 404") {
			return nil, nil
		}
		return nil, err
	}
	if resp.FinalScore == nil || resp.ScoreBand == "" {
		return nil, nil
	}
	return &Score{FinalScore: *resp.FinalScore, ScoreBand: resp.ScoreBand, Band: BandFromScoreBand(resp.ScoreBand),
		Provider: resp.LlmProvider, Model: resp.LlmModel, ScoredAt: resp.ScoredAt}, nil
}

// Request asks for a fresh score (POST /api/v1/scoring/requests). Async upstream.
func (c *ScoringClient) Request(ctx context.Context, customerID, invoiceID string) error {
	body := map[string]any{
		"loanApplicationId": invoiceID,
		"customerId":        scoringmodel.FlexibleCustomerID(customerID),
		"triggerEvent":      "INVOICE_FINANCE_REQUEST",
	}
	return c.sc.Post(ctx, c.base+"/api/v1/scoring/requests", body, nil)
}

// ─── fraud-detection ─────────────────────────────────────────────────────────

// FraudResult is the normalised fraud evaluation.
type FraudResult struct {
	RiskScore      float64
	TriggeredRules []string
	Available      bool
}

// FraudClient calls fraud-detection-service.
type FraudClient struct {
	base   string
	sc     *httputil.ServiceClient
	logger *zap.Logger
}

// Evaluate posts POST /api/v1/fraud/evaluate.
func (c *FraudClient) Evaluate(ctx context.Context, customerID string, amount decimal.Decimal, invoiceID string) (FraudResult, error) {
	body := map[string]any{
		"customerId":  customerID,
		"eventType":   "INVOICE_FINANCE_REQUEST",
		"amount":      amount,
		"subjectType": "invoice",
		"subjectId":   invoiceID,
	}
	var resp struct {
		RiskScore      float64 `json:"riskScore"`
		TriggeredRules []struct {
			RuleCode string `json:"ruleCode"`
			Severity string `json:"severity"`
		} `json:"triggeredRules"`
	}
	if err := c.sc.Post(ctx, c.base+"/api/v1/fraud/evaluate", body, &resp); err != nil {
		return FraudResult{}, err
	}
	out := FraudResult{RiskScore: resp.RiskScore, Available: true}
	for _, r := range resp.TriggeredRules {
		out.TriggeredRules = append(out.TriggeredRules, r.RuleCode)
	}
	return out, nil
}

// ─── account-service ─────────────────────────────────────────────────────────

// Customer is the account-service customer (subset).
type Customer struct {
	ID         string     `json:"id"` // UUID PK
	CustomerID string     `json:"customerId"`
	FirstName  string     `json:"firstName"`
	LastName   string     `json:"lastName"`
	Email      string     `json:"email"`
	Phone      string     `json:"phone"`
	Status     string     `json:"status"`
	KYCStatus  string     `json:"kycStatus"`
	CreatedAt  *time.Time `json:"createdAt"`
}

// Account is the account-service account (subset).
type Account struct {
	ID            string `json:"id"`
	AccountNumber string `json:"accountNumber"`
	CustomerID    string `json:"customerId"`
	AccountType   string `json:"accountType"`
	Status        string `json:"status"`
	Currency      string `json:"currency"`
}

// AccountClient calls account-service.
type AccountClient struct {
	base   string
	sc     *httputil.ServiceClient
	logger *zap.Logger
}

// GetCustomer looks a customer up by business id (nil,nil when 404).
func (c *AccountClient) GetCustomer(ctx context.Context, customerID string) (*Customer, error) {
	var out Customer
	err := c.sc.Get(ctx, c.base+"/api/v1/customers/by-customer-id/"+customerID, &out)
	if err != nil {
		if strings.Contains(err.Error(), "returned 404") {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

// CreateCustomer creates a BUSINESS customer with the given business id.
func (c *AccountClient) CreateCustomer(ctx context.Context, customerID, name, email, phone, taxID string) (*Customer, error) {
	first, last := name, "Ltd"
	if i := strings.LastIndex(strings.TrimSpace(name), " "); i > 0 {
		first, last = name[:i], name[i+1:]
	}
	if first == "" {
		first = customerID
	}
	body := map[string]any{
		"customerId": customerID, "firstName": first, "lastName": last,
		"email": email, "phone": phone, "nationalId": taxID,
		"customerType": "BUSINESS", "source": "INVOICE_FINANCE",
	}
	var out Customer
	if err := c.sc.Post(ctx, c.base+"/api/v1/customers", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAccounts returns the customer's accounts (newest first).
func (c *AccountClient) ListAccounts(ctx context.Context, customerID string) ([]Account, error) {
	var out []Account
	if err := c.sc.Get(ctx, c.base+"/api/v1/accounts/customer/"+customerID, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateAccount opens a CURRENT account for the customer.
func (c *AccountClient) CreateAccount(ctx context.Context, customerID, currency, name string) (*Account, error) {
	// account-service CreateAccountRequest: kycTier is an int (1-3), not a label.
	body := map[string]any{"customerId": customerID, "accountType": "CURRENT", "currency": currency, "kycTier": 2, "accountName": name}
	var out Account
	if err := c.sc.Post(ctx, c.base+"/api/v1/accounts", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResolveAccount picks the disbursement account: first WALLET, else first ACTIVE, else creates one.
func (c *AccountClient) ResolveAccount(ctx context.Context, customerID, currency, name string) (*Account, error) {
	accts, err := c.ListAccounts(ctx, customerID)
	if err != nil {
		return nil, err
	}
	for _, a := range accts {
		if a.AccountType == "WALLET" && (a.Status == "" || a.Status == "ACTIVE") {
			return &a, nil
		}
	}
	for _, a := range accts {
		if a.Status == "" || a.Status == "ACTIVE" || a.Status == "DORMANT" {
			return &a, nil
		}
	}
	return c.CreateAccount(ctx, customerID, currency, name)
}

// Credit posts POST /api/v1/accounts/{id}/credit and returns the transaction id.
func (c *AccountClient) Credit(ctx context.Context, accountID string, amount decimal.Decimal, description, reference, channel string) (string, error) {
	body := map[string]any{"amount": amount, "description": description, "reference": reference, "channel": channel, "idempotencyKey": reference}
	var out struct {
		ID string `json:"id"`
	}
	if err := c.sc.Post(ctx, c.base+"/api/v1/accounts/"+accountID+"/credit", body, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// ─── loan-origination ────────────────────────────────────────────────────────

// Application is the origination ApplicationResponse subset.
type Application struct {
	ID              string           `json:"id"`
	CustomerID      string           `json:"customerId"`
	ProductID       string           `json:"productId"`
	Status          string           `json:"status"`
	ApprovedAmount  *decimal.Decimal `json:"approvedAmount"`
	DisbursedAmount *decimal.Decimal `json:"disbursedAmount"`
}

// OriginationClient drives create → submit → review → approve → disburse.
type OriginationClient struct {
	base   string
	sc     *httputil.ServiceClient
	logger *zap.Logger
}

// Create opens an application.
func (c *OriginationClient) Create(ctx context.Context, customerID, productID string, amount decimal.Decimal, tenorMonths int, currency, purpose, disbursementAccount string) (*Application, error) {
	body := map[string]any{
		"customerId": customerID, "productId": productID, "requestedAmount": amount,
		"tenorMonths": tenorMonths, "currency": currency, "purpose": purpose, "disbursementAccount": disbursementAccount,
	}
	var out Application
	if err := c.sc.Post(ctx, c.base+"/api/v1/loan-applications", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Submit moves DRAFT → SUBMITTED.
func (c *OriginationClient) Submit(ctx context.Context, id string) error {
	return c.sc.Post(ctx, c.base+"/api/v1/loan-applications/"+id+"/submit", nil, nil)
}

// StartReview moves SUBMITTED → UNDER_REVIEW.
func (c *OriginationClient) StartReview(ctx context.Context, id string) error {
	return c.sc.Post(ctx, c.base+"/api/v1/loan-applications/"+id+"/review/start", nil, nil)
}

// Approve moves UNDER_REVIEW → APPROVED.
func (c *OriginationClient) Approve(ctx context.Context, id string, amount, annualRatePct decimal.Decimal, notes string) error {
	body := map[string]any{"approvedAmount": amount, "interestRate": annualRatePct, "reviewNotes": notes}
	return c.sc.Post(ctx, c.base+"/api/v1/loan-applications/"+id+"/review/approve", body, nil)
}

// Disburse moves APPROVED → DISBURSED and credits the account.
func (c *OriginationClient) Disburse(ctx context.Context, id string, amount decimal.Decimal, accountID string) (*Application, error) {
	body := map[string]any{"disbursedAmount": amount, "disbursementAccount": accountID}
	var out Application
	if err := c.sc.Post(ctx, c.base+"/api/v1/loan-applications/"+id+"/disburse", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get fetches an application.
func (c *OriginationClient) Get(ctx context.Context, id string) (*Application, error) {
	var out Application
	if err := c.sc.Get(ctx, c.base+"/api/v1/loan-applications/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── loan-management ─────────────────────────────────────────────────────────

// Loan is the loan-management LoanResponse subset.
type Loan struct {
	ID                   string          `json:"id"`
	ApplicationID        string          `json:"applicationId"`
	CustomerID           string          `json:"customerId"`
	Status               string          `json:"status"`
	DisbursedAmount      decimal.Decimal `json:"disbursedAmount"`
	OutstandingPrincipal decimal.Decimal `json:"outstandingPrincipal"`
	OutstandingInterest  decimal.Decimal `json:"outstandingInterest"`
	OutstandingFees      decimal.Decimal `json:"outstandingFees"`
	TotalOutstanding     decimal.Decimal `json:"totalOutstanding"`
	DPD                  int             `json:"dpd"`
	MaturityDate         *time.Time      `json:"maturityDate"`
}

// LoanClient reads loan positions.
type LoanClient struct {
	base   string
	sc     *httputil.ServiceClient
	logger *zap.Logger
}

// Get fetches one loan.
func (c *LoanClient) Get(ctx context.Context, id string) (*Loan, error) {
	var out Loan
	if err := c.sc.Get(ctx, c.base+"/api/v1/loans/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FindByApplication scans the customer's loans for one created from applicationID.
func (c *LoanClient) FindByApplication(ctx context.Context, customerID, applicationID string) (*Loan, error) {
	var loans []Loan
	if err := c.sc.Get(ctx, c.base+"/api/v1/loans/customer/"+customerID, &loans); err != nil {
		return nil, err
	}
	for i := range loans {
		if loans[i].ApplicationID == applicationID {
			return &loans[i], nil
		}
	}
	return nil, nil
}

// ─── payment-service ─────────────────────────────────────────────────────────

// PaymentClient records repayments.
type PaymentClient struct {
	base   string
	sc     *httputil.ServiceClient
	logger *zap.Logger
}

// LoanRepayment initiates and completes a LOAN_REPAYMENT (INTERNAL channel).
// externalReference must be unique per tenant and carries the invoice id.
func (c *PaymentClient) LoanRepayment(ctx context.Context, customerID, loanID string, amount decimal.Decimal, currency, externalReference, description string) (string, error) {
	body := map[string]any{
		"customerId": customerID, "paymentType": "LOAN_REPAYMENT", "paymentChannel": "INTERNAL",
		"amount": amount, "currency": currency, "externalReference": externalReference, "description": description,
	}
	if loanID != "" {
		body["loanId"] = loanID
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := c.sc.Post(ctx, c.base+"/api/v1/payments", body, &out); err != nil {
		return "", err
	}
	if err := c.sc.Post(ctx, c.base+"/api/v1/payments/"+out.ID+"/complete", map[string]any{"externalReference": externalReference, "notes": description}, nil); err != nil {
		return out.ID, fmt.Errorf("payment %s created but not completed: %w", out.ID, err)
	}
	return out.ID, nil
}

// ─── product-service ─────────────────────────────────────────────────────────

// Product is the product-service response subset.
type Product struct {
	ID           string `json:"id"`
	ProductCode  string `json:"productCode"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	ScheduleType string `json:"scheduleType"`
}

// ProductClient finds/creates the INVOICE_FINANCE product.
type ProductClient struct {
	base   string
	sc     *httputil.ServiceClient
	logger *zap.Logger
}

// FindByCode lists products and filters on productCode (no by-code route upstream).
func (c *ProductClient) FindByCode(ctx context.Context, code string) (*Product, error) {
	for page := 0; page < 20; page++ {
		var resp struct {
			Content    []Product `json:"content"`
			Items      []Product `json:"items"`
			TotalPages int       `json:"totalPages"`
		}
		if err := c.sc.Get(ctx, fmt.Sprintf("%s/api/v1/products?page=%d&size=100", c.base, page), &resp); err != nil {
			return nil, err
		}
		items := resp.Content
		if len(items) == 0 {
			items = resp.Items
		}
		for i := range items {
			if items[i].ProductCode == code {
				return &items[i], nil
			}
		}
		if len(items) < 100 || (resp.TotalPages > 0 && page+1 >= resp.TotalPages) {
			break
		}
	}
	return nil, nil
}

// Create creates a product (DRAFT) from the given body and activates it.
func (c *ProductClient) Create(ctx context.Context, body map[string]any) (*Product, error) {
	var out Product
	if err := c.sc.Post(ctx, c.base+"/api/v1/products", body, &out); err != nil {
		return nil, err
	}
	if err := c.sc.Post(ctx, c.base+"/api/v1/products/"+out.ID+"/activate", nil, &out); err != nil {
		c.logger.Warn("product created but activation failed", zap.String("id", out.ID), zap.Error(err))
	}
	return &out, nil
}

// Activate activates an existing product.
func (c *ProductClient) Activate(ctx context.Context, id string) error {
	return c.sc.Post(ctx, c.base+"/api/v1/products/"+id+"/activate", nil, nil)
}
