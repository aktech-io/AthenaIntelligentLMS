# RESUME — RBAC config workflow + UI (in progress)

**Saved:** 2026-06-29 (token-budget pause; auto-resume scheduled after reset).
**Branch:** master · all WIP committed + pushed · `go build ./...` green.

## Goal / decision (agreed with user)
Build a **UI-managed, global role→permission matrix**, with effective permissions
**carried on the JWT** and enforced locally by each service via
`auth.RequirePermission(perm, fallbackRoles...)` — falling back to a role check
when a token has no permissions claim (safe incremental rollout). Token-carried
(not cached-central) so there is **no new inter-service runtime dependency** at
authz-decision time. Matrix is **global** for now (per-tenant later).

## Task board (TaskList IDs)
- #1 matrix store (account-service) — ✅ DONE
- #2 shared Claims.Permissions + RequirePermission middleware — ✅ DONE
- #3 stamp permissions into JWT at login — ✅ DONE
- #4 matrix management API (GET/PUT, admin-gated, audited) — 🚧 IN PROGRESS (handler written, **routes not yet wired in main.go**)
- #5 pilot on compliance + verify end-to-end — ⏳ next
- #6 admin UI page (Access Control matrix) — ⏳ (parallelizable once #4 contract is live)
- #7 migrate remaining services to RequirePermission + finish rollout — ⏳ (incl. product activate, fraud watchlist)
- #8 go-live hardening audit — ✅ DONE (report: docs/GO_LIVE_HARDENING_AUDIT.md) → fixes tracked as #10–#13 (create if missing)
- #9 return→outbox-retry — DEFERRED (tracked)

## What's done (committed)
- Migration `migrations/account/000014_rbac_matrix.{up,down}.sql`: tables
  `rbac_permissions`, `rbac_role_permissions`, `rbac_meta(version)`; seeds 9
  permissions + grants reproducing current allowlists EXACTLY. **account-service
  auto-applies migrations, but apply manually too (runbook):**
  `cat go-services/migrations/account/000014_rbac_matrix.up.sql | sudo k3s kubectl exec -i -n infra postgres-0 -- psql -U admin -d athena_accounts`
- `internal/account/rbac/rbac.go`: Store (Matrix, ListPermissions, PermissionsForRoles, SetRolePermissions[+version bump, validates keys], Version).
- `internal/account/rbac/handler.go`: GET /api/v1/rbac/matrix, GET /api/v1/rbac/permissions, PUT /api/v1/rbac/roles/{role} (audited via account audit.Logger; refuses to strip ADMIN's rbac.manage). **Not yet registered.**
- Shared `internal/common/auth`: Claims.Permissions/PermissionsSet/PermVersion (jwt.go parses `permissions`/`permVersion`); WithPermissions/PermissionsFromContext (tenant.go); Handler stamps perms into ctx; **RequirePermission(perm, fallbackRoles...)** with SERVICE bypass + claim-check + role fallback. Unit tests: `permission_test.go` (6 cases, green).
- `account/handler/auth.go`: NewAuthHandler now takes a PermissionResolver; generateToken stamps `permissions`+`permVersion` (fail-open if matrix read errors). Wired in `cmd/account-service/main.go` (rbacStore created, passed to auth handler).

## NEXT STEPS (resume here)
1. **Finish #4** — wire the rbac.Handler routes in `cmd/account-service/main.go`
   inside the authenticated group:
   ```go
   rbacAudit := audit.New(repo, logger) // common/audit; repo satisfies Inserter
   rbacHandler := rbac.NewHandler(rbacStore, rbacAudit, logger)
   rbacHandler.RegisterRoutes(r, auth.RequirePermission("rbac.manage", "ADMIN"))
   ```
   (import `internal/common/audit` if not already.) Build.
2. **Deploy account-service** (docker build vendor → ctr import → rollout restart;
   see docs/CONTINUATION_STATUS.md runbook). Apply migration manually first.
   Verify login now returns a token whose payload has `permissions` (decode JWT).
3. **#5 pilot compliance** — in `internal/compliance/handler/handler.go` replace
   `decide := auth.RequireRole("ADMIN","MANAGER")` with
   `decide := auth.RequirePermission("compliance.decide", "ADMIN", "MANAGER")`.
   Rebuild+deploy compliance. Re-run `tests/test_33_rbac_compliance.py` (must stay
   green: officer 403, manager pass). Then prove the workflow: PUT
   /api/v1/rbac/roles/MANAGER removing compliance.decide → re-login manager →
   manager now 403 on SAR; restore.
4. **#6 UI** — lms-portal-ui: add `accessControlService.ts` + an Access Control
   page (role×permission grid, toggles, Save→PUT /rbac/roles/{role}); admin-only
   route + sidebar entry; note "changes apply on next sign-in". Matrix API base:
   account-service (proxy /proxy/account → 18086 per vite.config / actually 28086
   host). Check `lms-portal-ui/src/lib/api.ts` + AppSidebar + App.tsx patterns.
5. **#7 migrate** accounting/account/origination/product/fraud inline RequireRole
   → RequirePermission with matching fallback roles + perm keys (see catalog in
   migration). Covers leftover product-activate / watchlist gating.

## Permission catalog (keys → fallback roles)
accounting.manage→ADMIN,MANAGER,ACCOUNTANT · account.config.manage→ADMIN ·
account.approval.decide→ADMIN,MANAGER · loan.decision.decide→ADMIN,MANAGER ·
loan.config.manage→ADMIN · product.manage→ADMIN,MANAGER · fraud.manage→ADMIN,MANAGER ·
compliance.decide→ADMIN,MANAGER · rbac.manage→ADMIN

## Go-live hardening — CRITICALs to schedule (from #8 report)
- CRIT-1: gateway accepts forgeable `X-Service-Key` from external clients (full
  auth/tenant bypass) — strip X-Service-* at gateway ingress; move key to a Secret.
- CRIT-2: default creds (admin/admin123 …) live; set LMS_AUTH_* or move to DB users.
- CRIT-3: secrets in plaintext ConfigMap — move JWT/DB/RMQ/service-key to k8s Secrets.
- HIGH: CORS reflects any Origin w/ credentials; empty JWT_SECRET fails open;
  replicas:1 + no liveness probe/HPA; no login rate-limit. See full report.
