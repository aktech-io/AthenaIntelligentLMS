# Role-based access control (RBAC) for sensitive operations

Authorisation beyond authentication: certain mutations may only be performed by
specific roles. This complements the existing maker-checker / segregation-of-duties
controls (which govern *who can approve what they didn't create*) with a coarser
*who is even allowed to perform this action* gate.

## Mechanism

`auth.RequireRole(allowed ...string)` (`internal/common/auth/middleware.go`) — a
chi middleware chained **after** the auth `Handler` (which populates roles in
context from the JWT). It allows the request if the caller holds any of the
`allowed` roles (case-insensitive); otherwise **403 Forbidden**.

- **Internal `SERVICE` calls always pass.** Service-to-service calls authenticate
  with the internal service key and carry the `SERVICE` role, so system flows —
  e.g. a loan disbursement crediting an account, or `loan.disbursed` →
  `accounting.posted` GL postings — are never blocked by RBAC.
- Apply per-route with `r.With(auth.RequireRole(...)).Post(...)`. Reads are left
  open to any authenticated user.

```go
fin := auth.RequireRole("ADMIN", "MANAGER", "ACCOUNTANT")
r.With(fin).Post("/journal-entries", h.postEntry)
```

## What's gated today (verified live)

| Service | Operation | Allowed roles |
|---------|-----------|---------------|
| accounting | post / submit / approve / reject / reverse journal entry; create GL account; close / reopen fiscal period | ADMIN, MANAGER, ACCOUNTANT |
| account | change dual-control (maker-checker) config (`PUT /control-config`) | ADMIN |
| account | approve / reject a pending approval | ADMIN, MANAGER |
| loan-origination | approve / reject / disburse a loan application | ADMIN, MANAGER |
| loan-origination | change loan dual-control config | ADMIN |

Changes to the account dual-control config are also **audited** (a
`CONTROL_CONFIG_UPDATE` entry with before/after in the tamper-evident trail), so
who changed a financial control — and to what — is provable.

Reads (trial balance, ledger, periods, audit log, config/pending lists) stay open
to any authenticated user. Verified: `admin` passes; `manager` passes where
allowed and is 403 on ADMIN-only ops; `officer` (roles `[OFFICER, USER]`) is 403
on all gated writes and 200 on reads.

## Roles
JWT carries `roles` (e.g. `[ADMIN, USER]`, `[OFFICER, USER]`). Test accounts:
admin→ADMIN, manager→MANAGER, officer→OFFICER, teller→teller.

## Rollout follow-ups
Extend `RequireRole` to other sensitive mutations the same way: loan-origination
control-config + approve/disburse (already SoD-guarded; add role gate), KYC
verification (compliance role), product create/activate (product manager), fraud
rule changes, watchlist edits. A central role→permission matrix (vs. inline role
lists) is the longer-term refinement.
