// Package repository is the pgx data layer for invoice-service. Ledger
// appends are the only place chain links are computed (ledger.Link); the
// per-tenant advisory lock serialises concurrent appends so the chain never
// forks.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	cerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/invoice/fingerprint"
	"github.com/athena-lms/go-services/internal/invoice/ledger"
	"github.com/athena-lms/go-services/internal/invoice/model"
)

// Querier is satisfied by *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repository provides data access for all invoice entities.
type Repository struct {
	pool    *pgxpool.Pool
	hmacKey []byte
}

// New creates a Repository; hmacKey signs ledger events (the service key).
func New(pool *pgxpool.Pool, hmacKey []byte) *Repository {
	return &Repository{pool: pool, hmacKey: hmacKey}
}

// Pool exposes the pool for transactions.
func (r *Repository) Pool() *pgxpool.Pool { return r.pool }

// HMACKey returns the ledger signing key (for verification).
func (r *Repository) HMACKey() []byte { return r.hmacKey }

// Begin opens a transaction.
func (r *Repository) Begin(ctx context.Context) (pgx.Tx, error) { return r.pool.Begin(ctx) }

func (r *Repository) q(tx Querier) Querier {
	if tx == nil {
		return r.pool
	}
	return tx
}

func jsonb(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil || v == nil {
		return []byte("null")
	}
	return b
}

func nilUUID(u *uuid.UUID) any {
	if u == nil {
		return nil
	}
	return *u
}

func nilDec(d *decimal.Decimal) any {
	if d == nil {
		return nil
	}
	return *d
}

func nilTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func nilStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ─── Invoices ────────────────────────────────────────────────────────────────

const invoiceCols = `id, tenant_id, seller_customer_id, fingerprint, fields_hash, media_id, file_name, invoice_number,
	issue_date, due_date, currency, subtotal, vat, total, seller_name, seller_tax_id, seller_email, buyer_id, buyer_snapshot,
	line_items, ocr, needs_confirmation, requested_amount, status, status_reason, decision_outcome, fraud_flags, invoice_score,
	seller_band, approved_by, approved_at, pipeline_attempts, pipeline_error, pipeline_next_at, created_by, created_at, updated_at`

func scanInvoice(row pgx.Row) (*model.Invoice, error) {
	var inv model.Invoice
	var fieldsHash, mediaID, fileName, invNo, currency, sellerName, sellerTax, sellerEmail, statusReason, outcome, band, approvedBy, pipeErr, createdBy *string
	var buyerSnap, lineItems, ocr, needs, flags []byte
	var score *int
	err := row.Scan(&inv.ID, &inv.TenantID, &inv.SellerCustomerID, &inv.Fingerprint, &fieldsHash, &mediaID, &fileName, &invNo,
		&inv.IssueDate, &inv.DueDate, &currency, &inv.Subtotal, &inv.VAT, &inv.Total, &sellerName, &sellerTax, &sellerEmail, &inv.BuyerID, &buyerSnap,
		&lineItems, &ocr, &needs, &inv.RequestedAmount, &inv.Status, &statusReason, &outcome, &flags, &score,
		&band, &approvedBy, &inv.ApprovedAt, &inv.PipelineAttempts, &pipeErr, &inv.PipelineNextAt, &createdBy, &inv.CreatedAt, &inv.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, cerrors.NotFound("invoice not found")
		}
		return nil, err
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	inv.FieldsHash, inv.MediaID, inv.FileName, inv.InvoiceNumber, inv.Currency = deref(fieldsHash), deref(mediaID), deref(fileName), deref(invNo), deref(currency)
	inv.Seller = model.Party{Name: deref(sellerName), TaxID: deref(sellerTax), Email: deref(sellerEmail)}
	inv.StatusReason, inv.DecisionOutcome, inv.SellerBand, inv.ApprovedBy, inv.PipelineError, inv.CreatedBy = deref(statusReason), deref(outcome), deref(band), deref(approvedBy), deref(pipeErr), deref(createdBy)
	inv.InvoiceScore = score
	inv.Fingerprint = strings.TrimSpace(inv.Fingerprint)
	inv.FieldsHash = strings.TrimSpace(inv.FieldsHash)
	_ = json.Unmarshal(buyerSnap, &inv.Buyer)
	_ = json.Unmarshal(lineItems, &inv.LineItems)
	_ = json.Unmarshal(ocr, &inv.OCR)
	_ = json.Unmarshal(needs, &inv.NeedsConfirm)
	_ = json.Unmarshal(flags, &inv.FraudFlags)
	if inv.LineItems == nil {
		inv.LineItems = []model.LineItem{}
	}
	if inv.NeedsConfirm == nil {
		inv.NeedsConfirm = []string{}
	}
	if inv.FraudFlags == nil {
		inv.FraudFlags = []string{}
	}
	return &inv, nil
}

// CreateInvoice inserts a new invoice (id/timestamps set here).
func (r *Repository) CreateInvoice(ctx context.Context, tx Querier, inv *model.Invoice) error {
	if inv.ID == uuid.Nil {
		inv.ID = uuid.New()
	}
	now := time.Now().UTC()
	inv.CreatedAt, inv.UpdatedAt = now, now
	_, err := r.q(tx).Exec(ctx, `INSERT INTO invoices (`+invoiceCols+`) VALUES
		($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37)`,
		inv.ID, inv.TenantID, inv.SellerCustomerID, inv.Fingerprint, nilStr(inv.FieldsHash), nilStr(inv.MediaID), nilStr(inv.FileName), nilStr(inv.InvoiceNumber),
		nilTime(inv.IssueDate), nilTime(inv.DueDate), nilStr(inv.Currency), nilDec(inv.Subtotal), nilDec(inv.VAT), nilDec(inv.Total),
		nilStr(inv.Seller.Name), nilStr(inv.Seller.TaxID), nilStr(inv.Seller.Email), nilUUID(inv.BuyerID), jsonb(inv.Buyer),
		jsonb(inv.LineItems), jsonb(inv.OCR), jsonb(inv.NeedsConfirm), nilDec(inv.RequestedAmount), inv.Status, nilStr(inv.StatusReason),
		nilStr(inv.DecisionOutcome), jsonb(inv.FraudFlags), inv.InvoiceScore, nilStr(inv.SellerBand), nilStr(inv.ApprovedBy), nilTime(inv.ApprovedAt),
		inv.PipelineAttempts, nilStr(inv.PipelineError), nilTime(inv.PipelineNextAt), nilStr(inv.CreatedBy), inv.CreatedAt, inv.UpdatedAt)
	return err
}

