# ekyc-ml-service

In-house eKYC engine for the Nemo platform (gap A2). Python/FastAPI sidecar
called by `compliance-service` when `EKYC_PROVIDER=inhouse` (the default in
the compose overlay and Helm chart). Founder decision: eKYC is **in-house
first**; commercial vendors (Smile ID / Veriff class) remain a pluggable
secondary option per client — see "Plugging in a commercial vendor" below.

Mirrors the `fraud-ml-service` sidecar pattern: FastAPI app, slim Docker
image, called from Go over plain HTTP inside the cluster network (no auth —
never expose this port publicly).

## Endpoints

| Endpoint | Body | Returns |
|---|---|---|
| `POST /v1/document/extract` | multipart `file` (ID-document image) | `fields` map (`fullName`, `documentNumber`, `dateOfBirth`) each `{value, confidence}`; `mrz` block when a machine-readable zone was found (with per-check-digit results and `valid`) |
| `POST /v1/face/match` | multipart `document` + `selfie` images | `{engine, score (0..1), documentFaceFound, selfieFaceFound}` |
| `POST /v1/face/liveness` | multipart, 1..5 `frame` parts (selfie frames) | `{liveScore (0..1, min across frames), label (LIVE\|SPOOF\|UNKNOWN), perFrame:[{score, faceFound}], model}` |
| `POST /v1/screen` | JSON `{"fullName": "...", "threshold": 0.85?}` | `{sanctionsHit, pepHit, matches[], listsLoaded}` |
| `GET /health` | — | engine availability: OCR, face engine mode, loaded list files |

Error contract: the service **fails loudly** (4xx/5xx) rather than returning
fabricated results — no OCR binary → 503, no screening lists → 503,
undecodable image → 422. The Go provider treats any non-200 as a hard error
and the onboarding flow refers the applicant to a human (fail closed).

## Implementation choices

- **OCR: Tesseract** (`tesseract-ocr` apt package + pytesseract). Chosen over
  PaddleOCR because it adds ~40 MB and installs deterministically in a
  `python:3.12-slim` image, while paddlepaddle is a several-hundred-MB wheel
  with a history of breaking on slim images. Accuracy is adequate because the
  **MRZ is the trust anchor**: passports (TD3) and MRZ-bearing ID cards (TD1)
  are parsed with full ICAO 9303 check-digit verification (`engine/mrz.py`,
  pure stdlib), and a checksum-valid MRZ overrides visual-zone OCR at
  confidence 0.99. Visual-zone extraction is label-anchored ("FULL NAMES",
  "ID NUMBER", "DATE OF BIRTH"... — Kenyan-ID-class layouts) with regex
  fallbacks, per-field confidence = mean OCR word confidence (discounted for
  fallback heuristics).
- **Face match: YuNet + SFace** ONNX models (~2 MB total, OpenCV model zoo)
  run by `opencv-python-headless` — no GPU, no extra ML framework. Cosine
  similarity is mapped to [0,1] so SFace's published verification threshold
  (0.363) lands exactly on the compliance-service auto-approve threshold
  (0.85). When model files are absent the service degrades to a
  **deterministic fallback** (Haar cascade detection + normalized-correlation
  crop comparison) whose score is hard-capped at 0.75 — below the 0.85
  auto-approve line — so a fallback-scored applicant always reaches a human.
  The `engine` field in the response says which path ran; `/health` reports
  `faceEngine: sface|fallback`.
- **Passive liveness (Tier-2 PAD): MiniFASNetV2** ONNX (~600 KB, MiniVision
  Silent-Face-Anti-Spoofing, Apache-2.0) run by the same OpenCV dnn stack —
  an 80×80 face crop (YuNet detection widened by the standard 2.7× scale
  box) is classified live vs presentation attack; `liveScore = P(live)` and
  the reported score is the **minimum across frames**. When the model file
  is absent the engine degrades to a **deterministic fallback** that always
  labels `UNKNOWN` with `liveScore` hard-capped at 0.5 — it never fabricates
  a `LIVE` verdict, so an unprovisioned deployment cannot pass liveness on
  its own. Enforcement is a Go-side decision (`LIVENESS_ENFORCE`,
  shadow-mode first — see `docs/nemo/08-liveness-plan.md`); `/health`
  reports `livenessEngine: minifasnet_v2|fallback`.
- **Screening: normalized fuzzy name matcher** (`engine/names.py`): NFKD
  diacritic stripping + casefold + token-sort/token-set similarity, default
  threshold 0.85. Same algorithm mirrored in the Go provider for
  declared-vs-extracted name comparison.

