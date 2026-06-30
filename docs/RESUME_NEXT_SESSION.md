# Resume point — go-live hardening + Kenya regulatory epic

**Paused:** 2026-06-30 (spend limit). All shipped work is on `origin/master` @ `d0932b9`. Clean tree.

## ✅ Done & pushed
- **Security HIGH-3 + LOW-5** — gateway rate limiting + login lockout (`dd634ea`).
- **Accounting H-1** — year-end close fiscal-year bounded + prior-year-closed guard.
- **Accounting H-3** — event idempotency (dedupe on DomainEvent envelope UUID for
  overdraft.repaid / overdraft.interest.charged / float.repaid) (`9d6f1d3`).
- **Regulatory spec** — `docs/REGULATORY_REPORTING_KE.md` (gap analysis + build order) (`d0932b9`).

## ▶️ NEXT — pick up here (in order)
1. **H-2 (NOT done — agent died on spend limit, re-dispatch).** Enforce closed-period
   immutability on ALL system/event posters (they bypass `checkPeriodOpen` today).
   **Decision: REDIRECT to current open period** (re-date the entry into the open period,
   keep original event date; fail closed only if ALL periods closed). Centralize the check
   in the shared posting path. Worktree + review, no push to master without EM review.
2. **Regulatory epic — GREENLIT: "Foundation + CRB feed first".**
   - **Per-tenant regulatory profile** (foundation): license type → report set + provisioning
     table + CRB target + frequency. Org settings exist but no regulatory profile yet.
   - **CRB borrower feed** (go-live blocker, mandatory): **bureau-agnostic, config-driven**
     generator (pluggable output mapper; Metropol/TransUnion/Creditinfo per-tenant config).
3. **H-4 — CBK provisioning overlay (scoped, queued).** Post IFRS-9 **movement** (required ECL −
   current allowance) to P&L, stage-tagged, maker-checker, **DRAFT until PD/LGD calibrated**;
   book **excess of CBK provision over IFRS to a non-distributable Statutory Loan Loss Reserve
   in equity**. Add a **separate, correctly-banded CBK classification** (current code bands are
   WRONG vs CBK: Loss at >90d should be >360d — `management/model/model.go:24-31`).
4. Cheap wins: formal P&L / Balance-sheet + NPL-ratio endpoints (derive from GL).
5. Remaining hardening backlog: HIGH-5 (migration reliability), MED/LOW from
   `docs/GO_LIVE_HARDENING_AUDIT.md`; CRIT-3/HIGH-4 = ops apply on prod cluster (`deploy/k8s/`).

## Decisions on record
H-2 → redirect-to-open-period · H-4 → auto-post CBK overlay · CRB → bureau-agnostic ·
entity IS CBK-regulated (East Africa/Kenya, multi-tenant white-label: device finance,
invoice finance, Fuliza-style wallet overdraft, mobile lending).

## Quality rule
No diff reaches master without EM review; agents work in worktrees, commit but DO NOT push.

## Deploy/test
Live deploy is **local k3s** (no public URL). Changed services to rebuild+roll:
account-service, lms-api-gateway, accounting-service. See `docs/reference_k3s_deploy` /
`reference_k3s_deploy.md` build commands.
