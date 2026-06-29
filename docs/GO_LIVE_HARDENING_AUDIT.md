# Go-Live Hardening Audit — AthenaLMS Go Services + k3s

**Date:** 2026-06-29 · **Scope:** `go-services/` (16 services + shared `internal/common`),
migrations, and the live local **k3s** deployment (namespaces `infra`, `lms`).
**Method:** read-only code review + live cluster inspection (`sudo k3s kubectl get/describe`,
configmap dump). No files were modified and no mutating cluster/DB commands were run.

---

## Executive summary

The platform is functionally mature (transactional outbox, audit hash-chains, RBAC,
circuit breakers, graceful shutdown, DB-gated readiness) but is **not safe to expose at
scale without fixing a small number of authentication / secrets defects that allow full
privilege escalation and tenant bypass.** The three most serious issues are all in the
"front door": a known internal service key that the public gateway accepts and forwards,
default login credentials live in production, and all secrets stored in a plaintext
ConfigMap.

The hint that prompted this audit — "the cluster does not set `LMS_INTERNAL_SERVICE_KEY`
so service auth is disabled" — was **checked and is the opposite of what is live.** The
key *is* set (ConfigMap `lms-go-common`), to a value that is **committed in the repo**, and
the gateway accepts it from external clients. So service-to-service auth is enabled but
trivially forgeable. Details in CRIT-1.

**Finding counts:** CRITICAL 3 · HIGH 5 · MEDIUM 6 · LOW 5 (19 total).

Availability posture is also weak for "at scale": every service runs a **single replica
with no liveness probe, no HPA, and no PodDisruptionBudget**, so any wedged pod or rollout
is a user-visible outage.

### Remediation status (updated 2026-06-30)
- **CRIT-1** gateway auth bypass — ✅ FIXED & verified live (forged `X-Service-Key` → 401; headers stripped at proxy; gateway no longer honours the service key).
- **CRIT-2** default credentials — ✅ FIXED (no hardcoded passwords; env/Secret-sourced; fail-closed; unit-tested).
- **HIGH-1** CORS reflect-any-origin + credentials — ✅ FIXED (origin allowlist via `LMS_CORS_ALLOWED_ORIGINS`).
- **HIGH-2** empty JWT secret fails open — ✅ FIXED (reject secret < 32 bytes).
- **CRIT-3** plaintext secrets + **HIGH-4** no liveness/HA — 📦 production manifests delivered in `deploy/k8s/` (Secret template, liveness/replicas/PDB/HPA, NetworkPolicy); apply + rotate on the prod cluster (ops step, not auto-applied to dev).
- Open: **HIGH-3** login rate-limit/lockout, **HIGH-5** migration reliability, and the MEDIUM/LOW items below.

---

## CRITICAL

### CRIT-1 — Committed internal service key + gateway accepts & forwards it → full auth/RBAC/tenant bypass
**Evidence:**
- Live ConfigMap sets `LMS_INTERNAL_SERVICE_KEY: 1473bdcbf4d90d90833bb90cf042faa16d3f5729c258624de9118eb4519ffe17`
  (`sudo k3s kubectl get cm lms-go-common -n lms -o yaml`).
- That exact value is committed: `docker-compose.lms.yml:41` and
  `_archived_java/lms-api-gateway/src/main/resources/application.yml:107`.
- Any request with this header bypasses JWT and is granted `SERVICE` + `ADMIN` on an
  arbitrary tenant: `internal/common/auth/middleware.go` (service-key branch) sets
  `WithRoles(ctx, []string{"SERVICE", "ADMIN"})` and reads tenant from the
  client-supplied `X-Service-Tenant` header (defaulting to `default`).
- `RequireRole` (same file) **always passes the `SERVICE` role**, so every RBAC guard is
  void for these requests.
- The public gateway is configured with the same key (`cmd/lms-api-gateway/main.go` →
  `auth.NewMiddleware(jwtUtil, cfg.InternalServiceKey, logger)`), so it **accepts**
  `X-Service-Key` from outside, and its reverse-proxy director (`newReverseProxy`,
  `cmd/lms-api-gateway/main.go:311`) only rewrites path/host — it **does not strip
  inbound `X-Service-*` headers**, so the header is forwarded to the backend too.
- CORS even advertises the header as allowed (`main.go:443`).

**Impact:** Anyone with repo access (or who guesses/leaks the static key) can call the
gateway with `X-Service-Key: 1473…` + `X-Service-Tenant: <any>` and act as ADMIN against
any tenant — read/move money, post journals, approve loans — with zero RBAC. This is a
complete authentication and multi-tenancy bypass.

