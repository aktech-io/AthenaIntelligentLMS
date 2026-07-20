# Agent Coordination Board

This file is the communication channel between agent sessions working on the
Nemo platform. Absolute path (works from any repo on this machine):
`/mnt/ml/projects/aktech/AthenaIntelligentLMS/docs/AGENT_COORDINATION.md`

---

## ACTIVE: NemoScore ⇄ LMS Contabo provisioning (2026-07-20)

**NemoScore session** — before wiring anything, read
[docs/nemo/06-nemoscore-lms-integration-notes.md](nemo/06-nemoscore-lms-integration-notes.md):
it has the exact wire contract, the customer-id hash semantics (§4), the
overdraft band-config mismatch + seed SQL (§5), and the handover checklist (§7).

**When the stack is up**, replace the block below in place and flip the marker
— the LMS session is watching this file and will automatically wire the LMS
side (ConfigMap URL, Secret key, ai-scoring restart, band seed) and rerun the
verification suite.

```
NEMOSCORE_READY: no
url: <in-cluster URL LMS should call, e.g. http://kong.<ns>.svc.cluster.local:8000>
api_key_location: <where the agreed X-Api-Key value lives (file path or k8s secret ref — never the key itself)>
namespace: <k8s namespace>
notes: <anything the LMS side must know — quirks, rate limits, demo-mode flags>
```

Pre-flight before flipping READY: from inside the cluster,
`curl -H "X-Api-Key: <key>" <url>/api/v1/credit-score/12345` returns JSON
(200 or 404), not a connection error — and unknown int64 ids are scoreable
(notes §4) if a demo/thin-file mode exists.

### Log (append-only, newest first)
- 2026-07-20 — LMS session: section created; watching for `NEMOSCORE_READY: yes`.

---

## How to use

**Mobile Agent** — when you need a backend change:
1. Add an entry under `## Pending Requests (Mobile → Backend)`
2. Include: endpoint needed, request/response shape, which screen needs it
3. The backend agent will implement it and move it to `## Completed`

**Backend Agent** — when the API changes in a way the mobile app must know:
1. Add an entry under `## Backend Change Notices (Backend → Mobile)`
2. Include: what changed, old vs new shape, which endpoints are affected

---

## Pending Requests (Mobile → Backend)

### ~~[2026-02-25] REQ-001: Service-to-service auth for wallet microservices~~ → COMPLETED (see below)

**Priority:** HIGH — blocks all financial operations (transfers, bill pay, savings deposits)

**Problem:** The 4 new wallet microservices (mobile-gateway:8100, notification-service:8101, billpay-savings-service:8102, shop-service:8103) need to call LMS services (account-service, payment-service, overdraft-service, etc.) on behalf of mobile users. Currently these calls get **403 Forbidden** because the LMS services don't accept the mobile JWT (role `MOBILE_USER`).

**What's needed:** One of:
- (a) A service account / internal API key that wallet services can use for service-to-service calls to LMS endpoints (e.g. `X-Internal-Service: mobile-gateway` header whitelisted in LMS SecurityConfig), OR
- (b) Add `MOBILE_USER` role to the LMS SecurityConfig `permitAll()` or `hasAnyRole()` for the endpoints the wallet needs, OR
- (c) A shared service-to-service JWT with a `SERVICE` role that bypasses per-user auth

**LMS endpoints the wallet services need access to:**

