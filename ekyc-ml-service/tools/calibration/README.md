# Liveness threshold calibration

Turns `ekyc.liveness` shadow-mode fusion logs (one structured line per
`POST /v1/face/liveness`, see `api/face.py`) into a markdown threshold
recommendation report. This is the calibration evidence
`docs/nemo/08-liveness-plan.md` requires before `LIVENESS_ENFORCE=true`
(gate in `go-services/internal/compliance/ekyc/inhouse.go`, placeholder
threshold 0.5).

Dependencies: **stdlib + numpy** only (matplotlib optional for PNG plots,
no pandas — matches the lean service image).

## Quick start

Pull the last 48h of shadow logs from the Contabo box and run
(deployment name per `deploy/helm/nemo/templates/ekyc-ml.yaml`:
`nemo-ekyc-ml`, namespace `lms`):

```bash
ssh deploy@lms.athenafinance.cloud \
  "sudo kubectl -n lms logs deploy/nemo-ekyc-ml --timestamps --since=48h" \
  | python -m tools.calibration run --logs - --out tools/calibration/reports/calibration.md
```

(run from `ekyc-ml-service/`; `--timestamps` matters — the log body carries
no timestamp of its own, and dedupe/enforcement-replay ordering use it.)

Ingested records accumulate in a jsonl store
(`tools/calibration/store/records.jsonl`, gitignored), so re-running the
one-liner daily keeps extending the analyzed history; overlapping log pulls
deduplicate. Analyze the accumulated store without new input:

```bash
python -m tools.calibration run --out tools/calibration/reports/calibration.md
```

## With outcome weak labels

Export onboarding outcomes from the compliance DB (columns per
`go-services/internal/compliance/repository/onboarding_repository.go`):

```sql
\copy (
  SELECT id AS application_id, created_at, decided_at, status, risk_tier,
         decision_reasons, liveness_score, liveness_mode, liveness_provider
  FROM onboarding_applications
  WHERE liveness_mode IS NOT NULL
  ORDER BY created_at
) TO 'outcomes.csv' WITH CSV HEADER
```

```bash
python -m tools.calibration run --logs shadow.log --outcomes outcomes.csv
```

Officer-approved referrals count as weak-genuine, officer rejections whose
reasons mention liveness/spoofing as weak-suspicious. These are **proxies**
(the report carries the caveats): BPCER/APCER figures are not lab
measurements.

## What the report contains

1. Data provenance + malformed-line count + caveats
2. Distributions (fused score, padMedian/padMin, parallax/motionPx,
   moiré/moirePeakRatio) — text histograms, PNGs when matplotlib exists
3. Old-vs-new policy A/B: verdict flips between the retired min-frame
   policy (`padMin`) and the fused score, per threshold
4. Component correlation matrix (fusion-weight sanity)
5. Threshold sweep 0.05–0.95 with per-stratum pass-rates
   (engine, frame count, challenge, provider/mode when joined) and the
   BPCER-proxy operating range
6. Recommendations: fused-score threshold (with confidence basis),
   minimum-frames policy, weight-sanity findings
7. **Enforcement replay** — what `LIVENESS_ENFORCE=true` would have done to
   the last N sessions at the current placeholder (0.5) and recommended
   thresholds, gating on top-level `liveScore` exactly as `inhouse.go` does

## Options

```
python -m tools.calibration run
  --logs <file|->        log file or stdin (omit to analyze the store)
  --outcomes <csv|json>  outcome export for weak labels
  --out <path>           report path (default reports/calibration-<stamp>.md)
  --store <path>         history store (default store/records.jsonl)
  --no-store             one-shot analysis, no persistence
  --bpcer-target 0.05    max tolerated weak-genuine rejection rate
  --last 200             enforcement-replay window
  --join-window 180      outcome join timestamp tolerance (s)
  --no-plots             skip matplotlib PNGs
python -m tools.calibration ingest <file|->   # ingest only
python -m tools.calibration.ingest <file|->   # same, direct module
```

## Gotchas

- `reports/` is deliberately not named `data/` or `logs/` — the repo
  `.gitignore` swallows `**/data/**` and `**/logs/**` (it has bitten source
  directories twice before). Reports are committable; the record store is
  not (contains real-traffic biometrics telemetry).
- The shadow log line is emitted at INFO on the `ekyc.liveness` logger. If a
  deployment's log config drops non-uvicorn INFO records, zero fusion lines
  is a logging problem, not zero traffic — the tool prints a hint when it
  parses nothing.
- Fallback-engine sessions have `liveScore` capped at 0.5 (never
  confidently live); the sweep calibrates the *fused* score, the
  enforcement replay uses `liveScore` as Go does. The report separates the
  two.
