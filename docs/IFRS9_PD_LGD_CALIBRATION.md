# IFRS 9 PD / LGD Calibration — Readiness & Methodology

**Status:** Calibration **deferred — blocked on data**, not on engineering.
**Date:** 2026-06-29 · **Owner:** Finance/Risk + Lending platform

The ECL report (`GET /api/v1/loans/ecl-provision`) computes **ECL = EAD × PD × LGD**
per IFRS 9 stage. EAD is measured directly from the loan book (gross outstanding
principal per stage). PD and LGD are currently **point estimates**, not calibrated
from this institution's own loss experience. This document records *why*, what the
current parameters are based on, and exactly what is required to calibrate them.

## 1. Finding: the data required to calibrate does not yet exist

Snapshot of `athena_loans` on 2026-06-29:

| Metric | Value |
|---|---|
| Total loans | 51 |
| Stage distribution | 51 PERFORMING (Stage 1); 0 Stage 2; 0 Stage 3 |
| Status | 51 ACTIVE |
| Loans with DPD > 90 | 0 |
| Observed defaults | 0 |
| Recovery / write-off / charge-off columns | none exist |
| Stage-transition history | none (only current snapshots) |

Consequences:

- **PD is unestimable.** Empirical PD = defaults ÷ exposed accounts. With zero
  observed defaults the maximum-likelihood estimate is 0/51 = 0% for every stage,
  which is meaningless (and would zero the entire provision). There is also no
  time series of stage transitions, so cohort/roll-rate PD cannot be built.
- **LGD is unestimable.** LGD = 1 − recovery rate on defaulted exposure. There are
  no defaulted loans and **no recovery or write-off data is captured anywhere**, so
  realised loss given default cannot be measured.

**Fabricating "calibrated" figures from this data would be worse than the current
transparent benchmark parameters** — it would manufacture false precision and fail
an audit on model governance. The correct interim position is documented benchmark
parameters (below), which is exactly what is in place.

## 2. Basis for the current (interim) parameters

Defined in `go-services/internal/management/repository/portfolio_repo.go`:

| Parameter | Value | Basis |
|---|---|---|
| PD Stage 1 | 2% | 12-month PD, performing book — industry/supervisory benchmark for a healthy microfinance/retail book |
| PD Stage 2 | 20% | Lifetime PD given significant increase in credit risk |
| PD Stage 3 | 100% | Credit-impaired / in default by definition (IFRS 9) |
| LGD | 45% | Unsecured baseline; Basel foundation-IRB senior-unsecured reference LGD |

These are defensible **interim** values: conservative, aligned to recognised
references, and the report exposes EAD/PD/LGD per stage so the provision is fully
auditable and overridable.

## 3. What is needed to calibrate (data prerequisites)

1. **A default definition & flag** — mark a loan defaulted on the IFRS 9 trigger
   (DPD ≥ 90, or unlikeliness-to-pay). Persist the *date* of default, not just a
   current status, so cohorts can be formed.
2. **Stage-transition history** — a monthly snapshot of each loan's stage/DPD
   (e.g. `loan_stage_history(loan_id, as_of_date, stage, dpd, gross_outstanding)`).
   This is the substrate for both roll-rate PD and stage migration matrices.
3. **Recovery & write-off capture** — on default, record the exposure at default
   and every subsequent recovery cash flow and any write-off, so realised LGD =
   1 − (PV of recoveries ÷ EAD) can be measured.
4. **≥ 12 months of seasoned history** (ideally a full economic cycle) before PD/LGD
   are statistically meaningful.

## 4. Calibration method (once data exists)

- **PD** — build 12-month cohorts: of accounts in stage *s* at month *t*, the
  fraction that defaulted within 12 months gives the 12-month PD; chain monthly
  migration matrices for lifetime PD (Stages 2 & 3). Smooth across cohorts; apply a
  forward-looking macro overlay (IFRS 9 requires forward-looking information).
- **LGD** — for the defaulted population, discount realised recovery cash flows to
  the default date, LGD = 1 − recoveries/EAD; segment by collateral/product.
- **Validation & governance** — back-test predicted vs realised loss; document the
  model, owner, and review cadence; keep parameters versioned and overridable.

## 5. Recommended next engineering steps (separate from this report)

1. Add `loan_stage_history` monthly snapshotting (cheap, unblocks PD later).
2. Add default-date + recovery/write-off capture to the loan/collections model.
3. Move PD/LGD from code consts to **governed, versioned configuration** (per-tenant,
   effective-dated) so calibrated values can be applied without a deploy and changes
   are audit-logged.

Until (1)–(2) have accumulated seasoned history, the benchmark parameters in §2
remain the correct, auditable position.