## Model & data files (deploy-time responsibilities)

**Face models** — the Dockerfile downloads both from the OpenCV model zoo at
build time and **verifies pinned SHA-256 checksums** (`YUNET_SHA256`,
`SFACE_SHA256` build args). Semantics: an offline build (download *fails*)
still succeeds and runs in fallback mode, but a download that *succeeds with
the wrong hash fails the build loudly* — a URL answering with unexpected
bytes is the supply-chain event the pins exist to catch. To provision
manually, place in `/app/models` (or point the env vars elsewhere; in Helm,
mount a volume via `ekycMl.modelsPvc` / `ekycMl.modelsVolume`):

```
FACE_DETECTOR_MODEL  /app/models/face_detection_yunet_2023mar.onnx
FACE_EMBEDDER_MODEL  /app/models/face_recognition_sface_2021dec.onnx
```
Source: https://github.com/opencv/opencv_zoo (face_detection_yunet,
face_recognition_sface). Verify `/health` reports `"faceEngine": "sface"` —
or make the deployment enforce it with `EXPECTED_FACE_ENGINE=sface`
(readiness gating below).

**Liveness model** — MiniFASNetV2 is downloaded at build time the same way
(`MINIFASNET_URL` + `MINIFASNET_SHA256` args, same checksum-or-fail /
offline-fallback semantics), or dropped in by ops at:

```
FACE_LIVENESS_MODEL  /app/models/minifasnet_v2.onnx
```

Source: the reference weights are MiniVision
https://github.com/minivision-ai/Silent-Face-Anti-Spoofing (Apache-2.0,
PyTorch `.pth`); the pinned build-time download is the Hugging Face
conversion `garciafido/minifasnet-v2-anti-spoofing-onnx`
(sha256 `d7b3cd9b…`, computed 2026-08-01);
https://github.com/johnraivenolazo/face-antispoof-onnx is an alternative
conversion (override `MINIFASNET_URL`/`MINIFASNET_SHA256` to use it — the
class-order sanity eval in docs/ekyc/05 §top-risks applies to any
conversion). Verify `/health` reports `"livenessEngine": "minifasnet_v2"`,
or enforce with `EXPECTED_LIVENESS_ENGINE=minifasnet_v2`. **Until the file
is present** `/v1/face/liveness` still answers, but always with
`label: UNKNOWN` and `liveScore <= 0.5` (deterministic fallback — same
capped-fallback pattern as face match: an unprovisioned box can never claim
`LIVE`).

**Readiness gating** — the safe-degradation modes above are deliberate, but
a *silently* degraded pod is still an ops problem (fallback face scores all
land with a human; fallback liveness data is useless for calibration). Set
the deployment's expectations and `/health` turns them into readiness:

```
EXPECTED_FACE_ENGINE      sface | fallback   (empty/unset = don't care)
EXPECTED_LIVENESS_ENGINE  minifasnet_v2 | fallback
```

When set and the live mode differs, `GET /health` answers **503** with a
`degraded` list naming each mismatch, so a Kubernetes readinessProbe keeps
the pod out of the Service instead of letting it quietly serve fallback
verdicts (in the Helm chart: `ekycMl.expectedEngines: {face: sface,
liveness: minifasnet_v2}`; its livenessProbe is TCP-only so a degraded pod
is quarantined, not restart-thrashed). Unset = today's behavior: any mode
is healthy.

**Screening lists** — the matcher loads every `sanctions*.csv` and `pep*.csv`
in `EKYC_DATA_DIR` (default: the packaged `data/`). The shipped
`sanctions_demo.csv` / `pep_demo.csv` are fictional development data —
**production deployments must drop in real consolidated lists** (ops cron,
daily refresh):

- OFAC SDN + Consolidated — https://sanctionslist.ofac.treas.gov/
- UN Security Council Consolidated List
- EU Consolidated Financial Sanctions List
- PEP: commercial feed or curated national lists (separate `pep*.csv`)

CSV shape: `id,name,aliases,country,program` with aliases `;`-separated.
In Kubernetes, mount a volume/ConfigMap at `EKYC_DATA_DIR` and remove the
demo files. `/v1/screen` returns `listsLoaded` and `/health` returns entry
counts so ops can alert on stale/empty data.

**Demo-list production guard** — `EKYC_ALLOW_DEMO_LISTS` (default: allow,
dev-friendly). When set to `false` and ONLY `*_demo.csv` files are loaded,
`/v1/screen` answers **503** (fail closed, same as the no-lists case — a
real SDN name screened against fictional rows would otherwise come back
clean) and `/health` reports `screeningListsMode: demo-only` + degraded
(503). The Helm chart sets `false` (production posture,
`ekycMl.allowDemoLists`); the compose overlay leaves the default.

