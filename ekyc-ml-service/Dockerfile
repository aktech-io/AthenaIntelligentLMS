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

COPY . .

EXPOSE 8102

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8102", "--workers", "2"]
