# Nemo Invoice Finance — build spec (v1, 8 Nov 2026)

Goal: a country-neutral, fully automated invoice-financing module inside the Nemo platform.
Humans act only at money gates (REFER/FLAG review, disbursement above limit, write-off/recourse).
Core banking sits behind an adapter: Nemo's own services are the default implementation; a bank can
plug its own core in by implementing the same interface (HTTP webhook adapter provided).

Reference inventory and reasoning: https://claude.ai/artifact/BhuPECMTdoryRyYxhS9m5D

## 1. Components

| Component | Where | Owner agent |
|---|---|---|
| `invoice-service` (Go, Chi) | `go-services/cmd/invoice-service`, `go-services/internal/invoice/**`, `go-services/migrations/invoice/` | A |
| `invoice.advance` decision policy | `go-services/internal/common/decision/policies/invoice.advance.ke.yaml` | A |
| Core-banking adapter | `go-services/internal/invoice/corebanking/{provider.go,nemo.go,webhook.go}` | A |
| Invoice OCR profile + endpoint | `ekyc-ml-service/engine/invoice.py`, `api/document.py` (`POST /v1/invoice/extract`) | B |
| Portal pages | `lms-portal-ui/src/pages/invoice-finance/**`, `src/services/invoiceService.ts`, routes + sidebar | C |
| Public pages (no login) | `lms-portal-ui` routes `/confirm/:token`, `/verify/:fingerprint` | C |
| Wiring | gateway route, `docker-compose.go.yml`, helm `values.yaml`, `deploy/k8s/contabo/lms-nemo.yaml`, CI matrix, `scripts/build-nemo-images.sh` | A |

Ports: invoice-service **8101**, DB `athena_invoice`. Gateway prefix `/lms/api/v1/invoices/`,
`/lms/api/v1/buyers/`, `/lms/api/v1/invoice-policy/`, and **public** `/lms/api/public/invoices/`.

## 2. Domain

```
Invoice
  id (uuid), tenant_id, seller_customer_id (string, account-service customer),
  fingerprint (hex sha256 of pdf bytes), fields_hash (sha256 of canonical fields),
  media_id (media-service id), invoice_number, issue_date, due_date, currency, subtotal, vat, total,
  seller{name, tax_id, email}, buyer_id (uuid -> Buyer), buyer_snapshot{name, tax_id, email, phone},
  line_items jsonb [{description, qty, unit_price, amount}],
  ocr jsonb {engine, confidence per field, raw_word_count},
  requested_amount, status, status_reason, created_at, updated_at
Status: UPLOADED → EXTRACTED → AWAITING_BUYER → BUYER_CONFIRMED | BUYER_DISPUTED
        → SCORED → DECIDED(APPROVE|REFER|DECLINE) → OFFERED → ACCEPTED → FUNDED → PARTIALLY_PAID → PAID
        | OVERDUE | DECLINED | CANCELLED
Buyer
  id, tenant_id, name, tax_id (unique per tenant), email_domain, contacts jsonb, tier (A|B|C|D|UNRATED),
  limit_amount, exposure_amount, on_time_rate, confirmations_count, created_at
BuyerConfirmation
  id, invoice_id, token_hash, expires_at, sent_to, otp_hash, otp_expires_at,
  action (CONFIRMED|DISPUTED|ALREADY_PAID), dispute_reason, acted_at, ip, user_agent
Offer
  id, invoice_id, advance_rate, advance_amount, discount_fee_per_day, tenor_days, fee_amount,
  processing_fee, net_disbursement, reserve_amount, expires_at, status (OFFERED|ACCEPTED|EXPIRED|WITHDRAWN)
Advance (funded position)
  id, invoice_id, offer_id, core_ref (loan id in core banking), disbursed_at, due_date,
  principal, fees_accrued, repaid, reserve_released, status (ACTIVE|PAID|OVERDUE|DILUTED|WRITTEN_OFF)
Decision (store what common/decision returns + inputs used)
LedgerEvent (hash chain)
  seq bigserial, invoice_id, type, payload jsonb, actor, occurred_at,
  prev_hash, hash = sha256(prev_hash || canonical_json(type,payload,actor,occurred_at)), sig (HMAC-SHA256 with
  service key for v1; ed25519 later), anchor_id nullable
Anchor
  id, merkle_root, from_seq, to_seq, method (OTS|NONE), receipt bytea, anchored_at
```

Every status transition writes exactly one LedgerEvent in the same DB transaction.
Event types: `invoice.uploaded, invoice.extracted, invoice.duplicate_rejected, buyer.notified, buyer.confirmed,
buyer.disputed, invoice.scored, invoice.fraud_checked, invoice.decided, offer.created, offer.accepted,
advance.funded, payment.received, reserve.released, advance.overdue, advance.written_off, review.approved,
review.rejected, policy.changed`.