**List-sync tooling** — `scripts/sync-screening-lists.py` (repo root, pure
stdlib) downloads OFAC SDN + Consolidated and the UN consolidated XML —
plus the EU FSF export when you pass your tokened URL — and converts them
to the CSV shape above as `sanctions_ofac.csv` / `sanctions_un.csv` /
`sanctions_eu.csv`. Writes are temp-file + atomic rename and a failed or
suspiciously small download can never clobber a good previous file; any
source failing exits nonzero for alerting. `--verify` (no downloads) checks
the files exist, are populated and are fresher than `--max-age-hours`.

```bash
# daily refresh into the mounted EKYC_DATA_DIR (03:10, then freshness-check at 09:00)
10 3 * * * /usr/bin/python3 /opt/nemo/scripts/sync-screening-lists.py /srv/nemo/screening-lists \
    --eu-url "https://webgate.ec.europa.eu/fsd/fsf/public/files/xmlFullSanctionsList_1_1/content?token=<your-token>" \
    >> /var/log/nemo/list-sync.log 2>&1
0 9 * * * /usr/bin/python3 /opt/nemo/scripts/sync-screening-lists.py /srv/nemo/screening-lists --verify \
    || echo "screening lists stale" | mail -s "nemo list-sync ALERT" ops@example.com
```

PEP lists remain a manual/commercial responsibility (no free consolidated
source to automate); drop them in as `pep*.csv` alongside.

## Configuration

| Env | Default | Purpose |
|---|---|---|
| `EKYC_DATA_DIR` | packaged `data/` | screening list directory |
| `EKYC_ALLOW_DEMO_LISTS` | `true` (allow) | `false` → 503 on `/v1/screen` + degraded `/health` when only `*_demo.csv` lists are loaded (production guard; Helm sets `false`) |
| `FACE_DETECTOR_MODEL` | `/app/models/face_detection_yunet_2023mar.onnx` | YuNet ONNX |
| `FACE_EMBEDDER_MODEL` | `/app/models/face_recognition_sface_2021dec.onnx` | SFace ONNX |
| `FACE_LIVENESS_MODEL` | `/app/models/minifasnet_v2.onnx` | MiniFASNetV2 PAD ONNX |
| `EXPECTED_FACE_ENGINE` | unset (don't care) | `sface`\|`fallback` — `/health` 503s when the live face engine mode differs (readiness gating) |
| `EXPECTED_LIVENESS_ENGINE` | unset (don't care) | `minifasnet_v2`\|`fallback` — same gating for the liveness engine |

Go-side (compliance-service): `EKYC_PROVIDER=inhouse`,
`EKYC_ML_SERVICE_URL`, `MEDIA_SERVICE_URL` (+ `LMS_INTERNAL_SERVICE_KEY` for
media auth). Thresholds live in `internal/compliance/ekyc/inhouse.go`
(field-confidence floor 0.60, name-match 0.75) and the tiering policy
(face-match 0.85) in `internal/compliance/service/onboarding_service.go`.

## Tests

```bash
cd ekyc-ml-service && python3 -m pytest tests/ -v
```

Pure-logic only (MRZ check digits, field post-processing, name matcher,
screening, readiness gating, list-sync parsers) — no tesseract/OpenCV/model
files needed, stdlib imports only. Exceptions: `test_liveness.py` exercises
the liveness fallback path and endpoint contract, and `test_readiness.py`
exercises the `/health` gating endpoint; both need numpy/OpenCV/FastAPI for
those parts but **skip themselves** when they're missing and never need
model files. The OCR and face pipelines are exercised via the running
service.

## Plugging in a commercial vendor instead

The Go side selects the eKYC implementation by name
(`internal/compliance/ekyc/ekyc.go` registry, `EKYC_PROVIDER` env):

1. Implement `ekyc.Provider` (e.g. `smileid.go`) calling the vendor API and
   mapping onto `ekyc.Result` — fail closed on vendor errors, like `inhouse`.
2. `ekyc.Register(...)` it in `cmd/compliance-service/main.go`.
3. Deploy with `EKYC_PROVIDER=<vendor>`; this sidecar can then be disabled
   (`ekycMl.enabled=false` in Helm). Per-client provider choice is just this
   one env var — the onboarding flow, tiering and referral queue are
   provider-agnostic.
