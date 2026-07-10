# Nemo — Gap Analysis & Roadmap

*Version 1.0 — July 2026. Companion to [01-vision.md](01-vision.md).*

This is the honest inventory: what the platform already does at bank grade, and every
capability we still need to be a **top-notch neobank tech provider** — one-click
cloud/on-prem installs, per-market configuration, AI-operated, fully monitored. Gaps are
graded **[C]ritical** (can't credibly sell "neobank in a box" without it), **[E]xpected**
(buyers assume it; its absence loses deals), or **[D]ifferentiator** (wins deals).

---

## 1. What exists today (the foundation)

| Domain | In production shape today |
|--------|--------------------------|
| Deposits & accounts | CURRENT, SAVINGS, WALLET, FIXED_DEPOSIT, CALL_DEPOSIT; interest accrual service; account opening with maker-checker |
| Payments & transfers | Internal transfers with fail-closed charging; M-Pesa-style payment events; idempotent money paths |
| Lending | Full lifecycle: product factory → origination (fees netted at disbursement) → schedules, repayments, penalty accrual → collections → write-off against ECL |
| Accounting | Double-entry GL, chart of accounts, IFRS 9 ECL, maker-checker, fiscal periods, cash-flow, audit trail (tamper-evident) |
| Risk & AI | AI credit scoring service; fraud detection (20-rule engine + Isolation Forest/LightGBM ML sidecar, case management, network analysis) |
| Compliance | AML monitoring, SAR, CRB integration, CBK/DCP regulatory report stack, per-tenant licence profiles |
| Treasury | Float management service |
| Overdraft/BNPL | Wallet overdrafts with scoring |
| Platform | 16 Go microservices, one module; RabbitMQ event-driven with outbox + idempotency guards; tenant_id everywhere; JWT/RBAC; React back-office portal; pytest (234) + Playwright suites; k3s deployment; Grafana/promtail monitoring baseline |

This is roughly the **back half of a neobank** — the hard, regulated, money-handling
half. The gaps cluster in the customer-facing front half, the packaging/deployability
layer, and depth of automation.

---

## 2. Gap catalogue

### A. Customer channels — the visible neobank

| # | Gap | Grade | Notes |
|---|-----|-------|-------|
| A1 | **White-label customer mobile app** (React Native/Flutter): onboarding, accounts, cards, P2P, bills, savings, loan self-service, in-app support chat | **C** | The single biggest gap. Today's UI is staff back-office only. Must be brand-pack themeable per tenant. |
| A2 | **Self-service onboarding & eKYC**: phone/ID capture, document OCR, selfie liveness, sanctions/PEP screening, risk-tiered auto-approve | **C** | KYC plumbing exists but is officer-driven. Target: account opened in <5 min with zero human touch for low-risk tiers. |
| A3 | Customer web banking (responsive companion to A1) | E | Shares API layer with A1. |
| A4 | **USSD & agent/branch channel** | E | Emerging-market table stakes: feature-phone users and agent cash-in/out (float service is the foundation). |
| A5 | Push notifications, in-app inbox, transactional SMS/email per brand pack | E | Notification service exists; needs templating, localisation, per-tenant branding. |
| A6 | In-app support: AI chat with banking actions (→ E-block), ticketing, complaint tracking with regulatory timelines | E | |

### B. Everyday banking products

| # | Gap | Grade | Notes |
|---|-----|-------|-------|
| B1 | **Card issuing**: virtual-first debit cards via processor integration (Paymentology/Interswitch/partner-bank BIN), freeze/limits/PIN, 3-D Secure, disputes & chargebacks | **C** | Integration play, not a build. Card = neobank in most buyers' minds. |
| B2 | **P2P by alias** (phone number/handle), request-to-pay, QR pay/receive | **C** | Sits on existing wallet + transfer services. |
| B3 | **Bill payments & airtime**: biller abstraction + per-market biller catalogues | **C** | |
| B4 | **Savings pots/goals**: round-up rules, scheduled auto-save, goal tracking, pot interest | D | Fintech Farm's signature feature; cheap on top of existing savings accounts. |
| B5 | Standing orders, scheduled & future-dated payments, direct debits | E | |
| B6 | Term-deposit lifecycle automation: maturity, rollover, early-break penalties | E | Account types exist; lifecycle jobs don't. |
| B7 | Multi-currency accounts & FX (buy/hold/convert) | E | Also removes KES hardcoding (→ C2). |
| B8 | Lending depth: top-ups, refinancing, restructuring flows, group lending, credit lines | E | Some exists; needs product-factory coverage. |
| B9 | SME/merchant banking: business accounts, invoicing, bulk payouts/payroll, QR acquiring | D | Phase 3+; big revenue expansion. |
| B10 | Rewards/cashback/gamification engine | D | Credit-led growth lever from the reference model. |
| B11 | **Crypto / virtual-asset wallet**: custodial BTC/ETH/stablecoin (USDT/USDC) balances via a regulated custody+rails provider (Fireblocks/Circle class), buy–sell–hold, on-chain send/receive, KES on/off-ramp | D | Integration play, not a chain build — the account service already models multi-type balances; add a VIRTUAL_ASSET account type and a custody connector. Gated on licensing (Kenya VASP Act — CBK/CMA), Travel Rule (FATF R.16) messaging, segregated custody, and blockchain analytics screening (Chainalysis class) wired into the existing AML stack. Stablecoin remittances + inflation-hedge savings are the killer emerging-market use cases; volatile-asset credit exposure stays out of scope. |

### C. Platform packaging — tenant, market, product configuration

| # | Gap | Grade | Notes |
|---|-----|-------|-------|
| C1 | **Tenant provisioning as a product**: "create neobank" wizard/API — licence profile, market pack, product catalogue, brand pack, seeded GL, admin users, sandbox → one click to a running bank | **C** | The literal "one-click neobank". Pieces exist (tenant scoping, licence profiles); the orchestration and console don't. |
| C2 | **Market packs**: country as data — currency/locale, payment-rail connectors, KYC/AML rule sets, tax rules, regulatory report set, holiday calendars, credit-bureau adapters | **C** | Direct answer to the Ethiopia blocker: Kenya must become the *first pack*, not the hardcode. |
| C3 | **Product factory depth**: config-driven builder for any account/loan/fee/interest structure with versioning, approval workflow and simulation ("what does this product cost a customer?") | **C** | Product service exists; needs to reach "PM configures, no engineer" depth. |
| C4 | Brand packs / white-labelling: theme tokens, assets, app config, comms templates per tenant | E | |
| C5 | Feature flags & entitlements per tenant/tier (modules on/off = pricing tiers) | E | |
| C6 | Tenant usage metering & platform billing (our revenue engine) | E | |
| C7 | Per-tenant sandbox environments with synthetic data & test rails | E | Also powers sales demos. |

### D. Deployment & operability — "runs anywhere, one click"

| # | Gap | Grade | Notes |
|---|-----|-------|-------|
| D1 | **Helm umbrella chart / k8s operator**: single-command install of the entire platform (services, DBs, RabbitMQ, observability) with sane defaults | **C** | k3s deploy exists but is hand-rolled. This is the "one-click" substrate for cloud *and* on-prem. |
| D2 | **On-prem & air-gapped installer**: offline image bundles, licence activation, no-internet operation | **C** | Data-residency laws make this a wedge feature (vision §principles). |
| D3 | Zero-downtime upgrades: rolling deploys, automated+gated DB migrations, rollback | **C** | Banks cannot take maintenance windows. |
| D4 | HA/DR: multi-AZ/multi-node topology, automated backups + PITR, tested RPO/RTO, failover runbooks | **C** | Prudential regulators ask for this in writing. |
| D5 | GitOps reference (ArgoCD/Flux), env promotion (dev→staging→prod) | E | |
| D6 | Secrets & key management: Vault/KMS integration, key rotation, HSM option for card keys | E | |
| D7 | Performance engineering: load-test baselines, capacity model ("X TPS per node"), horizontal-scaling guides | E | Buyers ask "how many customers per cluster?" |

### E. AI at the core — decisioning & autonomous operations

| # | Gap | Grade | Notes |
|---|-----|-------|-------|
| E1 | **Unified decision engine**: every automated decision (credit, fraud, AML triage, limits, pricing) through one policy layer — champion/challenger, human-review thresholds, full decision log with explanations | **C** | Exists piecemeal (scoring, fraud); needs the spine that makes "AI-operated bank" true and auditable. |
| E2 | **Straight-through credit**: application → score → decision → disbursement with no human for in-policy cases; adverse-action reasons generated for declines | **C** | Regulator-friendly explainability is the constraint. |
| E3 | **Agentic operations (AIOps)**: AI agents that watch telemetry + business metrics, diagnose (event lag, recon breaks, stuck queues), draft or execute runbook remediations with approval gates | D | The "minimal human intervention" ask. Start with draft-only agents, graduate to auto-remediate. |
| E4 | AI collections: risk-based strategies, best time/channel to contact, hardship detection, message generation | D | Direct P&L impact; strong demo. |
| E5 | AML/fraud triage copilot: alert summarisation, entity resolution, draft SARs for officer review | D | Compliance teams are the cost centre this shrinks. |
| E6 | Customer-facing AI: support chat with banking actions (balance, block card, dispute), personalised nudges & product recommendations, churn prediction | E | Feeds A6. |
| E7 | **Model governance**: registry, versioning, drift/performance monitoring, bias testing, kill switches, per-market model configs | **C** | Without it, E1–E6 are unsellable to bank risk committees. |
| E8 | Data platform to feed it all: lakehouse/warehouse, feature store, tenant-scoped BI for bank execs, regulatory data marts | E | Reporting service exists; this is its grown-up form. |

### F. Trust — security, compliance, certifications

| # | Gap | Grade | Notes |
|---|-----|-------|-------|
| F1 | Security hardening to bank grade: mTLS service mesh, WAF, rate limiting, secrets scanning, SAST/DAST in CI, pen-test cycle | **C** | |
| F2 | Certification roadmap: **PCI-DSS** (required for B1 cards), ISO 27001, SOC 2 Type II | **C** | Sales-blocking for banks; start early, they take 12+ months. |
| F3 | Data protection: Kenya DPA/GDPR-style tooling — consent, retention, right-to-erasure, data residency controls | E | |
| F4 | Strong customer auth: device binding, biometrics, transaction signing, step-up auth, SIM-swap defence | **C** | Fraud service helps; the auth layer itself is thin. |
| F5 | Regulatory reporting as market-pack content for each new country (NBE for Ethiopia next) | E | Kenya stack exists and becomes the template. |

### G. Ecosystem — integrations & developer experience

| # | Gap | Grade | Notes |
|---|-----|-------|-------|
| G1 | **Payment-rail connector framework**: mobile money (M-Pesa first-class), PesaLink/RTGS/ACH, ISO 20022 readiness; connectors selected per market pack | **C** | Abstraction exists implicitly; make it a pluggable SDK. |
| G2 | Public API platform: partner-facing REST APIs, webhooks, API keys/OAuth, developer portal with docs & sandbox | E | Also the open-banking readiness story. |
| G3 | Credit-bureau adapter framework (CRB today → per-market bureaus) | E | |
| G4 | Accounting/ERP export, core-bank coexistence interfaces (for partner banks keeping their ledger of record) | D | |

### H. Support & operations mechanisms (the user's "full support" ask)

| # | Gap | Grade | Notes |
|---|-----|-------|-------|
| H1 | **Full observability stack shipped in the box**: metrics (Prometheus), logs (Loki — partial today), traces (OpenTelemetry), dashboards per service *and per business flow* | **C** | Baseline Grafana/promtail exists; tracing and business-flow dashboards don't. |
| H2 | **Business-level monitoring**: GL imbalance, reconciliation breaks, event-bus lag, payment success rates, disbursement funnel, queue depths — alerting with SLOs & error budgets | **C** | The F27 class of bug (silently dropped events) is exactly what this catches. |
| H3 | Alerting & incident management: Alertmanager → on-call (PagerDuty/Opsgenie), runbook library, status page per tenant | **C** | |
| H4 | Ops console: customer 360, audited impersonation w/ maker-checker, transaction search, manual-adjustment workflows | E | Portal is the seed; needs support-team ergonomics. |
| H5 | Reconciliation engine: automated matching vs rails/processors (M-Pesa, cards), break queues (→ E3 for AI handling) | **C** | Unreconciled money is how neobanks die. |
| H6 | Customer support tooling: ticketing/CRM integration, dispute & chargeback workflow (pairs with B1), complaint regulatory clock | E | |
| H7 | Vendor-grade support model: versioned releases + LTS, upgrade paths, SLA tiers, telemetry phone-home (opt-in) for proactive support | E | What "we are a tech *provider*" means operationally. |

---

## 3. Roadmap

Sequencing logic: **P1 makes the claim true enough to demo, P2 makes it sellable, P3
makes it winning.** Lending revenue depth (week-N money-path work) continues throughout.

### Phase 1 — Reposition & package (≈ quarter 1)
The platform becomes *installable, configurable Nemo* rather than "our LMS deployment".

- Nemo naming layer: docs (this), portal branding, repo/service rename plan [C]
- **D1** Helm umbrella chart — one-command full-platform install
- **C1** Tenant provisioning API + minimal console ("create neobank" end-to-end to sandbox)
- **C2** Market-pack skeleton; extract Kenya hardcoding into the first pack (unblocks Ethiopia)
- **H1/H2/H3** Observability: OTel tracing, business-metric exporters (GL balance, event lag, payment success), Alertmanager with a starter SLO/alert pack
- **A2** Self-service onboarding APIs (eKYC vendor integration, risk-tiered auto-approve)
- **E1** Decision-engine spine v1: route existing credit + fraud scoring through one logged, explainable policy layer

### Phase 2 — The visible neobank (≈ quarters 2–3)
What a customer/investor sees in a demo is a real bank in an app.

- **A1** White-label mobile app v1: onboarding, accounts, P2P (B2), bills/airtime (B3), loan self-service — brand-pack themed
- **B1** Virtual card issuing via one processor; freeze/limits; dispute workflow skeleton
- **B4** Savings pots with round-ups; **B5** scheduled payments; **B6** term-deposit lifecycle
- **H5** Reconciliation engine v1 (M-Pesa + card processor)
- **E2** Straight-through credit with adverse-action explanations; **E7** model governance v1
- **D2/D3/D4** On-prem installer, zero-downtime upgrades, documented HA/DR with tested RPO/RTO
- **F1/F4** Security hardening + strong customer auth; **F2** PCI-DSS gap assessment starts

### Phase 3 — AI-operated & ecosystem (≈ quarters 3–4)
The differentiators that win against Mambu-plus-SI and DIY.

- **E3** AIOps agents (draft-mode first: recon breaks, event-lag diagnosis, runbook drafts)
- **E4/E5** AI collections + AML/fraud triage copilot; **E6** customer AI chat with actions
- **A4** USSD + agent network channel
- **G1** Rail-connector SDK; second market pack (**Ethiopia/NBE**) proving config-over-code
- **G2** Public API platform + developer portal; **C6** tenant metering & billing
- **E8** Data platform (lakehouse + tenant BI); **H7** vendor support model & LTS releases
- **B9/B10** SME banking, rewards engine (as demand dictates)

### Standing tracks (never "done")
Money-path correctness & audits · regulatory currency per market · security posture ·
performance/capacity · certification programme (PCI-DSS → ISO 27001 → SOC 2).

---

## 4. Top 10, if we can only do ten

1. **D1** One-command install (Helm) — the "one-click" claim
2. **C1** Tenant provisioning — the "neobank in a box" claim
3. **C2** Market packs w/ Kenya extraction — the "any market" claim
4. **A1** Customer mobile app — the "neobank" claim
5. **A2** Self-service eKYC onboarding
6. **B1** Virtual cards
7. **B2+B3** P2P + bills
8. **E1+E2** Decision engine + straight-through credit — the "AI at the core" claim
9. **H1+H2+H3** Full observability & business alerting — the "fully supported" claim
10. **H5** Reconciliation engine — the "sleep at night" claim
