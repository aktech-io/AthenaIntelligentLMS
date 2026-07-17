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

**Decided — A1 app strategy**: the **AthenaMobileWallet Flutter app** audit
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
| C4/C5 | Brand packs, feature flags/entitlements | E | Design with C1 (both are tenant-config); implement after. |
| D2–D4 | On-prem/air-gapped installer, zero-downtime upgrades, HA/DR | C | After D1 — all three build on the chart. DR runbook is a doc+test exercise, start early. |
| C6/C7 | Usage metering & billing; per-tenant sandboxes | E | Phase 2; sandbox falls out of C1 if designed in. |

### Track 2 — Customer front end (the "neobank" claim)
| # | Item | Grade | Status / next action |
|---|------|-------|---------------------|
| A1 | White-label mobile app | C | **Fork-and-adapt** (04 audit). **Nemo rebrand done** on wallet branch `nemo-rebrand` (deep-water dark theme, stripes logo + launcher icons, Jost display font, "Nemo" naming; analyze clean). **Phase-0 BFF fold-in done** (branch `nemo/a1-bff-fold-in`): the wallet's 4 Go BFF services now live in the monorepo as `bff-gateway`/`bff-notification`/`bff-billpay-savings`/`bff-shop` (ports 8110–8113, hosts 28110–28113 + legacy 3010x) on `internal/common` (Viper config, zap, shared auth incl. promoted mobile-JWT issuance, health/metrics/tracing); wallet `shared/` lib retired, migrations ported, compose + Helm wired (no routeKey — app-facing), HTTP surface unchanged. Remaining A1: bundle-id + runtime brand packs (flavors work), app env config, k8s exposure of BFF via D1 ingress. |
| A2 | Self-service eKYC onboarding | C | API-side first (risk-tiered auto-approve on existing KYC plumbing); vendor (Smile ID class) behind an adapter chosen per market pack. |
| B2/B3 | P2P by alias, bills/airtime | C | Thin services over existing wallet + transfers; biller catalogue is market-pack content. |
| B1 | Virtual card issuing | C | Integration play (Paymentology/Interswitch class). Needs partner decision — **business blocker, flag to founder**. PCI scoping (F2) starts with it. |
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
1. **Card processor partner** (B1) — pick Paymentology / Interswitch / partner-bank BIN; blocks cards and starts the PCI clock.
2. **eKYC vendor** (A2) — pick provider (Smile ID / Veriff class) for the adapter's first implementation.
3. **Crypto**: VASP licence path + custody partner (B11) — parked until decided.
4. **First external tenant/market commitment** (Ethiopia?) — drives ET market-pack content and NBE report set (F5).

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
