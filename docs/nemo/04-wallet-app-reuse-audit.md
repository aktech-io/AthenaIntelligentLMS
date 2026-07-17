# Nemo — AthenaMobileWallet Reuse Audit

*Version 1.0 — July 2026. Companion to [02-gap-analysis-and-roadmap.md](02-gap-analysis-and-roadmap.md)
and the [app concept](deck/nemo-app-concept.html). Audited repo:
`aktech-io/AthenaMobileWallet` @ `f8f9e97` (local: `~/projects/aktech/AthenaMobileWallet`).*

**Verdict up front: fork-and-adapt.** The wallet app is not a throwaway prototype — it is a
real-API Flutter app already wired to the Athena LMS platform through its own Go BFF layer,
with a working OTP/PIN/JWT auth stack that was e2e-tested (64/64) against the LMS services.
It covers roughly half of the Nemo concept screens. It is, however, single-brand,
single-language, light-theme-only, untested (1 smoke test), and missing the four most
visible Nemo pillars: cards, eKYC, crypto, and Nia. Adapt it; do not rewrite it.

---

## 1. Inventory

### 1.1 Tech stack

| Aspect | Finding |
|---|---|
| App | `athena_wallet` v1.0.0+142, ~14.3k lines of Dart in `lib/` |
| SDK | `sdk: ^3.6.2` in pubspec; lockfile resolved for Dart ≥3.9 / Flutter ≥3.35 (recent commits fix Flutter 3.41 / Dart 3.11 compat) |
| State management | **Riverpod 2.6** (`flutter_riverpod`) — providers per feature + repository providers; no codegen |
| Navigation | **go_router 14.8** — declarative routes, `ShellRoute` + bottom nav, custom fade/slide transitions (`lib/routing/app_router.dart`) |
| DI | Riverpod providers only (`apiClientProvider`, `<x>RepositoryProvider`); no get_it/injectable |
| HTTP | **dio 5.4** with a `QueuedInterceptor` doing bearer injection + 401 → refresh-token rotation + retry (`lib/core/network/auth_interceptor.dart`) |
| Storage | `flutter_secure_storage` for tokens + user info |
| UI kit | Material 3, `google_fonts` (DM Sans, runtime-fetched), `lucide_icons`, `fl_chart`, `smooth_page_indicator`, `cached_network_image` |
| Platform targets | android, ios, web, linux, macos, windows folders all present; Android `applicationId com.athena.athena_wallet`, portrait-locked |
| Lints | Stock `flutter_lints` (unmodified `analysis_options.yaml`) |
| Tests | **One** smoke test (`test/widget_test.dart` — "App renders"). No unit, widget, golden, or integration tests. No CI config in repo |
| History | ~11 commits, 2026-02-27 → 2026-03-19; started as Java BFF + Flutter, then "Go microservices migration + Flutter real API integration + e2e test suite" |

### 1.2 Architecture

Clean, consistent, small:

```
lib/
  core/       config (ApiConfig), models (17), network (ApiClient, AuthInterceptor),
              providers, repositories (11), services (secure storage),
              theme (5 files), utils, widgets (11 shared: AthenaButton/Card/Chip/AppBar,
              custom numpad, PIN sheet, shimmer, error display, coming-soon)
  features/   onboarding, dashboard, activity, transfers, bills, wallet (top-up),
              finance (loans/overdraft/savings), shop (BNPL marketplace), settings
  routing/    app_router.dart
```

Pattern is uniform: screen → feature provider (`FutureProvider`/`StateNotifier`) →
repository → `ApiClient` (4 Dio instances: gateway, notifications, billpay, shop). Models
are hand-written `fromJson` classes. No mock/demo data left in money paths (one hardcoded
fallback in shop order-tracking).

### 1.3 The bundled backend (`backend/`) — the surprise asset

The repo ships its own **mobile BFF in Go** (`backend/go-services/`, single module
`github.com/athena-wallet/go-services`): 4 services + k8s manifests (namespace `wallet`,
NodePorts 30100–30103):

| Service | Port | Role |
|---|---|---|
| `gateway` | 30100 | Auth (OTP/PIN/JWT/refresh rotation/device registry), dashboard aggregation, transfers, top-up, contacts, loans proxy, overdraft proxy, profile |
| `notification` | 30101 | In-app inbox, OTP SMS sandbox |
| `billpay-savings` | 30102 | Biller catalogue, saved billers, bill payment; savings goals with deposit/withdraw and a **daily auto-save scheduler** |
| `shop` | 30103 | BNPL marketplace (products, cart, orders, eligibility via AI scoring) |

