"""Synthetic session generator — CI smoke fixtures, not attack material.

Produces tiny valid sessions (a few 64x64 mp4 clips plus a manifest) so the
whole rig — discovery, validation, sampling, scoring, sqlite, metrics, gates,
report — runs end to end without a single real capture. Frame content is
deliberately crude and species-suggestive (flat colour for prints, a stripe
grid for replays, drifting noise for genuine) so the fusion sub-scores vary a
little between species; **nothing here validates detector accuracy**, and no
synthetic run may ever be cited as evidence toward a certification gate.

With no MiniFASNet ONNX provisioned the engine runs its deterministic
fallback: no face is detected in synthetic frames, so every presentation
comes back UNKNOWN at 0.0 and every attack is correctly rejected. That is the
expected CI shape — the rig is what is under test, not the model.
"""
from __future__ import annotations

import os
import uuid
from datetime import datetime, timedelta, timezone

from . import taxonomy
from .session import Clip, Session, write_manifest

FRAME_SIZE = 64
CLIP_FRAMES = 12
CLIP_FPS = 12.0

_DEVICES = (
    ("Tecno Spark 10", "Android 13"),
    ("Redmi Note 12", "Android 13"),
    ("Samsung Galaxy A14", "Android 14"),
    ("Camon 20", "Android 13"),
)
_LIGHTING = taxonomy.LIGHTING
_TONES = ("monk_06", "monk_07", "monk_08", "monk_09", "monk_10")


def _pattern_for(species: str | None) -> str:
    if species is None:
        return "noise"
    category = taxonomy.SPECIES[species].category
    return {"print": "flat", "replay": "grid", "cutout": "flat", "mask": "blob"}[
        category
    ]


def _frame(pattern: str, index: int, rng, size: int = FRAME_SIZE):
    import numpy as np

    if pattern == "flat":
        base = np.full((size, size, 3), 128, dtype=np.uint8)
        base[:, :, 0] = 100 + (index % 3)  # near-static: a rigid print
        return base
    if pattern == "grid":
        xx = np.arange(size)
        stripes = ((xx + index) % 4 < 2).astype(np.uint8) * 90 + 60
        return np.repeat(stripes[None, :, None], size, axis=0).repeat(3, axis=2)
    if pattern == "blob":
        yy, xx = np.mgrid[0:size, 0:size]
        disc = (((xx - size / 2 - index) ** 2 + (yy - size / 2) ** 2) < (size / 3) ** 2)
        return (disc[..., None] * np.array([90, 120, 160], dtype=np.uint8)).astype(
            "uint8"
        )
    # "noise": textured and drifting — the only pattern with real parallax
    noise = rng.integers(0, 255, size=(size, size, 3), dtype=np.uint8)
    return np.roll(noise, index * 2, axis=1)


def write_clip(path: str, pattern: str, seed: int, frames: int = CLIP_FRAMES) -> None:
    import cv2
    import numpy as np

    rng = np.random.default_rng(seed)
    writer = cv2.VideoWriter(
        path, cv2.VideoWriter_fourcc(*"mp4v"), CLIP_FPS, (FRAME_SIZE, FRAME_SIZE)
    )
    if not writer.isOpened():  # pragma: no cover - depends on local codecs
        raise RuntimeError(f"cannot open VideoWriter for {path}")
    try:
        for i in range(frames):
            writer.write(_frame(pattern, i, rng))
    finally:
        writer.release()


def make_session(
    root: str,
    *,
    presentation_type: str,
    attack_type: str | None = None,
    clips: int = 1,
    seed: int = 0,
    name: str | None = None,
    challenge: str | None = None,
    challenge_result: bool | None = None,
) -> Session:
    """Write one synthetic session directory and return its Session."""
    session_id = str(uuid.UUID(int=seed | 0xA17E_0000_0000_0000_0000_0000_0000_0000))
    directory = os.path.join(root, name or f"session-{session_id[:8]}")
    os.makedirs(directory, exist_ok=True)

    pattern = _pattern_for(attack_type if presentation_type == "attack" else None)
    clip_records = []
    for i in range(clips):
        file_name = f"clip_{i + 1:03d}.mp4"
        write_clip(os.path.join(directory, file_name), pattern, seed * 97 + i)
        clip_records.append(
            Clip(
                file=file_name,
                duration_ms=int(CLIP_FRAMES / CLIP_FPS * 1000),
                fps=CLIP_FPS,
                challenge=challenge,
                challenge_result=challenge_result,
            )
        )

    device = _DEVICES[seed % len(_DEVICES)]
    captured = datetime(2026, 8, 1, tzinfo=timezone.utc) + timedelta(minutes=seed)
    sess = Session(
        session_id=session_id,
        subject_id=f"subj-{seed:04d}",
        consent_id=f"consent-{seed:04d}",
        type=presentation_type,
        attack_type=attack_type if presentation_type == "attack" else None,
        device_model=device[0],
        device_os=device[1],
        lighting=_LIGHTING[seed % len(_LIGHTING)],
        skin_tone=_TONES[seed % len(_TONES)],
        captured_at=captured.isoformat(timespec="seconds"),
        clips=tuple(clip_records),
        path=directory,
    )
    write_manifest(directory, sess)
    return sess


def make_battery(
    root: str,
    *,
    genuine: int = 3,
    per_species: int = 2,
    species: tuple[str, ...] | None = None,
    clips_per_session: int = 1,
    seed: int = 1,
) -> list[Session]:
    """A full synthetic battery: genuine sessions + N per attack species."""
    os.makedirs(root, exist_ok=True)
    species = species or taxonomy.l1_species()
    sessions = []
    counter = seed

    for i in range(genuine):
        sessions.append(
            make_session(
                root,
                presentation_type="genuine",
                clips=clips_per_session,
                seed=counter,
                name=f"genuine-{i + 1:03d}",
                challenge="blink" if i % 2 == 0 else None,
                challenge_result=True if i % 2 == 0 else None,
            )
        )
        counter += 1

    for key in species:
        for i in range(per_species):
            sessions.append(
                make_session(
                    root,
                    presentation_type="attack",
                    attack_type=key,
                    clips=clips_per_session,
                    seed=counter,
                    name=f"attack-{key}-{i + 1:03d}",
                    challenge="blink" if key == "cutout_paper" else None,
                    challenge_result=True if key == "cutout_paper" else None,
                )
            )
            counter += 1
    return sessions
