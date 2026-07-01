# Go-Live Functional & Compliance Audit — AthenaLMS

**Date:** 2026-07-02 · **Scope:** business-logic depth, money-path edge cases, product
coverage (device finance / overdraft / wallet / transfers / charges / taxes), and
multi-country regulatory portability (Kenya → Ethiopia without code changes).
**Method:** read-only code review of `go-services/` and `lms-portal-ui/`.
**Companion:** `GO_LIVE_HARDENING_AUDIT.md` (2026-06-29) covers security/infra; this
audit covers *what the system does with money* and found the functional layer is **not
go-live ready**. Several core revenue and repayment paths are wired up in config and
events but never actually execute.

**Finding counts:** BLOCKER 8 · HIGH 11 · MEDIUM 10 · Portability gaps 8.

---

## Executive summary

The platform looks feature-complete from the API surface (products with fees and
penalty rates, transaction charges with tiers, wallet + overdraft, transfers, payments,
collections with write-off workflow), but tracing the money paths end-to-end shows that
several of them are **facades**:

1. **Completed payments never reduce loan balances.** `payment.completed` is routed to
   the loan-management queue (`internal/common/rabbitmq/topology.go:89`) but the
   consumer ignores every event except `loan.disbursed`
   (`internal/management/consumer/consumer.go:59`). A borrower who pays via the payment
   service gets an SMS and a GL posting, but the loan stays outstanding, DPD keeps
   climbing, and collections escalates against a paid loan. Repayment only happens if
   someone separately calls the loans repayment API.
2. **Penalties never accrue.** `PenaltyRate`/`PenaltyGraceDays` exist on products but
   are referenced nowhere in the loan engine. `PenaltyDue` is set to zero at schedule
   generation (`internal/management/service/schedule.go:119`) and no job ever updates
   it. The repayment waterfall's "penalty first" step always allocates zero.
3. **Product fees are never charged.** `ProcessingFeeRate/Min/Max` and `ProductFee`
   rows are dead config: `FeeDue` is always zero in schedules, and origination applies
   no fee at disbursement (no fee/charge reference in
   `internal/origination/service/service.go`). The lender configured fee income it will
   never earn.
4. **Write-off is a dead end.** Collections publishes `writeoff.approved`
   (`internal/collections/event/publisher.go:79`) which nothing consumes; the
   `loan.written.off` event that collections itself subscribes to is never published by
   anyone; loans are never marked `WRITTEN_OFF` and no GL/provisioning entry results.
5. **The wallet loses money under concurrency and mis-books repayments** (details in
   BLOCKER-5/6/7).
6. **There is no tax engine at all** — no excise duty, VAT, WHT, or stamp duty anywhere
   in the codebase — which is non-compliant in Kenya *today* (20% excise on fees and
   charges, FA 2023/2024) before even considering Ethiopia (15% VAT on fee-based
   financial services).
7. **Device finance (M-Kopa-style) does not exist** in any form, and "MPESA" is an enum
   value with no integration behind it.
8. **Ethiopia deployment requires code changes** — regulatory enums, currency
   fallbacks, and report codes are Kenya-only and validated in Go code.

---

## BLOCKERS (money-path correctness — fix before any go-live)

### BLOCKER-1 — Payments don't apply to loans
- `topology.go:89` binds `payment.completed` → `LoanMgmtQueue`; the only consumer on
  that queue (`management/consumer/consumer.go:58-62`) returns early for anything that
  isn't `loan.disbursed`.
- Effect: paid borrowers remain delinquent; DPD/collections/CRB feed all report a paid
  loan as in arrears — a regulatory mis-reporting risk (CRB accuracy obligations), not
  just a UX bug.
- Fix: consume `payment.completed` (with the existing idempotency wrapper) and call
  `ApplyRepayment`, keyed by payment `InternalReference` for dedup.

### BLOCKER-2 — Penalty accrual does not exist
- `PenaltyRate` grep across `internal/` hits only the product model. No scheduler,
  no accrual — `PenaltyDue` stays 0 forever.
- Effect: zero penalty income; collections cases carry understated
  `OutstandingAmount`; ECL/provisioning inputs understate exposure.
- Fix: daily penalty accrual job in management service honoring
  `PenaltyGraceDays`, writing `PenaltyDue` per overdue installment and
  `OutstandingPenalty` on the loan, publishing to accounting.