**Fix:**
1. Rotate the key to a freshly generated secret stored only in a k8s Secret (see CRIT-3).
2. **The gateway must reject the internal-key auth path** — construct its auth middleware
   with an empty internal key, or add a dedicated gateway middleware that returns 401 if
   `X-Service-Key` is present on ingress.
3. In the gateway proxy director, explicitly delete inbound `X-Service-Key`,
   `X-Service-Tenant`, `X-Service-User` before forwarding (defence in depth), and only the
   gateway/services should add them on internal hops.
4. Restrict service-key auth to in-cluster traffic (NetworkPolicy) so it is never reachable
   from the ingress path.

**Effort:** ~0.5 day (code) + key rotation.

### CRIT-2 — Default login credentials are live in production
**Evidence:** `internal/account/handler/auth.go:43-86` builds an **in-memory** user table
with defaults `admin/admin123`, `manager/manager123`, `officer/officer123`, and
`teller@athena.com/teller123`. The admin/manager/officer passwords *can* be overridden via
`LMS_AUTH_ADMIN_PASSWORD` etc., but the live deployment sets **no such env** — the account
pod's only env is `SPRING_DATASOURCE_URL` + the common ConfigMap
(`kubectl get deploy account-service -o jsonpath=…env[*].name`), so the **defaults are in
effect**. `teller123` is hardcoded with **no override at all** (`auth.go:80`).

**Impact:** `admin/admin123` grants ADMIN on the production gateway today. Trivial full
compromise.

**Fix:** Replace the hardcoded map with a real user store (DB-backed, bcrypt/argon2 hashes)
or at minimum require all passwords via Secret env with **no defaults** and fail closed if
unset; remove the hardcoded `teller`. Force-rotate before go-live.
**Effort:** 0.5–2 days depending on whether a real user store is introduced.

### CRIT-3 — All secrets stored in a plaintext ConfigMap, not a Secret; weak/known values
**Evidence:** `kubectl get cm lms-go-common -n lms -o yaml` exposes, in cleartext:
`JWT_SECRET` (base64 of the literal `athena-lms-jwt-secret-key-for-signing-tokens-must-be-at-least-256-bits`),
`LMS_INTERNAL_SERVICE_KEY` (CRIT-1), `DB_USER: admin`, `DB_PASSWORD: password`,
`RABBITMQ_USER/PASSWORD: guest/guest`. `kubectl get secrets -n lms` → *No resources found*.

**Impact:** Any principal with `get configmap` in `lms` (a low bar — ConfigMaps are not
treated as sensitive and are easy to over-grant / dump in CI) obtains every credential and
the JWT signing key, enabling token forgery for any user/tenant. The JWT secret and
`DB_PASSWORD: password` are also guessable/placeholder-grade.

**Fix:** Move `JWT_SECRET`, `LMS_INTERNAL_SERVICE_KEY`, `DB_PASSWORD`, RabbitMQ creds into a
k8s `Secret` (or sealed-secrets/external-secrets), reference via `envFrom: secretRef`.
Generate strong random values; rotate the JWT secret (invalidates existing tokens — fine).
Remove committed secret values from `docker-compose.lms.yml` and use `${VAR}` with no
defaults.
**Effort:** 0.5 day.

---

## HIGH

### HIGH-1 — CORS reflects any Origin with credentials enabled
**Evidence:** `cmd/lms-api-gateway/main.go:434-447` — `Access-Control-Allow-Origin` is set
to the request's `Origin` (or `*`), together with `Access-Control-Allow-Credentials: true`.
**Impact:** Any website can issue credentialed cross-origin requests to the API; this is the
exact pattern browsers forbid for `*` and here it is bypassed by reflection. Combined with
any cookie/credential flow it enables CSRF/credential theft.
**Fix:** Allow-list explicit trusted origins; do not reflect arbitrary origins when
`Allow-Credentials: true`. **Effort:** 1–2 hrs.

### HIGH-2 — JWT auth fails open on a missing/empty secret
**Evidence:** `config.go` defaults `JWT_SECRET` to `""`; `auth.NewJWTUtil("")`
(`internal/common/auth/jwt.go:18`) base64-decodes `""` to an **empty key with no error**,
and signature validation then succeeds for any token signed with an empty HMAC key. Nothing
rejects an empty secret at startup.
**Impact:** If the ConfigMap/Secret is ever absent or mis-mounted, services start "healthy"
and silently accept attacker-forged tokens instead of failing closed.
**Fix:** In `NewJWTUtil`, error if the decoded key is empty / under 32 bytes; `main` already
`Fatal`s on `NewJWTUtil` error, so this makes it fail closed. **Effort:** 1 hr.

