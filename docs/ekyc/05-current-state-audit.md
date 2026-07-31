# eKYC Current-State Audit

*Part 5 of the `docs/ekyc/` set (peers: 01-product-overview, 02-system-architecture,
03-flows-and-sequences, 04-component-reference, 06-level2-upgrade-plan). Audit date
**2026-08-01**, verified against code at commit `b84277c` (LMS repo) and `d5d0c14`
(NemoWallet repo) — every claim below carries a file reference; where the plans
(docs/nemo/07–10) and the code disagree, the code wins. Unit tests were re-run as
part of this audit.*

## 1. Scorecard

| Capability | Status | Evidence | Gaps |
|---|---|---|---|
| Document OCR + MRZ | ✅ built | `ekyc-ml-service/engine/ocr.py`, `engine/mrz.py` (ICAO 9303 TD1+TD3, per-check-digit), `api/document.py:9-61` | Tesseract `eng` only — no `amh` traineddata in `Dockerfile:11`; accuracy on real (non-synthetic) documents unmeasured. **Addressed 2026-08-01**: PP-OCR (ONNX/onnxruntime, `engine/ppocr.py`) added as primary engine (`OCR_ENGINE=auto`), checksummed build downloads, validated E2E on clean + degraded synthetic KE-ID images; real-scan corpus still pending |
| Extraction profiles (KE ID / KE passport / ET Fayda / ET passport) | ✅ built | `engine/profiles.py:164-168` registry: `ke-national-id`, `passport-mrz` (covers both passports), `et-fayda` (FAN-16); wired from market packs `packs/ke.yaml:32-40`, `packs/et.yaml:30-38` | Validated on synthetic fixtures only; no corpus of real KE ID / Fayda scans |
| On-device OCR (doc-07 WS-2) | ✅ built (separate repo) | NemoWallet: `pubspec.yaml:27` (`google_mlkit_text_recognition`), `lib/core/kyc/document_ocr.dart`, `lib/core/kyc/mrz_parser.dart`, commit `544fe94` | Lives outside this repo; emulator-tested via gallery upload, limited real-device coverage |
| DocType taxonomy (doc-07 WS-1) | ✅ built | `ekyc/ekyc.go:23-24` (`DocumentType`, `OCRProfile` on Request); `service/onboarding_service.go:63-83` (pack validation + number pattern); `migrations/compliance/6_onboarding_document_type.up.sql`; BFF passthrough `bff/gateway/service/onboarding_service.go:64-71,110-123` | `NATIONAL_ID \| PASSPORT` only; `doc_type` accepted but unused by the engine (`api/document.py:33-35` — profile id alone dispatches) |
| Face match (1:1) | ✅ built | `engine/facematch.py` (YuNet detect + SFace embed); Go call `ekyc/inhouse.go:357-365`; threshold 0.85 `service/onboarding_service.go:22` | Runs in **fallback mode** (Haar + correlation, capped 0.75 at `facematch.py:35,173`) whenever ONNX files are absent — see model provisioning row |
| Tier-1 active challenge (blink/turn/smile) | ✅ built (app-side) | NemoWallet `lib/core/kyc/liveness_challenge.dart:12` (`enum LivenessChallenge { blink, turnLeft, turnRight, smile }`), `face_signals_adapter.dart`, `kyc_selfie_screen.dart`, commit `d5d0c14`; frames flow as `selfieFrameRefs` end-to-end | Challenge completion is **client-verified only** — the server receives frames but cannot prove a challenge happened; a tampered client can send arbitrary frames |
| Tier-2 passive liveness (MiniFASNetV2) | 🟡 partial | `engine/liveness.py` (full ONNX pipeline, 2.7× crop, class-index-1 = live), endpoint `api/face.py`; Go call `ekyc/inhouse.go:200-231` | **Model file not present** (`models/` holds only `.gitkeep`); Dockerfile download source unverified (HF community conversion, `Dockerfile:28`); threshold 0.5 is an explicit placeholder (`liveness.py:42`, `inhouse.go:40-43`) |
| Liveness enforcement / shadow | 🟡 shadow only | `inhouse.go:61,105` (`LIVENESS_ENFORCE`, default false), shadow/enforce branch `inhouse.go:215-231`; score recorded in reasons `service/onboarding_service.go:100-103` | `LIVENESS_ENFORCE` set in **no** deploy manifest (compose/Helm/k8s) — shadow everywhere, which is intended, but shadow scores land only in the free-text `decision_reasons` column: no dedicated DB column, no metric — weak substrate for calibration. **Addressed 2026-08-01**: structured `liveness_score`/`liveness_mode`/`liveness_provider` columns (migration 7) now persist alongside the reason string; a metric is still open |
| Tier-3 bridge SDK (doc-09 Stage 0) | 🔴 not started | No code. `grep -rn LivenessProvider go-services/` → 0 hits | Docs 09/10 describe mounting the bridge "behind the existing LivenessProvider seam" — **that seam does not exist**; liveness is inlined in the `Inhouse` eKYC provider. Vendor outreach is a pending founder action (doc 10). **Seam addressed 2026-08-01**: `internal/compliance/liveness` registry (`LIVENESS_PROVIDER`, default `inhouse`) now exists; the bridge SDK itself remains not started |
| Screening lists | 🟡 demo only | Matcher: `engine/screening.py`, `engine/names.py` (fuzzy, threshold 0.85); data: `data/sanctions_demo.csv`, `data/pep_demo.csv` — 5+5 **fictional** entries | No real OFAC/UN/EU lists anywhere in the repo; no ops cron/refresh tooling exists; nothing prevents demo lists reaching production |
| Referral queue / officer portal | ✅ built | `service/onboarding_service.go:200-247` (List status=REFERRED, Decide), `lms-portal-ui/src/pages/OnboardingReferralsPage.tsx`, cross-tenant officer queue (commit `d8ebbbf`), migration `5_onboarding.up.sql` | Referral view shows `decision_reasons` text; no structured display of per-check evidence (scores, extracted fields) |
| DPIA / compliance artifacts | 🔴 not started | `grep -rl DPIA docs/` → plan references only (docs/nemo/08:55, 10:33) | Required by the Kenya DPA before liveness/face-match at scale, and gates the NLD-EA capture campaign (doc 10 G1) |
| Model provisioning (ONNX actually present) | 🟡 fragile | Repo `models/`: **empty** (`.gitkeep` only). `Dockerfile:23-36`: best-effort build-time download of YuNet + SFace (OpenCV zoo) + MiniFASNetV2 (Hugging Face `garciafido/...`) | Offline/failed build silently ships fallback engines; no checksums pinned; no volume mounts in Helm (`deploy/helm/nemo/templates/ekyc-ml.yaml` — no `volumes:` at all) or compose (`docker-compose.go.yml:467-477`); only `/health` reveals which engine mode is live |
| Test coverage | 🟡 good logic, thin pipeline | ekyc-ml: **67 passed, 5 skipped** (this audit, 0.35 s); Go: 8 tests each in `ekyc/inhouse_test.go`, `service/onboarding_service_test.go`; pytest `tests/test_35_document_types.py`; Flutter `test/core/kyc/{mrz_parser,liveness_challenge,ocr_profiles,face_signals_adapter}_test.dart` | All Python tests are pure-logic (no Tesseract/OpenCV/model needed); the 5 skips are the liveness endpoint tests (FastAPI/numpy absent locally); real OCR/face/PAD pipelines exercised only via a running stack; **zero attack-image corpus** |

