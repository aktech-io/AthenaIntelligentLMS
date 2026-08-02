"""Standard PAD training augmentations, numpy/cv2 only (no albumentations).

All transforms take and return a **BGR uint8** HxWx3 frame plus a
numpy Generator, matching FramePadDataset's ``transform`` hook. The set is
the usual PAD recipe: photometric jitter, blur, JPEG artifacts, and a
moiré-ish sinusoidal overlay that simulates screen-replay interference so
the student doesn't overfit to the capture pipeline of any one dataset.
"""
from __future__ import annotations

from typing import Callable, Sequence

import numpy as np


def color_jitter(frame: np.ndarray, rng: np.random.Generator,
                 brightness: float = 0.25, contrast: float = 0.25,
                 saturation: float = 0.2) -> np.ndarray:
    f = frame.astype(np.float32)
    f *= 1.0 + rng.uniform(-contrast, contrast)              # contrast
    f += rng.uniform(-brightness, brightness) * 255.0        # brightness
    gray = f.mean(axis=2, keepdims=True)
    f = gray + (f - gray) * (1.0 + rng.uniform(-saturation, saturation))
    return np.clip(f, 0, 255).astype(np.uint8)


def gaussian_blur(frame: np.ndarray, rng: np.random.Generator,
                  max_sigma: float = 1.5) -> np.ndarray:
    import cv2

    sigma = float(rng.uniform(0.1, max_sigma))
    return cv2.GaussianBlur(frame, (0, 0), sigma)


def jpeg_artifacts(frame: np.ndarray, rng: np.random.Generator,
                   quality_range: tuple[int, int] = (30, 80)) -> np.ndarray:
    import cv2

    q = int(rng.integers(*quality_range))
    ok, buf = cv2.imencode(".jpg", frame, [cv2.IMWRITE_JPEG_QUALITY, q])
    if not ok:
        return frame
    out = cv2.imdecode(buf, cv2.IMREAD_COLOR)
    return frame if out is None else out


def moire_overlay(frame: np.ndarray, rng: np.random.Generator,
                  max_amplitude: float = 10.0) -> np.ndarray:
    """Crossed sinusoidal gratings — a cheap stand-in for the sampling moiré
    of re-captured screens. Applied to BOTH classes at low amplitude so the
    model cannot use 'any grating == spoof' as a shortcut; the synthetic
    spoof generator uses far stronger, structured versions."""
    h, w = frame.shape[:2]
    yy, xx = np.mgrid[0:h, 0:w].astype(np.float32)
    amp = rng.uniform(1.0, max_amplitude)
    f1, f2 = rng.uniform(0.2, 0.9, size=2)
    theta = rng.uniform(0, np.pi)
    u = xx * np.cos(theta) + yy * np.sin(theta)
    grate = amp * (np.sin(f1 * u + rng.uniform(0, 6)) + np.sin(f2 * yy)) / 2.0
    out = frame.astype(np.float32) + grate[..., None]
    return np.clip(out, 0, 255).astype(np.uint8)


def horizontal_flip(frame: np.ndarray, rng: np.random.Generator) -> np.ndarray:
    return np.ascontiguousarray(frame[:, ::-1])


def random_crop_scale(frame: np.ndarray, rng: np.random.Generator,
                      max_zoom: float = 0.12) -> np.ndarray:
    """Small random zoom-in crop, resized back — simulates face-box jitter
    from the serving-side detector (2.7x widened boxes are not pixel-exact)."""
    import cv2

    h, w = frame.shape[:2]
    z = float(rng.uniform(0, max_zoom))
    dy, dx = int(h * z / 2), int(w * z / 2)
    oy = int(rng.integers(0, max(1, dy + 1)))
    ox = int(rng.integers(0, max(1, dx + 1)))
    crop = frame[oy : h - (2 * dy - oy) or h, ox : w - (2 * dx - ox) or w]
    if crop.shape[:2] != (h, w):
        crop = cv2.resize(crop, (w, h), interpolation=cv2.INTER_LINEAR)
    return crop


Transform = Callable[[np.ndarray, np.random.Generator], np.ndarray]


def compose(transforms: Sequence[tuple[Transform, float]]) -> Transform:
    """[(transform, probability), ...] -> one transform."""

    def _apply(frame: np.ndarray, rng: np.random.Generator) -> np.ndarray:
        for t, p in transforms:
            if rng.random() < p:
                frame = t(frame, rng)
        return frame

    return _apply


def default_train_transform() -> Transform:
    """The standard student/teacher training augmentation stack."""
    return compose([
        (horizontal_flip, 0.5),
        (random_crop_scale, 0.5),
        (color_jitter, 0.7),
        (gaussian_blur, 0.25),
        (moire_overlay, 0.2),
        (jpeg_artifacts, 0.4),
    ])
