"""doc-09 Stage 1: multi-frame liveness fusion sub-scores (SHADOW MODE).

Upgrades the single-frame min-score PAD decision to the fusion prescribed by
docs/ekyc/06-level2-upgrade-plan.md §3 / docs/nemo/09-liveness-build-and-certify.md §3:

    median PAD across frames
  + inter-frame parallax (motion + non-rigidity)
  + moiré / frequency-domain screen-replay analysis
  + active-challenge result (when the caller supplies one)

All signals here are model-free (OpenCV + numpy only — no new wheels) and
deterministic, so they also run — and produce calibration data — on
unprovisioned deployments where the MiniFASNet ONNX model is absent.
Enforcement remains a Go-side concern (LIVENESS_ENFORCE, shadow-first per
docs/nemo/08-liveness-plan.md); this module only scores.

Sub-score semantics (each in [0, 1], higher = more live-like):

* **padMedian** — median of the per-frame MiniFASNet P(live) scores (frames
  with no detected face count as 0.0). Median instead of the old min: robust
  to one bad frame instead of letting the weakest frame decide. The old min
  is still reported (``padMin``) so shadow logs can compare both policies.

* **parallax** — consecutive frame pairs are compared with block-wise phase
  correlation on a 4x4 grid. The median block displacement *magnitude* is
  the motion activity (deliberately not the magnitude of the median vector:
  a face turning against a static background has near-zero net motion but
  plenty of block motion); the median deviation of block shifts from the
  median shift *vector* is the non-rigidity. A flat photo or screen replay
  moves (approximately) rigidly — every block shifts the same way — while a
  real 3D face produces depth-induced disagreement between blocks. Score:

      saturate(motion) * (0.4 + 0.6 * saturate(non-rigidity))

  so zero inter-frame motion scores 0 (a rigidly mounted photo — or a
  suspiciously frozen feed — earns nothing), rigid translation earns at most
  0.4 * motion-term, and only moving-AND-non-rigid content saturates to 1.
  ``None`` (absent, excluded from fusion) when there are < 2 frames or too
  little texture to track.

* **moire** — per-frame FFT screen-replay detector. The magnitude spectrum is
  whitened by its radial mean profile (removing the natural 1/f falloff),
  a ±6° wedge around both frequency axes is excluded (natural edge energy
  concentrates there), and the peak-to-median ratio inside the remaining
  mid/high-frequency band is measured. Screen-door / moiré interference is
  narrow-band periodic energy, producing an outlier peak; clean camera
  frames have a flat whitened band. The score is the inverted, saturated
  ratio (1 = clean, 0 = strong periodic artifact), median across frames.

* **challenge** — 1.0 / 0.0 mapping of the active-challenge pass/fail signal
  when the request carries one; ``None`` (excluded) otherwise.

Fusion: weighted mean over the *available* components with the weights below,
renormalized when parallax and/or challenge are absent, clamped to [0, 1].
All constants are provisional and exist to be calibrated from shadow-mode
logs — that is precisely the data this module makes observable.
"""
from __future__ import annotations

from dataclasses import dataclass

# ── analysis geometry ────────────────────────────────────────────────────────
_ANALYSIS_SIZE = 256  # frames are resized to this square for both analyses
_GRID = 4  # 4x4 blocks for block-wise phase correlation
_MIN_BLOCK_STD = 2.0  # skip near-flat blocks (nothing to correlate)
_MIN_PC_RESPONSE = 0.03  # discard low-confidence phase-correlation peaks
_MIN_BLOCKS_PER_PAIR = 4  # fewer trackable blocks -> pair is unusable

# ── parallax calibration (provisional; calibrate from shadow logs) ───────────
_MOTION_NOISE_FLOOR_PX = 0.15  # sub-pixel phase-correlation jitter -> no motion
_MOTION_REF_PX = 6.0  # block displacement that saturates the motion term
_NONRIGID_REF_PX = 3.0  # block-shift spread that saturates non-rigidity
_RIGID_MOTION_CREDIT = 0.4  # rigid translation earns at most this fraction

# ── moiré calibration (provisional; calibrate from shadow logs) ──────────────
_MOIRE_BAND_LO = 0.15  # band inner radius, fraction of Nyquist
_MOIRE_BAND_HI = 0.95  # band outer radius, fraction of Nyquist
_MOIRE_AXIS_WEDGE_DEG = 6.0  # exclude ±this many degrees around both axes
_MOIRE_RATIO_CLEAN = 25.0  # peak/median at or below this -> score 1.0
_MOIRE_RATIO_SPOOF = 80.0  # peak/median at or above this -> score 0.0