## 2. Detail findings

### 2.1 ekyc-ml-service (Python sidecar)

The service matches its README closely — the README is honest about fallbacks and
the code implements them as described:

- **Endpoints**: `/v1/document/extract`, `/v1/face/match`, `/v1/face/liveness`
  (1–5 `frame` parts), `/v1/screen`, `/health` (`main.py:19-42`). `/health`
  self-reports engine modes: `faceEngine: sface|fallback`,
  `livenessEngine: minifasnet_v2|fallback`, screening file list + entry count.
- **Fail-closed contract** is real: no Tesseract → 503 (`api/document.py:18-20`),
  undecodable image → 422; the Go provider hard-errors on any non-200
  (`inhouse.go:423-438`) and the tier policy converts that to REFERRED
  (`onboarding_service.go:159-162`).
- **Fallback caps are correctly conservative**: face-match fallback score capped
  at 0.75 (`facematch.py:35`) — below the 0.85 auto-approve line; liveness
  fallback always `UNKNOWN`, capped at 0.5 (`liveness.py:43,182-188`) — an
  unprovisioned box can never claim LIVE.
- **MRZ engine** (`engine/mrz.py`): TD3 + TD1, all check digits + composite,
  OCR-noise tolerant; checksum-valid MRZ overrides visual zone at 0.99
  confidence. The `passport-mrz` profile deliberately does **not** fall back to
  label extraction when the MRZ fails (`profiles.py:96-111`) so bad passports
  refer to an officer.
