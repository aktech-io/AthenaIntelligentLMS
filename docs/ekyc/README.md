# Nemo eKYC — Technical Documentation

*2026-08-01. Comprehensive functional + technical documentation of the Nemo eKYC
product ("Nemo KYC"): self-service onboarding with document OCR, 1:1 face match,
tiered liveness, sanctions/PEP screening and risk-tiered auto-approval — and the
roadmap to iBeta ISO/IEC 30107-3 PAD Level 2 as a certified white-label product.
Consolidates and extends `docs/nemo/07`–`10`.*

| # | Document | Audience | Contents |
|---|----------|----------|----------|
| 01 | [Product Overview](01-product-overview.md) | Product, business, partners | What the product is and does; user journey; market/document matrix; personas; regulatory context; provider strategy |
| 02 | [System Architecture](02-system-architecture.md) | Engineering | Context/container diagrams, services and boundaries, deployment topology, data model, configuration |
| 03 | [Flows & Sequence Diagrams](03-flows-and-sequences.md) | Engineering, QA | End-to-end onboarding sequences, verification pipeline, decision/tiering logic, referral queue, failure paths |
| 04 | [Component Reference — ekyc-ml-service](04-component-reference.md) | Engineering, ops | Every endpoint, model, threshold and fallback of the ML engine; fail-safe matrix; provisioning |
| 05 | [Current-State Audit](05-current-state-audit.md) | Everyone | What is built vs planned, verified against code; scorecard; top risks |
| 06 | [Level-2 Upgrade Plan](06-level2-upgrade-plan.md) | Founders, engineering | Bridge SDK (Stage 0), NLD-EA African-faces data campaign, model upgrades, iBeta L1→L2 certification roadmap, business case |

## Reading paths

- **"What are we selling?"** → 01, then 06 §business case.
- **"How does it work?"** → 02 → 03 → 04.
- **"Where are we really?"** → 05, then 06 for what happens next.

## Source documents

The planning history lives in `docs/nemo/`: 07 (OCR-first onboarding),
08 (liveness tiered plan), 09 (build-and-certify), 10 (Stage 0 + NLD-EA campaign).
This set is the consolidated, current statement; on conflict, this set wins.
