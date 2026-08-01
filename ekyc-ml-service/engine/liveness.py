"""Tier-2 passive liveness (presentation-attack detection) on selfie frames.

Primary engine — "minifasnet_v2": MiniVision Silent-Face-Anti-Spoofing
MiniFASNetV2 (Apache-2.0, ~600 KB as ONNX) run by OpenCV's dnn module — the
same no-GPU / no-extra-ML-framework footprint as the YuNet/SFace face-match
models. Face detection reuses the facematch plumbing (YuNet when its model
file is present, Haar cascade otherwise); the detected box is widened by the
standard MiniFASNet 2.7x scale factor and resized to 80x80. Model path is
configurable:

    FACE_LIVENESS_MODEL  (default /app/models/minifasnet_v2.onnx)

Preprocessing follows the minivision-ai reference implementation
(Silent-Face-Anti-Spoofing, anti_spoof_predict.py): the 80x80 crop is fed
as **BGR float32 in the raw 0..255 range** — the repo's custom ToTensor does
NOT divide by 255 and applies no mean/std normalization (it is not
torchvision's ToTensor).

CLASS-ORDER ASSUMPTION (documented per the Tier-2 plan): the reference
repo's test.py treats softmax **index 1 as the genuine/live class**
(``np.argmax(prediction) == 1`` -> "Real Face") with indices 0 and 2 the two
presentation-attack classes (2D/print-style and 3D/replay-style attacks).
live_score = P(class 1). If a given ONNX export already ends in a softmax
layer we detect that (outputs sum to ~1) and skip re-normalizing.

Fallback engine — "fallback": when the ONNX file is absent the engine stays
deterministic and NON-COMMITTAL, mirroring facematch's capped-fallback
pattern: the label is always UNKNOWN and live_score is hard-capped at 0.5 —
below any sensible enforcement threshold — so an unprovisioned deployment
can never fabricate a LIVE verdict. The result's ``model`` field tells the
caller which path ran; enforcement itself is a Go-side concern
(LIVENESS_ENFORCE, shadow-mode first per docs/nemo/08-liveness-plan.md).

Multi-frame fusion (doc-09 Stage 1, SHADOW MODE): ``live_score`` is no longer
the min across frames but the fused score from engine.liveness_fusion —
median PAD + inter-frame parallax + moiré/frequency analysis + the active
challenge result when the caller supplies one. The full sub-score breakdown
rides along in the response (``fusion``) so shadow logs carry threshold
calibration data; the old min-score policy is still reported as
``fusion.padMin`` for A/B comparison. The fallback engine's fused score stays
hard-capped at 0.5 — the model-free signals alone can never look LIVE.
"""
from __future__ import annotations

import os
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:  # pragma: no cover
    from engine.liveness_fusion import FusionBreakdown

_CROP_SCALE = 2.7  # MiniFASNetV2's standard detection-box widening factor
_INPUT_SIZE = 80  # MiniFASNet input resolution (80x80)
_LIVE_CLASS_INDEX = 1  # see class-order assumption in the module docstring
_LIVE_THRESHOLD = 0.5  # provisional; calibrate on real traffic (shadow mode)
_FALLBACK_CAP = 0.5  # fallback can never look confidently live


def liveness_model_path() -> str:
    return os.getenv("FACE_LIVENESS_MODEL", "/app/models/minifasnet_v2.onnx")


def model_available() -> bool:
    return os.path.isfile(liveness_model_path())


@dataclass
class FrameScore:
    score: float  # P(live) for this frame; 0.0 when no face was found
    face_found: bool

    def as_dict(self) -> dict:
        return {"score": round(self.score, 4), "faceFound": self.face_found}


@dataclass
class LivenessResult:
    live_score: float  # 0..1, multi-frame fused score (see liveness_fusion)
    label: str  # "LIVE" | "SPOOF" | "UNKNOWN"
    per_frame: list[FrameScore]
    model: str  # "minifasnet_v2" | "fallback"
    fusion: "FusionBreakdown | None" = None  # sub-scores for shadow calibration

    def as_dict(self) -> dict:
        d = {
            "liveScore": round(self.live_score, 4),
            "label": self.label,
            "perFrame": [f.as_dict() for f in self.per_frame],
            "model": self.model,
        }
        if self.fusion is not None:
            d["fusion"] = self.fusion.as_dict()
        return d


def _detect_largest_face(img):
    """(x, y, w, h) ints of the largest face, or None.

    Reuses the facematch detectors rather than duplicating them: YuNet when
    its model file is present, the wheel-shipped Haar cascade otherwise.
    """
    import cv2

    from engine import facematch

    if os.path.isfile(facematch.detector_model_path()):
        face = facematch._detect_largest_face_yunet(img)
        if face is None:
            return None
        x, y, w, h = face[:4]
    else:
        # OpenCV 5 wheels dropped the legacy Haar CascadeClassifier; without
        # any detector we report "no face", which keeps the result UNKNOWN
        # (fail-safe) instead of crashing — never a fabricated LIVE.
        if not hasattr(cv2, "CascadeClassifier"):
            return None
        gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
        face = facematch._detect_largest_face_haar(gray)
        if face is None:
            return None
        x, y, w, h = face
    if w <= 0 or h <= 0:
        return None
    return int(x), int(y), int(w), int(h)


