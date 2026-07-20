# NemoScore ⇄ LMS integration notes (Contabo provisioning)

*2026-07-20. Written for the session provisioning NemoScore into the Contabo k3s
cluster, from a code-level trace of the LMS side. Everything below is verified
against `master` (`1b42992`) and the live box. The API spec both sides share is
`AthenaCreditScore/docs/nemoscore-api.yaml` — the LMS client conforms to it.*

## 1. Topology — two hops, only one is NemoScore's problem

```
overdraft-service ──AI_SCORING_URL(+internal svc key)──▶ ai-scoring-service ──SCORING_API_URL(+X-Api-Key via Kong)──▶ NemoScore
bff-shop         ──────────────────────────────────────▶ (same)                                                      (this deploy)
```

- Hop 1 (overdraft → ai-scoring-service, in-cluster) already works on the box.
  It serves **stored** scoring results only.
- Hop 2 is what this deployment adds. On the box the ConfigMap currently has a
  **circular placeholder**: `SCORING_API_URL=http://ai-scoring-service.lms.svc.cluster.local:8096`
  (points at itself) and no `SCORING_API_KEY`. Replace, don't append.

## 2. The wire contract LMS expects (hop 2)

- `GET {SCORING_API_URL}/api/v1/credit-score/{customerId}` — `customerId` is
  **int64 in the path** (see §4 — it is often a hash, not a real id).
- Auth: `X-Api-Key: {SCORING_API_KEY}` — must match a NemoScore
  `SERVICE_API_KEYS` entry / Kong key-auth consumer.
- Client timeout is **15s** (`internal/scoring/client/external.go`); Kong rate
  limits must tolerate pytest-suite bursts or exempt the LMS consumer.
- Response schema: `ScoreSummary` from `nemoscore-api.yaml`. LMS decodes:
  `customer_id, base_score, crb_contribution, llm_adjustment, pd_probability,
  final_score, score_band, reasoning[], llm_provider, llm_model, scored_at,
  status, data_sufficiency, pd_source, model_version`.
- `status`: `SCORED` | `INSUFFICIENT_DATA` (empty = legacy SCORED).
  `INSUFFICIENT_DATA` → LMS marks the request SKIPPED → manual review. Use it;
  never return 200 with null score fields.
- `score_band`: display labels are fine (`"Very Good"`); LMS normalizes to
  `EXCELLENT|VERY_GOOD|GOOD|FAIR|MARGINAL|POOR`. Unknown strings collapse to
  `POOR` — don't invent new labels.
- `404` = "no score for this customer" — LMS treats it as scoring FAILED for
  that request (manual review), not an outage.

## 3. When LMS actually calls NemoScore

Only `ai-scoring-service` calls out, and only when its RabbitMQ consumer sees
`loan.application.submitted` or `loan.application.approved` on the LMS
exchange. There is **no on-demand call at read time**: overdraft-apply reads
stored results and fails closed if none exist. Implication: a customer who
never had a loan application has no score, and overdraft rejects them by
design. (Whether ai-scoring should score-on-miss at read time is an LMS-side
E1 decision — do not solve it inside NemoScore.)

## 4. GOTCHA — customer identity is a hash, not a shared key

LMS customer IDs are varchar (`CUST-…`, `EODINT-…`). ai-scoring coerces them
to int64 with a Java-style string hash (`h = h*31 + rune`, absolute value —
`FlexibleCustomerID` / consumer `resolveCustomerID`, identical algorithms) and
that hash is what NemoScore receives in the path. Consequences for NemoScore:

- It will receive customer ids it has **never seen** and cannot reverse.
- There is currently **no shared customer registry** between the platforms.
- For the Contabo demo to go green, NemoScore must be able to produce a score
  for an arbitrary unknown int64 id — e.g. a thin-file/scorecard path (fine to
  return `PARTIAL` data_sufficiency), or a demo mode. If it 404s unknown ids,
  every LMS scoring request stays FAILED and the ~120 red tests stay red.
- Real identity federation (LMS pushing customer features/bureau payloads via
  `POST /api/v1/credit-reports` at onboarding, or a shared id) is a design
  decision to schedule — flag it, don't improvise it.

## 5. GOTCHA — band taxonomy mismatch on the overdraft side

NemoScore emits `Excellent..Poor` (PDO 300–850). But overdraft's
`credit_band_configs` (seeded, box + local) contains bands **A/B/C/D** with
0–900 ranges, and `ApplyOverdraft` looks up the config **by band name** from
the score response. So even with NemoScore live, overdraft dies with
`No credit band configuration found for band: GOOD`.

Fix on the LMS side (data-only, no NemoScore work): seed configs for the six
canonical bands, e.g. on the box `athena_overdraft`:

```sql
INSERT INTO credit_band_configs
  (tenant_id, band, min_score, max_score, approved_limit, interest_rate, arrangement_fee, annual_fee) VALUES
  ('system','EXCELLENT',780,850,100000,0.15,1000,500),
  ('system','VERY_GOOD',720,779, 75000,0.18, 800,500),
  ('system','GOOD',     680,719, 50000,0.20, 750,500),
  ('system','FAIR',     640,679, 20000,0.25, 500,300),
  ('system','MARGINAL', 600,639, 10000,0.28, 400,250),
  ('system','POOR',     300,599,  5000,0.30, 250,200)
ON CONFLICT (tenant_id, band) DO NOTHING;
```

(Keep A–D rows; they're harmless. Whoever lands NemoScore should run this or
tell this session to.)

## 6. What the box already has (don't redo)

- 6 ACTIVE loan products seeded (`NANO-001`, `E2E-PL-001`, `BNPL-001`,
  `BNPL-SHOP`, `SME-001`, `GRP-001`), all currently `min_credit_score=0` — so
  product-level score gating is OFF; scoring pressure comes from the
  overdraft/decision paths, not product thresholds.
- Credentials rotated in `lms-secrets` (admin/manager/officer/teller).
  `SCORING_API_KEY` should be **added to that same Secret** (it wins over the
  ConfigMap on duplicate envFrom keys). Update the local gitignored
  `deploy/k8s/contabo/lms-secrets.yaml` as the source of truth.
- Shared infra: PostgreSQL = `deploy/postgres` in `alpa`
  (`postgres.infra.svc.cluster.local` ExternalName), RabbitMQ in `infra`.
  NemoScore DBs can live in the same PG; no conflict with the LMS CI deploy
  (its idempotent DB script only creates `athena_*` LMS databases).
- LMS CI (`.github/workflows/deploy.yml`) rewrites only the 25 lms Deployments/
  Services + the `ghcr-pull` secret. It never touches `lms-go-common`,
  `lms-secrets`, or other namespaces — NemoScore config survives LMS deploys.

## 7. Handover checklist when NemoScore is up

1. Tell the LMS side the in-cluster URL (e.g. `http://kong.<ns>.svc.cluster.local:8000`)
   and the agreed API key (out-of-band; never commit it).
2. LMS side then: set `SCORING_API_URL` in `lms-go-common`, add
   `SCORING_API_KEY` to `lms-secrets`, `rollout restart deploy ai-scoring-service`,
   seed the band configs (§5), rerun the pytest suite over the tunnel.
3. Success criteria: `loan.application.submitted` produces a stored scoring
   result (check `athena_scoring.scoring_results`), overdraft apply returns
   201, and the suite's ~111 scoring-caused errors clear.