// UpdateInvoice writes every mutable column.
func (r *Repository) UpdateInvoice(ctx context.Context, tx Querier, inv *model.Invoice) error {
	inv.UpdatedAt = time.Now().UTC()
	_, err := r.q(tx).Exec(ctx, `UPDATE invoices SET fields_hash=$2, media_id=$3, file_name=$4, invoice_number=$5, issue_date=$6, due_date=$7,
		currency=$8, subtotal=$9, vat=$10, total=$11, seller_name=$12, seller_tax_id=$13, seller_email=$14, buyer_id=$15, buyer_snapshot=$16,
		line_items=$17, ocr=$18, needs_confirmation=$19, requested_amount=$20, status=$21, status_reason=$22, decision_outcome=$23, fraud_flags=$24,
		invoice_score=$25, seller_band=$26, approved_by=$27, approved_at=$28, pipeline_attempts=$29, pipeline_error=$30, pipeline_next_at=$31, updated_at=$32
		WHERE id=$1`,
		inv.ID, nilStr(inv.FieldsHash), nilStr(inv.MediaID), nilStr(inv.FileName), nilStr(inv.InvoiceNumber), nilTime(inv.IssueDate), nilTime(inv.DueDate),
		nilStr(inv.Currency), nilDec(inv.Subtotal), nilDec(inv.VAT), nilDec(inv.Total), nilStr(inv.Seller.Name), nilStr(inv.Seller.TaxID), nilStr(inv.Seller.Email),
		nilUUID(inv.BuyerID), jsonb(inv.Buyer), jsonb(inv.LineItems), jsonb(inv.OCR), jsonb(inv.NeedsConfirm), nilDec(inv.RequestedAmount), inv.Status,
		nilStr(inv.StatusReason), nilStr(inv.DecisionOutcome), jsonb(inv.FraudFlags), inv.InvoiceScore, nilStr(inv.SellerBand), nilStr(inv.ApprovedBy),
		nilTime(inv.ApprovedAt), inv.PipelineAttempts, nilStr(inv.PipelineError), nilTime(inv.PipelineNextAt), inv.UpdatedAt)
	return err
}

// GetInvoice loads one invoice (tenant-scoped when tenantID != "").
func (r *Repository) GetInvoice(ctx context.Context, tx Querier, tenantID string, id uuid.UUID) (*model.Invoice, error) {
	if tenantID == "" {
		return scanInvoice(r.q(tx).QueryRow(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE id=$1`, id))
	}
	return scanInvoice(r.q(tx).QueryRow(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE id=$1 AND tenant_id=$2`, id, tenantID))
}

// GetInvoiceForUpdate row-locks the invoice inside tx.
func (r *Repository) GetInvoiceForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*model.Invoice, error) {
	return scanInvoice(tx.QueryRow(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE id=$1 FOR UPDATE`, id))
}

// FindDuplicate returns a live invoice with the same fingerprint, or the same
// seller + normalised invoice number + buyer tax id (SPEC §3.1).
func (r *Repository) FindDuplicate(ctx context.Context, tenantID, fp, sellerID, invoiceNumber, buyerTaxID string) (*model.Invoice, error) {
	inv, err := scanInvoice(r.pool.QueryRow(ctx, `SELECT `+invoiceCols+` FROM invoices
		WHERE tenant_id=$1 AND fingerprint=$2 AND status NOT IN ('DECLINED','CANCELLED') ORDER BY created_at LIMIT 1`, tenantID, fp))
	if err == nil {
		return inv, nil
	}
	var nf *cerrors.NotFoundError
	if !errors.As(err, &nf) {
		return nil, err
	}
	if invoiceNumber == "" {
		return nil, nil
	}
	norm := fingerprint.NormalizeID(invoiceNumber)
	rows, err := r.pool.Query(ctx, `SELECT `+invoiceCols+` FROM invoices
		WHERE tenant_id=$1 AND seller_customer_id=$2 AND status NOT IN ('DECLINED','CANCELLED')
		  AND regexp_replace(upper(coalesce(invoice_number,'')), '[^A-Z0-9]', '', 'g') = $3`, tenantID, sellerID, norm)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		cand, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		if buyerTaxID == "" || cand.Buyer.TaxID == "" || fingerprint.NormalizeID(cand.Buyer.TaxID) == fingerprint.NormalizeID(buyerTaxID) {
			return cand, nil
		}
	}
	return nil, rows.Err()
}

// FindByFingerprint is the public verify lookup (any tenant; most recent live first).
func (r *Repository) FindByFingerprint(ctx context.Context, fp string) (*model.Invoice, error) {
	inv, err := scanInvoice(r.pool.QueryRow(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE fingerprint=$1
		ORDER BY (status IN ('DECLINED','CANCELLED')), created_at DESC LIMIT 1`, fp))
	var nf *cerrors.NotFoundError
	if errors.As(err, &nf) {
		return nil, nil
	}
	return inv, err
}