def _scaled_crop(img, box, scale: float = _CROP_SCALE, size: int = _INPUT_SIZE):
    """size x size crop of the face box widened by ``scale``, following the
    reference CropImage.get_new_box: scale is limited so the widened box fits
    the frame, and the box is shifted back inside the frame when it overruns
    an edge."""
    import cv2

    src_h, src_w = img.shape[:2]
    x, y, w, h = box
    scale = min(scale, (src_h - 1) / h, (src_w - 1) / w)
    new_w, new_h = w * scale, h * scale
    cx, cy = x + w / 2, y + h / 2
    left = int(cx - new_w / 2)
    top = int(cy - new_h / 2)
    right = left + int(new_w)
    bottom = top + int(new_h)
    if right > src_w - 1:
        left -= right - (src_w - 1)
        right = src_w - 1
    if bottom > src_h - 1:
        top -= bottom - (src_h - 1)
        bottom = src_h - 1
    left, top = max(0, left), max(0, top)
    crop = img[top : bottom + 1, left : right + 1]
    return cv2.resize(crop, (size, size))


def _class_probabilities(raw):
    """Softmax over the 3 class outputs — unless the export already ends in
    a softmax layer (outputs in [0,1] summing to ~1), in which case the
    values are taken as-is instead of being flattened by a second softmax."""
    import numpy as np

    v = np.asarray(raw, dtype=np.float64).reshape(-1)
    if v.min() >= 0.0 and v.max() <= 1.0 and abs(v.sum() - 1.0) < 1e-3:
        return v
    e = np.exp(v - v.max())
    return e / e.sum()


# ─── primary engine ──────────────────────────────────────────────────────────


def _score_minifasnet(images, challenge_passed: bool | None) -> LivenessResult:
    import cv2
    import numpy as np

    from engine.liveness_fusion import compute_fusion

    net = cv2.dnn.readNetFromONNX(liveness_model_path())
    per_frame: list[FrameScore] = []
    for img in images:
        box = _detect_largest_face(img)
        if box is None:
            # no face -> conservatively the weakest possible frame
            per_frame.append(FrameScore(0.0, False))
            continue
        crop = _scaled_crop(img, box)
        # BGR, float32, raw 0..255, NCHW — see preprocessing note up top.
        blob = crop.astype(np.float32).transpose(2, 0, 1)[np.newaxis, ...]
        net.setInput(blob)
        probs = _class_probabilities(net.forward())
        per_frame.append(FrameScore(float(probs[_LIVE_CLASS_INDEX]), True))

    fusion = compute_fusion(
        images, [f.score for f in per_frame], challenge_passed
    )
    if not any(f.face_found for f in per_frame):
        # no face anywhere: fail-safe UNKNOWN/0.0 verdict, but keep the
        # breakdown — shadow logs still want the model-free sub-scores
        return LivenessResult(0.0, "UNKNOWN", per_frame, "minifasnet_v2", fusion)
    label = "LIVE" if fusion.score >= _LIVE_THRESHOLD else "SPOOF"
    return LivenessResult(fusion.score, label, per_frame, "minifasnet_v2", fusion)


# ─── deterministic fallback ──────────────────────────────────────────────────


def _score_fallback(images, challenge_passed: bool | None) -> LivenessResult:
    from engine.liveness_fusion import compute_fusion

    per_frame = []
    for img in images:
        found = _detect_largest_face(img) is not None
        per_frame.append(FrameScore(_FALLBACK_CAP if found else 0.0, found))
    fusion = compute_fusion(
        images, [f.score for f in per_frame], challenge_passed
    )
    if not any(f.face_found for f in per_frame):
        # same fail-safe as the primary engine: no face anywhere -> 0.0
        return LivenessResult(0.0, "UNKNOWN", per_frame, "fallback", fusion)
    # unprovisioned deployments must never look confidently live, no matter
    # what the model-free sub-scores say
    live_score = min(fusion.score, _FALLBACK_CAP)
    return LivenessResult(live_score, "UNKNOWN", per_frame, "fallback", fusion)


def score_frames(images, challenge_passed: bool | None = None) -> LivenessResult:
    """Score 1..n decoded BGR frames (np.ndarray). live_score is the doc-09
    Stage-1 fused score (median PAD + parallax + moiré + optional challenge
    result — see engine.liveness_fusion); the engine is chosen by model
    availability and the output always says which one ran."""
    if not images:
        raise ValueError("at least one frame is required")
    if model_available():
        return _score_minifasnet(images, challenge_passed)
    return _score_fallback(images, challenge_passed)
