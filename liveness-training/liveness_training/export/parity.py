"""cv2.dnn parity check — the deployment-compatibility gate.

Loads the exported ONNX with ``cv2.dnn.readNetFromONNX`` (the EXACT runtime
ekyc-ml-service uses — not onnxruntime) and compares its outputs against the
PyTorch student on realistic synthetic 80x80 face crops fed through the
serving-side blob construction (deployment.to_deployment_blob). Also asserts
the output honors serving's assumptions: shape (1,2), sums to ~1 (so the
"already ends in softmax" detection fires), index 1 = live.
"""
from __future__ import annotations

import numpy as np

from liveness_training.deployment import (
    INPUT_SIZE,
    LIVE_CLASS_INDEX,
    NUM_CLASSES,
    to_deployment_blob,
)


def _test_crops(n: int, seed: int = 123) -> list[np.ndarray]:
    """Realistic + adversarial inputs: synthetic live/spoof faces, plus the
    range extremes (all-0, all-255) and uniform noise."""
    from liveness_training.datasets.synthetic import make_face_frame

    rng = np.random.default_rng(seed)
    crops = [
        np.zeros((INPUT_SIZE, INPUT_SIZE, 3), np.uint8),
        np.full((INPUT_SIZE, INPUT_SIZE, 3), 255, np.uint8),
        rng.integers(0, 256, (INPUT_SIZE, INPUT_SIZE, 3), dtype=np.uint8).astype(np.uint8),
    ]
    while len(crops) < n:
        live = len(crops) % 2 == 0
        crops.append(make_face_frame(rng, live, None if live else "replay_phone",
                                     None, INPUT_SIZE))
    return crops[:n]


def check_cv2_dnn_parity(
    student,
    onnx_path,
    n_inputs: int = 12,
    atol: float = 1e-4,
    seed: int = 123,
) -> dict:
    """Raises AssertionError on any contract violation. Returns
    {"max_abs_diff": float, "n_inputs": int}."""
    import cv2
    import torch

    net = cv2.dnn.readNetFromONNX(str(onnx_path))
    student = student.eval()

    max_diff = 0.0
    for crop in _test_crops(n_inputs, seed):
        blob = to_deployment_blob(crop)  # serving-side construction, verbatim

        net.setInput(blob)
        cv_out = np.asarray(net.forward(), dtype=np.float64).reshape(-1)

        with torch.no_grad():
            logits = student(torch.from_numpy(blob))
            torch_out = torch.softmax(logits, dim=1).numpy().reshape(-1).astype(np.float64)

        assert cv_out.shape == (NUM_CLASSES,), (
            f"serving contract: output must be {NUM_CLASSES} values, got {cv_out.shape}"
        )
        assert cv_out.min() >= -1e-6 and cv_out.max() <= 1 + 1e-6 and abs(cv_out.sum() - 1) < 1e-3, (
            "serving contract: output must already be softmax probabilities "
            f"(engine/liveness.py sum-to-1 detection), got {cv_out}"
        )
        diff = float(np.abs(cv_out - torch_out).max())
        max_diff = max(max_diff, diff)
        assert diff <= atol, (
            f"cv2.dnn vs PyTorch divergence {diff:.2e} > atol {atol:.0e} "
            f"(cv2={cv_out}, torch={torch_out})"
        )
        # live probability is read from index 1 on both sides by construction;
        # assert the indexing stays meaningful
        assert abs(cv_out[LIVE_CLASS_INDEX] - torch_out[LIVE_CLASS_INDEX]) <= atol

    return {"max_abs_diff": max_diff, "n_inputs": n_inputs}