Crucially, the BFF's clients call the **LMS platform itself** through `lms-api-gateway`
(`.../k8s/configmap.yaml` → `http://lms-api-gateway.lms.svc.cluster.local:8105/lms`) using
the same `X-Service-Key` / `X-Service-Tenant` service-auth convention the LMS Go monorepo's
`internal/common/auth/middleware.go` implements, hitting real LMS routes
(`/api/v1/accounts/customer/{id}`, `/{id}/credit|debit`, payments, loan origination/management,
overdraft, AI scoring). `tenant_id` is already threaded through JWT claims and the user/device
tables (defaulting to `"default"`). `backend/QA-REPORT.md` records **64/64 e2e integration
tests passing** across all 47 BFF endpoints against live LMS services. (The original Java
Spring `mobile-gateway` is still in the repo but superseded by the Go rewrite.)

So the "integration gap" question is largely answered: **an integration layer already exists
and works**; the question is where it lives and what it's missing.

---

## 2. Feature inventory (what exists, and what it calls)

| Flow | Screens | Backend calls | State |
|---|---|---|---|
| Onboarding/auth | splash → welcome carousel → phone entry → OTP → PIN setup | `POST /api/v1/mobile/auth/otp/send`, `otp/verify`, `pin/setup`, `pin/verify`, `token/refresh`, `device/register` | Working; phone+OTP+PIN only — **no eKYC, no biometrics** |
| Dashboard | balance, quick actions, financial-services card, recent transactions | `GET /api/v1/mobile/dashboard` (gateway aggregates LMS account balance + transactions) | Working |
| Activity | transaction history | via dashboard/gateway `transactions` | Working |
| P2P transfer | contact picker (recent/search) → amount (custom numpad) → PIN sheet → success | `GET /mobile/contacts/recent|search`, `POST /mobile/transfers/send` (phone-alias, PIN in body) | Working — alias P2P core of B2 |
| Top-up | wallet top-up (M-Pesa-style method selection) | `POST /api/v1/mobile/topup` → LMS payment service | Working |
| Bills | bill-pay hub (categories, saved billers) → biller payment | `GET /billpay/categories`, `.../billers`, `POST /billpay/pay`, saved-biller CRUD | Working; catalogue is DB-seeded, no real biller rails, airtime not evidenced |
| Savings goals | goals list, create, deposit/withdraw, auto-save config | `/api/v1/savings/goals...` + server-side daily auto-save scheduler | Working — strong B4 seed; no round-ups, no locked pots, no pot interest display |
| Loans | marketplace (products) → application → active loans → schedule → repay | `/mobile/loans/products|apply|active|{id}/schedule|repay` → LMS product/origination/management | Working — loan self-service exists |
| Overdraft | overdraft setup/status/deposit/withdraw/suspend/charges | `/mobile/overdraft/...` → LMS overdraft service | Working |
| BNPL shop | marketplace, product detail, credit plan, application, order tracking | shop service + AI-scoring eligibility | Working; **not in the Nemo concept** — keep behind a feature flag |
| Notifications | in-app inbox, unread badge, mark-all-read | `/api/v1/notifications/user/{id}...` | Working; **polling only — no push (no firebase_messaging)** |
| Settings | profile (view/edit, employment, preferences), security (PIN change via sheet), more-menu | `/mobile/profile...` | Working |
| Stubs | "Coming Soon" screen used for: **KYC Verification, Live Chat, Help Center, Statements, Linked Accounts, My Credit, Language** | — | Placeholders only |

Not present anywhere: cards, QR pay/receive, request-to-pay, scheduled/standing orders,
insights/budgets, crypto, AI chat, multi-currency, localization.

---

## 3. White-label / theming readiness

Better than average, but built as a *single brand*:

- **Centralized tokens**: all colour (`core/theme/app_colors.dart`), gradients, shadows,
  typography, spacing/radii (`core/constants/app_constants.dart`) live in ~6 files, and
  screens consistently use `AppColors.*` / shared `Athena*` widgets. Re-skinning is a
  contained change, not a hunt through 40 screens.
- **But all tokens are `static const`** — compile-time, not per-tenant runtime config. A
  brand pack (C4) needs these lifted into a `ThemeExtension`/config object loaded from
  flavor assets or fetched tenant config.
- **Hardcoded brand**: "Athena Wallet" app title, `AthenaApp`, `Athena*` widget names,
  `com.athena.athena_wallet` applicationId, `isAthenaUser` badge in P2P, launcher icons.
  No `assets/` section in pubspec at all — logo/splash are stock Flutter.
- **Light theme only**; no dark mode (the concept renders dark).
- **No localization**: no `flutter_localizations`/ARB; English strings inline. Swahili
  (a stated Nia/market requirement) needs full l10n extraction (~40 screens).