### BLOCKER-3 — Product fees & processing fees never charged
- `ProcessingFeeRate/Min/Max`, `ProductFee` (UPFRONT/DISBURSEMENT/ANNUAL/MONTHLY/EXIT)
  have no consumer. Disbursement (`origination/service/service.go`) credits the full
  amount with no fee deduction or fee schedule entries.
- Fix: charge UPFRONT/DISBURSEMENT fees at disbursement (net or capitalized —
  make it product config), post fee income to GL, and roll periodic fees into
  `FeeDue` on schedules.

### BLOCKER-4 — Write-off pipeline is disconnected
- `PublishWriteOffApproved` → routing key has no queue binding; `loan.written.off`
  has a binding (collections) but no publisher.
- Fix: management service consumes `writeoff.approved`, sets loan `WRITTEN_OFF`,
  publishes `loan.written.off`, accounting posts the write-off against the ECL
  allowance (IFRS 9) / statutory reserve.

### BLOCKER-5 — Wallet operations are not atomic and not locked
- `overdraft/service/wallet.go` `Deposit`/`Withdraw`: read wallet → mutate → write, with
  **no DB transaction and no row lock** across 3-5 separate writes (fees, facility,
  wallet, transaction). Two concurrent withdrawals both pass the
  `AvailableBalance` check and double-spend; a crash mid-sequence leaves facility
  and wallet permanently inconsistent. Contrast: `account/service/transfer_service.go`
  does this correctly (tx + `GetBalanceForUpdate` + UUID-ordered locking).
- Also: **no idempotency** — a retried mobile-money callback deposits twice
  (`Reference` is required but never checked for uniqueness).

### BLOCKER-6 — Wallet deposit waterfall double-credits the customer
- `Deposit` adds the **full** amount to `CurrentBalance` (`wallet.go:137`), then
  *also* reduces `AccruedInterest` and marks fees repaid on the facility without
  deducting them from the wallet balance. Deposit 100 against 50 drawn + 10 accrued
  interest → wallet shows +50, interest shows repaid: the 10 exists in two places.
  Interest/fee income is recognized while the customer keeps the cash.
- Also `wallet.go:162-167`: a **partial** fee payment marks the entire fee `CHARGED`.

### BLOCKER-7 — Wallet status is never enforced
- Neither `Deposit` nor `Withdraw` checks `wallet.Status`. A SUSPENDED/FROZEN/CLOSED
  wallet (e.g. frozen for fraud or AML) can still transact. Fraud/AML freeze actions
  are therefore ineffective on the wallet rail.

### BLOCKER-8 — No tax engine (Kenya-non-compliant today, Ethiopia-blocking)
- Zero references to excise/VAT/WHT anywhere in `internal/`. Charges
  (`TransactionCharge`) and fees have no tax component; the CoA/posting rules have no
  tax-payable wiring.
- Kenya: 20% excise duty on fees/charges of licensed lenders must be collected and
  remitted; interest WHT for some structures. Ethiopia: 15% VAT on fee-based services,
  different WHT regime. This must be a per-country config table (tax type, rate, base,
  GL account, effective dates) applied wherever a fee/charge is levied.

---

## HIGH (correctness / revenue / compliance)

### HIGH-1 — Loan repayment API edge cases (`management/service/service.go:265`)
- **No amount validation**: `ApplyRepayment` never checks `req.Amount > 0`; a negative
  amount inserts a negative repayment row (waterfall allocates zero, but the record and
  `LastRepaymentAmount` go negative).
- **No idempotency**: `PaymentReference` is not deduplicated; a double-submitted
  repayment double-allocates.
- **No locking**: loan is fetched outside the transaction without `FOR UPDATE`;
  concurrent repayments interleave and corrupt outstanding balances.
- **Overpayment silently swallowed**: anything beyond outstanding principal vanishes —
  no credit balance, no refund record.
- **Backdating half-works**: `PaymentDate` affects only the stored date, not penalty or
  DPD recomputation as of that date.

### HIGH-2 — Transfer charge is fail-open and never becomes income
- `transfer_service.go:385-422`: if product-service is down or errors, charge = 0
  (silent revenue loss under any incident — and an incentive-compatible attack: flood
  product-service, transfer free).
