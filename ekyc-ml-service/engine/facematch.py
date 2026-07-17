"""Face detection + embedding comparison (document portrait vs selfie).

Primary engine — "sface": OpenCV's FaceDetectorYN (YuNet) for detection and
FaceRecognizerSF (SFace) for 128-d embeddings, both small ONNX models from the
OpenCV model zoo (~2 MB combined) run by opencv-python-headless. No GPU, no
extra ML framework. Model paths are configurable:

    FACE_DETECTOR_MODEL  (default /app/models/face_detection_yunet_2023mar.onnx)
    FACE_EMBEDDER_MODEL  (default /app/models/face_recognition_sface_2021dec.onnx)

The Dockerfile downloads both at build time; if the build environment is
offline, ops mount/copy them at deploy time (see README). Until they exist
the service degrades to the deterministic fallback engine.

Fallback engine — "fallback": Haar-cascade detection (cascade XML ships inside
the opencv wheel) + normalized-correlation comparison of aligned grayscale
face crops. It is deterministic and dependency-free but NOT biometric-grade,
so its score is hard-capped at 0.75 — below the compliance-service
auto-approve threshold (0.85) — meaning a fallback-scored application can
never auto-approve; it always lands with a human. The response's `engine`
field tells the caller which path ran.

Score mapping (sface): SFace's published cosine verification threshold is
0.363. We map cosine -> [0,1] so that 0.363 lands exactly on 0.85 (the
tiering threshold): same-person pairs sit above 0.85, different-person pairs
below, and the Go side never needs to know about cosine space.
"""
from __future__ import annotations

import os
from dataclasses import dataclass

_SFACE_COSINE_THRESHOLD = 0.363  # OpenCV zoo's published verification cutoff
_TIER_THRESHOLD = 0.85  # compliance-service faceMatchThreshold
_FALLBACK_CAP = 0.75  # fallback can never reach auto-approve


def detector_model_path() -> str:
    return os.getenv(
        "FACE_DETECTOR_MODEL", "/app/models/face_detection_yunet_2023mar.onnx"
    )


def embedder_model_path() -> str:
    return os.getenv(
        "FACE_EMBEDDER_MODEL", "/app/models/face_recognition_sface_2021dec.onnx"
    )


def models_available() -> bool:
    return os.path.isfile(detector_model_path()) and os.path.isfile(
        embedder_model_path()
    )


@dataclass
class MatchOutput:
    engine: str  # "sface" | "fallback"
    score: float  # 0..1, comparable against the 0.85 tier threshold
    document_face_found: bool
    selfie_face_found: bool

    def as_dict(self) -> dict:
        return {
            "engine": self.engine,
            "score": round(self.score, 4),
            "documentFaceFound": self.document_face_found,
            "selfieFaceFound": self.selfie_face_found,
        }


def map_cosine_to_score(cos: float) -> float:
    """Piecewise-linear map of SFace cosine similarity to [0,1] with the
    verification threshold pinned to the 0.85 tiering threshold."""
    if cos >= _SFACE_COSINE_THRESHOLD:
        span = 0.6 - _SFACE_COSINE_THRESHOLD  # cosine 0.6+ ~ certain match
        frac = min(1.0, (cos - _SFACE_COSINE_THRESHOLD) / span)
        return _TIER_THRESHOLD + (1.0 - _TIER_THRESHOLD) * frac
    frac = max(0.0, (cos + 1.0) / (_SFACE_COSINE_THRESHOLD + 1.0))
    return _TIER_THRESHOLD * frac


def _decode(image_bytes: bytes):
    import cv2
    import numpy as np

    arr = np.frombuffer(image_bytes, dtype=np.uint8)
    img = cv2.imdecode(arr, cv2.IMREAD_COLOR)
    if img is None:
        raise ValueError("could not decode image")
    # Bound the long side; YuNet degrades on very large inputs.
    h, w = img.shape[:2]
    if max(h, w) > 1600:
        scale = 1600 / max(h, w)
        img = cv2.resize(img, (round(w * scale), round(h * scale)))
    return img