### HIGH-3 — No rate limiting anywhere (login brute-force + API abuse)
**Evidence:** No rate/throttle middleware in the gateway (`grep -n rate/limit/throttle
cmd/lms-api-gateway/main.go` → only circuit-breaker matches) or in `internal/common`. Login
(`auth.go:107`) has no attempt counter/lockout; it logs each failure but never blocks.
**Impact:** Unlimited password guessing against `admin` (CRIT-2) and unbounded request
volume to every backend. At scale this is both a security and availability risk.
**Fix:** Add an IP/user rate limiter at the gateway (e.g. `httprate`/tollbooth) and a login
lockout/backoff. **Effort:** 0.5 day.

### HIGH-4 — No high availability: single replica, no liveness probe, no PDB
**Evidence:** Every `lms` deployment has `replicas: 1` (cluster dump) and **no
livenessProbe** — only a readinessProbe — for all services except fraud-detection
(`for d in …; jsonpath liveness=…` → empty for 15/16; gateway liveness empty, replicas 1).
No PodDisruptionBudget exists.
**Impact:** (a) A deadlocked/wedged pod that fails readiness is pulled from endpoints but
**never restarted** → indefinite outage of that service. (b) Single replica means every
rollout, node drain, or crash is a full outage of that capability. (c) No PDB → voluntary
disruptions can take the only replica down.
**Fix:** Add a `livenessProbe` (can reuse `/actuator/health` but liveness should be cheaper /
not DB-gated to avoid restart storms during DB blips — use a static liveness + DB-gated
readiness); raise `replicas >= 2` for stateless services; add PDBs. **Effort:** 0.5–1 day.

### HIGH-5 — Migration failures are non-fatal; auto-apply documented as unreliable
**Evidence:** `cmd/account-service/main.go:55-57` (pattern repeated across services) logs a
failed migration as `logger.Warn("Migration failed (may be first run)")` and **continues
startup**. `MIGRATE_ON_STARTUP: "true"` is live, yet `docs/CONTINUATION_STATUS.md`
explicitly states "Migrations auto-apply is unreliable — apply manually."
**Impact:** A service can come up "Ready" against a stale/partial schema and serve/write with
wrong assumptions, or a `dirty` migration state can wedge future deploys. Silent schema drift
is a data-integrity hazard at go-live.
**Fix:** Either make migrations a gated pre-deploy step (Job/initContainer that must succeed),
or make a non-`ErrNoChange` migration error **fatal**. Add dirty-state detection/alerting.
Reconcile the "unreliable auto-apply" note before relying on it. **Effort:** 0.5 day.

---

## MEDIUM

### MED-1 — Long-lived tokens, no refresh/revocation
**Evidence:** `auth.go:166` issues 24h tokens (`exp: now + 24h`, `ExpiresIn: 86400`); there
is no refresh, blacklist, or revocation path. A leaked token is valid for a full day and
cannot be killed without rotating the global secret.
**Fix:** Shorten TTL (e.g. 1h) + refresh tokens, or add a revocation list / token-version
claim. **Effort:** 0.5–1 day.

### MED-2 — DB connections unencrypted; RabbitMQ on default guest/guest
**Evidence:** ConfigMap `DB_SSL_MODE: disable`; config default also `disable`
(`config.go`). RabbitMQ `guest/guest`. **Impact:** Cleartext credentials/data on the wire
in-cluster; default broker creds. **Fix:** `sslmode=require`/`verify-full` with certs;
dedicated RabbitMQ user with a strong password. **Effort:** 0.5 day.

### MED-3 — Multi-tenancy isolation not uniformly enforced / tenant falls back to subject
**Evidence:** `jwt.go` sets `TenantID = subject` when no `tenantId` claim
(`ParseToken`), and all live users share `tenantId: "admin"`. Repos mostly scope by
`tenant_id` (e.g. `internal/account/repository/eod_repository.go:35`), but coverage is
uneven — across `internal/account/repository` there are ~69 `WHERE` clauses vs ~30
`tenant_id =` predicates, so some queries are not tenant-scoped. Service-key calls accept an
arbitrary `X-Service-Tenant` (see CRIT-1).
**Impact:** Cross-tenant data exposure if/when the system is genuinely multi-tenant; today
masked because everything is one tenant. **Fix:** Audit every query for a `tenant_id`
predicate (or enforce via Postgres RLS); reject requests with no resolved tenant rather than
falling back to subject. **Effort:** 1–2 days.

### MED-4 — Readiness probe timeout shorter than its own DB check; no liveness on gateway
**Evidence:** Probes use `timeoutSeconds: 1` (deploy dump) but the health handler's DB ping
uses a `2s` context (`internal/common/health` Handler). Under load the probe can time out
before the ping resolves, flapping the pod out of rotation.
**Fix:** Set probe `timeoutSeconds >= 3`; align with the handler budget. **Effort:** 15 min.

