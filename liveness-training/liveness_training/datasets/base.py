"""Common dataset contract for all PAD loaders.

* Every loader is an iterable of :class:`PadSample`.
* ``label`` follows the deployment convention (deployment.py): 1 = live,
  0 = spoof — identical to the serving softmax index of the genuine class.
* ``attack_type`` uses the NLD-EA vocabulary (:data:`ATTACK_TYPES`); public
  datasets map their own taxonomies onto it, unknown 2D attacks map to
  ``"unknown_2d"``.
* Train/val splits are ALWAYS subject-disjoint, via :func:`subject_shard` —
  a deterministic hash of the subject id, never a random split.
"""
from __future__ import annotations

import hashlib
from dataclasses import dataclass, field
from typing import Callable, Iterable, Iterator, Optional, Sequence

import numpy as np

from liveness_training.deployment import LABEL_LIVE, LABEL_SPOOF

# The NLD-EA attack vocabulary (docs/NLDEA_FORMAT.md) — the canonical
# attack_type values across the whole pipeline. "unknown_2d" is the mapping
# target for public-dataset attack species that don't cleanly correspond.
ATTACK_TYPES = (
    "print_flat",
    "print_curved",
    "replay_phone",
    "replay_monitor",
    "cutout_paper",
    "mask_3d",
)
ATTACK_TYPES_EXTENDED = ATTACK_TYPES + ("unknown_2d",)

SKIN_TONES = tuple(f"monk_{i:02d}" for i in range(1, 11))  # Monk scale 1..10

# NLD-EA sharding: deterministic hash of subjectId -> 70/15/15.
SHARD_TRAIN, SHARD_VAL, SHARD_REDTEAM = "train", "val", "redteam"
_SHARD_BOUNDS = ((SHARD_TRAIN, 70), (SHARD_VAL, 85), (SHARD_REDTEAM, 100))


def subject_shard(subject_id: str, salt: str = "nld-ea-v1") -> str:
    """Deterministic subject -> shard assignment (train 70 / val 15 / redteam 15).

    sha256(salt + ":" + subjectId), first 8 bytes as a big-endian integer,
    mod 100: [0,70) train, [70,85) val, [85,100) redteam. Salt is fixed for
    the lifetime of NLD-EA v1 so the capture tooling, this pipeline, and the
    red-team rig all agree on the assignment forever. Documented in
    liveness-training/docs/NLDEA_FORMAT.md — change it there or nowhere.
    """
    if not subject_id:
        raise ValueError("subject_id must be non-empty")
    digest = hashlib.sha256(f"{salt}:{subject_id}".encode("utf-8")).digest()
    bucket = int.from_bytes(digest[:8], "big") % 100
    for shard, upper in _SHARD_BOUNDS:
        if bucket < upper:
            return shard
    raise AssertionError("unreachable")


def split_subjects_disjoint(
    subject_ids: Iterable[str], val_fraction: float = 0.15, salt: str = "pad-split-v1"
) -> tuple[set[str], set[str]]:
    """Generic subject-disjoint train/val split for public datasets (which
    have no redteam shard). Deterministic hash bucketing, same recipe as
    :func:`subject_shard` but with a two-way boundary."""
    if not 0.0 < val_fraction < 1.0:
        raise ValueError("val_fraction must be in (0, 1)")
    train: set[str] = set()
    val: set[str] = set()
    cut = int(round(val_fraction * 10_000))
    for sid in subject_ids:
        digest = hashlib.sha256(f"{salt}:{sid}".encode("utf-8")).digest()
        bucket = int.from_bytes(digest[:8], "big") % 10_000
        (val if bucket < cut else train).add(sid)
    return train, val


