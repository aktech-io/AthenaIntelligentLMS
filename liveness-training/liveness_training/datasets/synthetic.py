"""Synthetic PAD fixtures — the whole pipeline is smoke-testable with NO real
dataset present.

Two layers:

* :func:`make_face_frame` draws a cheap procedural "face" (BGR uint8) whose
  spoof variants carry deliberately strong, learnable artifacts (moiré
  gratings for replay, paper borders for print, hard cut edges for cutouts,
  matte flattening for masks) so a tiny model separates live/spoof in 1-2
  CPU epochs.
* ``generate_*_fixture`` functions materialize those frames in the exact
  on-disk layout each real loader expects (NLD-EA session dirs with
  manifest.json + mp4 clips; CelebA-Spoof original tree + metas JSON;
  CelebA-Spoof HF parquet shards; CeFA directory convention) — the loader
  contract tests run against these.

:class:`SyntheticPadDataset` is the in-memory shortcut for training-loop
tests (no disk round-trip).
"""
from __future__ import annotations

import json
import uuid
from pathlib import Path
from typing import Iterator, Optional

import numpy as np

from liveness_training.datasets.base import (
    ATTACK_TYPES,
    SKIN_TONES,
    PadDatasetBase,
    PadSample,
    split_subjects_disjoint,
)
from liveness_training.deployment import LABEL_LIVE, LABEL_SPOOF

# Monk-scale-ish BGR skin colors, light -> dark
_SKIN_BGR = [
    (214, 230, 246), (196, 220, 242), (170, 205, 235), (140, 180, 220),
    (120, 160, 205), (95, 135, 180), (75, 110, 150), (60, 88, 120),
    (48, 68, 92), (36, 50, 66),
]


