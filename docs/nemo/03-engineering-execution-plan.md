# Nemo — Engineering Execution Plan

*Working document, July 2026. The EM view of [02-gap-analysis-and-roadmap.md](02-gap-analysis-and-roadmap.md):
what is actually pending, in what order we attack it, and what is in flight right now.
Update this file as items move; it is the single status board for the neobank build.*

## 1. Where we are

**Done / solid** (see §1 of the gap analysis for the full foundation): 16 Go services,
double-entry GL + IFRS 9, lending lifecycle with week-2 revenue engines, fraud + AML +
CBK/DCP/CRB regulatory stack, event-driven money paths hardened (idempotency, tenant
scoping, fail-closed charging), 234 pytest + Playwright suites, k3s deploy, baseline
Grafana/promtail.

**Just shipped**: **C2 market-pack skeleton** (`internal/common/market`, commit `13159a8`) —
Kenya is now the first data pack (`packs/ke.yaml`), not a hardcode. Currency, timezone,
support identity and regulatory seeding read the active pack; `MARKET_PACK=ET` plus one
YAML file is all a new market needs at the platform-defaults level.

**Decided — A1 app strategy**: the **NemoWallet Flutter app** (formerly AthenaMobileWallet) audit
([04-wallet-app-reuse-audit.md](04-wallet-app-reuse-audit.md)) returned
**fork-and-adapt**. Half the concept already works against the LMS APIs through the
app's own Go BFF (64/64 e2e green); missing pillars are cards, eKYC, crypto and Nia chat.
~30–36 engineer-weeks across 5 phases vs 50–70 for a rewrite; phases 0–2 (~13–16 wks)
deliver the sellable white-label v1. First moves: fold the BFF into this monorepo and
de-brand the compile-time "Athena" constants into brand packs (C4).

**Also shipped**: **D1 Helm umbrella chart** (`deploy/helm/nemo`, commit `c29a9e6`) —
one-command install of all 16 services + gateway routes, fraud-ML, in-cluster
PostgreSQL/RabbitMQ, market-pack env, demo-vs-secret credential model. Remaining
D-track: image pipeline, offline bundle (D2), migration gating (D3), HA values (D4).

## 2. The issue list, EM-ordered

Grades from the gap catalogue: **[C]ritical** to the "neobank in a box" claim,
**[E]xpected** by buyers, **[D]ifferentiator**. Order below is execution order within
each track, chosen for dependency flow, not grade alone.

### Track 1 — Package & install (the "in a box" claim)
| # | Item | Grade | Status / next action |
|---|------|-------|---------------------|
| D1 | Helm umbrella chart, one-command install | C | **Shipped** (`deploy/helm/nemo`): all 17 workloads, in-cluster PG/RabbitMQ, observability stack (Prometheus/Alertmanager/Grafana + Money Paths dashboard), severity-routed alerting, `hardening.enabled` (PDB/HPA/NetworkPolicy), `scripts/build-nemo-images.sh` builds the image set. Remaining: live-cluster install verification, image publish pipeline. |
| C1 | Tenant provisioning API + "create neobank" console | C | **API v1 shipped** (July 2026, account-service): `POST /api/v1/tenants` provisions a tenant atomically — registry row, org settings seeded from the market pack, initial admin user with one-time password (bcrypt-stored, returned once), `tenant.provisioned` outbox event — plus list/get/activate/suspend, gated by `tenant.manage` (ADMIN). Regulatory profile and GL need no seed (regulatory seeds lazily on first access; GL postings fall back to the shared `system` chart). Remaining: console UI, brand packs (C4), product-catalogue seeding, sandbox mode (C7), DB-backed login for provisioned admins. |
| C2 | Market packs | C | **Skeleton shipped.** Remaining: rails/bureau/KYC/tax ids consumed by G1/G3/A2 as they land; per-tenant pack override; scheduler use of holiday calendar. |
| C4/C5 | Brand packs, feature flags/entitlements | E | **C4 v1 shipped** (`c5cb599`): per-tenant brand JSONB on the tenant registry; GET brand (open to authenticated callers) with NemoWallet deep-water default, gated PUT with hex/name validation. Remaining: app/portal runtime consumption (ThemeExtension), comms templates, C5 feature flags. |
| D2–D4 | On-prem/air-gapped installer, zero-downtime upgrades, HA/DR | C | **D3 gating shipped** (`5271fe2`): `migrations.gated=true` runs schema migrations in pre-upgrade hook Jobs (MIGRATE_ONLY, fail-blocks rollout), pods skip startup migrations. D4 baseline in chart (`hardening.enabled`). Remaining: D2 offline bundle, rolling-deploy strategy tuning, backup/PITR + DR runbook with tested RPO/RTO. |
| C6/C7 | Usage metering & billing; per-tenant sandboxes | E | Phase 2; sandbox falls out of C1 if designed in. |

