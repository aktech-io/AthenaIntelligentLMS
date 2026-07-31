# syntax=docker/dockerfile:1
FROM python:3.12-slim

WORKDIR /app

# tesseract-ocr: the OCR engine (apt package, ~40MB with eng traineddata) —
# chosen over PaddleOCR because it installs reliably in a slim image.
# libglib2.0-0: runtime dep of opencv-python-headless.
# curl/ca-certificates: face-model download below.
RUN apt-get update && apt-get install -y --no-install-recommends \
    tesseract-ocr tesseract-ocr-eng libglib2.0-0 curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install -r requirements.txt

# Face models (YuNet detector + SFace embedder) from the OpenCV model zoo,
# plus MiniFASNetV2 passive PAD (docs/nemo/08; Apache-2.0, minivision-ai
# Silent-Face-Anti-Spoofing, community ONNX export).
#
# Supply-chain contract (docs/ekyc/05 action 1):
#   - offline build (download FAILS)        → warn, ship fallback engines
#     (face score capped at 0.75, liveness always UNKNOWN ≤ 0.5) — same
#     best-effort semantics as before; ops can still drop files into
#     /app/models or mount a volume there (see README "Model & data files").
#   - download SUCCEEDS but SHA-256 differs → **build fails loudly**. A URL
#     that answers with different bytes than the ones we pinned is exactly
#     the supply-chain event checksums exist to catch; never ship it.
# Pinned hashes computed 2026-08-01 from the URLs below.
ARG YUNET_URL=https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx
ARG YUNET_SHA256=8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4
ARG SFACE_URL=https://github.com/opencv/opencv_zoo/raw/main/models/face_recognition_sface/face_recognition_sface_2021dec.onnx
ARG SFACE_SHA256=0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79
ARG MINIFASNET_URL=https://huggingface.co/garciafido/minifasnet-v2-anti-spoofing-onnx/resolve/main/minifasnet_v2.onnx
ARG MINIFASNET_SHA256=d7b3cd9ba8a7ceb13baa8c4720902e27ca3112eff52f926c08804af6b6eecc7b
RUN mkdir -p /app/models && \
    fetch() { \
      url="$1"; dest="$2"; sha="$3"; \
      if curl -fsSL -o "$dest" "$url"; then \
        echo "$sha  $dest" | sha256sum -c - || { \
          echo "ERROR: SHA-256 mismatch for $dest (source: $url) —" \
               "refusing to ship an unverified model. Expected $sha, got" \
               "$(sha256sum "$dest" | cut -d' ' -f1)." >&2; \
          exit 1; }; \
      else \
        echo "WARNING: could not download $url; removing partial file" >&2; \
        rm -f "$dest"; return 1; \
      fi; \
    } && \
    { fetch "$YUNET_URL" /app/models/face_detection_yunet_2023mar.onnx "$YUNET_SHA256" && \
      fetch "$SFACE_URL" /app/models/face_recognition_sface_2021dec.onnx "$SFACE_SHA256" \
      || { echo "WARNING: face models not downloaded; service will use fallback comparator" >&2; \
           rm -f /app/models/face_detection_yunet_2023mar.onnx /app/models/face_recognition_sface_2021dec.onnx; }; } && \
    { fetch "$MINIFASNET_URL" /app/models/minifasnet_v2.onnx "$MINIFASNET_SHA256" \
      || echo "WARNING: PAD model not downloaded; liveness runs in UNKNOWN fallback" >&2; }
# NOTE: the fetch() groups above deliberately use { } (no subshell) so a
# checksum mismatch's `exit 1` aborts the whole RUN — the || rescue only
# catches *download* failures (curl nonzero), never verification failures.

# PP-OCR models (RapidOCR ONNX exports of PaddleOCR's det/rec nets, ~14MB
# total) — the primary OCR engine when present (OCR_ENGINE=auto); Tesseract
# stays installed as the fallback and MRZ second-reader. Checksums pinned:
# these exact files were validated end-to-end (engine/ppocr.py docstring).
ARG PPOCR_DET_URL=https://huggingface.co/SWHL/RapidOCR/resolve/main/PP-OCRv4/ch_PP-OCRv4_det_infer.onnx
ARG PPOCR_DET_SHA256=d2a7720d45a54257208b1e13e36a8479894cb74155a5efe29462512d42f49da9
ARG PPOCR_REC_URL=https://huggingface.co/SWHL/RapidOCR/resolve/main/PP-OCRv3/en_PP-OCRv3_rec_infer.onnx
ARG PPOCR_REC_SHA256=ef7abd8bd3629ae57ea2c28b425c1bd258a871b93fd2fe7c433946ade9b5d9ea
ARG PPOCR_DICT_URL=https://raw.githubusercontent.com/PaddlePaddle/PaddleOCR/main/ppocr/utils/en_dict.txt
ARG PPOCR_DICT_SHA256=5662df9d2d03f0e8ca0d3b0649d6acbab904b6a14b3d3521463c71c37c668ce3
# Offline build (curl fails) → warn and fall back to tesseract; but a
# download that SUCCEEDS with the wrong checksum is a supply-chain red flag
# and fails the build.
RUN set -e; \
    dl() { \
      if ! curl -fsSL -o "$2" "$1"; then rm -f "$2"; return 9; fi; \
      echo "$3  $2" | sha256sum -c - \
        || { echo "ERROR: checksum mismatch for $2 — refusing unverified model"; touch /tmp/ppocr-ckfail; return 1; }; \
    }; \
    ok=1; \
    dl "$PPOCR_DET_URL" /app/models/ppocr_det.onnx "$PPOCR_DET_SHA256" || ok=0; \
    dl "$PPOCR_REC_URL" /app/models/ppocr_rec.onnx "$PPOCR_REC_SHA256" || ok=0; \
    dl "$PPOCR_DICT_URL" /app/models/ppocr_keys_en.txt "$PPOCR_DICT_SHA256" || ok=0; \
    if [ -f /tmp/ppocr-ckfail ]; then exit 1; fi; \
    if [ "$ok" != "1" ]; then \
      echo "WARNING: PP-OCR models not downloaded; OCR falls back to tesseract"; \
      rm -f /app/models/ppocr_det.onnx /app/models/ppocr_rec.onnx /app/models/ppocr_keys_en.txt; \
    fi

COPY . .

EXPOSE 8102

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8102", "--workers", "2"]
