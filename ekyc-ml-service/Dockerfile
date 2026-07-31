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

# Face models (YuNet detector + SFace embedder, ~2MB total) from the OpenCV
# model zoo. Best effort: if the build environment is offline the image still
# builds and the service runs with the deterministic fallback comparator
# (score capped below the auto-approve threshold) until ops place the .onnx
# files in /app/models (see README "Model & data files").
ARG YUNET_URL=https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx
ARG SFACE_URL=https://github.com/opencv/opencv_zoo/raw/main/models/face_recognition_sface/face_recognition_sface_2021dec.onnx
# MiniFASNetV2 passive PAD (docs/nemo/08; Apache-2.0, minivision-ai
# Silent-Face-Anti-Spoofing, ONNX export). Same best-effort semantics:
# absent model → /v1/face/liveness returns UNKNOWN capped at 0.5.
ARG MINIFASNET_URL=https://huggingface.co/garciafido/minifasnet-v2-anti-spoofing-onnx/resolve/main/minifasnet_v2.onnx
RUN mkdir -p /app/models && \
    (curl -fsSL -o /app/models/face_detection_yunet_2023mar.onnx "$YUNET_URL" && \
     curl -fsSL -o /app/models/face_recognition_sface_2021dec.onnx "$SFACE_URL" \
     || (echo "WARNING: face models not downloaded; service will use fallback comparator" && \
         rm -f /app/models/face_detection_yunet_2023mar.onnx /app/models/face_recognition_sface_2021dec.onnx)) && \
    (curl -fsSL -o /app/models/minifasnet_v2.onnx "$MINIFASNET_URL" \
     || (echo "WARNING: PAD model not downloaded; liveness runs in UNKNOWN fallback" && \
         rm -f /app/models/minifasnet_v2.onnx))

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