### Track 2 — Customer front end (the "neobank" claim)
| # | Item | Grade | Status / next action |
|---|------|-------|---------------------|
| A1 | White-label mobile app | C | **Fork-and-adapt** (04 audit). **Nemo rebrand done** on wallet branch `nemo-rebrand` (deep-water dark theme, stripes logo + launcher icons, Jost display font, "Nemo" naming; analyze clean). **Phase-0 BFF fold-in done** (branch `nemo/a1-bff-fold-in`): the wallet's 4 Go BFF services now live in the monorepo as `bff-gateway`/`bff-notification`/`bff-billpay-savings`/`bff-shop` (ports 8110–8113, hosts 28110–28113 + legacy 3010x) on `internal/common` (Viper config, zap, shared auth incl. promoted mobile-JWT issuance, health/metrics/tracing); wallet `shared/` lib retired, migrations ported, compose + Helm wired (no routeKey — app-facing), HTTP surface unchanged. Remaining A1: bundle-id + runtime brand packs (flavors work), app env config, k8s exposure of BFF via D1 ingress. |
| A2 | Self-service eKYC onboarding | C | **API v1 shipped** (`3f481f4`, compliance-service): pluggable eKYC provider adapter (sandbox default, EKYC_PROVIDER selects), `POST /api/v1/onboarding` with risk-tiered auto-approve (LOW straight-through → PASSED kyc_record; screening hits/missing evidence → officer referral queue; fail-closed on provider errors). **BFF endpoints shipped** (bff-gateway, pre-auth: `POST /api/v1/mobile/onboarding` submit + nextStep hint, `GET /api/v1/mobile/onboarding/{id}` polling, `POST /api/v1/mobile/onboarding/media` doc/selfie upload → media-service ref). **Vendor decision made: in-house first** — commercial vendors stay a pluggable per-client option via the same registry. **In-house engine v1 shipped** (branch `nemo/a2-inhouse-ekyc`): new `ekyc-ml-service` Python sidecar (port 8102/28102, compose + Helm `nemo-ekyc-ml` + build script wired) — `POST /v1/document/extract` (Tesseract OCR, label-anchored field extraction with per-field confidence, ICAO 9303 MRZ parse with check-digit verification overriding the visual zone), `POST /v1/face/match` (YuNet+SFace ONNX from the OpenCV zoo; deterministic fallback comparator capped below the 0.85 auto-approve line when models absent), `POST /v1/screen` (normalized fuzzy name matcher over `sanctions*.csv`/`pep*.csv` — demo lists shipped, real OFAC/UN/EU + PEP lists are an ops drop-in, see service README). Go provider `inhouse` (`internal/compliance/ekyc/inhouse.go`, default via EKYC_PROVIDER): fetches doc/selfie bytes from media-service (X-Service-Key), maps engine output to Result (DocumentVerified = confident fields + declared-value agreement), fails closed on any engine/media error. Remaining: real screening list feeds + refresh cron, face-model provisioning in offline builds, true liveness (challenge-response capture — current v1 = selfie face detected; **app screens shipped** — wallet `main` `5a37b92`: details → ID capture → selfie → review → nextStep result with live referral polling, approved → OTP registration handoff), per-market rule sets from pack kycRuleSet, pre-auth rate limiter on the BFF onboarding routes. |
| B2/B3 | P2P by alias, bills/airtime | C | Thin services over existing wallet + transfers; biller catalogue is market-pack content. |
| B1 | Virtual card issuing | C | **Skeleton shipped** (branch `nemo/b1-card-service`): card-service (port 28107, `athena_cards`, gateway route `/lms/api/v1/cards/`) — tenant-scoped card domain (VIRTUAL/PHYSICAL, REQUESTED/ACTIVE/FROZEN/BLOCKED/CLOSED, JSONB spending limits, `card_events` audit trail), staff API (issue/list/get/freeze/unfreeze/block/limits, RBAC), lifecycle events via transactional outbox, pluggable processor registry (`CARD_PROCESSOR`): sandbox works end-to-end; **Paymentology adapter is a fail-fast stub awaiting API credentials** from the DTB commercial deal (config surface + TODO-marked calls in `internal/card/processor/paymentology.go`). PCI posture: only `processor_ref` + `pan_last4` stored — never PAN/CVV; PAN display via processor-side tokenized reveal (deferred; see `internal/card/README.md`). Remaining: wire Paymentology client + webhooks, activation/close/PIN/3DS, disputes (H6), BFF card endpoints with the app screens. |
| A3/A5 | Customer web banking; notification templating/localisation | E | A3 shares A1's API layer. A5: notification service exists, needs per-tenant/brand templates — small, good filler task. |
| B4–B6 | Savings pots, standing orders, term-deposit lifecycle | D/E | Cheap wins on existing account types once A1 exists to show them. |
| B11 | Crypto wallet | D | Concept done (deck). Gated on VASP licensing + custody partner — **business blocker, not engineering-ready**. |
| A4 | USSD + agent channel | E | Phase 3. |