## 3. Pipeline (automatic, runs on upload; each step idempotent and resumable)

1. **Upload** `POST /api/v1/invoices` multipart: `file`, `sellerCustomerId`, `requestedAmount?`, `buyerEmail?`.
   Store file in media-service (`mediaType=INVOICE`, `referenceId=invoice.id`). Compute fingerprint.
   Duplicate (same fingerprint or same seller+invoice_number+buyer tax_id) → status DECLINED, reason DUPLICATE, event
   `invoice.duplicate_rejected`, HTTP 409.
2. **Extract** call ekyc-ml `POST /v1/invoice/extract` → fields+confidence. Fields under 0.6 confidence are flagged
   `needsSellerConfirmation` (returned in API so the seller UI asks the seller, never an operator).
   Upsert Buyer by tax_id (or normalised name+email domain). Status EXTRACTED.
3. **Buyer confirmation** create BuyerConfirmation token (32 random bytes, store sha256), send via notification-service
   email (template `invoice.buyer.confirm`, link `${PUBLIC_PORTAL_URL}/confirm/{token}`), OTP via SMS when phone known.
   Status AWAITING_BUYER. Reminder at +2d, +4d; auto DECLINE at +5d reason BUYER_NO_RESPONSE.
   Public: `GET /api/public/invoices/confirm/{token}` → invoice summary (no PII beyond buyer-visible fields);
   `POST /api/public/invoices/confirm/{token}` `{action, otp?, disputeReason?}` → event, status.
   In dev/demo (`DEMO_MODE=true`) OTP is `000000` and the confirm link is returned in the upload response.
4. **Score** on BUYER_CONFIRMED: call ai-scoring `POST /api/v1/scoring/requests` for the seller (or reuse latest
   `GET /customers/{id}/latest` if < 30d); compute buyer tier from Buyer stats (UNRATED→C for v1); invoice score from
   aging, amount vs seller turnover, confidence. Status SCORED.
5. **Fraud** call fraud-detection `POST /api/v1/fraud/evaluate` `{eventType: INVOICE_FINANCE_REQUEST, amount, customerId}`
   plus local rules: duplicate seller/buyer bank account, buyer registered < 90d, amount > 3× seller 6-month average.
6. **Decide** evaluate `invoice.advance` policy via `common/decision` with inputs
   `{seller_band, buyer_tier, invoice_score, days_to_due, amount, fraud_flags, buyer_confirmed, concentration_pct}`.
   Policy outputs APPROVE/REFER/DECLINE + detail `{advance_rate, fee_per_day, max_amount}`.
   Record Decision + event. APPROVE → create Offer automatically → status OFFERED. REFER → status DECIDED, appears in review
   queue. DECLINE → DECLINED.
7. **Accept** `POST /api/v1/invoices/{id}/offer/accept` (seller or officer on behalf) → ACCEPTED.
8. **Fund** core-banking adapter `Disburse(ctx, FundingRequest)` → Advance; status FUNDED. Above
   `DISBURSEMENT_SECOND_APPROVAL_ABOVE` (default 500000) requires `POST /{id}/approve` by MANAGER first (maker-checker).
9. **Settle** `POST /api/v1/invoices/{id}/payments` `{amount, reference, payer}` (also consumed from `payment.completed`
   events carrying `invoiceId`). Apply to fees then principal; surplus → reserve release to seller via adapter
   `Credit(...)`. PAID when principal+fees covered. Daily job marks OVERDUE after due_date+grace.

## 4. Core-banking adapter

```go
type Provider interface {
    Name() string
    EnsureCustomer(ctx, CustomerRef) (CoreCustomerID, error)      // map seller to core customer
    Disburse(ctx, FundingRequest) (FundingResult, error)           // create & disburse the advance
    RecordRepayment(ctx, RepaymentRequest) (RepaymentResult, error)
    Credit(ctx, CreditRequest) (TxnRef, error)                     // reserve release / refunds
    Position(ctx, coreRef string) (Position, error)                // balance, status
}
```
- `nemo`: EnsureCustomer → account-service; Disburse → loan-origination create (productCode `INVOICE_FINANCE`,
  tenor days, amount = advance) → approve (service key) → disburse to seller account; RecordRepayment → payment-service
  LOAN_REPAYMENT; Credit → account-service credit; Position → loan-management.
  If product `INVOICE_FINANCE` is missing, create it from template on startup (SME_LOAN, FLAT, BULLET if available, fee UPFRONT %).
- `webhook`: POSTs signed JSON (HMAC `X-Signature`) to `COREBANKING_WEBHOOK_URL/{disburse|repayment|credit|position}` and
  expects a synchronous JSON reply; documented in `docs/nemo/invoice-finance/COREBANKING.md` so a bank can implement it.
- Selection: env `COREBANKING_PROVIDER=nemo|webhook`.

## 5. Policy `invoice.advance` v1 (market KE, tenant *)

