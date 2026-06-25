# AthenaLMS — End-to-End Client Journey Test

**Started:** 2026-06-24 · **Driver:** Playwright on live k3s portal (http://localhost:30088) · **Operator:** admin (tenant `admin`)

## Test clients
| Tag | customerId | Name | Type |
|-----|-----------|------|------|
| A | E2E-A-001 | Alice Wanjiru | INDIVIDUAL |
| B | E2E-B-001 | Bob Otieno | INDIVIDUAL |

## Workflow stages
1. Foundation — org settings, branch, staff user
2. Clients — create Alice & Bob
3. Accounts — open account, select account type / deposit product
4. Deposit — fund an account
5. Transfer — A → B
6. Loan — apply → review → approve → disburse
7. Repayment — repay schedule
8. Statements — account + loan statements
9. Accounting — settlement GL entries (trial balance / ledger)

---

## Findings log

Severity: 🔴 blocker · 🟠 major · 🟡 minor · 🔵 note

### Pre-flight (code + API inspection)

- 🔴 **F1 — No UI path to deposit/withdraw cash to a normal account.** Backend exposes `POST /api/v1/accounts/{id}/credit` and `/{id}/debit`, but the Account Detail page only offers freeze / unfreeze / close / post-interest. The Teller page (`/teller`) posts **only loan repayments** (`/proxy/loans/api/v1/repayments`) and hardcodes its cash float to 500,000. The Transactions page is read-only. Net effect: an account can only be funded by an incoming transfer or loan disbursement → chicken-and-egg for the first account. Likely the root cause of "data created didn't flow to the next workflow."
- 🟠 **F2 — Only SAVINGS accounts can be opened.** Of 8 deposit products only 2 are ACTIVE — *Load Test Savings* and *Jijenge Savings Plus*, **both SAVINGS**. Every Current-account and Fixed-Deposit product is still DRAFT, and the open-account wizard filters to `status === "ACTIVE"`, so "account type selection" is effectively Savings-only. Activating the DRAFT products (or seeding proper ones) is needed to test current/fixed-deposit flows.
- ✅ ~~F3 deposit-product null fields~~ — **false alarm**: real fields are `productCode`/`productCategory` (populated); my first query used non-existent `code`/`accountType`. No bug.
- 🔵 **F4 — "Organization" is not self-service multi-tenant.** Tenant is derived from the JWT (`tenantId`); UI offers org *settings* + branches only, no create-a-tenant flow. Multi-tenancy is data-scoping, not provisioning.

### Stage 1 — Foundation
- 🔵 Pre-existing: org `Athena Financial Services` (tenant `admin`, KES), 1 branch "Head Office" (HQ-001), 4 staff users, admin login OK. Org settings + branch render. Staff-user creation (`/users`) not yet exercised — optional.
- 🔵 **F-wiz** — Dashboard shows a persistent "complete your institution setup / Go to Setup Wizard" banner even though org + branch already exist; setup-complete state not detected. (to re-confirm)

### Stage 2 — Clients ✅
- Created **Alice Wanjiru (E2E-A-001)** and **Bob Otieno (E2E-B-001)** via the real Add-Customer dialog. Both persisted (status ACTIVE, kycStatus PENDING) and render in Customer Directory. Name search works.
- 🟡 **F5 — Customer search caps at 20 results, no pagination.** `GET /customers/search?q=E2E` returns exactly 20 and omits the just-created records; only a narrower query (e.g. surname) surfaces them. With 800+ customers this means newly created records "disappear" from broad searches — a strong candidate for the "created data didn't appear" symptom.
- 🟡 **F6 — New customer KYC stuck at PENDING** with no obvious in-flow prompt to complete KYC; downstream steps (account/loan) may or may not enforce it — to verify.

### Stage 3 — Accounts ✅ (with issues)
- Opened **ACC-ADM-01019101** for Alice via the 4-step wizard (Select Customer → Product → Details → Review & Submit). Product *Jijenge Savings Plus* (SAVINGS), initial deposit KES 50,000.
- ✅ **Initial deposit works**: balance = 50,000 (available/current/ledger) with an "Initial deposit" transaction. So an account *can* be funded at opening — it's only post-opening top-ups that have no UI path (see F1).
- 🟠 **F7 — Account list endpoint returns `balance: null` for every account.** `GET /api/v1/accounts` (the Account Directory source) omits balances; only the per-account detail/`/balance` call has them. The directory therefore shows blank/zero balances for all accounts unless you open each one. Strong match for the "created data doesn't show up" symptom.
- 🟡 **F8 — Transaction `type` is null** on the initial-deposit transaction (amount/description set, type empty) → transaction lists can't categorise credit vs debit.
- ✅ ~~F9 branch not persisted~~ — **corrected**: branch *is* stored (as `branchId`) and the detail page shows `HQ-001` correctly. Only the legacy `branchCode` field is null; cosmetic.
- 🟡 **F11 — Account detail shows the raw product UUID** (`d50118ed-…`) under "Product" instead of the product name "Jijenge Savings Plus". Needs a name lookup/join.
- ✅ Account-detail balance renders correctly (KES 40,000 after the transfer below).

### Stage 4 — Bob's account ✅
- Opened **ACC-ADM-07461399** for Bob (Jijenge Savings, initial deposit KES 1,000). ACTIVE.

### Stage 5 — Deposit 🔴 BLOCKED (UI)
- No way to deposit/withdraw to an existing account from the UI. Account-detail action bar = **Freeze · Close · Post Interest** only. Backend `POST /accounts/{id}/credit|debit` exist and work. → see **F1**. This blocks the standalone "deposit to the sample account" step.

### Stage 6 — Transfer 🔴 BLOCKED (UI), backend OK
- 🔴 **F10 — No transfer UI at all.** `accountService.initiateTransfer` / `getTransfersByAccount` exist but are wired to **zero** pages/components (grep of all `.tsx` finds no transfer screen).
- ✅ Backend transfer engine is solid: API transfer **Alice → Bob KES 10,000** returned `COMPLETED` (ref e2e-tf-002); balances moved Alice 50,000→40,000, Bob 1,000→11,000. It even enforces business rules (INTERNAL = same customer; cross-customer must be `THIRD_PARTY`). So the fix is purely "build the transfer screen," not backend work.

### Stage 7 — Loan application & lifecycle (mostly backend; UI truncated)
- 🟠 **F12 — Loan application form has NO dropdowns.** "New Loan Application" asks for **Customer ID** (free text) and **Product UUID** (free text). Operator must know/paste raw IDs — the clearest instance of your "previously created data doesn't appear in dropdowns" complaint. Should be customer + product pickers.
- 🟠 **F13 — Loan lifecycle truncated in UI.** Backend state machine is DRAFT→SUBMITTED→UNDER_REVIEW→APPROVED→DISBURSED (all work via API). The UI loan-detail only wires **Approve/Decline** — no **Submit**, **Start Review**, or **Disburse** buttons (those service methods exist, unused). So you can't take a loan live from the UI.
- ✅ Backend lifecycle verified end-to-end via API; loan created ACTIVE (id 2c99177c) with a correct **12-installment amortization schedule** (equal 2,707.75 @ 15%).
- 🟡 **F15 — Loan list/detail field mismatch.** UI reads `principalAmount`/`outstandingBalance`; backend returns `disbursedAmount`/`outstandingPrincipal` → loan amounts render blank in the UI list.

### Stage 8 — Disbursement & Repayment
- 🔴 **F14 — Loan disbursement does NOT credit the borrower's account.** Disbursing 30,000 to Alice's account left her balance at 40,000 (no credit transaction); loan's `disbursementAccountId` is null. GL posts the disbursement (see Stage 9) but the operational deposit ledger never receives it → **the accounting GL and the account-service balances diverge.** Core integration gap.
- 🔴 **F17 — Loan-detail repayment uses the wrong endpoint.** `LoanDetailPage` POSTs `/loans/{id}/repayments` which is **GET-only → 405**. Correct endpoint is `POST /api/v1/repayments` with `loanId` in the body (the Teller page uses it correctly). Repayment from the loan screen is broken.
- ✅ Repayment via the correct endpoint works: 2,707.75 split interest 375 + principal 2,332.75; outstanding 30,000→27,667.25; COMPLETED.

### Stage 9 — Statements & Settlement GLs ✅ (accounting solid)
- ✅ **Settlement GLs are correct double-entry.** Disbursement → **DR Loans Receivable 30,000 / CR Cash 30,000** (`sourceEvent: loan.disbursed`, system-generated, POSTED). Repayment also posts a balanced entry. **Trial balance balances** (DR=CR=11,748,119.12, `balanced: true`, 37 GL accounts).
- 🔵 **F20 — GL credits Cash on disbursement, not Customer Deposits.** Correct *if* loans are paid out as cash; but since the API accepts a `disbursementAccount`, disbursing "to an account" should DR Loans Receivable / CR Customer Deposits **and** credit that deposit account. Tied to F14 — decide the disbursement model (cash-out vs to-account) and make GL + account ledger consistent.
- ✅ Account statement: opening/closing balances, running `balanceAfter`, proper CREDIT/DEBIT types. Mini-statement works.
- 🟡 **F19 — Statement ignores `from`/`to` params** (returns a fixed ~30-day window). 🟡 **F18 — statement labels the account name as `customerName`.**
- 🟡 **F8 (refined)** — `/accounts/{id}/transactions` returns `type: null`, but `/statement` & `/mini-statement` return proper `transactionType`. The Transactions tab (using the former) can't categorise; statements can.

---

## Audit-readiness & internal controls (international/auditable standard)

Assessment (2026-06-24): only **accounting** was audit-grade (maker-checker + `financial_audit_log` + fiscal periods). Operational services had **no audit trail** and **no segregation of duties**; account transactions didn't even record who performed them. Plan: shared audit foundation, then a **configurable** maker-checker framework (enable/disable per operation + threshold, incl. product-level).

### Phase A — Shared audit trail
- ✅ **A.1 account-service DONE (2026-06-24)** — new reusable `internal/common/audit` package (auto-extracts user/role/tenant from context); per-service `audit_log` table (migration 000010) + `created_by` on `account_transactions`; `GET /api/v1/audit-log?entityType=&entityId=` to read the trail. Wired into **credit, debit, transfer, status change (freeze/close/reactivate)**. Playwright/API-verified: a UI deposit recorded `ACCOUNT_CREDIT / admin@athena.com / ADMIN` with before/after + details; transfer, freeze, reactivate all logged with actor. Transactions now carry `createdBy`.
- ✅ **A.2 loans DONE (2026-06-24)** — shared `audit_log` table on `athena_loans` (migration `loans/7`); audit wired into **repayment** (management) and the full **loan lifecycle** — submit, review-start, approve, reject, disburse (origination), each with actor/role + before/after + details. Read endpoint `GET /proxy/loans/api/v1/audit-log`. Verified: a full apply→submit→review→approve→disburse→repay run produced 5 audit entries all attributed to `admin/ADMIN` with amounts/rates/account captured.

### Phase B — Configurable maker-checker
- ✅ **B.2 account-service DONE (2026-06-24)** — `control_config` + `pending_approval` tables (migration 000011); configurable per-tenant toggle + threshold (defaults: credit/debit/transfer @ KES 100,000, closure always). Sensitive ops (credit, debit, transfer, closure) queue when over threshold (HTTP 202) instead of executing; a *different* user approves → executes; **`checker != maker` enforced** (422 on self-approval); all approvals/rejections audited. Endpoints: `GET/PUT /control-config`, `GET /pending-approvals`, `POST /pending-approvals/{id}/approve|reject`. Verified: 150k deposit queued (balance unchanged), self-approve blocked, manager approval executed (balance +150k), audit shows maker=admin/checker=manager.
- ✅ **B.3 loans DONE (2026-06-24)** — `control_config` on loans DB (migration `loans/8`); segregation of duties enforced on the existing workflow: **loan approval** requires approver ≠ application creator, **disbursement** requires disburser ≠ approver/creator; config-gated per tenant (default on). Typed `BusinessError` now maps to 422 in the origination handler. Endpoints `GET/PUT /proxy/loans/api/v1/control-config`. Verified: self-approve → 422, manager approve → 200, self-disburse → 422, manager disburse → 200, config toggle works.
- ✅ **B.4 UI DONE (2026-06-24, parallel agent + Playwright-verified)** — `approvalService.ts`; **Approvals** queue page (status filter, Approve/Reject, maker-checker violation surfaced); **Dual-Control Settings** page (toggle + threshold per operation); nav entries under Administration; deposit/withdraw now show "Submitted for approval" on the 202 response. Verified in real browser: 200k deposit → pending toast → appears in queue as PENDING (maker admin) → admin self-approve blocked → **manager approves → executes**.
- ✅ **B-followup DONE (2026-06-24, parallel agents)** — (1) Loan dual-control (LOAN_APPROVE/LOAN_DISBURSE) now on the Dual-Control Settings screen (UI base corrected to `/proxy/loan-applications`, where origination serves it — not `/proxy/loans` = loan-management). (2) **Per-product override**: origination honours the product's existing `requiresTwoPersonAuth`/`authThresholdAmount` as a tighten-only override (a product can require dual approval even when the tenant default is off). Product create form gained a "Require dual approval" toggle + threshold. Verified: tenant LOAN_APPROVE off + product override on → creator's approval blocked (422), manager allowed (200); settings screen shows both account & loan controls.
- 🔴 **F23 — Systemic: internal service-to-service calls dropped the tenant.** `httputil.ServiceClient` relied on a global `ContextExtractor` that was only ever the no-op default ("the auth package registers it" — but nothing did). So every internal call defaulted to the `"default"` tenant. Surfaced here as origination→product **404s**: loan **product validation (min/max amount) has silently never run** in this deployment (fail-open), and the per-product override couldn't read the product. **Fixed** by registering the real extractor in package `auth` (`extractor.go`), so all services propagate `X-Service-Tenant`/`X-Service-User`. Note: only `loan-origination` was rebuilt to pick this up; **the other 15 services should be rebuilt** to get the same fix (their internal calls are still dropping tenant until then).
- 🟡 **F24 — `PRODUCT_SERVICE_URL` was unset** on loan-origination (defaulted to `localhost:8087` → connection refused in k3s). Set on the live deploy and added to `services.yaml` (cluster URL `product-service…:8087`).
- ✅ **F23 fix rolled out to ALL 17 services (2026-06-25)** — full rebuild + redeploy so every service propagates tenant/user on internal calls. Verified after: cross-service product validation now works (loan above product max → `422 exceeds product maximum`, previously fail-open).
- 🟠 **F25 — Systemic: internal service URL defaults pointed at `localhost`/stale DNS.** Audit of every service found internal-call base URLs defaulting to unreachable hosts in k3s: **lms-api-gateway** (all 16 routes → `localhost:<port>`), **notification** (`lms-account-service`), **overdraft** (`go-ai-scoring-service`), **payment** (`lms-loan-management-service`), **origination** (`localhost:8087`). **Fixed** all code defaults to `<service>.lms.svc.cluster.local:<port>` (env overrides still win); rebuilt + redeployed the 5 affected services. Smoke test green (login/accounts/loans/control-config/approvals all 200), 17/17 deployments ready.
- 🔵 **F26 (dead config, no action needed)** — configmap `lms-common` has `LMS_ACCOUNT_URL` (:8092) and `LMS_PRODUCT_URL` (:8091) with wrong ports, but **no Go service reads them** (Java-era leftovers). Harmless; left as-is.

## Prioritized fix roadmap

### P0 — money must move through the UI (root cause of "data doesn't flow")
1. ✅ **F14 FIXED (2026-06-25)** — loan disbursement now **credits the borrower's account**. Origination synchronously calls a new account client to credit the disbursement account *before* marking the loan DISBURSED (disbursement fails if the credit fails; idempotent on application id, channel `LOAN_DISBURSEMENT`). Account-service maker-checker now bypasses internal **service-role** calls (`gateOpen`) so system credits execute immediately. Verified: disburse 15k → balance +15k; disburse 120k (>100k threshold) → balance +120k with **0** new pending approvals (not queued). ⚠️ GL leg still credits Cash, not Customer Deposits — see **F20** (now the only remaining piece of the disbursement story).
3. ✅ **F10 FIXED (2026-06-25)** — Transfer dialog + Transfers tab on the account detail page (source/dest-account-number/amount/type/narration), handling the 202 pending-approval response. Playwright-verified: Bob→Alice 5k moved (201, success toast); transfers list renders.
2. ✅ **F1 FIXED (2026-06-24)** — Added Deposit / Withdraw buttons + dialog on Account Detail (`accountService.deposit/withdraw` → `POST /accounts/{id}/credit|debit`). Playwright-verified: deposit 7,500 → 16,000→23,500; withdraw 2,000 → 23,500→21,500; balance/toast update live.
3. **F10** Build the Transfer screen (backend transfer engine already works, incl. THIRD_PARTY rules).
4. ✅ **F17 FIXED (2026-06-24)** — `loanManagementService.applyRepayment` now POSTs to `/loans/api/v1/repayments` with `loanId` in the body (was hitting GET-only `/loans/{id}/repayments` → 405). Playwright-verified: Post Payment → 201, "Payment Posted" toast, outstanding 27,667→25,305, installments 1–2 marked PAID.

### P1 — workflows are completable & data is visible
5. ✅ **F13 FIXED (2026-06-25)** — loan-detail Decision tab now shows status-driven lifecycle actions: DRAFT→**Submit**, SUBMITTED→**Start Review**, UNDER_REVIEW→Approve/Decline, APPROVED→**Disburse** (dialog with amount + borrower-account picker; surfaces the SoD 422). Playwright-verified: Submit → 200 → SUBMITTED → Start Review shown.
6. ✅ **F12 FIXED (2026-06-25)** — New Loan Application uses a **customer search picker** (sets the customerId string) and a **product dropdown** (ACTIVE loan products) instead of free-text ID/UUID. Verified: picked Bob + Personal Loan → 201. (Bonus: with F23 fixed, product min/max validation now rejects out-of-range amounts through the UI with a clear toast.)
7. ✅ **F7 FIXED (2026-06-25)** — `ListAccounts` batch-fetches balances for the page and attaches them; the Account Directory shows real balances (was all blank). Verified via API + directory screenshot.

### P2 — correctness & UX polish — ✅ ALL DONE (2026-06-25)
9. ✅ **F2** — activated Current + Fixed-Deposit deposit products; account-type selection now offers SAVINGS/CURRENT/FIXED_DEPOSIT.
10. ✅ **F5** — customer search now `ORDER BY created_at DESC` + configurable limit (default 50); newly created customers surface (E2E-A-001/B-001 now appear first — fixes the original symptom).
11. ✅ **F8** — false alarm: transactions endpoint returns `transactionType` (populated); original test read wrong field.
12. ✅ **F11** — account detail shows the deposit product **name** (not UUID).
13. ✅ **F18** — statement uses the customer's real name (resolves internal-uuid or business id). ✅ **F19** false alarm: handler honors `startDate`/`endDate` (original test used wrong param names).
14. ✅ **F6** — `PATCH /customers/{id}/kyc` (audited) + "Verify KYC" action in Customer Directory. Verified: PENDING → VERIFIED via UI. (KYC is not currently enforced by downstream steps — operators can now complete it explicitly.)
15. ✅ **F21** — reporting summary pulls **live** portfolio totals from new `GET /loans/portfolio-stats` (counts/disbursed/outstanding by status) instead of the stale/empty event-snapshot. Verified: live figures match the loan book.
16. ✅ **F22** — overdraft page guards numeric computes (`num()` helper); shows 0/0% instead of `NaN`.

### Stage 10 — Interest, EOD & DPD ✅
- ✅ **EOD run works** (`POST /eod/run` → COMPLETED): 29 accounts accrued, KES 1,093.30 interest. `interest-summary` / `post-interest` behave correctly (422 "no unposted interest" when nothing accrued).
- ✅ **DPD/staging works**: loan reports `dpd: 0, stage: PERFORMING`.
- 🔵 Same-day-opened account (Alice) didn't accrue on the EOD whose runDate predates the open time — arguably correct (no full day elapsed).

### Stage 11 — Other subsystems (probed)
- ✅ Compliance is **generating alerts** (LARGE_TRANSACTION / HIGH / OPEN).
- ✅ Overdraft wallets exist; float accounts exist; collections functional (0 open cases — expected, loan performing).
- 🟡 **F21 — Reporting summary looks stale/snapshot-based.** `reporting/summary` shows totalLoans 4 / totalDisbursed 145,000 — excludes the new loan and most of the 18 loans in loan-mgmt. Portfolio numbers don't reflect live data.
- 🟡 **F22 — Overdraft Management page renders a stray `NaN`** (utilization/calc cell).

### Stage 12 — UI render tour (18 pages) ✅
- Reports, Trial Balance, Balance Sheet, Income Statement, Cash Flow, Collections, Collections Workbench, Wallets, Float, Fixed Deposits, Interest Accrual, Active Loans, Loan Detail, Loan Applications, AML, Ledger, Transactions — **all render with no console/page errors** (only the overdraft NaN above).
- ✅ ~~F15 loan amounts blank~~ — **false alarm**: Loan Detail (Principal 30,000 / Outstanding 27,667 / 15% / full 12-row schedule, installment 1 PAID) and Active Loans list both show amounts correctly. My earlier "null" was wrong raw-API field names.
- ✅ **F17 confirmed live**: loan-detail "Post Payment" → red toast *"Payment Failed — Request failed with status 405"* (screenshot captured).

### Verified working ✅ (no action)
- Customer creation, account opening + **initial deposit funding**, transfer engine (API), loan state machine (API), amortization schedule, repayment allocation, **double-entry GL + balanced trial balance**, statements.

## Fresh E2E pass — 2026-06-25 (round 2)
Two new clients (Grace Njoroge / Daniel Kiprono) walked through the full journey with **dual-control left ON**. 16/17 stages green: KYC verify, sub-threshold funding executes immediately, **150k deposit correctly queued → admin self-approve blocked (422) → manager approved → executed**, THIRD_PARTY transfer, loan apply→submit→review, **loan SoD enforced (creator self-approve 422, manager approved)**, **disbursement credited the borrower (F14 holds: 85k→125k)**, trial balance balanced. One new finding:

- 🔴 **F27 — disbursement reports success but the loan is never created (events silently dropped).** Root cause: a **startup race in the shared RabbitMQ publisher**. `common/rabbitmq/connection.go` retries the broker 10×/20s then **gives up**; if RabbitMQ DNS (`rabbitmq.infra`) isn't resolvable in that window (cold k3s start — CoreDNS still coming up), the service starts with a non-connected `Connection`. `common/event/publisher.go` `NewPublisher` then builds a **permanent no-op publisher** (`ch=nil`) that **never calls the existing `Reconnect()`**. Its `Publish()` logs *"Event not published (no RabbitMQ connection)"* but **returns `nil`**, so the origination wrapper (`origination/event/publisher.go:128`) logs *"Published event"* (false success) and **disburse returns HTTP 200**. The borrower **is** credited (synchronous, F14) but `loan.disbursed` never reaches the broker → **loan-management never creates the ACTIVE loan → there is no loan to repay.** Confirmed it's not a transient broker blip: a fresh disburse with RabbitMQ *healthy* still didn't activate, because origination's publisher was stuck no-op since boot. Affected at this start: **loan-origination, account-service** (no-op=1); loan-management/product/payment/accounting connected within budget. **Operational remedy (verified):** `kubectl rollout restart` the affected service once RabbitMQ is up → publisher reconnects → next disburse activates the loan in **~2s**. **Real fix (DONE — commit 918c2c3):** the shared `Publisher` is now self-healing — it holds the `*Connection`, lazily `Reconnect()`s/reopens a confirm-enabled channel on each `Publish` when `ch==nil` or closed, drops the channel on publish error so it reopens next time, and returns a real error (logged) instead of a false "Published event". All call sites are fire-and-forget (log-only) so healthy-broker behaviour is unchanged. **Rolled out to all 16 Go services (2026-06-25)** via vendored rebuild + redeploy; verified after: every service logs `Connected to RabbitMQ`, **zero** no-op publishers, and a fresh disburse on the rebuilt system activates in ~2s.

- ✅ **Round-2 deeper subsystem probe — clean.** Accounting GL (trial balance + journal entries), reporting summary (live: 42 loans / 1.41M disbursed / 1.27M outstanding), reporting metrics, compliance (65 open / 14 critical alerts, 0 pending KYC), collections, overdraft EOD (12 facilities accrued), account interest EOD (`/api/v1/eod/run` COMPLETED), and `loans/portfolio-stats` (live, on loan-management) all return 200 with sane data. **No new findings.**

**Prevention — DONE (2026-06-25, see `docs/EDA_HARDENING.md`).** Shipped & rolled out to all 16 services: (2) connection retries forever + auto-reconnect; (3) DB-gated readiness (`common/health`); (4) **transactional outbox** (`common/outbox`, `event_outbox` migration `loans/9`) wired into origination disburse — kills the dual-write at the root; plus publisher fail-fast + `mandatory=true`, self-resubscribing consumers, and topology re-declared on reconnect (`OnReady`). **Verified end-to-end:** disburse during a *total* RabbitMQ outage parks in the outbox and auto-completes on broker recovery with **zero pod restarts** (~48s). Remaining follow-ups: route the other money-path events through the outbox, consumer idempotency guard, return→outbox-retry correlation, outbox/lag metrics + reconciliation job.


