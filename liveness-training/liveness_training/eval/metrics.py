"""ISO/IEC 30107-3 PAD metrics.

Convention: ``score`` = P(live); a presentation is ACCEPTED (classified
live) when ``score >= threshold``.

* APCER (per attack species / PAIS): fraction of that species' attack
  presentations wrongly accepted. The certification-relevant summary is the
  MAX over species (iBeta grades the worst species, not the average).
* BPCER: fraction of bona-fide presentations wrongly rejected.
* ACER: (APCER_max + BPCER) / 2 — reported for continuity with the PAD
  literature; certification uses the raw APCER/BPCER, not ACER.
* BPCER@APCER<=1%: the iBeta-Level-2-relevant operating point — the lowest
  BPCER achievable by any threshold whose worst-species APCER is <=1%.
* Per-skin-tone BPCER: the East-African calibration check — BPCER broken
  down by Monk tone over bona-fide presentations that carry labels.
"""
from __future__ import annotations

from typing import Optional, Sequence

import numpy as np

from liveness_training.deployment import LABEL_LIVE, LABEL_SPOOF


def _as_arrays(scores, labels, attack_types=None, skin_tones=None):
    scores = np.asarray(scores, dtype=np.float64)
    labels = np.asarray(labels, dtype=np.int64)
    if scores.shape != labels.shape:
        raise ValueError("scores and labels must have identical shape")
    n = scores.shape[0]
    attack_types = (
        np.asarray(attack_types, dtype=object) if attack_types is not None
        else np.asarray([""] * n, dtype=object)
    )
    skin_tones = (
        np.asarray(skin_tones, dtype=object) if skin_tones is not None
        else np.asarray([""] * n, dtype=object)
    )
    if attack_types.shape[0] != n or skin_tones.shape[0] != n:
        raise ValueError("attack_types/skin_tones length mismatch")
    return scores, labels, attack_types, skin_tones


def apcer_per_type(scores, labels, attack_types, threshold: float) -> dict[str, float]:
    scores, labels, attack_types, _ = _as_arrays(scores, labels, attack_types)
    out: dict[str, float] = {}
    spoof = labels == LABEL_SPOOF
    for at in sorted({str(a) for a in attack_types[spoof] if str(a)}):
        m = spoof & (attack_types == at)
        out[at] = float((scores[m] >= threshold).mean()) if m.any() else float("nan")
    return out


def apcer_max(scores, labels, attack_types, threshold: float) -> float:
    per = apcer_per_type(scores, labels, attack_types, threshold)
    vals = [v for v in per.values() if not np.isnan(v)]
    return max(vals) if vals else float("nan")


def bpcer(scores, labels, threshold: float) -> float:
    scores, labels, _, _ = _as_arrays(scores, labels)
    bona = labels == LABEL_LIVE
    if not bona.any():
        return float("nan")
    return float((scores[bona] < threshold).mean())


def acer(scores, labels, attack_types, threshold: float) -> float:
    a = apcer_max(scores, labels, attack_types, threshold)
    b = bpcer(scores, labels, threshold)
    return float((a + b) / 2.0)


def bpcer_at_apcer(
    scores, labels, attack_types, target_apcer: float = 0.01
) -> tuple[float, float]:
    """(bpcer, threshold) at the lowest threshold whose worst-species APCER
    is <= target. Higher thresholds only reject more attacks AND more
    bona-fides, so the lowest qualifying threshold minimizes BPCER.
    Returns (nan, nan) when no threshold qualifies (some attack scores 1.0)
    or when a class is absent."""
    scores, labels, attack_types, _ = _as_arrays(scores, labels, attack_types)
    if not (labels == LABEL_SPOOF).any() or not (labels == LABEL_LIVE).any():
        return float("nan"), float("nan")
    # candidate thresholds: every distinct score plus one above the max
    candidates = np.unique(scores)
    candidates = np.concatenate([candidates, [candidates[-1] + 1e-9]])
    for thr in candidates:  # ascending
        if apcer_max(scores, labels, attack_types, thr) <= target_apcer:
            return bpcer(scores, labels, thr), float(thr)
    return float("nan"), float("nan")


def bpcer_per_skin_tone(scores, labels, skin_tones, threshold: float) -> dict[str, float]:
    scores, labels, _, skin_tones = _as_arrays(scores, labels, None, skin_tones)
    bona = labels == LABEL_LIVE
    out: dict[str, float] = {}
    for tone in sorted({str(t) for t in skin_tones[bona] if str(t)}):
        m = bona & (skin_tones == tone)
        out[tone] = float((scores[m] < threshold).mean()) if m.any() else float("nan")
    return out


def compute_pad_metrics(
    scores: Sequence[float],
    labels: Sequence[int],
    attack_types: Optional[Sequence[str]] = None,
    skin_tones: Optional[Sequence[str]] = None,
    threshold: float = 0.5,
    target_apcer: float = 0.01,
) -> dict:
    """Everything the report needs, in one dict (floats are 0..1 rates)."""
    scores, labels, attack_types, skin_tones = _as_arrays(
        scores, labels, attack_types, skin_tones
    )
    per_type = apcer_per_type(scores, labels, attack_types, threshold)
    vals = [v for v in per_type.values() if not np.isnan(v)]
    a_max = max(vals) if vals else float("nan")
    b = bpcer(scores, labels, threshold)
    bp_at, thr_at = bpcer_at_apcer(scores, labels, attack_types, target_apcer)
    return {
        "threshold": float(threshold),
        "n_bona_fide": int((labels == LABEL_LIVE).sum()),
        "n_attack": int((labels == LABEL_SPOOF).sum()),
        "apcer_per_type": per_type,
        "apcer_max": float(a_max),
        "bpcer": float(b),
        "acer": float((a_max + b) / 2.0) if not np.isnan(a_max) else float("nan"),
        "target_apcer": float(target_apcer),
        "bpcer_at_target_apcer": float(bp_at),
        "threshold_at_target_apcer": float(thr_at),
        "bpcer_per_skin_tone": bpcer_per_skin_tone(scores, labels, skin_tones, threshold),
    }