### Track 3 — AI at the core (the differentiator)
| # | Item | Grade | Status / next action |
|---|------|-------|---------------------|
| E1 | Unified decision engine | C | **Design merged** ([05-decision-engine-design.md](05-decision-engine-design.md)): embedded `internal/common/decision` library + thin control-plane service, decisions logged via existing outboxes, versioned YAML policies, explicit fail semantics (kills the scoring mock fail-open), shadow-first rollout. **v1 implemented** (branch `nemo/e1-decision-spine-v1`, design §6 cut): `internal/common/decision` library (policy loader + evaluator + reasons + metrics), `decision.recorded` via the overdraft outbox, decision-service skeleton projecting `decision_log` (port 28106), overdraft shadow adoption. Next: soak the shadow diff, then increment 2 (~7–8 eng-wks remain across increments 2–4). |
| E2 | Straight-through credit + adverse-action reasons | C | Directly on E1. |
| E7 | Model governance (registry, drift, kill switches) | C | Required before any bank risk committee sees E1/E2. Start as metadata + logging conventions inside E1, grow into service. |
| E3–E6 | AIOps agents, AI collections, AML copilot, customer AI ("Nia") | D/E | Phase 3; Nia's UX already exists in the app concept. |
| E8 | Data platform / lakehouse | E | Phase 3. |

### Track 4 — Operate & trust (the "sleep at night" claim)
| # | Item | Grade | Status / next action |
|---|------|-------|---------------------|
| H1–H3 | Observability: OTel tracing, business-metric exporters (GL imbalance, event lag, payment success), Alertmanager + SLO pack | C | **H2 baseline shipped** (`096a6fe`): /metrics on all 17 services, outbox lag + GL integrity + payment outcome collectors, starter alert pack (`monitoring/prometheus-rules/`), scrape annotations in the chart. **H1 tracing baseline + in-chart observability shipped** (`b5c9b9d`, `fce929e`): env-gated OTLP spans on all services, Prometheus/Alertmanager/Grafana with the Money Paths dashboard installable from the chart. Remaining: OTel spans on event consumers/DB, Alertmanager receivers + on-call rota (H3), per-service dashboards. |
| H5 | Reconciliation engine (M-Pesa first) | C | Phase 2 start; design alongside G1 connector SDK. |
| F1/F4 | Security hardening; strong customer auth | C | F4 blocks real A1 launch. mTLS/WAF ride on D1's chart. |
| F2 | PCI-DSS → ISO 27001 → SOC 2 | C | 12+ month lead — **start gap assessment when B1 partner chosen**. |
| G1/G3 | Rail-connector SDK; bureau adapter framework | C/E | Extract from existing M-Pesa/CRB code; ids already reserved in market packs. |
| H4/H6/H7 | Ops console, support tooling, vendor support model | E | Phase 2–3. |
| G2 | Public API platform + dev portal | E | Phase 3. |

