"""NLD-EA (Nemo Liveness Dataset, East Africa) session-manifest loader.

THIS FILE IS A CONTRACT with the capture tooling built in parallel — the
manifest schema and the sharding recipe are specified in
liveness-training/docs/NLDEA_FORMAT.md and implemented here verbatim.
Do not change either without updating both sides.

Layout: a dataset root containing session directories; each session
directory holds a ``manifest.json`` (schema below) plus the clip files it
references::

    <root>/<anything>/manifest.json      # nesting depth is free-form
    <root>/<anything>/clip_001.mp4

Sharding: deterministic hash of ``subjectId`` -> train/val/redteam 70/15/15
(:func:`liveness_training.datasets.base.subject_shard`). The redteam shard
is the certification red-team rig's holdout — it is NEVER trained or
model-selected on. Loaders exclude it by default; it is only reachable via
an explicit ``shard="redteam"``, which emits a loud RuntimeWarning.
"""
from __future__ import annotations

import json
import re
import warnings
from dataclasses import dataclass
from pathlib import Path
from typing import Iterator, Optional

import numpy as np

from liveness_training.datasets.base import (
    ATTACK_TYPES,
    SKIN_TONES,
    PadDatasetBase,
    PadSample,
    subject_shard,
)
from liveness_training.deployment import LABEL_LIVE, LABEL_SPOOF

SCHEMA_VERSION = 1
SESSION_TYPES = ("genuine", "attack")
LIGHTING_VALUES = ("daylight", "indoor", "low_light")
CHALLENGE_VALUES = ("blink", "turn_left", "turn_right", "smile")
SHARDS = ("train", "val", "redteam")

_ISO8601_RE = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?$")

REDTEAM_WARNING = (
    "NLD-EA REDTEAM SHARD ACCESS: this shard is the certification holdout "
    "(docs/ekyc/06-level2-upgrade-plan.md §3). It must NEVER be used for "
    "training, distillation, threshold tuning or model selection — red-team "
    "rig evaluation only. If you are building a training set, stop."
)


class ManifestError(ValueError):
    """A manifest.json violates the NLD-EA schema."""


def _require(cond: bool, path: Path, msg: str) -> None:
    if not cond:
        raise ManifestError(f"{path}: {msg}")


@dataclass
class NLDEAClip:
    file: str
    duration_ms: int
    fps: float
    challenge: Optional[str]


@dataclass
class NLDEASession:
    path: Path  # session directory
    session_id: str
    subject_id: str
    consent_id: str
    type: str  # "genuine" | "attack"
    attack_type: Optional[str]
    device_model: str
    device_os: str
    lighting: str
    skin_tone: Optional[str]
    captured_at: str
    clips: list[NLDEAClip]

    @property
    def shard(self) -> str:
        return subject_shard(self.subject_id)

    @property
    def label(self) -> int:
        return LABEL_LIVE if self.type == "genuine" else LABEL_SPOOF


