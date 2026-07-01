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
  (national ID / name) — held outside management, needed for bureau matching; (c) ✅ DONE —
  `Classification` now uses CBK-correct bands via `provisioning.ClassifyCBK`; (d) gate on
  `CrbEnabled` / schedule by `CrbSubmissionFrequency`.

## ✅ H-4a done & pushed (2026-07-01) — CBK provisioning computation
- `internal/management/provisioning` — CORRECT CBK PG/04 5-bucket bands (Normal 0-30 1%,
  Watch 31-90 3%, Substandard 91-180 20%, Doubtful 181-360 50%, Loss 360+ 100%), distinct from
  internal staging. `BuildReport` reconciles CBK vs IFRS 9 ECL: higher-of required allowance,
  P&L impairment = IFRS ECL, and statutory loan-loss reserve = max(0, CBK-IFRS) → equity.
  `Repository.GetCBKBuckets`, `Service.CBKProvisioningReport`,
  `GET /api/v1/loans/cbk-provisioning` (read). Pure functions unit-tested (both higher-of dirs).
- Follow-ups: collateral netting on Substandard+ (v1 unsecured/conservative); confirm rates vs
  current CBK/PG/04 (consts). This also supplies CRB-feed follow-up (c) the CBK-correct class.

## ▶️ NEXT — pick up here (in order)
0. **H-4b — GL POSTING of the provision movement (accounting service).** The money-touching
   half of H-4, deliberately deferred to its own careful increment. Post the **IFRS ECL
   movement** (required ECL − current allowance balance on 1410) as DR impairment expense
   (6000) / CR allowance (1410); true up the **statutory loan-loss reserve** in equity for the
   excess of CBK over IFRS. Period-end, **maker-checker, balance-asserted, in DRAFT until
   PD/LGD calibrated**, one entry per period per tenant (idempotent). Consumes
   `management` `GET /api/v1/loans/cbk-provisioning` numbers. Do as a focused, well-tested push.
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

## Deploy/test (state as of 2026-07-01)
Live deploy is **local k3s** (no public URL — localhost/LAN only). Cluster was started and is
healthy. **BUT the running pods are OLD images (predate this session) — none of this session's
work is live yet.** To test it, rebuild + roll these 5 changed services via
`scripts/build-k3s.sh`: **account-service, lms-api-gateway, accounting-service,
compliance-service, loan-management-service**. (Heavy build; deferred for budget.)

Access when cluster is up:
- Portal UI: http://localhost:30088 (NodePort 30088; node IP was 172.20.10.4).
- Gateway API: `kubectl port-forward svc/lms-api-gateway 28105:8105 -n lms` → http://localhost:28105.
- Login: `POST /lms/api/auth/login` `{"username":"admin","password":"admin123"}` (default
  passwords enabled: `LMS_AUTH_ALLOW_DEFAULT_PASSWORDS=true`). admin/admin123, manager/manager123,
  officer/officer123.
- New endpoints to verify AFTER rebuild: `GET /lms/api/v1/loans/crb-feed?period=YYYY-MM`,
  `GET /lms/api/v1/loans/cbk-provisioning`, `GET/PUT /lms/api/v1/regulatory/profile`.