# ─── primary engine ──────────────────────────────────────────────────────────


def _detect_largest_face_yunet(img):
    """Largest face box via YuNet, or None."""
    import cv2

    det = cv2.FaceDetectorYN.create(
        detector_model_path(), "", (img.shape[1], img.shape[0]), 0.7
    )
    _, faces = det.detect(img)
    if faces is None or len(faces) == 0:
        return None
    return max(faces, key=lambda f: f[2] * f[3])


def _match_sface(doc_img, selfie_img) -> MatchOutput:
    import cv2

    doc_face = _detect_largest_face_yunet(doc_img)
    selfie_face = _detect_largest_face_yunet(selfie_img)
    if doc_face is None or selfie_face is None:
        return MatchOutput("sface", 0.0, doc_face is not None, selfie_face is not None)

    rec = cv2.FaceRecognizerSF.create(embedder_model_path(), "")
    emb_doc = rec.feature(rec.alignCrop(doc_img, doc_face))
    emb_selfie = rec.feature(rec.alignCrop(selfie_img, selfie_face))
    cos = float(rec.match(emb_doc, emb_selfie, cv2.FaceRecognizerSF_FR_COSINE))
    return MatchOutput("sface", map_cosine_to_score(cos), True, True)


# ─── deterministic fallback ──────────────────────────────────────────────────


def _detect_largest_face_haar(gray):
    import cv2

    cascade = cv2.CascadeClassifier(
        cv2.data.haarcascades + "haarcascade_frontalface_default.xml"
    )
    faces = cascade.detectMultiScale(gray, scaleFactor=1.1, minNeighbors=5)
    if len(faces) == 0:
        return None
    return max(faces, key=lambda f: f[2] * f[3])


def _match_fallback(doc_img, selfie_img) -> MatchOutput:
    import cv2
    import numpy as np

    gray_doc = cv2.cvtColor(doc_img, cv2.COLOR_BGR2GRAY)
    gray_selfie = cv2.cvtColor(selfie_img, cv2.COLOR_BGR2GRAY)
    fd = _detect_largest_face_haar(gray_doc)
    fs = _detect_largest_face_haar(gray_selfie)
    if fd is None or fs is None:
        return MatchOutput("fallback", 0.0, fd is not None, fs is not None)

    def crop(gray, box):
        x, y, w, h = box
        face = gray[y : y + h, x : x + w]
        face = cv2.resize(face, (128, 128))
        return cv2.equalizeHist(face).astype(np.float32)

    a, b = crop(gray_doc, fd), crop(gray_selfie, fs)
    # normalized cross-correlation of equalized crops, in [-1, 1]
    an, bn = a - a.mean(), b - b.mean()
    denom = float(np.linalg.norm(an) * np.linalg.norm(bn))
    ncc = float((an * bn).sum() / denom) if denom else 0.0
    # histogram correlation as a second, texture-insensitive signal
    ha = cv2.calcHist([a.astype(np.uint8)], [0], None, [64], [0, 256])
    hb = cv2.calcHist([b.astype(np.uint8)], [0], None, [64], [0, 256])
    hist = float(cv2.compareHist(ha, hb, cv2.HISTCMP_CORREL))

    raw = max(0.0, 0.6 * ncc + 0.4 * hist)
    return MatchOutput("fallback", min(_FALLBACK_CAP, raw * _FALLBACK_CAP), True, True)


def match_faces(document_bytes: bytes, selfie_bytes: bytes) -> MatchOutput:
    """Compare the ID-document portrait with the selfie. Engine is chosen by
    model availability; the output always says which one ran."""
    doc_img = _decode(document_bytes)
    selfie_img = _decode(selfie_bytes)
    if models_available():
        return _match_sface(doc_img, selfie_img)
    return _match_fallback(doc_img, selfie_img)