- **Liveness pipeline** (`engine/liveness.py`): faithful to the MiniVision
  reference (raw-0..255 BGR preprocessing, 2.7× crop, 80×80), min-across-frames
  scoring, softmax-detection guard. But it has **never run against real weights
  in this repo** — the class-order assumption (`liveness.py:19-24,41`) and the
  HF ONNX conversion are unverified. `_LIVE_THRESHOLD = 0.5` is annotated as
  provisional (`liveness.py:42`).
- **Screening**: the matcher is solid and tested; the data is 10 fictional rows.
  Loading is glob-based (`sanctions*.csv`/`pep*.csv` under `EKYC_DATA_DIR`), so
  production readiness is purely an ops drop-in — which no tooling in this repo
  performs (nothing in `deploy/` or `scripts/` touches list refresh).

### 2.2 Go compliance-service

- **Provider registry** (`ekyc/ekyc.go:58-77`): `EKYC_PROVIDER` selects
  `sandbox` (built-in) or `inhouse` (registered in
  `cmd/compliance-service/main.go`). **The code default when the env var is
  unset is `sandbox`** — a deterministic demo provider that auto-approves
  anything with media refs at face-match 0.97 (`ekyc.go:89-101`). All three
  deploy targets pin `inhouse` (compose `docker-compose.go.yml:281`, Helm
  `values.yaml:89`, k8s `deploy/k8s/contabo/lms-nemo.yaml:505`), but a
  misconfigured environment fails **open**, not closed.
  **Addressed 2026-08-01**: unset `EKYC_PROVIDER` is now a startup error
  (dev escape hatch `EKYC_ALLOW_SANDBOX_DEFAULT=true`, set in no manifest).
- **Inhouse provider** (`ekyc/inhouse.go`): orchestrates media fetch → extract
  (with `doc_type` + `profile` from the market pack) → face match → screen →
  liveness. Thresholds: field-confidence floor 0.60, name-match 0.75, liveness
  0.5 placeholder, ≤5 PAD frames (`inhouse.go:34-46`). Name similarity mirrors
  `engine/names.py` including a difflib-equivalent ratio (`inhouse.go:447-553`).
- **`LivenessPassed` today is still "a selfie face was found"**
  (`inhouse.go:195`) while shadow mode is on; only `LIVENESS_ENFORCE=true`
  switches it to the PAD verdict (`inhouse.go:224-226`). This matches doc 08's
  rollout plan.
- **Tier policy** (`onboarding_service.go:159-186`): screening hit → HIGH/
  REFERRED; any failed check → MEDIUM/REFERRED; all pass → LOW/AUTO_APPROVED;
  v1 never auto-rejects. Officer Decide flow + KYC record materialization are
  complete and tested.
- **Migrations**: `5_onboarding.up.sql` (applications table, one-open-per-
  identity partial unique index), `6_onboarding_document_type.up.sql`
  (docType column). **No column for liveness score/mode** — shadow output
  exists only inside the `decision_reasons` string
  (`onboarding_service.go:100-103`).
  **Addressed 2026-08-01**: `7_liveness_observability.up.sql` adds
  `liveness_score`/`liveness_mode`/`liveness_provider` (nullable), populated
  by Submit and exposed through the API and the referral portal dialog.

