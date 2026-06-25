# UI Review — verification, notifications & flow coherence

**Date:** 2026-06-26 · **Method:** Playwright suite (live portal) + source audit + manual flow trace

## 1. Playwright verification (what works)

Ran the full `tests/ui` suite against the live portal (`http://localhost:3001`):
**102 passed / 0 failed** across chromium + webkit. So at the page-load /
navigation / basic-interaction level the app is healthy — every sidebar
destination renders, auth + routing work, and the core list/detail pages load.

**Coverage gap (not a bug):** these tests are mostly "page loads correctly" and
navigation. They do **not** assert on operational outcomes (e.g. deposit → toast
text → balance updates). Deepening them to assert notification copy and
post-action state is the recommended next step (see §4).

## 2. Notification audit

Most operational toasts are good — money operations (deposit/withdraw/transfer)
include the formatted amount and correctly distinguish the maker-checker
"Submitted for Approval" (HTTP 202) path from immediate success; the loan
lifecycle (submit → review → approve → decline → disburse) and the approvals
queue all notify clearly. Issues found and **fixed**:

| Operation | Before | After |
|---|---|---|
| Freeze / Unfreeze / Close account | bare title only (e.g. "Account Frozen") | + description explaining the effect ("Debits, withdrawals and transfers are now blocked…") |
| KYC verify | "KYC Verified" (no detail) | + "identity has been verified — KYC status is now VERIFIED" |
| Loan submit/review/approve/decline | **showed raw UUID** (`<uuid> submitted for review`) | customer name (`Grace Njoroge's application submitted for review`) |
| Loan disburse | raw UUID | "Grace Njoroge disbursed KES 40,000 — funds credited to the borrower's account" |
| Create loan application | raw UUID | "KES 40,000 loan application created and ready for submission" |

**Known remaining (not yet addressed):**
- **Two toast systems coexist** — `useToast` (shadcn) in core/operational pages
  and `sonner` `toast.success/error` in newer pages (fraud, collections,
  strategies). Functionally fine but inconsistent in look/position. Recommend
  standardising on one (shadcn `useToast`, since the money paths use it).
- A few create flows are title-only (e.g. "Customer created successfully") —
  acceptable, low priority.

## 3. UI flow coherence

Flows are largely coherent: list rows navigate to detail pages
(`/account/:id`, `/loan/:id`), Customer 360 ties a customer's accounts /
transactions / statement together via tabs, and dialogs invalidate the right
queries + close on success. One concrete gap found and **fixed**:

- **Account opening dead-ended on the list.** On success it navigated to
  `/accounts`, leaving the operator to hunt for the new account to fund it. Now
  it routes to the new account's detail page (`/account/:id`) and the toast says
  "you can now fund it from the account page" — completing the open→fund flow.

**Observations for a deeper pass (need product direction):**
- After **disbursing** a loan, the flow could offer a direct link to the
  resulting active loan (currently you navigate manually).
- After **creating a customer**, offer "Open an account" as a next step CTA
  (currently you go back to the directory).
- The two customer surfaces (Borrowers/"Customers" directory vs Customer 360)
  could be unified or cross-linked more explicitly.

## 4. Recommended next steps
1. Standardise on one toast system.
2. Add operational Playwright assertions (deposit/transfer/loan lifecycle assert
   on toast copy + resulting balances/status), so "verify what works" covers
   behaviour, not just rendering.
3. Add the "next step" CTAs above to finish the create→use flows.