- The charge is debited from the customer but **credited nowhere**: no GL posting, no
  fee-income event. Money disappears from the customer's balance into no account —
  the books won't balance to the penny at reconciliation.

### HIGH-3 — Transfer idempotency is check-then-act
- `transfer_service.go:106-114`: reference lookup happens before the transaction with
  no unique constraint fallback; two concurrent requests with the same idempotency key
  both execute. Also a DB error on lookup is treated as "not found".

### HIGH-4 — Cross-tenant reads on the money path
- Transfer destination: `GetAccountByID`/`GetAccountByNumber` without tenant scoping
  (`transfer_service.go:148,156`) — funds can be sent to (and account numbers probed
  across) another tenant's accounts.
- Payments: `GetByReference` (`payment/service/service.go:127`) looks up by reference
  with **no tenant filter** at all.

### HIGH-5 — Payment lifecycle gaps
- No dedup on `ExternalReference` → a duplicated M-Pesa/callback reference creates two
  payments.
- `Reverse` flips status and publishes, but nothing unwinds the loan repayment,
  wallet credit, or GL entries → books diverge from payment states.
- `Complete` accepts any PENDING payment with no amount/channel verification against
  an external settlement record (no reconciliation module exists).

### HIGH-6 — Overdraft (Fuliza-parity) gaps
- **Silent auto-approval**: with no scoring client, `ApplyOverdraft` fabricates
  `{Score: 650, Band: "B"}` (`wallet.go:414`) and **approves a credit facility** — a
  misconfigured deployment grants overdrafts to everyone.
- No consent/T&C capture, no affordability check, fixed 1-year expiry with (verify)
  no enforcement at draw time, no limit-review cycle, no daily *access-fee* model
  (Fuliza's economics are tiered daily fees, not only interest — the fee schema
  supports ARRANGEMENT only).
- Day-count inconsistency: overdraft EOD accrues on /365 (`eod.go:111`), loan schedule
  daily rate uses /360 (`schedule.go:150`), product simulator uses 30/91-day months.
  Pick a convention per product and make it config.

### HIGH-7 — Schedule engine covers 2 of 8 advertised types
- Product service advertises EMI, FLAT, FLAT_RATE, ACTUARIAL, DAILY_SIMPLE, BALLOON,
  SEASONAL, GRADUATED. The loan engine implements EMI and FLAT_RATE;
  `resolveScheduleType` silently maps GRADUATED→FLAT_RATE and everything else→EMI
  (`management/service/service.go:599`). A customer sold a balloon/seasonal product
  gets a different contract than simulated. Reject unsupported types at product
  activation instead of silently substituting.

### HIGH-8 — Loan terms can't express nano/short-tenor lending
- Products define tenor in **days** (`MinTenorDays`), loans only accept **months**
  (`TenorMonths`; consumer defaults 0→12 at `management/consumer/consumer.go:100`).
  A 7-day or 21-day nano loan — the core Kenyan digital-lending product — cannot be
  represented. First repayment is hardcoded `now+1 month` regardless of frequency
  (`service.go:86`), so a "daily" loan's first installment is due after a month.
- Month-end drift: due dates advance via `AddDate(0,1,0)` from the previous due date,
  so Jan 31 → Mar 3 (Go normalizes Feb 31) and the error compounds.

### HIGH-9 — Restructure corrupts the loan record
- `Restructure` (`service.go:457`): overwrites `DisbursedAmount` with outstanding
  principal (destroys the historical record), deletes all schedules including unpaid
  penalty/fee rows — but leaves `OutstandingPenalty`/`OutstandingFees` on the loan,
  which the installment-driven waterfall can then **never collect**; loan can't close.
- No approval workflow, no restructure counter/limit, and no IFRS 9 / CBK
  consequence: a restructured loan stays PERFORMING with DPD reset — regulators
  (CBK PG/04 renegotiated-loan rules; NBE equivalents) require downgrade/watch
  classification and a cure period before upgrade.

### HIGH-10 — Origination underwriting gates are decorative
- `MinCreditScore` and `MaxDtir` on the product are never enforced; `CreditScore` on
  the application is caller-supplied. Approved amount is not re-validated against
  product min/max (only the *requested* amount at application time). There is **no KYC
  gate**: an application can be approved and disbursed for a customer whose KYC record
  is FAILED or absent (compliance service is never consulted).