- **KES hardcoded** in `currency_formatter.dart` — same C2 market-pack extraction the
  platform needs.
- **No build flavors / env config**: `ApiConfig` hardcodes `localhost`/`10.0.2.2` +
  NodePorts 30100–30103. Needs `--dart-define`/flavor-based per-tenant, per-env config.

Estimated to reach "brand-pack themeable per tenant": ~2–3 engineer-weeks (tokens →
runtime theme, flavors, asset pipeline, dark mode), plus ~2 weeks for l10n extraction.

---

## 4. Mapping to the Nemo app concept & gap items

Concept screens (from `deck/nemo-app-concept.html`):

| # | Concept screen | Status | Notes |
|---|---|---|---|
| 1 | Home — one glance, one insight | **ADAPT** | Dashboard exists with balance + recents + quick actions. Add account-card carousel, Nia insight card, new nav (Home/Pay/Save/Borrow/Crypto vs today's Home/Pay/Shop/Activity/More) |
| 2 | Pay hub — people, tills, bills | **ADAPT** | BillPay hub + send-money exist. Missing: QR/till scan, request-to-pay, scheduled/standing orders section |
| 3 | Send — instant, fee-transparent | **EXISTS** | Contact picker, custom numpad, PIN sheet, success screen all built. Add fee display + slide-to-send polish; device binding/txn signing is F4 backend work |
| 4 | Pots — goals, round-ups, rules | **ADAPT** | Goals + deposits/withdrawals + auto-save (incl. server scheduler) exist. Missing: round-ups, locked pots, sweep rules, pot interest display |
| 5 | Insights — spend, budgets, nudges | **MISSING** | No categorization/budget UI; `fl_chart` already a dependency. Needs backend spend-categorization too |
| 6 | Cards — control at the surface | **MISSING** | Nothing card-related in app or BFF. Depends on B1 processor integration |
| 7 | Credit offer — cost you can see | **ADAPT** | Loan marketplace + application exist; rework into pre-approved offer + amount slider once E2 straight-through credit lands |
| 8 | Active loan — no surprises | **EXISTS** | Active loans, schedule, repay (incl. early repay path) all working against LMS |
| 9 | Crypto wallet — stablecoins lead | **MISSING** | B11; nothing exists. New tab, new BFF service, custody connector |
| 10 | Buy USDT | **MISSING** | Part of B11 |
| 11 | Nia — acts, confirms, logs | **MISSING** | "Live Chat" is a Coming-Soon stub. Needs chat UI + E6 action-taking backend with confirmation gates |
| 12 | eKYC — zero-touch onboarding | **MISSING** | Onboarding is phone+OTP+PIN only; "KYC Verification" is a stub. Needs camera/OCR/liveness vendor SDK + A2 backend |

Gap-item view:

| Gap | Status in app | What's needed |
|---|---|---|
| **A1** white-label app | **ADAPT** — app exists, single-brand | Brand packs, flavors, dark mode, l10n, rename, push, biometrics, tests/CI |
| **A2** eKYC | **MISSING** (stub) | Vendor SDK (Smile ID/Onfido class) capture flow + gateway orchestration |
| **B1** cards | **MISSING** | Full new feature (UI ~3 wks) atop processor integration |
| **B2** P2P | **EXISTS (partial)** | Alias P2P done; add QR, request-to-pay |
| **B3** bills/airtime | **EXISTS (partial)** | Flow done; needs real biller-aggregator rails + airtime + per-market catalogues |
| **B4** savings pots | **ADAPT** | Add round-ups, locked pots, interest; rules engine partially exists (auto-save) |
| **B11** crypto | **MISSING** | New module, feature-flagged per market |
| **E6** Nia chat | **MISSING** | New module; the PIN-confirmation sheet is a reusable confirm-gate primitive |
| A5 push/inbox | **ADAPT** | Inbox exists; add FCM + templating |
| F4 strong auth | **ADAPT** | Device registry + PIN + refresh rotation exist; add biometrics (`local_auth`), device binding enforcement, txn signing |

---

## 5. Integration gap — pointing it at the Nemo Go platform

Smaller than assumed, because the Go BFF already fronts the LMS:

1. **Keep the BFF pattern; adopt it into the platform.** The clean move is to pull the 4
   wallet BFF services into the Nemo monorepo (`go-services/cmd/mobile-bff` or as a 17th+
   service set) so they share `internal/common` (config, auth, outbox, idempotency, market
   packs) instead of the duplicated `backend/go-services/shared`. The service-auth handshake
   (`X-Service-Key`/`X-Service-Tenant`) and route shapes already match the LMS Go services
   (verified against `internal/account` routes: `/api/v1/accounts/customer/{id}`,
   `/{id}/credit`, `/{id}/debit`). ~1–2 wks.
2. **Environment/config**: replace `ApiConfig` hardcoded NodePorts with flavor/dart-define
   config; decide gateway topology (single BFF host vs 4 base URLs — collapse to one
   ingress path per tenant). The docker-compose path (LMS on 28xxx) and the k3s path
   (`lms` namespace, gateway :8105) both exist; standardize on the Helm chart (D1).
3. **Customer identity**: the BFF's phone-based user store (OTP, PIN bcrypt, refresh-token
   rotation, device table, tenant_id) is effectively the platform's missing *customer* auth
   service (staff RBAC lives in LMS). Formalize it as such; wire eKYC status and risk tier
   into it for A2.