```yaml
rules:
  - id: IF-GATE   when: buyer_confirmed == false        -> DECLINE  reason BUYER_NOT_CONFIRMED
  - id: IF-FRAUD  when: fraud_flags > 0                 -> REFER    reason FRAUD_FLAG
  - id: IF-TENOR  when: days_to_due < 7 or > 120        -> DECLINE  reason TENOR_OUT_OF_RANGE
  - id: IF-CONC   when: concentration_pct > 40          -> REFER    reason BUYER_CONCENTRATION
  - id: IF-BAND   table by seller_band × buyer_tier:
        A/A..B: advance 0.90 fee 0.0005/day APPROVE ; A/C: 0.80 0.0007 APPROVE ; A/D: 0.70 0.0009 REFER
        B/*: 0.80/0.0007 APPROVE (D→REFER) ; C/*: 0.70/0.0009 (A,B APPROVE, C REFER, D DECLINE)
        D/*: REFER (A,B) else DECLINE
```
Processing fee 1% of advance, min 500. Reserve = total − advance.

## 6. HTTP API (JWT, roles) — all under `/api/v1`

```
POST   /invoices                          OFFICER+   multipart upload (starts pipeline)
GET    /invoices?status&sellerId&buyerId&page&size
GET    /invoices/{id}                     full record + offer + advance + decision
GET    /invoices/{id}/timeline            ledger events (verified: chainOk bool)
POST   /invoices/{id}/resend-confirmation
POST   /invoices/{id}/offer/accept
POST   /invoices/{id}/approve             MANAGER    (money gate: REFER→APPROVE or 2nd approval) {note}
POST   /invoices/{id}/reject              MANAGER    {reason}
POST   /invoices/{id}/fund                OFFICER+   triggers adapter (auto after accept unless gate)
POST   /invoices/{id}/payments            OFFICER+   {amount, reference, payer}
GET    /invoices/queue                    REFER/FLAG items with evidence
GET    /invoices/stats                    totals: funded, outstanding, overdue, avg tenor, by buyer tier
GET    /buyers, /buyers/{id}, PATCH /buyers/{id} {tier, limitAmount}
GET    /invoice-policy                    current policy yaml + version + hash
PUT    /invoice-policy                    ADMIN      new yaml → validates, bumps version, event policy.changed
GET    /invoices/anchors                  anchor list
Public (no JWT, rate-limited):
GET    /public/invoices/confirm/{token}
POST   /public/invoices/confirm/{token}
GET    /public/invoices/verify/{fingerprint}   → {known, status (coarse), buyerConfirmedAt, fundedAt?, anchoredAt?, chainOk}
POST   /public/invoices/verify                  multipart file → computes fingerprint → same as above
```

## 7. Portal (lms-portal-ui) — Nemo deep-water tokens, shadcn

Sidebar section **Invoice Finance**: Overview (stats + funnel), Review Queue, Invoices, Buyers, Policy, Anchors.
Invoice detail: header (status pill, fingerprint short + copy, verify link/QR), left: extracted fields with confidence
bars + PDF preview (media download), right: decision card (outcome, reasons, policy hash), offer card, advance card,
buyer card; bottom: timeline from ledger with chain-OK badge; actions by role.
"New invoice" dialog (officer on behalf of seller): file + seller select + buyer email; shows pipeline progress live
(poll every 2s) and in demo mode shows the buyer confirm link.
Public `/confirm/:token`: branded, invoice summary, Confirm / Dispute / Already paid, OTP field.
Public `/verify/:fingerprint` and `/verify` (drop a PDF): status card.
Replace lms-portal-ui `index.css` tokens with Nemo deep-water set (see artifact section 2) — whole portal, not just new pages.

## 8. Demo path (what "end to end" means)

1. Login as officer → New invoice → upload sample PDF (`docs/nemo/invoice-finance/samples/*.pdf`, generated by agent B).
2. Pipeline shows: extracted (fields+confidence) → buyer email sent (link shown in demo mode).
3. Open `/confirm/{token}` in another tab → OTP 000000 → Confirm.
4. Back in detail: scored → fraud ok → decided APPROVE → offer created.
5. Accept offer → funded via Nemo adapter → loan visible in Loans page, GL posting in Ledger page.
6. Record buyer payment → PAID, reserve released. Timeline shows the full chain with chain-OK; verify page shows public status.
7. Second invoice with same PDF → 409 duplicate; third with buyer tier D → REFER → appears in Review Queue → manager approves.

## 9. Conventions

Follow overdraft-service as the template (config, db.MigrateGate, outbox relay, metrics, tracing, auth middleware,
handler/service/repository/model layout, zap). Decimal via shopspring. Tests: unit tests for fingerprint, chain
verification, policy evaluation, state machine; a Python test for the OCR profile. Commit per component with the
project convention and push (CLAUDE.md).