### 2.3 Mobile app (NemoWallet — separate repo, `/home/adira/projects/aktech/NemoWallet`)

Doc-07 WS-2 and the doc-08 Tier-1 challenge **have landed**, but in the wallet
repo, not this one:

- Doc-type picker (`kyc_doc_type_screen.dart`, `document_types_provider.dart`),
  on-device ML Kit OCR (`document_ocr.dart`), Dart MRZ parser with fragment
  recovery (`mrz_parser.dart`, commit `bce7d8e`), pre-filled confirmation
  screen (`kyc_details_screen.dart`).
- Active challenge (`liveness_challenge.dart`): randomized blink / turnLeft /
  turnRight / smile with eye-open, smile-probability and Euler-yaw thresholds,
  at most one head turn per sequence; frame capture per completed step;
  frames upload as `selfieFrameRefs` (`onboarding_repository.dart:66-82`).
- Unit tests exist for the MRZ parser, challenge state machine, OCR profiles
  and face-signal adapter (`test/core/kyc/`). Real-camera liveness remains a
  physical-device task (emulators can't exercise it), per doc 08 §4.

### 2.4 Deployment wiring

- Compose overlay: `ekyc-ml-service` on 28102 with `/health` healthcheck
  (`docker-compose.go.yml:467-477`); compliance depends on it and defaults
  `EKYC_PROVIDER=inhouse` (`:281`). **No model or data volume mounts.**
- Helm: `ekycMl.enabled=true` (`deploy/helm/nemo/values.yaml:180-186`),
  deployment+service in `templates/ekyc-ml.yaml` — again **no volumes**, so
  model/list provisioning depends entirely on what the image build downloaded.
- `LIVENESS_ENFORCE` appears in no manifest → shadow mode everywhere (correct
  for now, but enforcement day requires touching every deploy target).

### 2.5 Test run (this audit)

```
cd ekyc-ml-service && python3 -m pytest tests/ -v
→ 67 passed, 5 skipped in 0.35s
```

The 5 skips are `tests/test_liveness.py::TestLivenessEndpoint` (needs
FastAPI/numpy, absent in the audit environment — by design the suite
self-skips). The pytest API suites (`tests/test_35_document_types.py`,
`test_12_compliance.py`, `test_24_compliance_comprehensive.py`) call live
services over HTTP via `requests`/`conftest.url` and were **not run** — they
require the full docker-compose stack.

## 3. Top risks

1. **MiniFASNetV2 demographic skew** — training data skews East-Asian; doc 08
   documents elevated false-reject risk in low light and on darker skin tones.
   With threshold 0.5 uncalibrated and no local eval data, flipping
   `LIVENESS_ENFORCE=true` today would reject genuine East-African users at an
   unknown rate. This is precisely why the NLD-EA campaign (doc 10) exists —
   and that campaign has not started (DPIA gate G1 unmet).
2. **Demo screening lists in production = compliance hole.** The shipped CSVs
   are fictional; the service will happily report `sanctionsHit=false` for a
   real SDN name. There is no list-refresh automation, no "is this demo data?"
   guard, and only a `/health` entry count for ops to alert on.
3. **Silent fallback engines.** Model downloads are best-effort at image build
   (`Dockerfile:29-36`) with no checksums; an offline or 404'd build ships an
   image whose face match is capped-correlation and whose liveness is always
   UNKNOWN. Caps make this fail-safe for decisions, but shadow-mode liveness
   data collected from a fallback deployment is worthless for calibration —
   and nothing but `/health` says which mode is live.
4. **Unverified PAD model provenance.** The MiniFASNetV2 ONNX source is a
   community Hugging Face conversion; the class-order assumption
   (`liveness.py:19-24`) has not been validated against known live/spoof
   images anywhere in the repo. First calibration step must be a sanity eval.
5. **No LivenessProvider seam.** Docs 09/10 plan Stage 0 (bridge SDK) around a
   seam that is not in the code — liveness is welded into the `inhouse` eKYC
   provider. The bridge SDK, VeriFayda routing, and in-house-vs-bridge shadow
   comparison all need this refactor first.
   **Addressed 2026-08-01** — `internal/compliance/liveness` registry
   (`Provider` = `Name()` + `Score(ctx, frames)`, `LIVENESS_PROVIDER` env,
   in-house scorer extracted from `inhouse.go`); bridge SDK can now register.
6. **Shadow scores are unqueryable.** Liveness mode/score live in a free-text
   `decision_reasons` column. Threshold calibration on real traffic (doc 08's
   stated must-do) needs a structured store (column or event) plus a metric.
   **Addressed 2026-08-01** (columns) — migration 7 adds
   `liveness_score`/`liveness_mode`/`liveness_provider`; a Prometheus
   histogram is still open.
7. **Tier-1 challenge is client-attestable only.** The server cannot
   distinguish genuine challenge frames from a replayed video of someone doing
   a challenge; Tier 2 (not yet enforcing) is the only server-side check.
8. **Fail-open provider default.** Unset `EKYC_PROVIDER` resolves to `sandbox`
   (`ekyc.go:68-70`), which auto-approves. Deploy manifests all pin `inhouse`,
   but the safe default would be to refuse to start (or default to `inhouse`)
   in production builds.
   **Addressed 2026-08-01** — `ekyc.FromEnv` now errors on an unset
   `EKYC_PROVIDER` unless `EKYC_ALLOW_SANDBOX_DEFAULT=true` (dev only; verified
   set in no compose/Helm/k8s manifest — all three still pin `inhouse`).
9. **DPIA not started** — legally gates both production
   liveness/face-match enforcement in Kenya and the NLD-EA data campaign.

## 4. Recommended next actions (ranked, feeding doc 06)

1. **Provision and verify the real models**: pin checksummed YuNet/SFace/
   MiniFASNetV2 ONNX files (volume mount or baked layer), add a startup sanity
   eval (known live + known spoof image → expected labels), and alert on
   `/health` engine-mode != expected. Unblocks all liveness work.
2. **Structure the shadow data**: add `liveness_score`/`liveness_mode` columns
   (or a compliance event) + a Prometheus histogram, so calibration has a
   dataset from day one of real traffic.
   *Addressed 2026-08-01 (columns + provider column, API + portal display;
   histogram still open).*
3. **Extract the LivenessProvider seam** (`{frames} → {liveScore, decision,
   provider, auditRef}` registry, doc 08 style) out of `inhouse.go` — the
   prerequisite for Stage-0 bridge SDK, VeriFayda (ET), and A/B shadow.
   *Addressed 2026-08-01 (`internal/compliance/liveness`).*
4. **Kill the demo-list hazard**: ship a list-sync job (OFAC/UN/EU → CSV,
   daily), refuse to report clean screens when only `*_demo.csv` files are
   loaded in production mode, and alert on list staleness.
5. **File the DPIA** (counsel + ODPC) — gates both enforcement and NLD-EA
   capture (doc 10 G1); everything demographic-calibration-related queues
   behind it.
6. **Execute Stage 0 + NLD-EA** per doc 10: bridge-SDK eval (APCER ≤1%,
   BPCER ≤10% on our captures) behind the new seam; 400-subject capture
   campaign with the Monk 7–10 over-sampling.
7. **Build a real-image regression corpus**: genuine KE ID / KE passport /
   Fayda scans + iBeta-L1-repertoire attack images, wired into CI against the
   running sidecar — today all confidence rests on synthetic fixtures.
8. **Close the fail-open default**: make an unset `EKYC_PROVIDER` fatal (or
   `inhouse`) outside dev builds.
   *Addressed 2026-08-01 (unset is fatal; `EKYC_ALLOW_SANDBOX_DEFAULT=true`
   is the dev escape hatch, set in no manifest).*
9. **Enforcement switch dry-run**: once calibrated, stage `LIVENESS_ENFORCE`
   into compose/Helm/k8s manifests explicitly (it appears in none today) with
   referral-not-reject UX confirmed.
