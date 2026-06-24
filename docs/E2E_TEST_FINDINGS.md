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

### Stage 7+ — Loan / Repayment / Statements / GL (pending)

