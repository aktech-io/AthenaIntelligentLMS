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
- **Screening: normalized fuzzy name matcher** (`engine/names.py`): NFKD
  diacritic stripping + casefold + token-sort/token-set similarity, default
  threshold 0.85. Same algorithm mirrored in the Go provider for
  declared-vs-extracted name comparison.

## Model & data files (deploy-time responsibilities)

**Face models** — the Dockerfile downloads both from the OpenCV model zoo at
build time (best effort; an offline build still succeeds and runs in fallback
mode). To provision manually, place in `/app/models` (or point the env vars
elsewhere):

```
FACE_DETECTOR_MODEL  /app/models/face_detection_yunet_2023mar.onnx
FACE_EMBEDDER_MODEL  /app/models/face_recognition_sface_2021dec.onnx
```
Source: https://github.com/opencv/opencv_zoo (face_detection_yunet,
face_recognition_sface). Verify `/health` reports `"faceEngine": "sface"`.

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

## Configuration

| Env | Default | Purpose |
|---|---|---|
| `EKYC_DATA_DIR` | packaged `data/` | screening list directory |
| `FACE_DETECTOR_MODEL` | `/app/models/face_detection_yunet_2023mar.onnx` | YuNet ONNX |
| `FACE_EMBEDDER_MODEL` | `/app/models/face_recognition_sface_2021dec.onnx` | SFace ONNX |

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
screening) — no tesseract/OpenCV/model files needed, stdlib imports only.
The OCR and face pipelines are exercised via the running service.

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
