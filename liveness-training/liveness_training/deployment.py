"""The deployment contract — single source of truth for serving compatibility.

Mirrors ekyc-ml-service/engine/liveness.py exactly:

* Input: one 80x80 face crop, **BGR** channel order, float32 in the **raw
  0..255 range** (the MiniFASNet reference ToTensor does NOT divide by 255
  and applies no mean/std), NCHW layout ``(1, 3, 80, 80)``.
* Runtime: ``cv2.dnn.readNetFromONNX`` — the exported graph must be loadable
  and runnable by OpenCV's dnn module, no onnxruntime on the serving side.
* Output: class probabilities where **index 1 is the live/genuine class**
  (serving reads ``probs[_LIVE_CLASS_INDEX]`` with ``_LIVE_CLASS_INDEX = 1``).
  We export 2 classes — index 0 = spoof, index 1 = live — with an in-graph
  Softmax so serving's "already ends in softmax" detection (outputs in [0,1]
  summing to ~1) short-circuits its re-normalization.
* Face crop geometry: serving widens the detector box by 2.7x and resizes to
  80x80 (``_scaled_crop``); training crops should approximate the same
  loose-crop framing (face ~1/2.7 of the crop width, background visible).
"""
from __future__ import annotations

import numpy as np

INPUT_SIZE = 80          # 80x80 crops, engine/liveness.py _INPUT_SIZE
CROP_SCALE = 2.7         # detector-box widening factor, _CROP_SCALE
LIVE_CLASS_INDEX = 1     # softmax index of the genuine class, _LIVE_CLASS_INDEX
SPOOF_CLASS_INDEX = 0
NUM_CLASSES = 2
CHANNEL_ORDER = "BGR"    # crops are fed as decoded (BGR), never RGB-swapped
PIXEL_RANGE = (0.0, 255.0)  # raw range — no /255, no mean/std at the boundary

# Labels used across the training pipeline: 1 = live (== LIVE_CLASS_INDEX),
# 0 = spoof, so a trained head maps onto the serving convention with no
# permutation step anywhere.
LABEL_LIVE = 1
LABEL_SPOOF = 0


def to_deployment_blob(crop_bgr_uint8: np.ndarray) -> np.ndarray:
    """80x80x3 BGR uint8 crop -> the exact (1,3,80,80) float32 raw-range blob
    serving builds (engine/liveness.py: ``crop.astype(np.float32).transpose(
    2, 0, 1)[np.newaxis, ...]``)."""
    if crop_bgr_uint8.shape[:2] != (INPUT_SIZE, INPUT_SIZE):
        raise ValueError(
            f"expected {INPUT_SIZE}x{INPUT_SIZE} crop, got {crop_bgr_uint8.shape}"
        )
    return crop_bgr_uint8.astype(np.float32).transpose(2, 0, 1)[np.newaxis, ...]