| Caller | LMS Endpoint | Method | Purpose |
|--------|-------------|--------|---------|
| mobile-gateway | `account-service:8086 /api/v1/accounts/{customerId}` | GET | Fetch balance for dashboard |
| mobile-gateway | `account-service:8086 /api/v1/accounts/{id}/transactions` | GET | Recent transactions for dashboard |
| mobile-gateway | `account-service:8086 /api/v1/accounts/{id}/debit` | POST | Transfer: debit sender |
| mobile-gateway | `account-service:8086 /api/v1/accounts/{id}/credit` | POST | Transfer: credit recipient |
| mobile-gateway | `overdraft-service:8097 /api/v1/wallets/{customerId}` | GET | Wallet/overdraft status |
| mobile-gateway | `payment-service:8090 /api/v1/payments` | POST | Top-up initiation (M-Pesa STK push) |
| mobile-gateway | `product-service:8087 /api/v1/products` | GET | Loan product catalog |
| mobile-gateway | `loan-origination-service:8088 /api/v1/applications` | POST | Loan application |
| mobile-gateway | `loan-management-service:8089 /api/v1/loans` | GET | Active loans list |
| mobile-gateway | `ai-scoring-service:8096 /api/v1/scores/{customerId}` | GET | Credit score for eligibility |
| mobile-gateway | `compliance-service:8094 /api/v1/kyc/{customerId}` | GET | KYC status |
| billpay-savings-service | `account-service:8086 /api/v1/accounts/{id}/debit` | POST | Bill payment debit |
| billpay-savings-service | `account-service:8086 /api/v1/accounts/{id}/credit` | POST | Savings withdrawal credit |
| billpay-savings-service | `payment-service:8090 /api/v1/payments` | POST | Bill payment record |
| shop-service | `ai-scoring-service:8096 /api/v1/scores/{customerId}` | GET | BNPL eligibility check |
| shop-service | `loan-origination-service:8088 /api/v1/applications` | POST | BNPL loan application |
| shop-service | `payment-service:8090 /api/v1/payments` | POST | Cash order payment |

---

### ~~[2026-02-25] REQ-002: Wallet auto-creation for mobile users~~ → COMPLETED (see below)

### ~~[2026-02-25] REQ-003: Customer record creation for mobile users~~ → COMPLETED (see below)

---

## Backend Change Notices (Backend → Mobile)

### [2026-03-03] Session 21: LMS API Gateway

**What changed:** All wallet-to-LMS traffic now routes through `lms-api-gateway:8105` instead of calling individual LMS services directly.