# ── fusion weights (renormalized over available components) ──────────────────
_WEIGHTS = {"pad": 0.55, "parallax": 0.15, "moire": 0.15, "challenge": 0.15}

_FUSION_VERSION = 1


@dataclass
class FusionBreakdown:
    """Fused score plus every sub-score and its raw measurement, so shadow
    logs carry enough signal to calibrate thresholds per docs/ekyc/06 §3."""

    score: float  # fused, [0, 1]
    pad_median: float
    pad_min: float  # the old min-score policy, kept for A/B comparison
    parallax: float | None  # None: < 2 frames or untrackable content
    moire: float
    challenge: float | None  # None: caller sent no challenge result
    motion_px: float | None  # raw median block displacement magnitude (px @ 256)
    nonrigidity_px: float | None  # raw median block-shift spread (px @ 256)
    moire_peak_ratio: float  # raw whitened peak/median band ratio
    frames: int

    def as_dict(self) -> dict:
        rnd = lambda v: None if v is None else round(v, 4)  # noqa: E731
        return {
            "score": round(self.score, 4),
            "padMedian": round(self.pad_median, 4),
            "padMin": round(self.pad_min, 4),
            "parallax": rnd(self.parallax),
            "moire": round(self.moire, 4),
            "challenge": rnd(self.challenge),
            "motionPx": rnd(self.motion_px),
            "nonRigidityPx": rnd(self.nonrigidity_px),
            "moirePeakRatio": round(self.moire_peak_ratio, 2),
            "frames": self.frames,
            "version": _FUSION_VERSION,
        }


def _saturate(value: float, ref: float) -> float:
    return min(max(value / ref, 0.0), 1.0)


def _to_analysis_gray(img):
    """BGR (or already-gray) frame -> float32 _ANALYSIS_SIZE² grayscale."""
    import cv2

    if img.ndim == 3:
        img = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    return cv2.resize(img, (_ANALYSIS_SIZE, _ANALYSIS_SIZE)).astype("float32")


# ─── inter-frame parallax ────────────────────────────────────────────────────


def _block_shifts(g1, g2):
    """Per-block (dx, dy) phase-correlation shifts on a _GRID x _GRID grid.

    Near-flat blocks and low-confidence correlation peaks are skipped: they
    carry no motion evidence (e.g. blank backgrounds)."""
    import cv2

    step = _ANALYSIS_SIZE // _GRID
    win = cv2.createHanningWindow((step, step), cv2.CV_32F)
    shifts = []
    for by in range(_GRID):
        for bx in range(_GRID):
            b1 = g1[by * step : (by + 1) * step, bx * step : (bx + 1) * step]
            b2 = g2[by * step : (by + 1) * step, bx * step : (bx + 1) * step]
            if b1.std() < _MIN_BLOCK_STD or b2.std() < _MIN_BLOCK_STD:
                continue
            (dx, dy), response = cv2.phaseCorrelate(b1, b2, win)
            if response < _MIN_PC_RESPONSE:
                continue
            shifts.append((dx, dy))
    return shifts


def parallax_analysis(images):
    """(score, motion_px, nonrigidity_px) over consecutive frame pairs.

    All three are None when parallax cannot be measured (< 2 frames, or no
    pair has enough trackable blocks) — the component is then *absent* from
    fusion rather than counted as evidence either way."""
    import numpy as np

    if len(images) < 2:
        return None, None, None

    grays = [_to_analysis_gray(img) for img in images]
    motions, deviations = [], []
    for g1, g2 in zip(grays, grays[1:]):
        shifts = _block_shifts(g1, g2)
        if len(shifts) < _MIN_BLOCKS_PER_PAIR:
            continue
        arr = np.asarray(shifts, dtype=np.float64)
        # motion activity: median per-block displacement magnitude — robust
        # to a static background cancelling a moving face
        motions.append(float(np.median(np.hypot(arr[:, 0], arr[:, 1]))))
        # non-rigidity: spread of block shifts around the median shift vector
        med = np.median(arr, axis=0)
        deviations.append(
            float(np.median(np.hypot(arr[:, 0] - med[0], arr[:, 1] - med[1])))
        )
    if not motions:
        return None, None, None

    motion = float(np.median(motions))
    nonrigidity = float(np.median(deviations))
    if motion < _MOTION_NOISE_FLOOR_PX:
        # sub-pixel correlation jitter, not motion: identical frames must
        # measure exactly zero parallax (raw values still reported)
        return 0.0, motion, nonrigidity
    score = _saturate(motion, _MOTION_REF_PX) * (
        _RIGID_MOTION_CREDIT
        + (1.0 - _RIGID_MOTION_CREDIT) * _saturate(nonrigidity, _NONRIGID_REF_PX)
    )
    return score, motion, nonrigidity