def make_face_frame(
    rng: np.random.Generator,
    live: bool,
    attack_type: Optional[str] = None,
    skin_tone: Optional[str] = None,
    size: int = 128,
) -> np.ndarray:
    """One synthetic BGR uint8 frame. Spoof artifacts are exaggerated on
    purpose — these fixtures test pipeline mechanics, not model quality."""
    tone_idx = (
        SKIN_TONES.index(skin_tone) if skin_tone in SKIN_TONES else int(rng.integers(0, 10))
    )
    skin = np.array(_SKIN_BGR[tone_idx], dtype=np.float32)

    # background: vertical gradient + noise
    g = np.linspace(60, 140, size, dtype=np.float32)[:, None]
    frame = np.repeat(g, size, axis=1)[..., None].repeat(3, axis=2)
    frame += rng.normal(0, 6, frame.shape).astype(np.float32)

    # face ellipse (center jittered)
    cy, cx = size / 2 + rng.uniform(-4, 4), size / 2 + rng.uniform(-4, 4)
    ry, rx = size * 0.32, size * 0.24
    yy, xx = np.mgrid[0:size, 0:size].astype(np.float32)
    mask = (((yy - cy) / ry) ** 2 + ((xx - cx) / rx) ** 2) <= 1.0
    shade = 1.0 - 0.25 * ((xx - cx) / rx).clip(-1, 1)  # lateral shading
    for c in range(3):
        ch = frame[..., c]
        ch[mask] = (skin[c] * shade[mask]) + rng.normal(0, 3, int(mask.sum()))

    # eyes + mouth
    for ex in (cx - rx * 0.45, cx + rx * 0.45):
        ey = cy - ry * 0.25
        eye = (((yy - ey) / (ry * 0.09)) ** 2 + ((xx - ex) / (rx * 0.22)) ** 2) <= 1.0
        frame[eye] = (40, 35, 30)
    my = cy + ry * 0.45
    mouth = (((yy - my) / (ry * 0.07)) ** 2 + ((xx - cx) / (rx * 0.45)) ** 2) <= 1.0
    frame[mouth] = (70, 60, 150)

    if not live:
        at = attack_type or "unknown_2d"
        if at in ("replay_phone", "replay_monitor"):
            # moiré: two crossed sinusoidal gratings + slight color cast
            f1 = rng.uniform(0.35, 0.65)
            f2 = rng.uniform(0.35, 0.65)
            grate = 18 * (np.sin(f1 * xx + rng.uniform(0, 6)) + np.sin(f2 * yy))
            frame[..., 1:] += grate[..., None]
            frame[..., 0] += 12  # bluish screen cast
        elif at in ("print_flat", "print_curved"):
            # paper: flatten contrast, white border, dot noise
            frame = frame * 0.75 + 50
            b = max(2, size // 16)
            frame[:b], frame[-b:], frame[:, :b], frame[:, -b:] = 235, 235, 235, 235
            dots = rng.random((size, size)) > 0.985
            frame[dots] = 245
        elif at == "cutout_paper":
            # hard cut edge across the face + white backing
            cut = int(cy)
            frame[cut : cut + 2] = 250
            frame[: size // 8] = 240
        elif at == "mask_3d":
            # matte, low-texture, slightly gray face
            face_px = frame[mask]
            frame[mask] = face_px.mean(axis=0) * 0.9 + 15
        else:  # unknown_2d
            frame = frame * 0.7 + 40
            frame[..., 2] += 15

    return np.clip(frame, 0, 255).astype(np.uint8)


# ─── in-memory dataset ──────────────────────────────────────────────────────


class SyntheticPadDataset(PadDatasetBase):
    """In-memory synthetic PadSamples, subject-disjoint train/val like every
    other loader. ~half genuine, half attacks cycling all attack types;
    skin tones cycle monk_01..10 with an occasional None."""

    def __init__(
        self,
        n_subjects: int = 20,
        samples_per_subject: int = 4,
        split: str = "train",
        val_fraction: float = 0.25,
        frames_per_sample: int = 2,
        size: int = 128,
        seed: int = 7,
    ) -> None:
        if split not in ("train", "val"):
            raise ValueError(f"split must be train|val, got {split!r}")
        subjects = [f"synth_{i:04d}" for i in range(n_subjects)]
        train_subj, val_subj = split_subjects_disjoint(subjects, val_fraction, salt="synth")
        keep = train_subj if split == "train" else val_subj
        self._specs: list[tuple[str, int, Optional[str], Optional[str], int]] = []
        for si, subject in enumerate(subjects):
            if subject not in keep:
                continue
            for k in range(samples_per_subject):
                is_attack = (si + k) % 2 == 1
                attack = ATTACK_TYPES[(si + k) % len(ATTACK_TYPES)] if is_attack else None
                tone = None if (si + k) % 7 == 6 else SKIN_TONES[si % len(SKIN_TONES)]
                label = LABEL_SPOOF if is_attack else LABEL_LIVE
                self._specs.append((subject, label, attack, tone, seed * 100_003 + si * 131 + k))
        self.frames_per_sample = frames_per_sample
        self.size = size

    def __len__(self) -> int:
        return len(self._specs)

    def __iter__(self) -> Iterator[PadSample]:
        for subject, label, attack, tone, sd in self._specs:
            rng = np.random.default_rng(sd)
            frames = [
                make_face_frame(rng, label == LABEL_LIVE, attack, tone, self.size)
                for _ in range(self.frames_per_sample)
            ]
            yield PadSample(frames, label, attack, tone, subject, {"dataset": "synthetic"})


# ─── on-disk fixtures ───────────────────────────────────────────────────────


def _write_clip(path: Path, frames: list[np.ndarray], fps: float = 8.0) -> None:
    import cv2

    h, w = frames[0].shape[:2]
    for fourcc_name in ("mp4v", "avc1"):
        fourcc = cv2.VideoWriter_fourcc(*fourcc_name)
        writer = cv2.VideoWriter(str(path), fourcc, fps, (w, h))
        if writer.isOpened():
            for f in frames:
                writer.write(f)
            writer.release()
            if path.stat().st_size > 0:
                return
        writer.release()
    raise RuntimeError(
        f"cv2.VideoWriter could not encode {path.name} — the OpenCV build "
        "lacks an mp4 encoder (opencv-python-headless wheels include one)"
    )


def generate_nldea_fixture(
    root,
    n_subjects: int = 12,
    sessions_per_subject: int = 2,
    clips_per_session: int = 1,
    frames_per_clip: int = 8,
    size: int = 128,
    seed: int = 11,
) -> Path:
    """Session directories with schema-exact manifest.json + tiny mp4 clips.
    Subjects alternate genuine/attack sessions across all attack types,
    lighting values and devices; skinTone cycles the Monk scale with an
    occasional null. Returns root."""
    root = Path(root)
    root.mkdir(parents=True, exist_ok=True)
    rng = np.random.default_rng(seed)
    lightings = ("daylight", "indoor", "low_light")
    devices = (("Tecno Spark 10", "Android 13"), ("Samsung A24", "Android 14"),
               ("iPhone 12", "iOS 17"))
    challenges = (None, "blink", "turn_left", "turn_right", "smile")
    for si in range(n_subjects):
        subject = f"nldea_subj_{si:04d}"
        tone = None if si % 9 == 8 else SKIN_TONES[si % len(SKIN_TONES)]
        for sess in range(sessions_per_subject):
            is_attack = (si + sess) % 2 == 1
            attack = ATTACK_TYPES[(si + sess) % len(ATTACK_TYPES)] if is_attack else None
            sdir = root / f"session_{si:04d}_{sess}"
            sdir.mkdir(parents=True, exist_ok=True)
            clips = []
            for ci in range(clips_per_session):
                fname = f"clip_{ci + 1:03d}.mp4"
                frames = [
                    make_face_frame(rng, not is_attack, attack, tone, size)
                    for _ in range(frames_per_clip)
                ]
                _write_clip(sdir / fname, frames)
                clips.append({
                    "file": fname,
                    "durationMs": int(frames_per_clip / 8.0 * 1000),
                    "fps": 8,
                    "challenge": challenges[(si + sess + ci) % len(challenges)],
                })
            manifest = {
                "schemaVersion": 1,
                "sessionId": str(uuid.UUID(int=si * 1000 + sess)),
                "subjectId": subject,
                "consentId": f"consent_{si:04d}",
                "type": "attack" if is_attack else "genuine",
                "attackType": attack,
                "device": dict(zip(("model", "os"), devices[si % len(devices)])),
                "lighting": lightings[(si + sess) % len(lightings)],
                "skinTone": tone,
                "capturedAt": f"2026-08-{(si % 27) + 1:02d}T10:{sess:02d}:00Z",
                "clips": clips,
            }
            (sdir / "manifest.json").write_text(json.dumps(manifest, indent=2))
    return root


def generate_celeba_spoof_fixture(
    root, n_subjects: int = 8, imgs_per_class: int = 3, size: int = 128, seed: int = 13
) -> Path:
    """Original-archive layout: Data/train/<id>/{live,spoof}/*.png plus the
    metas/intra_test/train_label.json 43-attr vectors (index 40 = spoof type)."""
    import cv2

    root = Path(root)
    rng = np.random.default_rng(seed)
    labels: dict[str, list[int]] = {}
    spoof_codes = list(range(1, 11))
    for si in range(n_subjects):
        subject = f"{100000 + si}"
        for kind in ("live", "spoof"):
            d = root / "Data" / "train" / subject / kind
            d.mkdir(parents=True, exist_ok=True)
            for k in range(imgs_per_class):
                code = 0 if kind == "live" else spoof_codes[(si + k) % len(spoof_codes)]
                from liveness_training.datasets.celeba_spoof import SPOOF_TYPE_MAP

                attack = SPOOF_TYPE_MAP.get(code) if code else None
                img = make_face_frame(rng, kind == "live", attack, None, size)
                fname = f"{si * 100 + k:06d}.png"
                cv2.imwrite(str(d / fname), img)
                attrs = [0] * 43
                attrs[40] = code
                labels[f"Data/train/{subject}/{kind}/{fname}"] = attrs
    meta_dir = root / "metas" / "intra_test"
    meta_dir.mkdir(parents=True, exist_ok=True)
    (meta_dir / "train_label.json").write_text(json.dumps(labels))
    return root


def generate_celeba_spoof_parquet_fixture(
    root, n_subjects: int = 6, imgs_per_class: int = 2, size: int = 128,
    seed: int = 17, shards: int = 2,
) -> Path:
    """HF parquet-mirror layout (Ar4ikov/celebA_spoof): data/train-*.parquet
    with rows {Filepath: struct{bytes,path}, Bbox: list<int64>, Class: str}.
    Requires pyarrow (raises ImportError otherwise — callers/tests skip)."""
    import cv2
    import pyarrow as pa
    import pyarrow.parquet as pq

    root = Path(root)
    (root / "data").mkdir(parents=True, exist_ok=True)
    rng = np.random.default_rng(seed)
    rows: list[dict] = []
    for si in range(n_subjects):
        subject = f"{200000 + si}"
        for kind in ("live", "spoof"):
            for k in range(imgs_per_class):
                img = make_face_frame(rng, kind == "live", "replay_phone", None, size)
                ok, buf = cv2.imencode(".png", img)
                assert ok
                rows.append({
                    "Filepath": {
                        "bytes": buf.tobytes(),
                        "path": f"Data/train/{subject}/{kind}/{si * 10 + k:06d}.png",
                    },
                    "Bbox": [10, 10, size - 20, size - 20],
                    "Class": kind,
                })
    schema = pa.schema([
        ("Filepath", pa.struct([("bytes", pa.binary()), ("path", pa.string())])),
        ("Bbox", pa.list_(pa.int64())),
        ("Class", pa.string()),
    ])
    per = max(1, len(rows) // shards)
    chunks = [rows[i : i + per] for i in range(0, len(rows), per)]
    for i, chunk in enumerate(chunks):
        tbl = pa.Table.from_pylist(chunk, schema=schema)
        pq.write_table(tbl, root / "data" / f"train-{i:05d}-of-{len(chunks):05d}.parquet")
    return root


def generate_cefa_fixture(
    root, n_subjects_per_race: int = 3, frames_per_video: int = 4,
    size: int = 128, seed: int = 19,
) -> Path:
    """CeFA directory convention: <race>_<subject>_<session>_<pai>_<rep>/profile/*.jpg
    with races 1/2/3 and PAI 1 (real) / 2 (print) / 3 (replay)."""
    import cv2

    root = Path(root)
    rng = np.random.default_rng(seed)
    for race in ("1", "2", "3"):
        for si in range(n_subjects_per_race):
            for pai, attack in ((1, None), (2, "print_flat"), (3, "replay_monitor")):
                d = root / f"{race}_{si + 1:03d}_1_{pai}_1" / "profile"
                d.mkdir(parents=True, exist_ok=True)
                for k in range(frames_per_video):
                    img = make_face_frame(rng, pai == 1, attack, None, size)
                    cv2.imwrite(str(d / f"{k + 1:04d}.jpg"), img)
    return root
