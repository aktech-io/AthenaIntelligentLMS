"""CASIA-SURF CeFA loader (cross-ethnicity PAD — includes African subjects).

RESEARCH-ONLY LICENSE — see liveness-training/DATASETS.md. CeFA matters to us
because it is the only large public PAD set with a dedicated African-subject
partition — the closest public proxy for our East-African calibration goal
until NLD-EA lands. It carries ethnicity labels, NOT Monk skin-tone labels,
so ``skin_tone`` is None and the ethnicity rides in ``meta["ethnicity"]``.

Assumed layout (CeFA distribution, RGB modality)::

    <root>/<race>_<subject>_<session>_<pai>_<rep>/profile/*.jpg
    e.g.  <root>/2_045_1_1_3/profile/0001.jpg

Directory-name fields:
    race:    1 = African, 2 = Central Asian, 3 = East Asian (AF/CA/EA also
             accepted for tolerance)
    subject: numeric subject id (unique within a race code)
    pai:     1 = real, 2 = print (cloth) attack, 3 = video-replay attack

Best-effort caveat: this loader is written against the published CeFA
directory convention and validated on synthetic fixtures; verify the field
order against your actual download (some repackagings differ) before a real
training run — the regex below is the single point to adjust.
"""
from __future__ import annotations

import re
from pathlib import Path
from typing import Iterator, Optional

from liveness_training.datasets.base import (
    PadDatasetBase,
    PadSample,
    split_subjects_disjoint,
)
from liveness_training.deployment import LABEL_LIVE, LABEL_SPOOF

_DIR_RE = re.compile(
    r"^(?P<race>[123]|AF|CA|EA)_(?P<subject>\d+)_(?P<session>\d+)"
    r"_(?P<pai>\d+)_(?P<rep>\d+)$"
)
RACE_NAMES = {"1": "african", "2": "central_asian", "3": "east_asian",
              "AF": "african", "CA": "central_asian", "EA": "east_asian"}
PAI_MAP = {1: None, 2: "print_flat", 3: "replay_monitor"}  # 1 = genuine
_IMG_EXTS = (".jpg", ".jpeg", ".png")


class CeFADataset(PadDatasetBase):
    """split: "train" | "val" — subject-disjoint hash split (subjects are
    namespaced by race code so 1_045 and 3_045 stay distinct people).
    ethnicities: optional filter, e.g. ("african",) to train/eval on the
    African partition only."""

    def __init__(
        self,
        root,
        split: str = "train",
        val_fraction: float = 0.15,
        ethnicities: Optional[tuple[str, ...]] = None,
        max_frames_per_video: int = 8,
        max_samples: Optional[int] = None,
    ) -> None:
        if split not in ("train", "val"):
            raise ValueError(f"split must be train|val, got {split!r}")
        self.root = Path(root)
        if not self.root.is_dir():
            raise FileNotFoundError(f"CeFA root not found: {self.root}")
        self.max_frames_per_video = max_frames_per_video

        videos: list[tuple[Path, int, Optional[str], str, str]] = []
        for d in sorted(p for p in self.root.iterdir() if p.is_dir()):
            m = _DIR_RE.match(d.name)
            if not m:
                continue
            race = RACE_NAMES[m.group("race")]
            if ethnicities is not None and race not in ethnicities:
                continue
            pai = int(m.group("pai"))
            if pai not in PAI_MAP:
                continue  # unknown PAI code — skip rather than mislabel
            attack = PAI_MAP[pai]
            label = LABEL_LIVE if attack is None else LABEL_SPOOF
            subject = f"cefa_{m.group('race')}_{m.group('subject')}"
            videos.append((d, label, attack, subject, race))

        subjects = {v[3] for v in videos}
        train_subj, val_subj = split_subjects_disjoint(subjects, val_fraction)
        keep = train_subj if split == "train" else val_subj
        videos = [v for v in videos if v[3] in keep]
        if max_samples is not None:
            videos = videos[:max_samples]
        self._videos = videos

    def __len__(self) -> int:
        return len(self._videos)

    def subjects(self) -> set[str]:
        return {v[3] for v in self._videos}

    def _frames_of(self, video_dir: Path) -> list:
        import cv2
        import numpy as np

        frame_dir = video_dir / "profile"  # RGB modality; depth/ir ignored
        if not frame_dir.is_dir():
            frame_dir = video_dir
        files = sorted(f for f in frame_dir.iterdir() if f.suffix.lower() in _IMG_EXTS)
        if not files:
            return []
        if len(files) > self.max_frames_per_video:
            idx = np.linspace(0, len(files) - 1, self.max_frames_per_video).round().astype(int)
            files = [files[i] for i in idx]
        frames = []
        for f in files:
            img = cv2.imread(str(f), cv2.IMREAD_COLOR)
            if img is not None:
                frames.append(img)
        return frames

    def __iter__(self) -> Iterator[PadSample]:
        for video_dir, label, attack, subject, race in self._videos:
            frames = self._frames_of(video_dir)
            if not frames:
                continue
            yield PadSample(
                frames=frames,
                label=label,
                attack_type=attack,
                skin_tone=None,  # CeFA has ethnicity, not Monk-scale labels
                subject_id=subject,
                meta={"ethnicity": race, "video": video_dir.name, "dataset": "cefa"},
            )