4. **Models**: Flutter models map to BFF DTOs, not raw LMS DTOs — so LMS-side refactors
   don't ripple into the app. Keep it that way; add contract tests between BFF and LMS.
5. **Events**: BFF publishes `mobile.user.registered` etc. to RabbitMQ — align exchange/
   routing-key naming with the platform outbox conventions when merging.

---

## 6. Engineering-manager verdict

**Fork-and-adapt.** Reasons:

- **The expensive parts are done and proven**: real auth with refresh rotation, a
  consistent Riverpod/go_router/repository architecture with zero mock data in money
  paths, and — decisive — a working, e2e-tested Go BFF speaking the LMS's own service-auth
  dialect. A rewrite would re-spend ~10–14 engineer-weeks to get back to this baseline.
- **The codebase is small and clean enough to bend** (14.3k lines, uniform patterns,
  centralized theme). None of the missing pieces (cards, crypto, Nia, eKYC) fight the
  existing architecture; they are additive feature modules plus nav restructure.
- **The debt is real but bounded and mostly non-structural**: no tests/CI, no l10n, no
  dark mode, no push, no biometrics, hardcoded brand/env/currency. All are
  retrofit-friendly; none argue for a rewrite.
- Why not "reuse as-is": single-brand assumptions (name, IDs, tokens, KES), the Shop/BNPL
  tab occupying prime navigation, and zero test coverage mean it cannot ship as tenant #2's
  app without the Phase-0 work below. "Fork" here means: rename to the Nemo app skeleton,
  fold its BFF into the platform monorepo, and treat the current repo as the seed.

### Phased adaptation plan (app-side; backend gap items tracked in 02)

| Phase | Scope | Effort (eng-wks) |
|---|---|---|
| **0. Nemo-ify the skeleton** | Rename/re-namespace; brand-pack theming (runtime tokens, flavors, asset pipeline, dark mode); env config via dart-define; l10n scaffold (en+sw); fold BFF into Nemo monorepo on `internal/common`; CI + test harness with first widget/integration tests | **5–6** |
| **1. Production hardening** | FCM push + notification templating (A5); biometrics + device-binding enforcement + step-up auth UX (F4); crash/error reporting; offline caching of dashboard; contract tests app↔BFF; retire/flag Shop tab, new 5-tab nav (Home/Pay/Save/Borrow/+1) | **4–5** |
| **2. eKYC onboarding (A2)** | Vendor SDK integration (ID OCR, selfie liveness), progress checklist screen, risk-tier routing, instant account+card issuance hook | **4–5** |
| **3. Concept build-out** | Cards module UI (B1, atop processor APIs) 3; Pots v2 round-ups/locked/interest (B4) 2; Insights & budgets 2–3; QR + request-to-pay + scheduled payments (B2/B5) 3 | **10–11** |
| **4. Differentiators** | Nia chat with action confirm-gates (E6) 3–4; Crypto module feature-flagged (B11) 4–5 | **7–9** |

**Total: ~30–36 engineer-weeks** of app-side work to reach the full 12-screen concept
(Phases 0–2, ~13–16 wks, yield a sellable white-label v1 matching roadmap Phase 2's A1
scope). A from-scratch rewrite to the same endpoint estimates at 50–70 wks with new
integration risk — reuse saves roughly half and keeps a proven money path.

### Top risks

1. **Zero test coverage** on a money-moving app — make Phase-0 CI/test work non-negotiable.
2. **Divergent BFF module** (`backend/go-services` duplicating `internal/common`) will rot
   if not merged into the platform monorepo early.
3. **PIN-in-request-body** for transfers is weaker than the F4 target (device binding +
   transaction signing); fine for demo, must be replaced before external tenants.
4. **google_fonts runtime fetch** breaks offline/air-gapped and per-brand fonts — bundle
   fonts in the brand pack.
5. Concept's dark, insight-led home is a **significant visual redesign** even where flows
   exist — budget design time, not just engineering.
