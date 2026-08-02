"""Frame sampling from capture clips.

Frame-count contract (confirmed against the service and the Go provider):

* ``ekyc-ml-service/api/face.py`` — ``_MAX_LIVENESS_FRAMES = 10``; more than
  10 ``frame`` parts is a 400.
* ``go-services/internal/compliance/liveness/inhouse.go`` — ``maxFrames = 5``;
  the in-house provider truncates to the first 5 frames, so **5 is what the
  mobile app's capture actually reaches the engine with today**.

The rig therefore defaults to 5 (production parity) and allows up to 10
(doc-09 Stage-1 target aggregation), so a run can measure what the extra
frames buy before the Go cap is widened.
"""
from __future__ import annotations

import os

DEFAULT_FRAME_COUNT = 5  # Go provider cap — production parity
MAX_FRAME_COUNT = 10  # ekyc-ml-service hard limit (_MAX_LIVENESS_FRAMES)

_IMAGE_SUFFIXES = (".jpg", ".jpeg", ".png", ".bmp", ".webp")


class ClipError(RuntimeError):
    """The clip could not be opened or yielded no frames."""


def clamp_frame_count(count: int) -> int:
    if count < 1:
        raise ValueError("frame count must be >= 1")
    if count > MAX_FRAME_COUNT:
        raise ValueError(
            f"frame count {count} exceeds the engine limit of "
            f"{MAX_FRAME_COUNT} (api/face.py _MAX_LIVENESS_FRAMES)"
        )
    return count


def _sample_indices(total: int, count: int) -> list[int]:
    """``count`` evenly spread indices over ``total`` frames (dedup, sorted).

    Endpoints included: the first and last frame carry the most inter-frame
    displacement, which is exactly what the parallax sub-score measures.
    """
    if total <= 0:
        return []
    if count >= total:
        return list(range(total))
    if count == 1:
        return [total // 2]
    step = (total - 1) / (count - 1)
    return sorted({int(round(i * step)) for i in range(count)})


def sample_frames(
    clip_path: str,
    count: int = DEFAULT_FRAME_COUNT,
    fps: float | None = None,
):
    """Return up to ``count`` decoded BGR frames (numpy arrays) from a clip.

    ``fps`` (optional) samples at a fixed rate instead of evenly spreading:
    the frames nearest each 1/fps instant are taken, still capped at
    ``count``. A still image is accepted as a one-frame clip.
    """
    import cv2

    count = clamp_frame_count(count)
    if not os.path.isfile(clip_path):
        raise ClipError(f"clip not found: {clip_path}")

    if clip_path.lower().endswith(_IMAGE_SUFFIXES):
        img = cv2.imread(clip_path, cv2.IMREAD_COLOR)
        if img is None:
            raise ClipError(f"undecodable image: {clip_path}")
        return [img]

    cap = cv2.VideoCapture(clip_path)
    if not cap.isOpened():
        raise ClipError(f"cannot open clip: {clip_path}")
    try:
        frames = []
        while True:
            ok, frame = cap.read()
            if not ok:
                break
            frames.append(frame)
    finally:
        cap.release()

    if not frames:
        raise ClipError(f"clip decoded 0 frames: {clip_path}")

    if fps is not None and fps > 0:
        source_fps = _probe_fps(clip_path) or float(len(frames))
        stride = max(1, int(round(source_fps / fps)))
        picked = frames[::stride][:count]
        return picked or frames[:1]

    return [frames[i] for i in _sample_indices(len(frames), count)]


def _probe_fps(clip_path: str) -> float | None:
    import cv2

    cap = cv2.VideoCapture(clip_path)
    try:
        value = cap.get(cv2.CAP_PROP_FPS)
    finally:
        cap.release()
    return float(value) if value and value > 0 else None


def encode_jpeg(frame, quality: int = 92) -> bytes:
    """BGR array -> JPEG bytes, as the mobile app uploads them."""
    import cv2

    ok, buf = cv2.imencode(".jpg", frame, [int(cv2.IMWRITE_JPEG_QUALITY), quality])
    if not ok:
        raise ClipError("JPEG encode failed")
    return buf.tobytes()