# ─── moiré / frequency analysis ──────────────────────────────────────────────


def _moire_peak_ratio(gray) -> float:
    """Whitened peak-to-median ratio in the off-axis mid/high-frequency band.

    ~<10 for typical camera frames, >>100 for screen-door interference."""
    import cv2
    import numpy as np

    win = cv2.createHanningWindow((_ANALYSIS_SIZE, _ANALYSIS_SIZE), cv2.CV_32F)
    spectrum = np.abs(np.fft.fftshift(np.fft.fft2(gray * win)))
    h, w = spectrum.shape
    yy, xx = np.ogrid[:h, :w]
    dy, dx = yy - h // 2, xx - w // 2
    radius = np.hypot(dy, dx)

    # whiten by the radial mean profile: removes the natural 1/f falloff so
    # narrow-band periodic energy stands out at any frequency
    rint = radius.astype(int)
    counts = np.bincount(rint.ravel())
    sums = np.bincount(rint.ravel(), spectrum.ravel())
    profile = sums / np.maximum(counts, 1)
    whitened = spectrum / (profile[rint] + 1e-9)

    nyquist = min(h, w) / 2.0
    angle = np.degrees(np.arctan2(dy, dx)) % 90.0
    off_axis = np.minimum(angle, 90.0 - angle) >= _MOIRE_AXIS_WEDGE_DEG
    band = (
        (radius >= _MOIRE_BAND_LO * nyquist)
        & (radius <= _MOIRE_BAND_HI * nyquist)
        & off_axis
    )
    values = whitened[band]
    return float(values.max() / (np.median(values) + 1e-9))


def moire_analysis(images):
    """(score, peak_ratio): medians across frames. 1.0 = clean, 0.0 = strong
    periodic screen-replay artifact."""
    import numpy as np

    ratios = [_moire_peak_ratio(_to_analysis_gray(img)) for img in images]
    ratio = float(np.median(ratios))
    span = _MOIRE_RATIO_SPOOF - _MOIRE_RATIO_CLEAN
    score = 1.0 - min(max((ratio - _MOIRE_RATIO_CLEAN) / span, 0.0), 1.0)
    return score, ratio


# ─── fusion ──────────────────────────────────────────────────────────────────


def fuse(
    pad_median: float,
    parallax: float | None,
    moire: float,
    challenge: float | None,
) -> float:
    """Weighted mean over available components, weights renormalized when
    parallax and/or challenge are absent. Clamped to [0, 1]."""
    components = {
        "pad": pad_median,
        "parallax": parallax,
        "moire": moire,
        "challenge": challenge,
    }
    present = {k: v for k, v in components.items() if v is not None}
    total_weight = sum(_WEIGHTS[k] for k in present)
    fused = sum(_WEIGHTS[k] * v for k, v in present.items()) / total_weight
    return min(max(fused, 0.0), 1.0)


def compute_fusion(
    images, pad_scores, challenge_passed: bool | None = None
) -> FusionBreakdown:
    """Full Stage-1 fusion over decoded BGR frames.

    ``pad_scores`` are the per-frame P(live) values from the PAD engine
    (0.0 for frames with no detected face — one such frame is absorbed by
    the median, unlike the old min policy)."""
    import numpy as np

    pad_median = float(np.median(pad_scores))
    pad_min = float(min(pad_scores))
    parallax, motion_px, nonrigidity_px = parallax_analysis(images)
    moire, moire_ratio = moire_analysis(images)
    challenge = None if challenge_passed is None else float(challenge_passed)
    return FusionBreakdown(
        score=fuse(pad_median, parallax, moire, challenge),
        pad_median=pad_median,
        pad_min=pad_min,
        parallax=parallax,
        moire=moire,
        challenge=challenge,
        motion_px=motion_px,
        nonrigidity_px=nonrigidity_px,
        moire_peak_ratio=moire_ratio,
        frames=len(images),
    )