### MED-5 — No autoscaling for lms services
**Evidence:** `kubectl get hpa -A` shows only an unrelated `alpa/api-hpa` (replicas 0); no
HPA in `lms`. **Impact:** No elastic capacity for "at scale" load; manual scaling only.
**Fix:** Add HPAs (CPU/memory or custom metrics) once `replicas>=2`. **Effort:** 0.5 day.

### MED-6 — No metrics endpoint / Prometheus instrumentation
**Evidence:** Observability is structured logs (`internal/common/middleware/logging.go`,
zap JSON) only; no `/metrics`, no RED/latency/error counters exposed. The `monitoring/`
dir exists but services expose nothing to scrape.
**Impact:** No SLO visibility, no alerting on error-rate/latency/saturation at go-live.
**Fix:** Add Prometheus middleware (request count/latency/in-flight) + outbox/circuit-breaker
gauges; scrape + alert. **Effort:** 1 day.

---

## LOW

### LOW-1 — Gateway health always reports `status: UP`
`cmd/lms-api-gateway/main.go:460-485` returns top-level `"status": "UP"` with HTTP 200 even
when downstream circuit breakers are open (only per-component fields change). Probes/alerts
keyed on the top-level status will miss degradation. **Fix:** Return 503/`DOWN` when any
critical route breaker is open. **Effort:** 1 hr.

### LOW-2 — Unauthenticated health endpoints leak dependency state
`/actuator/health` (services + gateway) is unauthenticated and reports DB/broker/route
up-down and target URLs. Minor info disclosure. **Fix:** Keep the probe minimal/public, move
detailed component state behind auth. **Effort:** 1 hr.

### LOW-3 — fraud-detection resource limits inconsistent and very small
Limits `cpu:200m / memory:64Mi` vs the 512Mi standard for peers (cluster dump). Risks OOM
under real load and is the only service with a liveness probe (asymmetry). **Fix:** Normalise
to the standard envelope. **Effort:** 15 min.

### LOW-4 — `envInt` silently ignores parse errors
`cmd/*/main.go` `envInt` uses `fmt.Sscanf` and ignores the error, falling back to default on
any malformed value. Low impact but can mask misconfig (e.g. a typo'd `PORT`). **Fix:** Log on
parse failure. **Effort:** 15 min.

### LOW-5 — Login logs raw username on every failed attempt
`auth.go:122` logs the attempted username at Warn on failure. With no rate limit this is also
a log-volume/PII vector. **Fix:** Pair with lockout (HIGH-3); consider hashing/truncating.
**Effort:** trivial.

---

## Fix-first — prioritized top 10 for go-live

1. **CRIT-1** Rotate the internal service key; make the gateway reject & strip `X-Service-*`
   on ingress; restrict service-key auth to in-cluster (NetworkPolicy).
2. **CRIT-2** Remove default/hardcoded login credentials; require hashed passwords via Secret,
   fail closed; rotate `admin`.
3. **CRIT-3** Move all secrets from the ConfigMap into a k8s Secret; regenerate strong values;
   purge committed secrets.
4. **HIGH-2** Make an empty/short JWT secret fail closed at startup.
5. **HIGH-1** Lock CORS to an explicit origin allow-list (no reflection with credentials).
6. **HIGH-3** Add gateway rate limiting + login lockout.
7. **HIGH-5** Make migrations a gated step / fatal on failure; detect dirty state.
8. **HIGH-4** Add liveness probes, `replicas>=2`, and PDBs for HA.
9. **MED-2 / MED-1** Enable DB TLS + non-default RabbitMQ creds; shorten token TTL / add
   revocation.
10. **MED-3 / MED-6** Audit tenant scoping on all queries (or RLS); add Prometheus metrics +
    alerting before scale.

---

## Confirmed-good (not findings — credit where due)
- Panic-recovery middleware on all routers (`internal/common/middleware/recovery.go`); scheduler
  also recovers (`internal/management/scheduler/scheduler.go:49`).
- Graceful shutdown with 10s drain + signal handling in every `main.go`.
- DB-gated readiness that deliberately does **not** fail on broker loss (outbox buffers) —
  `internal/common/health`.
- Resilient RabbitMQ: connect-forever + auto-reconnect + topology re-declare on `OnReady`
  (`internal/common/rabbitmq/connection.go`).
- JWT algorithm is pinned to HMAC (rejects `alg` confusion) — `jwt.go` `ParseToken` checks
  `*jwt.SigningMethodHMAC`.
- Sensible pgx pool config (MaxConns 20, lifetimes) and HTTP server timeouts set on every
  service.
- Transactional outbox + idempotency + audit hash-chains already in place.
- Every migration has a matching `.down.sql` (70 up / 70 down).
