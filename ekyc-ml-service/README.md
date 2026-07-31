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

- **OCR: PP-OCR primary, Tesseract fallback** (`OCR_ENGINE=auto|ppocr|tesseract`,
  default `auto` = PP-OCR when its models are provisioned). `engine/ppocr.py`
  runs PaddleOCR's detection/recognition networks as plain ONNX via
  onnxruntime (RapidOCR-style, ~14 MB total) — no paddlepaddle wheel, so the
  original slim-image objection to PaddleOCR is moot, and real-world phone
  photos (skew, noise, low contrast) read far better than under Tesseract.
  Tesseract (`tesseract-ocr` apt + pytesseract, `engine/ocr.py`) remains the
  no-model fallback, and its MRZ-charset pass is appended to PP-OCR output as
  a second MRZ reading when the binary is present. Either way the **MRZ is
  the trust anchor**: passports (TD3) and MRZ-bearing ID cards (TD1) are
  parsed with full ICAO 9303 check-digit verification (`engine/mrz.py`, pure
  stdlib), and a checksum-valid MRZ overrides visual-zone OCR at confidence
  0.99. Visual-zone extraction is label-anchored ("FULL NAMES", "ID NUMBER",
  "DATE OF BIRTH"... — Kenyan-ID-class layouts) with regex fallbacks,
  per-field confidence = mean OCR word/line confidence (discounted for
  fallback heuristics). There is no capped-fallback for OCR: no engine at
  all → 503 (fabricated fields have no safe cap).
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
build time (best effort; an offline build still succeeds and runs in fallback
mode). To provision manually, place in `/app/models` (or point the env vars
elsewhere):

```
FACE_DETECTOR_MODEL  /app/models/face_detection_yunet_2023mar.onnx
FACE_EMBEDDER_MODEL  /app/models/face_recognition_sface_2021dec.onnx
```
Source: https://github.com/opencv/opencv_zoo (face_detection_yunet,
face_recognition_sface). Verify `/health` reports `"faceEngine": "sface"`.

**PP-OCR models** — downloaded at build time with **pinned SHA256 checksums**
(offline build → Tesseract fallback; checksum mismatch → build fails). To
provision manually, place in `/app/models` (or point the env vars elsewhere):

```
PPOCR_DET_MODEL  /app/models/ppocr_det.onnx      (ch_PP-OCRv4_det_infer)
PPOCR_REC_MODEL  /app/models/ppocr_rec.onnx      (en_PP-OCRv3_rec_infer)
PPOCR_REC_DICT   /app/models/ppocr_keys_en.txt   (PaddleOCR en_dict.txt)
```
Source: https://huggingface.co/SWHL/RapidOCR (RapidOCR's ONNX exports of the
PaddleOCR nets; checksums in the Dockerfile). Verify `/health` reports
`"ocr": "ppocr"`.

**Liveness model** — MiniFASNetV2 is an ops drop-in the same way (no
build-time download wired up yet). Place the ONNX file at:

```
FACE_LIVENESS_MODEL  /app/models/minifasnet_v2.onnx
```

Source: the reference weights are MiniVision
https://github.com/minivision-ai/Silent-Face-Anti-Spoofing (Apache-2.0,
PyTorch `.pth`); ready-made ONNX conversions exist, e.g.
https://github.com/johnraivenolazo/face-antispoof-onnx or Hugging Face
`garciafido/minifasnet-v2-anti-spoofing-onnx`. Verify `/health` reports
`"livenessEngine": "minifasnet_v2"`. **Until the file is present**
`/v1/face/liveness` still answers, but always with `label: UNKNOWN` and
`liveScore <= 0.5` (deterministic fallback — same capped-fallback pattern as
face match: an unprovisioned box can never claim `LIVE`).

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
| `OCR_ENGINE` | `auto` | `auto` (PP-OCR if provisioned, else Tesseract) \| `ppocr` \| `tesseract` |
| `PPOCR_DET_MODEL` | `/app/models/ppocr_det.onnx` | PP-OCR detection ONNX |
| `PPOCR_REC_MODEL` | `/app/models/ppocr_rec.onnx` | PP-OCR recognition ONNX |
| `PPOCR_REC_DICT` | `/app/models/ppocr_keys_en.txt` | recognition charset (PaddleOCR dict format) |
| `FACE_DETECTOR_MODEL` | `/app/models/face_detection_yunet_2023mar.onnx` | YuNet ONNX |
| `FACE_EMBEDDER_MODEL` | `/app/models/face_recognition_sface_2021dec.onnx` | SFace ONNX |
| `FACE_LIVENESS_MODEL` | `/app/models/minifasnet_v2.onnx` | MiniFASNetV2 PAD ONNX |

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
Exception: `test_liveness.py` exercises the liveness fallback path and the
endpoint contract; it needs numpy/OpenCV (and FastAPI for the endpoint
tests) but **skips itself** when they're missing and never needs model
files. The OCR and face pipelines are exercised via the running service.

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