### HIGH-11 — Compliance service is a passive database
- No automated transaction monitoring: AML alerts are only created by manual API call;
  nothing watches transfers/deposits/wallet draws for thresholds (CTR at
  KES 1M equivalent / USD 10k), structuring, or velocity (the fraud service has
  velocity rules but no binding to AML).
- KYC "pass/fail" is a status flip — no ID-registry integration (IPRS/Fayda), no
  sanctions/PEP screening at onboarding or payment time.
- SAR filing stores metadata only (no goAML XML export); regulator label defaults to
  hardcoded `"FRC Kenya"` (`compliance/service/service.go:163`).

---

## MEDIUM

- **MED-1** DPD refresh loads *all* loans across all tenants into memory in one slice
  (`RefreshAllDpd`) — O(book size), will not scale; runs at fixed 01:00 UTC (not
  tenant-local); ignores product grace days in DPD computation.
- **MED-2** `LoanDisbursedPayload.TenorMonths <= 0` silently defaults to **12 months**
  (`management/consumer/consumer.go:100`) — a malformed event quietly writes a wrong
  contract instead of dead-lettering.
- **MED-3** Loan `Currency` hardcoded `"KES"` at activation
  (`management/service/service.go:101`) — product currency ignored; repayment request
  currency accepted without matching the loan's.
- **MED-4** Accounting journal lines hardcode `"KES"` in ~10 places
  (`accounting/service/service.go`, `yearend.go`) — a multi-currency tenant's books
  are mislabeled; no FX/translation concept anywhere (the CurrenciesFx UI page has no
  backend).
- **MED-5** Flat-rate schedule: interest rounding remainder is not trued-up on the
  last installment (principal is); total interest collected ≠ contracted interest by
  up to n/2 cents.
- **MED-6** `ChargeTier` boundaries are not validated for gaps/overlaps at creation;
  a transaction amount falling in a gap gets charge 0 silently.
- **MED-7** Transaction charges apply a flat list by `TransactionType` with min/max,
  but there's no channel/product/customer-segment dimension and no effective-date
  versioning workflow (`EffectiveFrom/To` exist but nothing prevents overlapping
  active charges for the same type — first-match wins is order-dependent). *(verify
  exact selection logic in product service before fixing)*
- **MED-8** Collections `generateCaseNumber(tenantID)` — check uniqueness under
  concurrency; PTP (promise-to-pay) exists but no automated kept/broken detection is
  tied to incoming repayment events beyond case updates.
- **MED-9** Notification templates: single-language; no i18n/locale per customer —
  Ethiopia needs Amharic (and consumer-protection rules require disclosures in an
  official language).
- **MED-10** No business-day/holiday calendar service: due dates, EOD jobs, and
  penalty grace land on weekends/holidays with no rolling convention; Ethiopian
  calendar/fiscal-year (Hamle–Sene, EFY) reporting is unrepresentable.

---

## Product-coverage gaps vs. stated go-live scope

| Target product | Status in code | What's missing |
|---|---|---|
| Device finance (M-Kopa style) | **Absent** — zero references | Asset/device registry, IMEI binding, lock/unlock integration (e.g. Nuovopay/Trustonic-style APIs), down-payment + PAYG plans, repossession workflow, asset collateral in provisioning |
| Fuliza-style overdraft | ~60% skeleton | Daily access-fee tiers, expiry enforcement, limit reviews, consent capture, CRB linkage for OD, real scoring integration (see HIGH-6) |
| Wallet | Skeleton with money bugs | Atomicity/locking, status enforcement, statements, liens/holds, KYC-tiered wallet limits (CBK e-money tiers), dormancy handling |
| Transfers | Works for internal ledger | Charges→income posting, taxes, limits (per-txn/daily), AML hooks, external rails (PesaLink/RTGS/mobile money), settlement & reconciliation |
| BNPL | Product-type label only | Merchant onboarding, order/invoice linkage, merchant settlement, discount/MDR economics |
| Charges & taxes config | Charges: API exists; Taxes: **absent** | Tax engine (BLOCKER-8); charge selection dimensions (MED-7); UI page for charge config not found in portal (`ProductConfigPage` covers products only) |
| Mobile money (M-Pesa/Telebirr) | Enum values only | Daraja STK/C2B/B2C client, callback verification, Telebirr equivalent, per-country payment-rail plug-in abstraction |