## 3. Business blockers (founder decisions, not code)
1. **Card processor partner** (B1) — **DECIDED (2026-07-18): Paymentology**
   (Kenya BIN sponsorship via Diamond Trust Bank; multi-network, Africa-wide).
   Engineering proceeds on the adapter + card domain skeleton; founder action:
   open the Paymentology/DTB commercial conversation — API credentials and the
   PCI-DSS gap assessment (F2) are calendar-bound on it.
2. **eKYC** (A2) — **DECIDED (2026-07-18): in-house first** — ekyc-ml-service
   engine shipped (`nemo/a2-inhouse-ekyc`); commercial vendors (Smile ID class)
   remain a pluggable per-client secondary option via the provider registry.
3. **Crypto**: VASP licence path + custody partner (B11) — parked until decided.
4. **Second market** — **DECIDED (2026-07-18): Ethiopia.** ET built-in pack shipped
   (`8b37bd5`); NBE licence profile + report set (F5) queued.

## 4. Operating model
Work runs continuously in parallel tracks: the main line executes Track 1→4 priority
order top-down; separate agents take bounded, parallel-safe audits/builds (as with the
wallet audit). Every completed item: tests green → commit/push → tick here and in the
gap analysis. Standing tracks (money-path correctness, regulatory currency, security)
interleave as audits surface work.

**Immediate queue** (D1 ✓, C1 API ✓, C2 ✓, H1/H2/H3 baselines ✓, E1 design ✓,
wallet rebrand merged to wallet `main` ✓, portal Nemo branding ✓):
**A1 Phase 0** (fold wallet BFF into monorepo — done on branch `nemo/a1-bff-fold-in`, pending merge) → **E1 v1 implementation**
(decision library + decision_log + overdraft shadow, per 05 design) →
D3 migration gating → A2 eKYC API skeleton → live-cluster chart install test.

**Contabo production (2026-07-20)**: full Nemo image set live via the new **CI
pipeline** (`.github/workflows/deploy.yml`: push to master → 25 images to GHCR →
SSH rollout; tar/scp retired to offline-fallback). Shakedown fixed three latent
bugs — portal `src/data` swallowed by `**/data/` gitignore; account migrations
frozen since March by a 000005 version collision (all envs silently unmigrated);
scoring by-customer lookup ParseInt'ing varchar IDs (fail-closed every overdraft).
Credentials rotated (admin/manager/officer/teller from `lms-secrets`; admin123
dead), 6 loan products seeded. Public edge DONE (2026-07-20): DNS live for lms +
app.lms.athenafinance.cloud, Let's Encrypt certs on both, BFF ingress applied,
portal login verified with rotated creds.

**NemoScore ⇄ LMS integration COMPLETE (2026-07-31)**: the scoring stack had
been live in the `nemoscore` namespace since 07-20 (that session also wired the
LMS ConfigMap/Secret and seeded the six bands, but died before flipping the
coordination marker). The 07-31 session verified the chain and cleared five
stacked defects the old "one cause" diagnosis was hiding:
1. **Test-suite credential rot** (`e16d2cc`) — six test files hardcoded
   admin123; on the rotated box every login tripped the HIGH-3 per-username
   lockout and cascaded 429s that masqueraded as ~111 "scoring" errors.