// ListInvoices pages invoices for a tenant.
func (r *Repository) ListInvoices(ctx context.Context, tenantID string, f model.ListFilter) (*model.Page[model.Invoice], error) {
	where := []string{"tenant_id=$1"}
	args := []any{tenantID}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	if f.SellerID != "" {
		args = append(args, f.SellerID)
		where = append(where, fmt.Sprintf("seller_customer_id=$%d", len(args)))
	}
	if f.BuyerID != nil {
		args = append(args, *f.BuyerID)
		where = append(where, fmt.Sprintf("buyer_id=$%d", len(args)))
	}
	w := strings.Join(where, " AND ")
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM invoices WHERE `+w, args...).Scan(&total); err != nil {
		return nil, err
	}
	if f.Size <= 0 || f.Size > 200 {
		f.Size = 20
	}
	if f.Page < 0 {
		f.Page = 0
	}
	args = append(args, f.Size, f.Page*f.Size)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`SELECT %s FROM invoices WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, invoiceCols, w, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &model.Page[model.Invoice]{Items: []model.Invoice{}, Page: f.Page, Size: f.Size, TotalItems: total}
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, *inv)
	}
	page.TotalPages = int((total + int64(f.Size) - 1) / int64(f.Size))
	return page, rows.Err()
}

// PipelineDue returns invoices in automatic statuses whose retry time has
// passed (or that are stuck: updated_at older than stale).
func (r *Repository) PipelineDue(ctx context.Context, statuses []string, stale time.Duration, limit int) ([]model.Invoice, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+invoiceCols+` FROM invoices
		WHERE status = ANY($1) AND (pipeline_next_at IS NULL OR pipeline_next_at <= now()) AND updated_at <= now() - $2::interval
		ORDER BY updated_at LIMIT $3`, statuses, fmt.Sprintf("%d seconds", int(stale.Seconds())), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

// InvoicesByStatus lists all invoices in the given statuses (jobs).
func (r *Repository) InvoicesByStatus(ctx context.Context, statuses []string, limit int) ([]model.Invoice, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE status = ANY($1) ORDER BY updated_at LIMIT $2`, statuses, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

// SellerHistory returns count and average total of the seller's prior
// invoices in the last six months (fraud rule AMOUNT_SPIKE) and the seller's
// distinct prior buyer tax ids.
func (r *Repository) SellerHistory(ctx context.Context, tenantID, sellerID string, exclude uuid.UUID) (count int, avg decimal.Decimal, err error) {
	var a *decimal.Decimal
	err = r.pool.QueryRow(ctx, `SELECT count(*), avg(total) FROM invoices
		WHERE tenant_id=$1 AND seller_customer_id=$2 AND id<>$3 AND total IS NOT NULL
		  AND status NOT IN ('DECLINED','CANCELLED') AND created_at >= now() - interval '6 months'`, tenantID, sellerID, exclude).Scan(&count, &a)
	if a != nil {
		avg = *a
	}
	return
}

// Exposure returns the tenant's outstanding advance principal overall and for one buyer.
func (r *Repository) Exposure(ctx context.Context, tenantID string, buyerID *uuid.UUID) (total decimal.Decimal, buyer decimal.Decimal, activeCount int, err error) {
	var t, b *decimal.Decimal
	err = r.pool.QueryRow(ctx, `SELECT
		coalesce(sum(a.principal - a.repaid),0),
		coalesce(sum(CASE WHEN i.buyer_id = $2 THEN a.principal - a.repaid ELSE 0 END),0),
		count(*)
		FROM advances a JOIN invoices i ON i.id = a.invoice_id
		WHERE i.tenant_id=$1 AND a.status IN ('ACTIVE','OVERDUE')`, tenantID, nilUUID(buyerID)).Scan(&t, &b, &activeCount)
	if t != nil {
		total = *t
	}
	if b != nil {
		buyer = *b
	}
	return
}

// ─── Buyers ──────────────────────────────────────────────────────────────────

const buyerCols = `id, tenant_id, name, tax_id, email_domain, contacts, tier, limit_amount, exposure_amount, on_time_rate, confirmations_count, disputes_count, created_at, updated_at`

func scanBuyer(row pgx.Row) (*model.Buyer, error) {
	var b model.Buyer
	var taxID, domain *string
	var contacts []byte
	if err := row.Scan(&b.ID, &b.TenantID, &b.Name, &taxID, &domain, &contacts, &b.Tier, &b.LimitAmount, &b.ExposureAmount, &b.OnTimeRate, &b.ConfirmationsCount, &b.DisputesCount, &b.CreatedAt, &b.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, cerrors.NotFound("buyer not found")
		}
		return nil, err
	}
	if taxID != nil {
		b.TaxID = *taxID
	}
	if domain != nil {
		b.EmailDomain = *domain
	}
	_ = json.Unmarshal(contacts, &b.Contacts)
	if b.Contacts == nil {
		b.Contacts = []model.Party{}
	}
	return &b, nil
}

// UpsertBuyer finds a buyer by tax id, else by normalised name + email
// domain, else creates one (UNRATED). Contacts are merged.
func (r *Repository) UpsertBuyer(ctx context.Context, tx Querier, tenantID string, p model.Party) (*model.Buyer, error) {
	q := r.q(tx)
	domain := fingerprint.EmailDomain(p.Email)
	normName := fingerprint.NormalizeName(p.Name)
	var b *model.Buyer
	var err error
	if p.TaxID != "" {
		b, err = scanBuyer(q.QueryRow(ctx, `SELECT `+buyerCols+` FROM buyers WHERE tenant_id=$1 AND tax_id=$2`, tenantID, fingerprint.NormalizeID(p.TaxID)))
	}
	if b == nil && normName != "" {
		b, err = scanBuyer(q.QueryRow(ctx, `SELECT `+buyerCols+` FROM buyers WHERE tenant_id=$1 AND normalized_name=$2 AND coalesce(email_domain,'')=$3`, tenantID, normName, domain))
	}
	var nf *cerrors.NotFoundError
	if err != nil && !errors.As(err, &nf) {
		return nil, err
	}
	if b == nil {
		name := p.Name
		if name == "" {
			name = "Unknown buyer"
			normName = "unknown buyer " + uuid.NewString()[:8]
		}
		b = &model.Buyer{ID: uuid.New(), TenantID: tenantID, Name: name, TaxID: fingerprint.NormalizeID(p.TaxID), EmailDomain: domain,
			Contacts: []model.Party{}, Tier: model.TierUnrated, LimitAmount: decimal.Zero, ExposureAmount: decimal.Zero, OnTimeRate: decimal.Zero}
		if p.Email != "" || p.Phone != "" {
			b.Contacts = append(b.Contacts, model.Party{Name: p.Name, Email: p.Email, Phone: p.Phone})
		}
		now := time.Now().UTC()
		b.CreatedAt, b.UpdatedAt = now, now
		_, err = q.Exec(ctx, `INSERT INTO buyers (`+buyerCols+`, normalized_name) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			b.ID, b.TenantID, b.Name, nilStr(b.TaxID), nilStr(b.EmailDomain), jsonb(b.Contacts), b.Tier, b.LimitAmount, b.ExposureAmount, b.OnTimeRate, 0, 0, b.CreatedAt, b.UpdatedAt, normName)
		return b, err
	}
	// merge contact
	if p.Email != "" {
		known := false
		for _, c := range b.Contacts {
			if strings.EqualFold(c.Email, p.Email) {
				known = true
				break
			}
		}
		if !known {
			b.Contacts = append(b.Contacts, model.Party{Name: p.Name, Email: p.Email, Phone: p.Phone})
			if b.EmailDomain == "" {
				b.EmailDomain = domain
			}
			_, err = q.Exec(ctx, `UPDATE buyers SET contacts=$2, email_domain=coalesce(email_domain,$3), updated_at=now() WHERE id=$1`, b.ID, jsonb(b.Contacts), nilStr(domain))
		}
	}
	return b, err
}

