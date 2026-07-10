# Nemo — Vision & Positioning

*Version 1.0 — July 2026*

## One-liner

**Nemo is a neobank in a box**: everything a bank, MFI, fintech or telco needs to launch
and run a fully-fledged digital bank — core banking, lending, payments, cards, compliance,
accounting, fraud and a customer app — delivered as one platform, deployed on cloud or
on-premise in a day, configured (not coded) for each market, and operated largely by AI.

## Why now, why us

The reference for this model is [Fintech Farm](https://www.fintech-farm.com/) (Leobank,
Liobank, Roarbank, Simbank): they pair a licensed local bank with a complete tech stack
and consumer app, and grow credit-led with AI credit models. Their constraint is that
they operate the neobanks themselves, market by market.

Nemo takes the same product thesis — credit-led neobanking for emerging markets, an app
customers love, AI underwriting — and packages it as a **product other operators can
run**. Two things make this credible rather than aspirational:

1. **The core already exists.** This platform is not a lending bolt-on looking for a
   bank; it is a banking core that happens to have started with lending. Today it runs:
   customer accounts (current, savings, wallet, fixed & call deposits) with interest
   accrual, transfers with charges, a full double-entry general ledger with IFRS-grade
   controls (maker-checker, fiscal periods, audit trail), loan origination through
   collections and write-off, overdrafts/BNPL, float/treasury, AML & regulatory
   reporting, ML-backed fraud detection, and AI credit scoring — 16 Go microservices,
   event-driven, multi-tenant from the first table.
2. **Multi-tenancy is the business model.** Every entity is tenant-scoped and tenants
   already carry per-market regulatory license profiles. "Tenant" is not a technical
   detail; **a tenant is a neobank**. One Nemo installation can run many banks; one
   operator can run many markets.

## What Nemo is (and is not)

| Nemo is | Nemo is not |
|---------|-------------|
| A complete neobank operating platform (core + channels + ops) | A core-banking library that needs a system integrator |
| Deployable by the customer: their cloud, our cloud, or their data centre | SaaS-only (data-residency laws in our target markets make on-prem a feature, not a compromise) |
| Configured per market: currency, language, rails, tax, reports, products | A codebase forked per country |
| AI-operated: decisions and routine operations are automated with human oversight | A dashboard that emails a human for every exception |
| Credit-led: lending profitability funds free everyday banking | A payments wrapper with no balance-sheet products |

## The tenant model

```
Nemo installation (cloud or on-prem)
└── Tenant = one neobank
    ├── Partner/licence: bank, MFI, DCP or fintech licence profile
    ├── Market pack: country config — currency, languages, payment rails,
    │   KYC rules, tax rules, regulatory report set, holiday calendar
    ├── Product catalogue: accounts, loan products, fees, interest matrices
    ├── Brand pack: app theme, name, assets, comms templates
    └── Its own customers, ledgers, reports and settlement
```

Launching a new neobank = provisioning a tenant + selecting a market pack + configuring
products + applying a brand pack. Target: **one click to a running, compliant sandbox
bank; days (not months) to production.**

## Who buys it

1. **Banks going digital** — a tier-2/3 bank that wants a Leobank-style digital brand
   without a 3-year core replacement. (Fintech Farm's partner banks, but self-operated.)
2. **MFIs & digital credit providers** — already the platform's home turf (CBK DCP
   regime); Nemo upgrades them from lender to bank-grade operation.
3. **Fintechs & telcos** — licensed or bank-partnered, needing the full stack fast.
4. **Neobank operators** — Fintech-Farm-like ventures that want the box without
   building it.

Primary geography: East Africa first (Kenya live-grade today, Ethiopia next), then the
emerging-market band where the reference model is proven — South/Southeast/Central Asia,
MENA, West Africa.

## Business model

- **Platform licence** — per-tenant annual licence (cloud subscription or on-prem term
  licence), tiered by modules (core, lending, cards, AI ops).
- **Performance alignment** — optional revenue-share on lending income for
  operator-partners (the Fintech Farm incentive model, offered rather than required).
- **Usage metering** — active customers / accounts / transactions bands.
- **Services** — market-pack authoring for new countries, integrations, launch support.

## Principles (the bar for "top-notch")

1. **Config over code.** A new market or product must never require a fork. Anything a
   country regulator or product manager changes is data, not Go.
2. **One-click, anywhere.** The same artefact installs on managed k8s, a customer's
   cluster, or an air-gapped data centre. Day-2 operations (upgrade, backup, scale) are
   automated.
3. **AI at the core, humans on the loop.** Credit, fraud, AML triage, collections
   strategy, reconciliation exceptions, incident response — AI decides or drafts,
   humans set policy and review by exception. Every model is explainable, monitored,
   and overridable (regulators require adverse-action reasons).
4. **Observable at every level.** Infrastructure, service, event-bus, and *business*
   telemetry (ledger imbalance, recon breaks, payment success rate, event lag) with
   SLOs and alerting out of the box. If ops needs `kubectl`, we shipped it wrong.
5. **Money paths are sacred.** Double-entry always balances, events are idempotent and
   outboxed, every shilling is traceable end-to-end. (Hard-won in the go-live audits —
   this discipline is a selling point.)
6. **Compliance is a product feature.** Regulatory reporting, AML, data protection and
   audit trails ship per market pack, current with local law.

## Competitive frame

| | Nemo | Fintech Farm | Mambu / Thought Machine | Temenos / legacy cores | DIY |
|---|---|---|---|---|---|
| Full stack incl. consumer app | ✔ | ✔ (self-operated) | ✖ (core only, needs SI) | partial | — |
| On-premise / data residency | ✔ | ✖ | limited | ✔ (heavy) | ✔ |
| Time to launch | days–weeks | months (they run it) | 6–18 months w/ SI | 18 m+ | years |
| AI-native operations | ✔ core design | credit only | ✖ | ✖ | — |
| Emerging-market packs (mobile money, CRB, CBK/NBE) | ✔ | ✔ | ✖ built per project | ✖ | — |
| Cost profile | product licence | revenue share, equity | enterprise SaaS + SI fees | enterprise | headcount |

The wedge: **the only neobank-complete platform an emerging-market operator can run
themselves, on their own infrastructure, with AI doing the operational heavy lifting.**
