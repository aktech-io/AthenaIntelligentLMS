# Invoice Finance — core-banking adapter (v1)

invoice-service never books money itself. Every money movement goes through
`corebanking.Provider` (`go-services/internal/invoice/corebanking/provider.go`):

```go
type Provider interface {
    Name() string
    EnsureCustomer(ctx, CustomerRef) (CoreCustomerID, error)   // map the seller to a core customer
    Disburse(ctx, FundingRequest) (FundingResult, error)        // create & disburse the advance
    RecordRepayment(ctx, RepaymentRequest) (RepaymentResult, error)
    Credit(ctx, CreditRequest) (TxnRef, error)                  // reserve release / refunds
    Position(ctx, coreRef string) (Position, error)             // balance, status
}
```

Selection: `COREBANKING_PROVIDER=nemo` (default) or `webhook`.

| Step in SPEC §3 | Provider call | When |
|---|---|---|
| 8 Fund | `EnsureCustomer` then `Disburse` | invoice ACCEPTED (and past the second-approval gate) |
| 9 Settle | `RecordRepayment` for fees + principal; `Credit` for the surplus released to the seller | each `POST /invoices/{id}/payments` or `payment.completed` with `externalReference = invoice:<id>[:ref]` |
| reconciliation | `Position` | on demand (not called by the pipeline in v1) |

All calls are synchronous and bounded by the pipeline step timeout
(`PIPELINE_FUND_TIMEOUT`, default 90s). A transport/5xx failure is returned as
`*corebanking.ErrUnavailable`; the step is retried by the background loop with
exponential backoff. A 4xx business rejection is not retried.

---

## 1. `nemo` provider (default)

Implementation: `corebanking/nemo.go`. Uses Nemo's own services with the
`X-Service-Key` service identity.

| Operation | Nemo calls |
|---|---|
| `EnsureCustomer` | account-service `GET /api/v1/customers/by-customer-id/{sellerCustomerId}`; on 404 `POST /api/v1/customers` (`customerType=BUSINESS`, `customerId` = the seller id). |
| `Disburse` | account-service `GET /api/v1/accounts/customer/{id}` → pick the first `WALLET`, else first ACTIVE account, else `POST /api/v1/accounts` (CURRENT). Then loan-origination `POST /api/v1/loan-applications` (`productId` = INVOICE_FINANCE, `requestedAmount` = advance, `tenorMonths` = see below) → `/submit` → `/review/start` → `/review/approve` (`approvedAmount` = advance, `interestRate` = fee/day × 365 × 100) → `/disburse` (`disbursedAmount` = **net disbursement** = advance − discount fee − processing fee, `disbursementAccount` = the account UUID). Origination credits the account and publishes `loan.disbursed`; loan-management creates the loan asynchronously. The provider polls `GET /api/v1/loans/customer/{id}` (~3s) for the loan whose `applicationId` matches; `coreRef` is the loan id, or `app:<applicationId>` until the loan exists (resolved lazily by `Position`/`RecordRepayment`). |
| `RecordRepayment` | payment-service `POST /api/v1/payments` (`paymentType=LOAN_REPAYMENT`, `paymentChannel=INTERNAL`, `loanId`, `externalReference` = `invadv:<invoiceId>:<ref>`) then `POST /api/v1/payments/{id}/complete`. loan-management applies it from `payment.completed`. |
| `Credit` | account-service `POST /api/v1/accounts/{sellerAccount}/credit` (`channel=INVOICE_FINANCE`, `idempotencyKey` = `invres:<invoiceId>:<ref>`). |
| `Position` | loan-management `GET /api/v1/loans/{id}` → `status`, `totalOutstanding`, `dpd`, `maturityDate`. |

### Maker-checker

loan-origination enforces approver ≠ creator and disburser ≠ approver ≠
creator. The provider therefore acts under three service identities
(`X-Service-User`): `invoice-service-maker` (create/submit),
`invoice-service-checker` (review/approve), `invoice-service-disburser`
(disburse). They all authenticate with the same `LMS_INTERNAL_SERVICE_KEY`.

### Product bootstrap

On startup (and again on first funding if startup failed) the provider lists
product-service for `productCode = INVOICE_FINANCE`. If missing it creates
and activates it as the `invoice-service-admin` identity:

```
productType SME_LOAN, scheduleType FLAT_RATE, repaymentFrequency MONTHLY,
nominalRate 25.55 (% p.a. ≈ 0.07%/day), minAmount 1000, maxAmount 100,000,000,
minTenorDays 7, maxTenorDays 120, processingFeeRate 0, penaltyRate 0, gracePeriodDays 5
```

### Limitations of the Nemo core (documented, not worked around)

* **Monthly schedules only.** loan-management supports `EMI` and `FLAT_RATE`
  with DAILY/WEEKLY/BIWEEKLY/MONTHLY frequencies and sets the first repayment
  at disbursement + 1 month; `BULLET` is accepted by product-service but not
  scheduled. Invoice tenors are days, so the provider books
  `tenorMonths = 1` when tenor ≤ 30 days and otherwise rounds **up** to whole
  months (`corebanking.TenorMonths`). The core's schedule is therefore an
  approximation; the authoritative due date, fees and reserve live in
  invoice-service (`advances` table) and the ledger.