2. **NemoScore thin-file demo mode** (AthenaCreditScore `4c9ec6a`,
   `NEMOSCORE_THIN_FILE_DEMO` — demo box only): score-on-miss serves the
   scorecard floor as SCORED/PARTIAL for unknown LMS-hashed ids instead of
   honest-but-blocking INSUFFICIENT_DATA (notes §4's sanctioned path).
3. **Aborted-transaction 500 + box schema drift** (AthenaCreditScore
   `699bf75`): missing `bank_transactions` (init SQL 13–15 never ran — the DB
   predates them) aborted the score-event insert; schema applied + the
   score-on-miss path now commits the placeholder row first and rolls back on
   feature-pipeline failure.
4. **LMS score-on-miss** (`f2d0af1`): ai-scoring's by-customer GET now runs
   TriggerScoring synchronously on miss (trigger `score.on.read.miss`) — an
   overdraft applicant with no loan application finally gets scored; FAILED/
   SKIPPED still store nothing so every caller keeps failing closed (HIGH-6).
5. **Schema relics**: overdraft `credit_band` VARCHAR(1) (built for A–D,
   rejected "POOR") → migration 000006; loans `reviewer_id` UUID (code stores
   usernames) → migration 15. Both applied to the box.

Suite vs box after: **482 pass / 0 fail / 0 error** (from 320 pass + ~127 red);
remaining skips are environment-conditional. Two test-model updates rode along:
test_27 got a 900s timeout marker (500 sequential HTTP calls over WAN/tunnel)
and its balance audit now nets out the arrangement fee the deposit waterfall
settles (BLOCKER-6 behaviour it predated); test_26 accepts the canonical band
names alongside legacy A–D. **A1 APK BUILT** (2026-07-31):
`NemoWallet/build/nemo-wallet-contabo-20260731.apk`, release, against
`https://app.lms.athenafinance.cloud` (wallet `main` `efb5356`). Follow-ups:
real identity federation LMS→NemoScore (notes §4, design item), decide whether
thin-file demo mode stays acceptable for investor demos, device smoke-test of
the APK.

**Emulator E2E of the APK (2026-07-31 evening)**: full onboarding ran against
the box — welcome → details → synthetic ID upload → selfie → submit →
REFERRED → officer approve (service-key API) → app polled to "You're
approved!" → OTP → PIN → logged-in dashboard. Found + fixed en route: the
BFF's overdraft client called `/api/v1/float/wallets…` paths that exist
nowhere in the monorepo (pre-fold-in relic; every mobile overdraft call
404'd) and its account client used non-existent `accounts/customer/{id}/
balance|transactions` routes + decoded the account list as an object —
both rewritten against the real APIs. Open items from the run:
1. **`mobile.user.registered` has no consumers** — the Java-era listeners
   (auto-create Customer + WALLET account + wallet) were never ported to the
   Go account/overdraft services, so mobile users have no LMS account and the
   dashboard balance stays empty until something creates one (A1 remaining).
2. **Tenant mismatch**: BFF submits onboarding under tenant `default`; portal
   staff live on tenant `admin` — officers can't see mobile referrals in the
   portal queue (approved this run via service-key API directly).
3. **ekyc-ml on the box has no screening lists** (`/v1/screen` 503) — every
   onboarding falls to referral via fail-closed PROVIDER_ERROR; drop the demo
   CSVs into the pod/image or wire real feeds.
4. **DEV OTP is returned to the release app** (sandbox SMS mode) — fine for
   demo, must be gated before any real launch (F4).
5. Cosmetics: dashboard greets "User" (registered name not shown), amounts
   format as `$` not KES (market pack not consumed in app), `??` avatar glyph.

**Mobile money path CLOSED (2026-07-31 late)**: after the BFF client fixes
(`5ca2c9d`, `6109562`) the app shows the live facility (KES 5,000 limit,
30% APR from NemoScore POOR band) and an OVERDRAFT_DRAW executed via
`/api/v1/mobile/overdraft/withdraw` (0 → −1,500, PIN-verified) renders as
Used 1,500 / Available 3,500. Extra findings: session tokens expire with
no refresh flow (app drops to welcome; re-login also RESETS the PIN
instead of verifying); withdraw/dashboard errors surface raw
DioException text; the Outstanding card reads a field the status payload
doesn't carry; PIN verify has no attempt throttling (HIGH-3 analogue for
mobile — add before launch, F4).

**OCR-first onboarding (docs/nemo/07) — WS-1 LANDED (2026-07-31)**:
documentType through the stack (`5f50ebd`: pack kycDocuments for KE/ET,
compliance migration 6, ekyc provider passthrough) and ekyc-ml
per-document profiles (`b9f480e`: passport-mrz MRZ-primary, et-fayda
FAN-16, dispatch-level tests; note ekyc-ml-service lives in THIS repo,
symlinked into AthenaCreditScore for its image build). Dart MrzParser +
15 tests staged in the NemoWallet working tree (WS-2 in progress —
remaining: ML Kit capture screens, doc-type picker, prefilled
confirmation flow). WS-4 (synthetic KE/ET passport + Fayda fixtures,
emulator E2E per doc) queued.