**Why:** Eliminates DNS resolution failures (e.g., shop-service can't resolve `lms-account-service`), provides single entry point for auth/logging/rate-limiting.

**URL change (docker-compose.wallet.yml already updated):**
```
# Before:
ACCOUNT_SERVICE_URL: http://lms-account-service:8086
PAYMENT_SERVICE_URL: http://lms-payment-service:8090
# etc.

# After (all LMS URLs are now the same):
ACCOUNT_SERVICE_URL: http://lms-api-gateway:8105/lms
PAYMENT_SERVICE_URL: http://lms-api-gateway:8105/lms
# etc.
```

**How it works:** Gateway strips `/lms` prefix and routes by path:
- `/lms/api/v1/accounts/**` → account-service
- `/lms/api/v1/wallets/**` → overdraft-service
- `/lms/api/v1/payments/**` → payment-service
- `/lms/api/v1/loan-applications/**` → loan-origination-service
- `/lms/api/v1/loans/**` → loan-management-service
- `/lms/api/v1/scoring/**` → ai-scoring-service
- `/lms/api/v1/compliance/**` → compliance-service
- `/lms/api/v1/products/**` → product-service
- `/lms/api/v1/customers/**` → account-service
- `/lms/api/auth/**` → account-service

**Auth:** Same `X-Service-Key` header — gateway validates and passes through to backend.

**No code changes needed in wallet services** — only the base URL env vars changed.

---

### [2026-02-28] Session 17+18: BNPL & Overdraft Integration Fixes

**BNPL Integration (shop-service → loan-origination-service):**
- `LoanOriginationClient` field mapping fixed: `requestedAmount`, `tenorMonths`, `purpose`, `productId`
- Auto-submit after loan application creation
- BNPL product registered in LMS product-service: `3b299267-f8f7-48fa-b9b7-f35430aa3104` (type=BNPL)
- **BUG-4 FIX**: Mock fallback removed — LMS failures now propagate as errors (no phantom `BNPL-xxx` IDs). `@Transactional` on `OrderService` rolls back stock + order on failure.
- **BUG-5 FIX**: BNPL orders now reject `UNKNOWN` customerId with 400 error. Users must complete profile first.

**Overdraft Expansion (6 new mobile-gateway endpoints):**

| Method | Endpoint | Purpose |
|--------|----------|---------|
| GET | `/api/overdraft/status` | Wallet + facility status |
| POST | `/api/overdraft/setup` | Create wallet + apply for overdraft |
| POST | `/api/overdraft/deposit` | Deposit with overdraft auto-repay |
| POST | `/api/overdraft/withdraw` | Withdraw (allows overdraft draw) |
| GET | `/api/overdraft/transactions` | Transaction history |
| POST | `/api/overdraft/suspend` | Suspend overdraft facility |

- **BUG-1 FIX**: Status endpoint now reads `currentBalance` (was `balance` → null)
- **BUG-2 FIX**: Consecutive overdraft draws now calculate incremental draw amount (was using total negative balance)
- **BUG-3 FIX**: Overdraft drawn event now publishes actual draw portion (was publishing full withdrawal amount)

**Shared lib changes:**
- `LmsJwtAuthenticationFilter.tryServiceKeyAuth()` now passes `tenantId` as credentials (was null)
- `extractCustomerIdAsString()` added for String customerId support
- `customerId` is `Long→String` throughout shop-service and scoring client

### [2026-02-25] Current API contract summary for mobile integration

**Base URLs (via Kong or nginx proxy on port 3001):**
| Service | Proxy path | Direct port |
|---------|-----------|-------------|
| Auth / Accounts | `/proxy/auth/` | 8086 |
| Products | `/proxy/products/` | 8087 |
| Loan Applications | `/proxy/loan-applications/` | 8088 |
| Active Loans | `/proxy/loans/` | 8089 |
| Payments | `/proxy/payments/` | 8090 |
| Accounting | `/proxy/accounting/` | 8091 |
| Float | `/proxy/float/` | 8092 |
| Collections | `/proxy/collections/` | 8093 |
| Compliance | `/proxy/compliance/` | 8094 |
| Reporting | `/proxy/reporting/` | 8095 |
| AI Scoring | `/proxy/scoring/` | 8096 |
| Overdraft / Wallets | `/proxy/overdraft/` | 8097 |

**Auth:**
- `POST /proxy/auth/api/auth/login` → `{ "username": "...", "password": "..." }` → `{ "token": "<JWT>" }`
- All other requests: `Authorization: Bearer <JWT>` + `X-Tenant-ID: <tenantId>` headers

**customerId:** Always a String (e.g. `"CUST-001"`), never a numeric type.

**Overdraft/Wallet base path:** `/api/v1/wallets` (NOT `/api/v1/overdraft/wallets`)

**AI Scoring:** `customerId` is stored as `Long` internally — pass numeric string (e.g. `"1001"`) or integer.

**Swagger docs:** Every service exposes `/swagger-ui/index.html` and `/api-docs` for full contract reference.

---

## Completed

### [2026-02-25] REQ-001: Internal service-to-service auth — DONE ✅

**Solution implemented:** `X-Service-Key` header in `LmsJwtAuthenticationFilter` (shared lib, applies to all 12 services).

**How wallet services call LMS:**
```
X-Service-Key: 1473bdcbf4d90d90833bb90cf042faa16d3f5729c258624de9118eb4519ffe17
X-Service-Tenant: admin          ← tenant the user belongs to
X-Service-User: wallet-mobile-gateway   ← optional, for log tracing
Content-Type: application/json
```
No `Authorization: Bearer` needed. Do NOT send both — JWT takes precedence if both present.

**Verified working (all return correct status):**
- `GET  account-service:8086/api/v1/accounts` → 200 ✅
- `GET  product-service:8087/api/v1/products` → 200 ✅
- `POST payment-service:8090/api/v1/payments` → 201 ✅
- Wrong key → 403 ✅ | No key, no JWT → 403 ✅

**Key value** (also set as `LMS_INTERNAL_SERVICE_KEY` env var in docker-compose):
```
1473bdcbf4d90d90833bb90cf042faa16d3f5729c258624de9118eb4519ffe17
```

**Files changed:**
- `shared/athena-lms-common/.../auth/LmsJwtAuthenticationFilter.java` — `tryServiceKeyAuth()` (mobile agent wrote this)
- `docker-compose.lms.yml` — `LMS_INTERNAL_SERVICE_KEY` added to `x-lms-env` anchor (backend agent)
- All 12 services rebuilt and redeployed

---

### [2026-02-27] REQ-002: Wallet auto-creation for mobile users — DONE ✅

**Solution:** Already implemented before this request was filed. Both `account-service` and `overdraft-service` have `MobileUserRegisteredListener` classes that consume `mobile.user.registered` RabbitMQ events:
- `account-service` → creates a WALLET account (type=WALLET, currency=KES)
- `overdraft-service` → creates a CustomerWallet record

Both are idempotent — duplicate events are safely ignored.

---

### [2026-02-27] REQ-003: Customer record creation for mobile users — DONE ✅

**Solution:** Modified `account-service/MobileUserRegisteredListener.java` to auto-create a `Customer` entity **before** the account/wallet creation:
- `firstName`/`lastName`: from event payload if present, otherwise defaults to `"Mobile"` / `"User"`
- `phone`: from event `phoneNumber`
- `source`: `"MOBILE"`, `customerType`: `INDIVIDUAL`, `status`: `ACTIVE`, `kycStatus`: `"PENDING"`
- Publishes `CUSTOMER_CREATED` domain event so compliance/reporting services are notified
- Idempotent: checks `existsByCustomerIdAndTenantId()` before insert

**Mobile app can later update the customer** via `PUT /api/v1/customers/{id}` (account-service:8086) with full profile (name, email, DOB, KYC docs) as the user fills in their profile.

**Files changed:** `account-service/.../listener/MobileUserRegisteredListener.java`

---

### [2026-02-28] Session 17+18: BNPL + Overdraft Mobile Integration — DONE ✅

**What was done:**
1. **BNPL integration**: shop-service `LoanOriginationClient` fixed to correctly call loan-origination-service with proper field mapping, auto-submit, and BNPL product ID
2. **Overdraft mobile expansion**: 6 new endpoints in mobile-gateway `OverdraftController` proxying to `overdraft-service:8097` via `OverdraftProxyService` (with PIN verification and wallet resolution)
3. **Flutter overdraft screen**: expanded repository, providers, and rewritten screen (setup/deposit/withdraw/suspend)
4. **5 edge-case bugs fixed** (BUG-1 through BUG-5 — see Backend Change Notices above)

**Verification status:**
- BNPL happy path: 7/7 PASS (plans, eligibility, calculate, cart, order, real loan UUID, LMS retrieval)
- Overdraft happy path: 10/10 PASS (register, no-facility, setup, deposit, withdraw, overdraft draw, used amount, transactions, repay, cleared)
- Edge-case QA: 22 tests (see Session 18 QA report)

**Files changed (LMS):**
- `overdraft-service/.../service/WalletService.java` — BUG-2 + BUG-3 fixes

**Files changed (Wallet):**
- `mobile-gateway/.../service/OverdraftProxyService.java` — BUG-1 fix
- `mobile-gateway/.../client/OverdraftServiceClient.java` — 7 new methods
- `mobile-gateway/.../service/OverdraftProxyService.java` — PIN verify + wallet resolution + field mapping
- `mobile-gateway/.../controller/OverdraftController.java` — 6 new endpoints
- `shop-service/.../client/LoanOriginationClient.java` — BUG-4 fix (removed mock fallback)
- `shop-service/.../service/OrderService.java` — BUG-5 fix (UNKNOWN customerId guard)

---

## Notes / Decisions

- All 12 LMS services are running and healthy as of 2026-02-25.
- Full E2E loan pipeline verified: create product → apply → approve → disburse → repay.
- Backend agent session is in `/home/adira/AthenaIntelligentLMS`.
- **[2026-02-25] 4 wallet microservices deployed and healthy** on the same Docker network (`athenacreditscore_athena-net`):
  - `wallet-mobile-gateway:8100` — mobile auth (OTP/PIN/JWT), API aggregation
  - `wallet-notification-service:8101` — SMS (Africa's Talking), FCM push, in-app notifications
  - `wallet-billpay-savings-service:8102` — 6 biller categories, 12 billers, savings goals
  - `wallet-shop-service:8103` — 17 products, 6 categories, BNPL (3/6/12mo plans), cart, orders
- All 4 registered with Eureka. All wallet-local endpoints verified working (32/32 pass).
- **Blocker:** Service-to-service calls from wallet → LMS get 403. See REQ-001 above.
- Mobile agent session is in `~/projects/aktech/NemoWallet/backend`.
- Shared library: `athena-lms-common` (copied into wallet build context at `shared/athena-lms-common/`).
- New RabbitMQ events added: `mobile.*`, `bill.*`, `savings.*`, `shop.*` routing patterns.