// GetBuyer loads a buyer.
func (r *Repository) GetBuyer(ctx context.Context, tx Querier, id uuid.UUID) (*model.Buyer, error) {
	return scanBuyer(r.q(tx).QueryRow(ctx, `SELECT `+buyerCols+` FROM buyers WHERE id=$1`, id))
}

// ListBuyers lists a tenant's buyers.
func (r *Repository) ListBuyers(ctx context.Context, tenantID string) ([]model.Buyer, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+buyerCols+` FROM buyers WHERE tenant_id=$1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Buyer{}
	for rows.Next() {
		b, err := scanBuyer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// PatchBuyer updates tier / limit.
func (r *Repository) PatchBuyer(ctx context.Context, id uuid.UUID, tier *string, limit *decimal.Decimal) error {
	_, err := r.pool.Exec(ctx, `UPDATE buyers SET tier=coalesce($2,tier), limit_amount=coalesce($3,limit_amount), updated_at=now() WHERE id=$1`, id, tier, nilDec(limit))
	return err
}

// BumpBuyerStats adjusts counters/exposure atomically.
func (r *Repository) BumpBuyerStats(ctx context.Context, tx Querier, id uuid.UUID, confirmations, disputes int, exposureDelta decimal.Decimal) error {
	_, err := r.q(tx).Exec(ctx, `UPDATE buyers SET confirmations_count=confirmations_count+$2, disputes_count=disputes_count+$3,
		exposure_amount=greatest(exposure_amount+$4,0), updated_at=now() WHERE id=$1`, id, confirmations, disputes, exposureDelta)
	return err
}

// ─── Buyer confirmations ─────────────────────────────────────────────────────

const confCols = `id, invoice_id, token_hash, expires_at, sent_to, otp_hash, otp_expires_at, otp_attempts, reminders_sent, action, dispute_reason, acted_at, ip, user_agent, created_at, demo_token`

func scanConf(row pgx.Row) (*model.BuyerConfirmation, error) {
	var c model.BuyerConfirmation
	var sentTo, otpHash, action, reason, ip, ua, demo *string
	if err := row.Scan(&c.ID, &c.InvoiceID, &c.TokenHash, &c.ExpiresAt, &sentTo, &otpHash, &c.OTPExpiresAt, &c.OTPAttempts, &c.RemindersSent, &action, &reason, &c.ActedAt, &ip, &ua, &c.CreatedAt, &demo); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, cerrors.NotFound("confirmation not found")
		}
		return nil, err
	}
	d := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	c.SentTo, c.OTPHash, c.Action, c.DisputeReason, c.IP, c.UserAgent, c.DemoToken = d(sentTo), d(otpHash), d(action), d(reason), d(ip), d(ua), d(demo)
	c.TokenHash = strings.TrimSpace(c.TokenHash)
	c.OTPHash = strings.TrimSpace(c.OTPHash)
	return &c, nil
}

// CreateConfirmation inserts a confirmation request.
func (r *Repository) CreateConfirmation(ctx context.Context, tx Querier, c *model.BuyerConfirmation) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	c.CreatedAt = time.Now().UTC()
	_, err := r.q(tx).Exec(ctx, `INSERT INTO buyer_confirmations (`+confCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		c.ID, c.InvoiceID, c.TokenHash, c.ExpiresAt, nilStr(c.SentTo), nilStr(c.OTPHash), nilTime(c.OTPExpiresAt), c.OTPAttempts, c.RemindersSent,
		nilStr(c.Action), nilStr(c.DisputeReason), nilTime(c.ActedAt), nilStr(c.IP), nilStr(c.UserAgent), c.CreatedAt, nilStr(c.DemoToken))
	return err
}

// UpdateConfirmation writes the mutable columns.
func (r *Repository) UpdateConfirmation(ctx context.Context, tx Querier, c *model.BuyerConfirmation) error {
	_, err := r.q(tx).Exec(ctx, `UPDATE buyer_confirmations SET otp_attempts=$2, reminders_sent=$3, action=$4, dispute_reason=$5, acted_at=$6, ip=$7, user_agent=$8, expires_at=$9 WHERE id=$1`,
		c.ID, c.OTPAttempts, c.RemindersSent, nilStr(c.Action), nilStr(c.DisputeReason), nilTime(c.ActedAt), nilStr(c.IP), nilStr(c.UserAgent), c.ExpiresAt)
	return err
}

// ConfirmationByToken looks up by sha256(token).
func (r *Repository) ConfirmationByToken(ctx context.Context, tx Querier, tokenHash string) (*model.BuyerConfirmation, error) {
	return scanConf(r.q(tx).QueryRow(ctx, `SELECT `+confCols+` FROM buyer_confirmations WHERE token_hash=$1`, tokenHash))
}

// LatestConfirmation returns the newest confirmation for an invoice (nil when none).
func (r *Repository) LatestConfirmation(ctx context.Context, tx Querier, invoiceID uuid.UUID) (*model.BuyerConfirmation, error) {
	c, err := scanConf(r.q(tx).QueryRow(ctx, `SELECT `+confCols+` FROM buyer_confirmations WHERE invoice_id=$1 ORDER BY created_at DESC LIMIT 1`, invoiceID))
	var nf *cerrors.NotFoundError
	if errors.As(err, &nf) {
		return nil, nil
	}
	return c, err
}

// ─── Offers ──────────────────────────────────────────────────────────────────

const offerCols = `id, invoice_id, advance_rate, advance_amount, discount_fee_per_day, tenor_days, fee_amount, processing_fee, net_disbursement, reserve_amount, expires_at, status, accepted_by, accepted_at, created_at`

func scanOffer(row pgx.Row) (*model.Offer, error) {
	var o model.Offer
	var by *string
	if err := row.Scan(&o.ID, &o.InvoiceID, &o.AdvanceRate, &o.AdvanceAmount, &o.DiscountFeePerDay, &o.TenorDays, &o.FeeAmount, &o.ProcessingFee, &o.NetDisbursement, &o.ReserveAmount, &o.ExpiresAt, &o.Status, &by, &o.AcceptedAt, &o.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, cerrors.NotFound("offer not found")
		}
		return nil, err
	}
	if by != nil {
		o.AcceptedBy = *by
	}
	return &o, nil
}

// CreateOffer inserts an offer.
func (r *Repository) CreateOffer(ctx context.Context, tx Querier, o *model.Offer) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	o.CreatedAt = time.Now().UTC()
	_, err := r.q(tx).Exec(ctx, `INSERT INTO offers (`+offerCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		o.ID, o.InvoiceID, o.AdvanceRate, o.AdvanceAmount, o.DiscountFeePerDay, o.TenorDays, o.FeeAmount, o.ProcessingFee, o.NetDisbursement, o.ReserveAmount, o.ExpiresAt, o.Status, nilStr(o.AcceptedBy), nilTime(o.AcceptedAt), o.CreatedAt)
	return err
}

// UpdateOffer writes status/acceptance.
func (r *Repository) UpdateOffer(ctx context.Context, tx Querier, o *model.Offer) error {
	_, err := r.q(tx).Exec(ctx, `UPDATE offers SET status=$2, accepted_by=$3, accepted_at=$4 WHERE id=$1`, o.ID, o.Status, nilStr(o.AcceptedBy), nilTime(o.AcceptedAt))
	return err
}

// LatestOffer returns the newest offer for an invoice (nil when none).
func (r *Repository) LatestOffer(ctx context.Context, tx Querier, invoiceID uuid.UUID) (*model.Offer, error) {
	o, err := scanOffer(r.q(tx).QueryRow(ctx, `SELECT `+offerCols+` FROM offers WHERE invoice_id=$1 ORDER BY created_at DESC LIMIT 1`, invoiceID))
	var nf *cerrors.NotFoundError
	if errors.As(err, &nf) {
		return nil, nil
	}
	return o, err
}

// ─── Advances ────────────────────────────────────────────────────────────────

const advCols = `id, invoice_id, offer_id, provider, core_ref, core_customer_id, disbursed_at, due_date, principal, fees_accrued, repaid, fees_repaid, reserve_amount, reserve_released, status, created_at, updated_at`

func scanAdvance(row pgx.Row) (*model.Advance, error) {
	var a model.Advance
	var ref, cust *string
	if err := row.Scan(&a.ID, &a.InvoiceID, &a.OfferID, &a.Provider, &ref, &cust, &a.DisbursedAt, &a.DueDate, &a.Principal, &a.FeesAccrued, &a.Repaid, &a.FeesRepaid, &a.ReserveAmount, &a.ReserveReleased, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, cerrors.NotFound("advance not found")
		}
		return nil, err
	}
	if ref != nil {
		a.CoreRef = *ref
	}
	if cust != nil {
		a.CoreCustomerID = *cust
	}
	return &a, nil
}

// CreateAdvance inserts the funded position.
func (r *Repository) CreateAdvance(ctx context.Context, tx Querier, a *model.Advance) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	now := time.Now().UTC()
	a.CreatedAt, a.UpdatedAt = now, now
	_, err := r.q(tx).Exec(ctx, `INSERT INTO advances (`+advCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		a.ID, a.InvoiceID, a.OfferID, a.Provider, nilStr(a.CoreRef), nilStr(a.CoreCustomerID), nilTime(a.DisbursedAt), a.DueDate, a.Principal, a.FeesAccrued, a.Repaid, a.FeesRepaid, a.ReserveAmount, a.ReserveReleased, a.Status, a.CreatedAt, a.UpdatedAt)
	return err
}

// UpdateAdvance writes the mutable columns.
func (r *Repository) UpdateAdvance(ctx context.Context, tx Querier, a *model.Advance) error {
	a.UpdatedAt = time.Now().UTC()
	_, err := r.q(tx).Exec(ctx, `UPDATE advances SET core_ref=$2, repaid=$3, fees_repaid=$4, reserve_released=$5, status=$6, fees_accrued=$7, updated_at=$8 WHERE id=$1`,
		a.ID, nilStr(a.CoreRef), a.Repaid, a.FeesRepaid, a.ReserveReleased, a.Status, a.FeesAccrued, a.UpdatedAt)
	return err
}

// AdvanceByInvoice returns the advance (nil when none). forUpdate locks the row.
func (r *Repository) AdvanceByInvoice(ctx context.Context, tx Querier, invoiceID uuid.UUID, forUpdate bool) (*model.Advance, error) {
	sql := `SELECT ` + advCols + ` FROM advances WHERE invoice_id=$1`
	if forUpdate {
		sql += " FOR UPDATE"
	}
	a, err := scanAdvance(r.q(tx).QueryRow(ctx, sql, invoiceID))
	var nf *cerrors.NotFoundError
	if errors.As(err, &nf) {
		return nil, nil
	}
	return a, err
}

// OverdueCandidates returns ACTIVE advances past due_date + grace days.
func (r *Repository) OverdueCandidates(ctx context.Context, graceDays int) ([]model.Advance, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+advCols+` FROM advances WHERE status='ACTIVE' AND due_date + ($1 * interval '1 day') < current_date`, graceDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Advance
	for rows.Next() {
		a, err := scanAdvance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// ─── Payments ────────────────────────────────────────────────────────────────

const payCols = `id, invoice_id, advance_id, amount, applied_fees, applied_principal, released_reserve, reference, payer, source, core_ref, created_by, created_at`

// CreatePayment inserts a payment; returns Conflict on a duplicate reference.
func (r *Repository) CreatePayment(ctx context.Context, tx Querier, p *model.Payment) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	p.CreatedAt = time.Now().UTC()
	_, err := r.q(tx).Exec(ctx, `INSERT INTO invoice_payments (`+payCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		p.ID, p.InvoiceID, nilUUID(p.AdvanceID), p.Amount, p.AppliedFees, p.AppliedPrincipal, p.ReleasedReserve, nilStr(p.Reference), nilStr(p.Payer), p.Source, nilStr(p.CoreRef), nilStr(p.CreatedBy), p.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return cerrors.Conflict("payment with this reference already recorded")
	}
	return err
}

// PaymentsByInvoice lists payments oldest first.
func (r *Repository) PaymentsByInvoice(ctx context.Context, invoiceID uuid.UUID) ([]model.Payment, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+payCols+` FROM invoice_payments WHERE invoice_id=$1 ORDER BY created_at`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Payment{}
	for rows.Next() {
		var p model.Payment
		var ref, payer, core, by *string
		if err := rows.Scan(&p.ID, &p.InvoiceID, &p.AdvanceID, &p.Amount, &p.AppliedFees, &p.AppliedPrincipal, &p.ReleasedReserve, &ref, &payer, &p.Source, &core, &by, &p.CreatedAt); err != nil {
			return nil, err
		}
		d := func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		}
		p.Reference, p.Payer, p.CoreRef, p.CreatedBy = d(ref), d(payer), d(core), d(by)
		out = append(out, p)
	}
	return out, rows.Err()
}

// ─── Decisions ───────────────────────────────────────────────────────────────

const decCols = `id, invoice_id, tenant_id, policy_id, policy_version, policy_hash, inputs, outcome, detail, reasons, actor_type, actor_id, parent_id, latency_ms, decided_at`

// CreateDecision stores a decision record.
func (r *Repository) CreateDecision(ctx context.Context, tx Querier, d *model.Decision) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	if d.DecidedAt.IsZero() {
		d.DecidedAt = time.Now().UTC()
	}
	_, err := r.q(tx).Exec(ctx, `INSERT INTO decisions (`+decCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		d.ID, d.InvoiceID, d.TenantID, d.PolicyID, d.PolicyVersion, d.PolicyHash, jsonb(d.Inputs), d.Outcome, jsonb(d.Detail), jsonb(d.Reasons), d.ActorType, d.ActorID, nilUUID(d.ParentID), d.LatencyMS, d.DecidedAt)
	return err
}

// LatestDecision returns the newest decision for an invoice (nil when none).
func (r *Repository) LatestDecision(ctx context.Context, tx Querier, invoiceID uuid.UUID) (*model.Decision, error) {
	var d model.Decision
	var inputs, detail, reasons []byte
	var latency *decimal.Decimal
	err := r.q(tx).QueryRow(ctx, `SELECT `+decCols+` FROM decisions WHERE invoice_id=$1 ORDER BY decided_at DESC LIMIT 1`, invoiceID).
		Scan(&d.ID, &d.InvoiceID, &d.TenantID, &d.PolicyID, &d.PolicyVersion, &d.PolicyHash, &inputs, &d.Outcome, &detail, &reasons, &d.ActorType, &d.ActorID, &d.ParentID, &latency, &d.DecidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(inputs, &d.Inputs)
	_ = json.Unmarshal(detail, &d.Detail)
	_ = json.Unmarshal(reasons, &d.Reasons)
	if latency != nil {
		d.LatencyMS, _ = latency.Float64()
	}
	return &d, nil
}

// ─── Ledger ──────────────────────────────────────────────────────────────────

const ledgerCols = `seq, tenant_id, invoice_id, type, payload, actor, occurred_at, prev_hash, hash, sig, anchor_id`

func scanLedger(rows pgx.Rows) (*model.LedgerEvent, error) {
	var ev model.LedgerEvent
	var payload []byte
	if err := rows.Scan(&ev.Seq, &ev.TenantID, &ev.InvoiceID, &ev.Type, &payload, &ev.Actor, &ev.OccurredAt, &ev.PrevHash, &ev.Hash, &ev.Sig, &ev.AnchorID); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(payload, &ev.Payload)
	if ev.Payload == nil {
		ev.Payload = map[string]any{}
	}
	ev.PrevHash, ev.Hash, ev.Sig = strings.TrimSpace(ev.PrevHash), strings.TrimSpace(ev.Hash), strings.TrimSpace(ev.Sig)
	ev.OccurredAt = ev.OccurredAt.UTC()
	return &ev, nil
}

// AppendEvent links and inserts one ledger event inside tx. The tenant chain
// is serialised with a transaction-scoped advisory lock.
func (r *Repository) AppendEvent(ctx context.Context, tx pgx.Tx, ev *model.LedgerEvent) error {
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now()
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('ledger_events:' || $1))`, ev.TenantID); err != nil {
		return err
	}
	var prev *string
	if err := tx.QueryRow(ctx, `SELECT hash FROM ledger_events WHERE tenant_id=$1 ORDER BY seq DESC LIMIT 1`, ev.TenantID).Scan(&prev); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	prevHash := ""
	if prev != nil {
		prevHash = strings.TrimSpace(*prev)
	}
	if err := ledger.Link(ev, prevHash, r.hmacKey); err != nil {
		return err
	}
	return tx.QueryRow(ctx, `INSERT INTO ledger_events (tenant_id, invoice_id, type, payload, actor, occurred_at, prev_hash, hash, sig)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING seq`,
		ev.TenantID, nilUUID(ev.InvoiceID), ev.Type, jsonb(ev.Payload), ev.Actor, ev.OccurredAt, ev.PrevHash, ev.Hash, ev.Sig).Scan(&ev.Seq)
}

// EventsByInvoice lists an invoice's events in chain order.
func (r *Repository) EventsByInvoice(ctx context.Context, invoiceID uuid.UUID) ([]model.LedgerEvent, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+ledgerCols+` FROM ledger_events WHERE invoice_id=$1 ORDER BY seq`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LedgerEvent{}
	for rows.Next() {
		ev, err := scanLedger(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ev)
	}
	return out, rows.Err()
}

// EventsRange lists a tenant's events with seq in [from, to] (chain verification).
func (r *Repository) EventsRange(ctx context.Context, tenantID string, from, to int64) ([]model.LedgerEvent, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+ledgerCols+` FROM ledger_events WHERE tenant_id=$1 AND seq BETWEEN $2 AND $3 ORDER BY seq`, tenantID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.LedgerEvent
	for rows.Next() {
		ev, err := scanLedger(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ev)
	}
	return out, rows.Err()
}

// UnanchoredEvents returns a tenant's unanchored events in chain order.
func (r *Repository) UnanchoredEvents(ctx context.Context, tx Querier, tenantID string, limit int) ([]model.LedgerEvent, error) {
	rows, err := r.q(tx).Query(ctx, `SELECT `+ledgerCols+` FROM ledger_events WHERE tenant_id=$1 AND anchor_id IS NULL ORDER BY seq LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.LedgerEvent
	for rows.Next() {
		ev, err := scanLedger(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ev)
	}
	return out, rows.Err()
}

// TenantsWithUnanchored lists tenants that have unanchored events.
func (r *Repository) TenantsWithUnanchored(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT tenant_id FROM ledger_events WHERE anchor_id IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateAnchor inserts the anchor and stamps the covered events.
func (r *Repository) CreateAnchor(ctx context.Context, tx pgx.Tx, a *model.Anchor) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.AnchoredAt.IsZero() {
		a.AnchoredAt = time.Now().UTC()
	}
	if _, err := tx.Exec(ctx, `INSERT INTO anchors (id, tenant_id, merkle_root, from_seq, to_seq, event_count, method, receipt, anchored_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		a.ID, a.TenantID, a.MerkleRoot, a.FromSeq, a.ToSeq, a.EventCount, a.Method, a.Receipt, a.AnchoredAt); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE ledger_events SET anchor_id=$1 WHERE tenant_id=$2 AND anchor_id IS NULL AND seq BETWEEN $3 AND $4`, a.ID, a.TenantID, a.FromSeq, a.ToSeq)
	return err
}

// ListAnchors lists a tenant's anchors newest first.
func (r *Repository) ListAnchors(ctx context.Context, tenantID string, limit int) ([]model.Anchor, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, tenant_id, merkle_root, from_seq, to_seq, event_count, method, receipt, anchored_at FROM anchors WHERE tenant_id=$1 ORDER BY anchored_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Anchor{}
	for rows.Next() {
		var a model.Anchor
		if err := rows.Scan(&a.ID, &a.TenantID, &a.MerkleRoot, &a.FromSeq, &a.ToSeq, &a.EventCount, &a.Method, &a.Receipt, &a.AnchoredAt); err != nil {
			return nil, err
		}
		a.MerkleRoot = strings.TrimSpace(a.MerkleRoot)
		out = append(out, a)
	}
	return out, rows.Err()
}

// AnchorForInvoice returns the newest anchor covering any of the invoice's events.
func (r *Repository) AnchorForInvoice(ctx context.Context, invoiceID uuid.UUID) (*model.Anchor, error) {
	var a model.Anchor
	err := r.pool.QueryRow(ctx, `SELECT a.id, a.tenant_id, a.merkle_root, a.from_seq, a.to_seq, a.event_count, a.method, a.receipt, a.anchored_at
		FROM anchors a JOIN ledger_events e ON e.anchor_id = a.id WHERE e.invoice_id=$1 ORDER BY a.anchored_at DESC LIMIT 1`, invoiceID).
		Scan(&a.ID, &a.TenantID, &a.MerkleRoot, &a.FromSeq, &a.ToSeq, &a.EventCount, &a.Method, &a.Receipt, &a.AnchoredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.MerkleRoot = strings.TrimSpace(a.MerkleRoot)
	return &a, nil
}

// ─── Policies (policy.Store) ─────────────────────────────────────────────────

// LatestPolicy returns the highest stored version for (tenant, policy).
func (r *Repository) LatestPolicy(ctx context.Context, tenantID, policyID string) (int, string, string, string, time.Time, bool, error) {
	var version int
	var hash, doc string
	var by *string
	var at time.Time
	err := r.pool.QueryRow(ctx, `SELECT version, hash, yaml, created_by, created_at FROM invoice_policies WHERE tenant_id=$1 AND policy_id=$2 ORDER BY version DESC LIMIT 1`, tenantID, policyID).
		Scan(&version, &hash, &doc, &by, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", "", "", time.Time{}, false, nil
	}
	if err != nil {
		return 0, "", "", "", time.Time{}, false, err
	}
	createdBy := ""
	if by != nil {
		createdBy = *by
	}
	return version, hash, doc, createdBy, at, true, nil
}

// SavePolicy stores a new version.
func (r *Repository) SavePolicy(ctx context.Context, tenantID, policyID string, version int, hash, yamlDoc, createdBy string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO invoice_policies (id, tenant_id, policy_id, version, hash, yaml, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		uuid.New(), tenantID, policyID, version, hash, yamlDoc, nilStr(createdBy))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return cerrors.Conflict("policy version already exists; reload and retry")
	}
	return err
}

// ─── Stats / queue ───────────────────────────────────────────────────────────

// Stats aggregates the tenant portfolio.
func (r *Repository) Stats(ctx context.Context, tenantID string) (*model.Stats, error) {
	s := &model.Stats{Counts: map[string]int64{}, ByBuyerTier: map[string]model.TierStat{}, DecisionCounts: map[string]int64{},
		FundedTotal: decimal.Zero, Outstanding: decimal.Zero, OverdueTotal: decimal.Zero, FeesEarned: decimal.Zero}
	rows, err := r.pool.Query(ctx, `SELECT status, count(*) FROM invoices WHERE tenant_id=$1 GROUP BY status`, tenantID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var st string
		var n int64
		if err := rows.Scan(&st, &n); err != nil {
			rows.Close()
			return nil, err
		}
		s.Counts[st] = n
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `SELECT decision_outcome, count(*) FROM invoices WHERE tenant_id=$1 AND decision_outcome IS NOT NULL GROUP BY decision_outcome`, tenantID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var o string
		var n int64
		if err := rows.Scan(&o, &n); err != nil {
			rows.Close()
			return nil, err
		}
		s.DecisionCounts[o] = n
	}
	rows.Close()
	var avgTenor *float64
	if err := r.pool.QueryRow(ctx, `SELECT coalesce(sum(a.principal),0), coalesce(sum(a.principal - a.repaid + a.fees_accrued - a.fees_repaid) FILTER (WHERE a.status IN ('ACTIVE','OVERDUE')),0),
		coalesce(sum(a.principal - a.repaid) FILTER (WHERE a.status='OVERDUE'),0), count(*) FILTER (WHERE a.status='OVERDUE'),
		avg(o.tenor_days), coalesce(sum(a.fees_repaid),0)
		FROM advances a JOIN invoices i ON i.id=a.invoice_id JOIN offers o ON o.id=a.offer_id WHERE i.tenant_id=$1`, tenantID).
		Scan(&s.FundedTotal, &s.Outstanding, &s.OverdueTotal, &s.OverdueCount, &avgTenor, &s.FeesEarned); err != nil {
		return nil, err
	}
	if avgTenor != nil {
		s.AvgTenorDays = *avgTenor
	}
	rows, err = r.pool.Query(ctx, `SELECT coalesce(b.tier,'UNRATED'), count(*), coalesce(sum(a.principal),0),
		coalesce(sum(a.principal - a.repaid) FILTER (WHERE a.status IN ('ACTIVE','OVERDUE')),0)
		FROM advances a JOIN invoices i ON i.id=a.invoice_id LEFT JOIN buyers b ON b.id=i.buyer_id WHERE i.tenant_id=$1 GROUP BY 1`, tenantID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var tier string
		var ts model.TierStat
		if err := rows.Scan(&tier, &ts.Count, &ts.FundedTotal, &ts.Outstanding); err != nil {
			rows.Close()
			return nil, err
		}
		s.ByBuyerTier[tier] = ts
	}
	rows.Close()
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM invoices WHERE tenant_id=$1 AND (status='DECIDED' OR (status='ACCEPTED' AND status_reason='AWAITING_SECOND_APPROVAL'))`, tenantID).Scan(&s.ReviewQueue); err != nil {
		return nil, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM anchors WHERE tenant_id=$1`, tenantID).Scan(&s.Anchors); err != nil {
		return nil, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM buyers WHERE tenant_id=$1`, tenantID).Scan(&s.Buyers); err != nil {
		return nil, err
	}
	return s, nil
}

// QueueInvoices returns invoices awaiting a human money gate.
func (r *Repository) QueueInvoices(ctx context.Context, tenantID string) ([]model.Invoice, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE tenant_id=$1 AND
		(status='DECIDED' OR (status='ACCEPTED' AND status_reason='AWAITING_SECOND_APPROVAL')) ORDER BY updated_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Invoice{}
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}