* **Interest vs. discount fee.** The discount fee is charged by invoice-service
  (deducted from the disbursement). The loan is still booked at a nominal
  rate (`fee/day × 365`) so the core's interest accrual roughly mirrors it;
  small differences between the core's outstanding and the advance's
  outstanding are expected and reconciled by `Position`.
* **Fees.** The product carries no fees (`processingFeeRate 0`) so origination
  credits exactly the net disbursement we pass; the processing fee is
  recorded in invoice-service only.
* **Loan id is asynchronous.** If the loan has not appeared within the poll
  window, `coreRef` stays `app:<applicationId>` and is resolved on the next
  `RecordRepayment`/`Position`.

---

## 2. `webhook` provider (bring your own core)

Implementation: `corebanking/webhook.go`. Set

```
COREBANKING_PROVIDER=webhook
COREBANKING_WEBHOOK_URL=https://core.bank.example/nemo      # base URL
COREBANKING_WEBHOOK_SECRET=<shared secret>
COREBANKING_WEBHOOK_TIMEOUT=20s                              # optional
```

Every operation is one `POST` of JSON to `COREBANKING_WEBHOOK_URL/<op>` and
expects a **synchronous** JSON reply.

### Request headers

| Header | Value |
|---|---|
| `Content-Type` | `application/json` |
| `X-Timestamp` | Unix seconds when the request was signed |
| `X-Idempotency-Key` | Stable per logical operation (e.g. `fund:<invoiceId>`); replaying it must return the same result, not a second disbursement |
| `X-Signature` | `sha256=` + hex(HMAC-SHA256(secret, `<X-Timestamp>` + `.` + raw body)) |
| `User-Agent` | `nemo-invoice-service/1` |

Verify with constant-time comparison and reject timestamps older than a few
minutes. Reference implementation: `corebanking.VerifySignature`.

### Operations

All amounts are decimal numbers (JSON numbers), currency ISO-4217, timestamps RFC 3339.

**`POST /customer`** — map/create the seller.
```json
{"tenantId":"default","customerId":"CUST-001","name":"Acme Ltd","taxId":"P051234567A","email":"ar@acme.co.ke","phone":"+2547..."}
→ {"coreCustomerId":"C-9981"}
```

**`POST /disburse`** — book and pay out the advance.
```json
{"tenantId":"default","invoiceId":"<uuid>","offerId":"<uuid>","idempotencyKey":"fund:<uuid>",
 "customer":{...CustomerRef...},"coreCustomerId":"C-9981","currency":"KES",
 "advanceAmount":90000,"netDisbursement":87075,"feeAmount":2025,"processingFee":900,"reserveAmount":10000,
 "tenorDays":45,"dueDate":"2026-12-23T00:00:00Z","feePerDay":0.0005,"invoiceNumber":"INV-001","buyerName":"Buyer Plc"}
→ {"coreRef":"LN-123","coreCustomerId":"C-9981","accountRef":"ACC-1","disbursedAt":"2026-11-08T09:00:00Z","raw":{...}}
```
`coreRef` is required; `disbursedAt` defaults to now when omitted. Pay the
seller **`netDisbursement`**; book **`advanceAmount`** as principal.

**`POST /repayment`** — a buyer payment applied to the advance.
```json
{"tenantId":"default","invoiceId":"<uuid>","coreRef":"LN-123","coreCustomerId":"C-9981","customerId":"CUST-001",
 "amount":50000,"currency":"KES","reference":"MPESA-XYZ","payer":"Buyer Plc","idempotencyKey":"invadv:<uuid>:MPESA-XYZ"}
→ {"txnRef":"TX-1","outstanding":42925,"closed":false}
```

**`POST /credit`** — pay the seller outside the loan (reserve release, refund).
```json
{"tenantId":"default","invoiceId":"<uuid>","coreCustomerId":"C-9981","customerId":"CUST-001","amount":7075,"currency":"KES",
 "reference":"RES-MPESA-XYZ","description":"Invoice INV-001 reserve release","idempotencyKey":"invres:<uuid>:MPESA-XYZ"}
→ {"txnRef":"TX-CR-1"}
```

**`POST /position`**
```json
{"coreRef":"LN-123"}
→ {"coreRef":"LN-123","status":"ACTIVE","principal":90000,"outstanding":42925,"dpd":0,"maturityAt":"2026-12-23T00:00:00Z"}
```
`status` ∈ `ACTIVE | CLOSED | OVERDUE | WRITTEN_OFF | UNKNOWN`.

### Error replies

* `2xx` with the documented body → success.
* `4xx` with `{"error":"human text","code":"MACHINE_CODE"}` → business
  rejection; invoice-service records the error and does **not** retry.
* `5xx` / `429` / network failure → treated as an outage and retried with
  backoff (the idempotency key makes the retry safe).

A minimal bank-side implementation (used as the test double) is in
`corebanking/webhook_test.go`.

---

## 3. Ledger and idempotency keys you will see

| Key | Meaning |
|---|---|
| `fund:<invoiceId>` | one disbursement per invoice |
| `invadv:<invoiceId>:<ref>` | one repayment per buyer payment reference |
| `invres:<invoiceId>:<ref>` | one reserve release per buyer payment reference |
| `cust:<tenant>:<customerId>` | customer mapping |

The hash-chained ledger (`ledger_events`) records `advance.funded`,
`payment.received` (with `coreTxnRef`) and `reserve.released` with the core
references returned by the provider, so a bank can reconcile its own books
against `GET /api/v1/invoices/{id}/timeline`.
