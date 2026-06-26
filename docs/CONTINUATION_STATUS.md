# Continuation / handoff status — audit-readiness epic

**As of:** 2026-06-26 · **Branch:** `master` (everything committed + pushed). Use
this to resume if a session ends.

## ✅ Shipped, deployed to k3s, and verified live

### Reliability (F27 epic)
- **Transactional outbox** (`internal/common/outbox`) — events written in the same
  DB tx as the state change; relay publishes at-least-once (SKIP LOCKED, backoff,
  dead-letter, retention, partial index). Wired into: loan-origination
  (disburse + submit/approve/reject), payment, accounting, account
  (credit/debit/transfer). Migrations: per-service `event_outbox`.
- **Resilient RabbitMQ**: connection retries forever + auto-reconnect; publisher
  fails fast + `mandatory=true` (unroutable logged); consumers self-resubscribe;
  topology re-declared on reconnect (`OnReady`); DB-gated readiness (`common/health`).
- **Idempotency**: `common/idempotency.Wrap` (`processed_events`) on loan-mgmt consumer.
- **Reconciliation + metrics**: outbox `Stats()` + 30s gauge log; origination
  reconcile job flags disbursed-but-no-loan gaps.
- Docs: `docs/EDA_HARDENING.md`.

### Auditability
- **Tamper-evident audit trail on ALL 9 audit tables** (account, loans,
  accounting `financial_audit_log`, overdraft, fraud, payment, collections, float,
  product): SHA-256 hash chain (per-tenant, advisory-locked `BEFORE INSERT`
  trigger) + append-only (`BEFORE UPDATE/DELETE` block) + `audit_verify(tenant)` +
  `…/audit-log/verify` endpoint. All chains verify `intact`. Docs:
  `docs/AUDIT_TAMPER_EVIDENCE.md`.
- **Audit coverage** added to payment/collections/float/product (had none).
- **Maker-checker config changes audited** (`CONTROL_CONFIG_UPDATE`, before/after).
- **17-test regression suite** `tests/test_32_audit_readiness.py` (all green).

### RBAC (`auth.RequireRole`, internal SERVICE calls bypass) — docs/RBAC.md
- accounting: post/submit/approve/reject/reverse entry, create account, period
  close/reopen → ADMIN/MANAGER/ACCOUNTANT.
- account: control-config PUT → ADMIN; pending-approval approve/reject → ADMIN/MANAGER.
- loan-origination: approve/reject/disburse + control-config → ADMIN/MANAGER (config ADMIN).
- product + fraud: admin mutations (create/update/activate, rules, watchlist, bulk) → ADMIN/MANAGER.
- All verified: officer→403, admin/manager→pass, reads open.

### Reports / UI
- **PAR / ageing** report `GET /api/v1/loans/par-report`.
- **IFRS 9 ECL provisioning** `GET /api/v1/loans/ecl-provision` (simplified flat
  stage rates 1%/10%/50% — proper PD/LGD/EAD is a follow-up; consts in
  `internal/management/repository/portfolio_repo.go`).
- **Bank reconciliation** (accounting): `POST /api/v1/accounting/bank-reconciliation/import`
  (role-gated) + `GET /api/v1/accounting/bank-reconciliation` (auto-match vs Cash
  GL 1000). Migration `accounting/9`.
- **CSV export** trial-balance / cash-flow / ledger via `?format=csv`.
- **UI**: verify-integrity for all 9 audit chains on Audit Logs page; PAR page;
  CSV download buttons. Docs: `docs/UI_REVIEW.md`.

## 🚧 In progress / next (priority order)
1. **Year-end close** (accounting) — roll net P&L (REVENUE − EXPENSE for the year)
   into **Retained Earnings** via a balanced system closing journal entry, then
   lock the year's periods. Key refs: `service.go` `ClosePeriod` (~L840),
   `repository.go` `createJournalEntryTx` (~L128), `GetNetBalance` (~L291),
   `is_system_generated` entries bypass maker-checker. Retained Earnings + P&L
   accounts are in `migrations/accounting/1_initial` + `3_complete_chart_of_accounts`.
   Add `POST /periods/{year}/year-end-close` (role-gated). **Get the double-entry
   exactly balanced** (DR revenue / CR expense / net to Retained Earnings).
2. **return→outbox-retry** (`common/event/publisher.go`) — make an unroutable
   (basic.return) publish fail so the outbox retries instead of marking dispatched.
   Needs confirm+return correlation; fail-safe design (never worse than today).
   Requires rebuilding ALL services (shared lib).
3. **Proper IFRS 9 PD/LGD/EAD** modelling to replace flat ECL rates.
4. RBAC rollout to KYC / SAR / remaining surfaces; central role→permission matrix.

## 🛠️ Operational runbook (local k3s)
- **Cluster was found scaled to 0 after a restart.** Bring up:
  `sudo systemctl start k3s` (if stopped), then
  `sudo k3s kubectl scale statefulset --all --replicas=1 -n infra` and
  `sudo k3s kubectl scale deploy --all --replicas=1 -n lms`. Wait for postgres ready.
- **kubectl needs root:** `sudo k3s kubectl ...`.
- **Migrations auto-apply is unreliable** — apply manually:
  `cat migrations/<svc>/<n>.up.sql | sudo k3s kubectl exec -i -n infra postgres-0 -- psql -U admin -d athena_<db>`.
  DBs: account→athena_accounts, loans→athena_loans, accounting→athena_accounting,
  payment→athena_payments, product→athena_products, overdraft→athena_overdraft,
  fraud→athena_fraud, collections→athena_collections, float→athena_float.
- **Build/deploy a Go service** (vendored, ~10s):
  `cd go-services && docker build -q -t lms/<svc>:latest --build-arg SERVICE=<svc> -f deploy/docker/Dockerfile.vendor . && docker save lms/<svc>:latest | sudo k3s ctr images import - && sudo k3s kubectl rollout restart deploy/<svc> -n lms`.
- **Portal**: docker image-store is corrupted for the nginx base — build via the
  isolated buildx builder:
  `docker buildx build --builder lmsbuilder -t lms/lms-portal-ui:latest -f lms-portal-ui/Dockerfile --output type=docker,dest=/tmp/.../portal.tar lms-portal-ui && sudo k3s ctr images import /tmp/.../portal.tar && sudo k3s kubectl rollout restart deploy/lms-portal-ui -n lms`.
- **Port-forwards** (host 28xxx → svc 8xxx, +20000): they die on pod restart and the
  `pkill`+relaunch races are flaky — relaunch the specific service forward you need.
  Login: `POST localhost:28086/api/auth/login {admin/admin123}`. Roles: admin=ADMIN,
  manager=MANAGER, officer=OFFICER. Portal UI: port-forward `svc/lms-portal-ui 3001:3000`.
- **Tests** use system python: `/usr/bin/python3 -m pytest tests/ -v` (NOT conda python).
- **Subagent backend was flaky** this session (connection errors mid-run); salvage a
  cut-off agent's worktree (`git -C .claude/worktrees/agent-<id> status`), commit +
  merge its branch, or finish in the main session.
