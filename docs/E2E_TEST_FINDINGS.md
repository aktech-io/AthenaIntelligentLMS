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
- 🟡 **B-followup (in progress)** — surfacing loan dual-control on the Settings screen + per-product override. Note: product model already carries `requiresTwoPersonAuth`/`authThresholdAmount`; wiring origination to honour them as a tighten-only per-product override (parallel agents: origination Go + UI).

## Prioritized fix roadmap

### P0 — money must move through the UI (root cause of "data doesn't flow")
1. **F14** Wire loan disbursement to credit the disbursement account (account-service credit + matching GL), persist `disbursementAccountId`. Decide cash-out vs to-account model (F20).
2. ✅ **F1 FIXED (2026-06-24)** — Added Deposit / Withdraw buttons + dialog on Account Detail (`accountService.deposit/withdraw` → `POST /accounts/{id}/credit|debit`). Playwright-verified: deposit 7,500 → 16,000→23,500; withdraw 2,000 → 23,500→21,500; balance/toast update live.
3. **F10** Build the Transfer screen (backend transfer engine already works, incl. THIRD_PARTY rules).
4. ✅ **F17 FIXED (2026-06-24)** — `loanManagementService.applyRepayment` now POSTs to `/loans/api/v1/repayments` with `loanId` in the body (was hitting GET-only `/loans/{id}/repayments` → 405). Playwright-verified: Post Payment → 201, "Payment Posted" toast, outstanding 27,667→25,305, installments 1–2 marked PAID.

### P1 — workflows are completable & data is visible
5. **F13** Add Submit / Start-Review / Disburse actions to loan detail (complete the lifecycle in UI).
6. **F12** Replace free-text Customer ID / Product UUID in loan application with pickers.
7. **F7** Include balances in the account list endpoint (or batch-fetch) so the directory isn't all blanks.

### P2 — correctness & UX polish
9. **F2** Activate/seed Current + Fixed-Deposit products so account-type selection is real.
10. **F5** Paginate / raise cap on customer search.
11. **F8** Populate transaction `type` on the transactions endpoint.
12. **F11** Show product name (not UUID) on account detail.
13. **F19/F18** Honor statement date range; fix `customerName` label.
14. **F6** Surface KYC completion in-flow; decide whether downstream steps enforce KYC.
15. **F21** Make reporting summary reflect live portfolio (or refresh snapshot) — currently stale.
16. **F22** Fix the `NaN` cell on the Overdraft Management page.

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