def parse_manifest(manifest_path: Path) -> NLDEASession:
    """Parse + validate one manifest.json. Raises ManifestError on any schema
    violation. Unknown top-level keys are tolerated (forward compatibility);
    everything specified is validated strictly."""
    p = Path(manifest_path)
    try:
        raw = json.loads(p.read_text(encoding="utf-8"))
    except json.JSONDecodeError as e:
        raise ManifestError(f"{p}: invalid JSON ({e})") from e
    _require(isinstance(raw, dict), p, "manifest must be a JSON object")

    _require(raw.get("schemaVersion") == SCHEMA_VERSION, p,
             f"schemaVersion must be {SCHEMA_VERSION}, got {raw.get('schemaVersion')!r}")

    for key in ("sessionId", "subjectId", "consentId", "capturedAt"):
        _require(isinstance(raw.get(key), str) and raw[key].strip() != "", p,
                 f"{key} must be a non-empty string")
    _require(_ISO8601_RE.match(raw["capturedAt"]) is not None, p,
             f"capturedAt must be ISO-8601, got {raw['capturedAt']!r}")

    stype = raw.get("type")
    _require(stype in SESSION_TYPES, p, f"type must be one of {SESSION_TYPES}, got {stype!r}")

    attack_type = raw.get("attackType")
    if stype == "genuine":
        _require(attack_type is None, p, "genuine session must have attackType null")
    else:
        _require(attack_type in ATTACK_TYPES, p,
                 f"attack session needs attackType in {ATTACK_TYPES}, got {attack_type!r}")

    device = raw.get("device")
    _require(isinstance(device, dict), p, "device must be an object")
    for key in ("model", "os"):
        _require(isinstance(device.get(key), str) and device[key].strip() != "", p,
                 f"device.{key} must be a non-empty string")

    lighting = raw.get("lighting")
    _require(lighting in LIGHTING_VALUES, p,
             f"lighting must be one of {LIGHTING_VALUES}, got {lighting!r}")

    skin_tone = raw.get("skinTone")
    _require(skin_tone is None or skin_tone in SKIN_TONES, p,
             f"skinTone must be null or monk_01..monk_10, got {skin_tone!r}")

    clips_raw = raw.get("clips")
    _require(isinstance(clips_raw, list) and len(clips_raw) > 0, p,
             "clips must be a non-empty array")
    clips: list[NLDEAClip] = []
    for i, c in enumerate(clips_raw):
        _require(isinstance(c, dict), p, f"clips[{i}] must be an object")
        _require(isinstance(c.get("file"), str) and c["file"].strip() != "", p,
                 f"clips[{i}].file must be a non-empty string")
        _require("/" not in c["file"] and "\\" not in c["file"] and c["file"] != "..", p,
                 f"clips[{i}].file must be a bare filename inside the session dir")
        _require(isinstance(c.get("durationMs"), int) and c["durationMs"] >= 0, p,
                 f"clips[{i}].durationMs must be a non-negative integer")
        _require(isinstance(c.get("fps"), (int, float)) and c["fps"] >= 0, p,
                 f"clips[{i}].fps must be a non-negative number")
        challenge = c.get("challenge")
        _require(challenge is None or challenge in CHALLENGE_VALUES, p,
                 f"clips[{i}].challenge must be null or one of {CHALLENGE_VALUES}")
        clips.append(NLDEAClip(c["file"], int(c["durationMs"]), float(c["fps"]), challenge))

    return NLDEASession(
        path=p.parent,
        session_id=raw["sessionId"],
        subject_id=raw["subjectId"],
        consent_id=raw["consentId"],
        type=stype,
        attack_type=attack_type,
        device_model=device["model"],
        device_os=device["os"],
        lighting=lighting,
        skin_tone=skin_tone,
        captured_at=raw["capturedAt"],
        clips=clips,
    )


def discover_sessions(root: Path) -> list[NLDEASession]:
    """Find and parse every manifest.json under root (sorted for determinism)."""
    root = Path(root)
    if not root.is_dir():
        raise FileNotFoundError(f"NLD-EA root not found: {root}")
    return [parse_manifest(m) for m in sorted(root.rglob("manifest.json"))]


def _decode_clip(clip_path: Path, max_frames: int) -> list[np.ndarray]:
    """Evenly-spaced BGR frames from a clip via cv2.VideoCapture."""
    import cv2

    cap = cv2.VideoCapture(str(clip_path))
    try:
        frames: list[np.ndarray] = []
        while True:
            ok, frame = cap.read()
            if not ok:
                break
            frames.append(frame)
    finally:
        cap.release()
    if not frames:
        raise IOError(f"could not decode any frames from {clip_path}")
    if max_frames > 0 and len(frames) > max_frames:
        idx = np.linspace(0, len(frames) - 1, max_frames).round().astype(int)
        frames = [frames[i] for i in idx]
    return frames


class NLDEADataset(PadDatasetBase):
    """Iterate NLD-EA sessions as PadSamples (one PadSample per clip).

    shard: "train" (default) | "val" | "redteam". The redteam shard is the
    certification holdout — requesting it emits :data:`REDTEAM_WARNING` as a
    RuntimeWarning. There is deliberately no way to iterate "everything".
    """

    def __init__(self, root, shard: str = "train", max_frames_per_clip: int = 8) -> None:
        if shard not in SHARDS:
            raise ValueError(f"shard must be one of {SHARDS}, got {shard!r}")
        if shard == "redteam":
            warnings.warn(REDTEAM_WARNING, RuntimeWarning, stacklevel=2)
        self.root = Path(root)
        self.shard = shard
        self.max_frames_per_clip = max_frames_per_clip
        self.sessions = [s for s in discover_sessions(self.root) if s.shard == shard]

    def __len__(self) -> int:
        return sum(len(s.clips) for s in self.sessions)

    def subjects(self) -> set[str]:
        return {s.subject_id for s in self.sessions}

    def __iter__(self) -> Iterator[PadSample]:
        for session in self.sessions:
            for clip in session.clips:
                frames = _decode_clip(session.path / clip.file, self.max_frames_per_clip)
                yield PadSample(
                    frames=frames,
                    label=session.label,
                    attack_type=session.attack_type,
                    skin_tone=session.skin_tone,
                    subject_id=session.subject_id,
                    meta={
                        "sessionId": session.session_id,
                        "lighting": session.lighting,
                        "device": session.device_model,
                        "challenge": clip.challenge,
                        "shard": session.shard,
                    },
                )
