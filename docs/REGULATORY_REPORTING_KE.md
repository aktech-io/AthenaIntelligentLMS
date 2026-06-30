# Regulatory Reporting Spec — Kenya / East Africa (CBK)

**Date:** 2026-06-30 · **Owner:** Finance/Compliance engineering
**Context:** AthenaLMS is an **API-first, multi-tenant white-label** lending + wallet platform
(device finance, invoice finance, Fuliza-style wallet overdraft, mobile lending) deployed
mostly in **East Africa / Kenya**. Tenants/clients carry **different licenses** (standalone
Digital Credit Provider, MFB/bank, or bank-partner), so the regulatory report set and the
provisioning rules **must be per-tenant configuration, not hardcoded**.

This doc = the target reporting obligations + a gap analysis against the current codebase +
a prioritized build order. Rates/formats below must be confirmed against the **current**
CBK/PG, CRB, and FRC publications before go-live; keep them configurable.

---

## 1. Obligation stack (what a Kenyan digital lender must produce)

1. **IFRS financial statements** — Trial balance, Income statement (P&L), Balance sheet, Cash flow.
2. **CBK prudential returns** (bank/MFB-licensed tenants) — loan classification & provisioning,
   NPL ratio, capital adequacy (core/total capital vs RWA), liquidity ratio, large-exposure /
   single-obligor, insider lending, sectoral lending.
3. **CBK Digital Credit Provider returns** (DCP Regs 2022) — periodic loan-book/customer returns,
   **APR / total-cost-of-credit pricing disclosure**, complaints reporting.
4. **CRB feed** (Credit Reference Bureau Regs) — *mandatory* monthly borrower data (positive +
   negative) to a licensed bureau (Metropol / TransUnion / Creditinfo) in their Data
   Specification Template.
5. **AML/CFT** (POCAMLA → Financial Reporting Centre) — STR + Large/Cash Transaction Reports,
   FRC **goAML XML**.
6. **Data Protection** (ODPC, DPA 2019) — consent/audit hooks on the API data flows.

### Provisioning mechanic (drives H-4)
Hold the **higher of IFRS 9 ECL and the CBK prudential provision**. Post the **IFRS movement**
(required ECL − current allowance balance) to P&L (stage-tagged, maker-checker, DRAFT until
PD/LGD calibrated). Book any **excess of CBK provision over IFRS** to a **non-distributable
Statutory Loan Loss Reserve in equity** (appropriation of retained earnings — NOT a P&L charge).

**CBK 5-bucket classification (CONFIRM vs current CBK/PG/04; configurable; specific provisions
net of realizable collateral):**

| Class | Days past due | Provision |
|-------|---------------|-----------|
| Normal | 0–30 | 1% |
| Watch | 31–90 | 3% |
| Substandard | 91–180 | 20% |
| Doubtful | 181–360 | 50% |
| Loss | >360 | 100% |

---

## 2. Gap analysis (current codebase, 2026-06-30)

| # | Requirement | Status | Evidence / note |
|---|-------------|--------|-----------------|
| 1 | Trial balance / GL | ✅ EXISTS | `accounting/handler/handler.go:263`, CSV |
| 2 | P&L / Balance sheet (formal) | ⚠️ PARTIAL | GL-complete; no statement endpoint — derive from TB |
| 3 | Cash flow | ✅ EXISTS | `accounting/handler/handler.go:522`, CSV |
| 4 | PAR / ageing | ✅ EXISTS | `management/repository/portfolio_repo.go:77` |
| 5 | IFRS 9 ECL | ⚠️ PARTIAL | `portfolio_repo.go:191` — **read-only, does NOT post to GL** |
| 6 | CBK 5-bucket classification | ❌ WRONG BANDS | `management/model/model.go:24-31` — labels match CBK but DPD thresholds do not (Loss at >90 vs CBK >360). Needs a separate, correctly-banded CBK classification distinct from the internal/IFRS staging |
| 7 | NPL ratio | ⚠️ PARTIAL | `lossLoans` count only (`reporting/model/model.go:66`); no ratio endpoint |
| 8 | Capital adequacy / liquidity | ❌ ABSENT | bank/MFB tenants only |
| 9 | CRB borrower feed | ❌ ABSENT | **mandatory; go-live blocker** |
| 10 | AML / SAR / CTR | ✅ EXISTS | `fraud/handler/handler.go:78`, `compliance/handler/handler.go:152`; **FRC goAML XML format unverified** |
| 11 | Concentration / large-exposure / sectoral | ❌ ABSENT | bank/MFB tenants only |
| 12 | Wallet / overdraft / BNPL reporting | ⚠️ PARTIAL | overdraft billing `overdraft/handler/handler.go:350`; invoice-finance reporting absent |
| 13 | DCP APR / total-cost-of-credit disclosure | ❌ ABSENT | DCP Regs requirement |
| 14 | Per-tenant regulatory profile | ❌ ABSENT | org settings exist but no license-type/report-set config |

---

## 3. Recommended build order

1. **Per-tenant regulatory profile** (foundation) — license type → report set + provisioning
   table + CRB target + frequency. Everything below keys off this.
2. **CRB feed** (go-live blocker) — bureau-agnostic generator, chosen bureau as config; the
   monthly borrower performance export in the bureau Data Spec Template.
3. **CBK prudential classification + provisioning → GL (H-4)** — correct CBK bands (separate
   from internal staging) + IFRS-movement posting + statutory loss reserve overlay.
4. **DCP pricing/APR disclosure** — total-cost-of-credit per loan/product.
5. **Formal P&L / Balance sheet + NPL ratio endpoints** — cheap, derive from GL.
6. **Prudential returns** (capital adequacy, liquidity, concentration/large-exposure) — for
   bank/MFB tenants.
7. **AML → FRC goAML XML** — verify/map the existing SAR/CTR to FRC submission format.

---

## 4. Open decisions
- **Which CRB** do clients submit to (Metropol / TransUnion / Creditinfo)? Determines the
  data template. Default: build bureau-agnostic, chosen bureau as config.
- **First license profile to target** — recommend **DCP** (most products fit) with the
  per-tenant profile designed to add MFB/bank prudential returns later.
- Confirm exact CBK provisioning rates/bands + collateral-netting rules against current CBK/PG/04.