@dataclass
class PadSample:
    """One presentation (a clip or a still image) from any loader.

    frames: list of HxWx3 **BGR uint8** numpy arrays (1 frame for image
    datasets, several for video datasets).
    """

    frames: list  # list[np.ndarray], BGR uint8
    label: int  # 1 = live, 0 = spoof (deployment convention)
    attack_type: Optional[str]  # None for genuine, else ATTACK_TYPES_EXTENDED
    skin_tone: Optional[str]  # "monk_01".."monk_10" or None when unlabeled
    subject_id: str
    meta: dict = field(default_factory=dict)  # loader-specific extras

    def __post_init__(self) -> None:
        if self.label not in (LABEL_LIVE, LABEL_SPOOF):
            raise ValueError(f"label must be 0/1, got {self.label!r}")
        if self.label == LABEL_LIVE and self.attack_type is not None:
            raise ValueError("genuine sample must have attack_type=None")
        if self.label == LABEL_SPOOF and self.attack_type is None:
            raise ValueError("attack sample must carry an attack_type")
        if self.attack_type is not None and self.attack_type not in ATTACK_TYPES_EXTENDED:
            raise ValueError(f"unknown attack_type {self.attack_type!r}")
        if self.skin_tone is not None and self.skin_tone not in SKIN_TONES:
            raise ValueError(f"unknown skin_tone {self.skin_tone!r}")


class PadDatasetBase:
    """Iterable-of-PadSample base. Subclasses implement __iter__ and __len__
    (len = number of presentations, not frames)."""

    def __iter__(self) -> Iterator[PadSample]:  # pragma: no cover - interface
        raise NotImplementedError

    def __len__(self) -> int:  # pragma: no cover - interface
        raise NotImplementedError

    def subjects(self) -> set[str]:
        return {s.subject_id for s in self}


class FramePadDataset:
    """torch.utils.data.Dataset adapter: flattens PadSamples into per-frame
    training records.

    Returns dicts: image (CHW float32 tensor, **BGR raw 0..255** at
    ``size``), label (int), attack_type (str, "" for genuine), skin_tone
    (str, "" when unlabeled), subject_id (str). Keeping the raw-range BGR
    convention here means the tensor a model trains on is bit-for-bit the
    tensor serving will feed it (deployment.py).

    ``transform`` (optional) is applied to the uint8 BGR frame *before*
    resizing — plug transforms.py augmentation pipelines in here.
    """

    def __init__(
        self,
        source: Iterable[PadSample],
        size: int,
        transform: Optional[Callable[[np.ndarray, np.random.Generator], np.ndarray]] = None,
        frames_per_sample: int = 1,
        seed: int = 0,
    ) -> None:
        import torch  # local import: keep numpy-only tooling torch-free

        self._torch = torch
        self.size = int(size)
        self.transform = transform
        self.records: list[tuple[np.ndarray, int, str, str, str]] = []
        for sample in source:
            frames: Sequence[np.ndarray] = sample.frames
            if frames_per_sample > 0 and len(frames) > frames_per_sample:
                idx = np.linspace(0, len(frames) - 1, frames_per_sample).round().astype(int)
                frames = [frames[i] for i in idx]
            for frame in frames:
                self.records.append(
                    (
                        frame,
                        sample.label,
                        sample.attack_type or "",
                        sample.skin_tone or "",
                        sample.subject_id,
                    )
                )
        self._rng = np.random.default_rng(seed)

    def __len__(self) -> int:
        return len(self.records)

    def __getitem__(self, i: int) -> dict:
        import cv2

        frame, label, attack_type, skin_tone, subject_id = self.records[i]
        if self.transform is not None:
            frame = self.transform(frame, self._rng)
        if frame.shape[:2] != (self.size, self.size):
            frame = cv2.resize(frame, (self.size, self.size), interpolation=cv2.INTER_AREA)
        tensor = self._torch.from_numpy(
            np.ascontiguousarray(frame.transpose(2, 0, 1), dtype=np.float32)
        )
        return {
            "image": tensor,  # BGR, raw 0..255 — the deployment convention
            "label": int(label),
            "attack_type": attack_type,
            "skin_tone": skin_tone,
            "subject_id": subject_id,
        }