---

## Country portability (deploy in Ethiopia without code changes)

The `regulatory` service is the right *shape* (per-tenant profile with `country`,
`reportingCurrency`, provisioning-rule pointer, report set) but everything behind it is
Kenya-hardcoded:

1. **Enums are closed in Go code** (`regulatory/model/model.go`): license types
   (DCP/MFB/BANK), bureaus (Metropol/TransUnion/CreditInfo), report codes (DCP_*/CBK_*),
   provisioning keys (CBK_PG_04). Ethiopia needs NBE license types (bank/MFI),
   NBE loan classification (Pass/Special-Mention/Substandard/Doubtful/Loss at
   1/3/20/50/100%), NBE credit-bureau feed format, and Financial Intelligence
   Service (not FRC) AML reporting. → Move these to DB-backed reference data with a
   country seed pack; validation against the table, not a switch.
2. **`"KES"` fallback in ~15 files** (accounting, payment, product, account,
   origination, management, reporting, overdraft). → One source of truth: tenant
   profile's currency; make missing-currency a hard error, not a silent default.
3. **Provisioning engine** binds to CBK PG/04 buckets; needs a second rule-set keyed
   by `ProvisioningTableKey` (the pointer already exists — implement `NBE_DIRECTIVE`
   as data, not code).
4. **CountryConfigPage is cosmetic**: hardcoded 5 countries (KE/UG/TZ/GH/NG — **no
   Ethiopia**), writes to org settings that no backend consumes.
5. **Interest-rate caps / pricing rules**: no per-jurisdiction cap enforcement at
   product activation or approval (Kenya risk-based pricing approvals; Ethiopia has a
   history of MFI caps). Needs a country rule: max nominal/APR per product type.
6. **APR / total-cost-of-credit disclosure**: `DCP_APR_DISCLOSURE` report code exists,
   but there is no per-loan APR/TCC computation at origination for pre-contract
   disclosure (required by CBK DCP regs; equivalent consumer-protection directives in
   Ethiopia). Compute APR from the actual schedule incl. fees at approval time and
   store it on the application.
7. **AML thresholds** are nowhere configured (no threshold config table; no automated
   CTR) — thresholds and reporting formats must be per-country config.
8. **Language/calendar**: notifications single-language; no Ethiopian calendar/EFY
   fiscal-year support in fiscal periods (accounting assumes Gregorian year-end).

---

## Suggested sequencing

1. **Week 1 (correctness):** BLOCKER-1, 5, 6, 7 + HIGH-1, 3, 4, 5 — these are silent
   money-loss/corruption bugs reachable through normal operation.
2. **Week 2 (revenue + lifecycle):** BLOCKER-2, 3, 4 + HIGH-2, 9 — penalty/fee
   engines, write-off pipeline, charge→GL posting.
3. **Weeks 3-4 (compliance minimum):** BLOCKER-8 (tax engine), HIGH-10 (KYC gate +
   underwriting enforcement), HIGH-11 (CTR automation + sanctions screening), APR
   disclosure.
4. **Then product depth:** nano-loan tenor support (HIGH-8), schedule types (HIGH-7),
   overdraft fee model (HIGH-6), mobile-money rails; device finance as its own epic.
5. **Ethiopia pack:** reference-data-driven regulatory enums, currency cleanup,
   NBE provisioning rule-set, country seed data, i18n.

## Verified-good (credit where due)

- Transfer service does locking/atomicity correctly (UUID-ordered `FOR UPDATE`, single
  tx, outbox event) — use it as the template for the wallet.
- Transactional outbox on payment completion and transfers (F27 fix) is sound.
- Loan activation consumer is idempotency-wrapped (`processed_events` guard).
- Maker-checker exists on origination approve/disburse (SoD enforced), product
  two-person auth, and account-service ops; audit hash-chains widespread.
- Decimal arithmetic (`shopspring/decimal`) used consistently — no float money.
- IFRS 9 ECL + CBK PG/04 higher-of reconciliation (H-4a) and CRB feed v1 are real
  and tested.
