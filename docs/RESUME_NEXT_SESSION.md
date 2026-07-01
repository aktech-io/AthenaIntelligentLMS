# Resume point — go-live hardening + Kenya regulatory epic

**Paused:** 2026-06-30 (spend limit). All shipped work is on `origin/master` @ `d0932b9`. Clean tree.

## ✅ Done & pushed
- **Security HIGH-3 + LOW-5** — gateway rate limiting + login lockout (`dd634ea`).
- **Accounting H-1** — year-end close fiscal-year bounded + prior-year-closed guard.
- **Accounting H-3** — event idempotency (dedupe on DomainEvent envelope UUID for
  overdraft.repaid / overdraft.interest.charged / float.repaid) (`9d6f1d3`).
- **Regulatory spec** — `docs/REGULATORY_REPORTING_KE.md` (gap analysis + build order) (`d0932b9`).

## ✅ Also done & pushed (2026-07-01)
- **Per-tenant regulatory profile FOUNDATION** (`a6404be`) — `internal/regulatory` in the
  compliance service: `regulatory_profile` table + hash-chained append-only `audit_log`
  (compliance migrations 2/3/4), license types → default report sets, rate-free provisioning
  pointer keys, bureau-agnostic CRB config, `GET/PUT /api/v1/regulatory/profile` (GET open,
  PUT ADMIN+audited, seeds DCP default on first read). Build/vet/tests green.

## ✅ H-2 done & pushed (2026-07-01)
- **Closed-period immutability on system postings** — all 9 event-driven posters in
  `internal/accounting/service/service.go` now route through `postSystemEntry`, which
  redirects an entry out of a CLOSED period into the current open period (re-dated, original
  event date preserved in the description), fail-closed if no open period in 24mo. Pure
  `resolveOpenPostingDate` helper unit-tested. Year-end's own posting path left untouched.

## ✅ CRB feed v1 done & pushed (2026-07-01)
- **CRB borrower-performance feed** in the **management** service: `internal/management/crb`
  (canonical bureau-agnostic `Record` + `Mapper` interface + generic `CSVMapper`),
  `Repository.GetCRBFeedRecords` (loans active as of period end + real overdue aggregated from
  past-due unpaid schedule installments), `Service.CRBFeedRecords`, and
  `GET /api/v1/loans/crb-feed?period=YYYY-MM` (ADMIN/MANAGER, CSV download). Unit-tested.
- **CRB v1 follow-ups (open):** (a) select the concrete bureau template from the tenant's
  regulatory profile `CrbBureau` (v1 emits generic CSV); (b) borrower PII enrichment
  (national ID / name) — held outside management, needed for bureau matching; (c) swap
  `Classification` from internal stage to the CBK-correct bands (lands with H-4); (d) gate on
  `CrbEnabled` / schedule by `CrbSubmissionFrequency`.

## ▶️ NEXT — pick up here (in order)
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
