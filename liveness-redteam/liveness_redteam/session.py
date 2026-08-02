"""NLD-EA capture-session format (schemaVersion 1) — load and validate.

A session is a **directory** containing ``manifest.json`` plus its clips:

    session-0af1.../
      manifest.json
      clip_001.mp4
      clip_002.mp4

``manifest.json`` (contract shared with the capture app and the dataset
tooling — implemented here exactly as specified):

    {"schemaVersion": 1,
     "sessionId": "<uuid>",
     "subjectId": "<pseudonymous-id>",
     "consentId": "<id>",
     "type": "genuine" | "attack",
     "attackType": null | "print_flat" | "print_curved" | "replay_phone"
                   | "replay_monitor" | "cutout_paper" | "mask_3d",
     "device": {"model": "<str>", "os": "<str>"},
     "lighting": "daylight" | "indoor" | "low_light",
     "skinTone": "monk_01".."monk_10" | null,
     "capturedAt": "<iso8601>",
     "clips": [{"file": "clip_001.mp4", "durationMs": 0, "fps": 0,
                "challenge": null | "blink" | "turn_left" | "turn_right"
                             | "smile"}]}

Red-team extensions (additive, optional, ignored by the other consumers):

* ``attackType`` additionally accepts ``replay_tablet``, ``mask_silicone``,
  ``mask_latex``, ``mask_resin`` — material granularity matters for
  worst-species APCER (see taxonomy.py).
* a clip may carry ``"challengeResult": "passed" | "failed"`` recording what
  the on-device active challenge actually returned, so the rig can forward it
  to the engine's optional ``challenge`` form field. Absent = not run, and the
  rig then sends nothing rather than inventing an outcome.

Unknown keys are preserved (``extra``) and never rejected: the format is
shared with two sibling tools and must stay forward-compatible.
"""
from __future__ import annotations

import json
import os
from dataclasses import dataclass, field
from datetime import datetime

from . import taxonomy

SCHEMA_VERSION = 1
MANIFEST_NAME = "manifest.json"


class ManifestError(ValueError):
    """Raised when a manifest cannot be loaded or fails validation."""


@dataclass(frozen=True)
class Clip:
    file: str
    duration_ms: int = 0
    fps: float = 0.0
    challenge: str | None = None
    challenge_result: bool | None = None  # red-team extension; None = not run

    def path(self, session_dir: str) -> str:
        return os.path.join(session_dir, self.file)

    def as_manifest(self) -> dict:
        d = {
            "file": self.file,
            "durationMs": self.duration_ms,
            "fps": self.fps,
            "challenge": self.challenge,
        }
        if self.challenge_result is not None:
            d["challengeResult"] = "passed" if self.challenge_result else "failed"
        return d


@dataclass(frozen=True)
class Session:
    session_id: str
    subject_id: str
    consent_id: str
    type: str
    attack_type: str | None
    device_model: str
    device_os: str
    lighting: str
    skin_tone: str | None
    captured_at: str
    clips: tuple[Clip, ...]
    path: str = ""
    schema_version: int = SCHEMA_VERSION
    extra: dict = field(default_factory=dict)

    @property
    def is_attack(self) -> bool:
        return self.type == taxonomy.ATTACK

    @property
    def species(self) -> str | None:
        """Attack species key, or None for a genuine presentation."""
        return self.attack_type if self.is_attack else None

    @property
    def level(self) -> str | None:
        return None if self.species is None else taxonomy.level_of(self.species)

    def as_manifest(self) -> dict:
        d = {
            "schemaVersion": self.schema_version,
            "sessionId": self.session_id,
            "subjectId": self.subject_id,
            "consentId": self.consent_id,
            "type": self.type,
            "attackType": self.attack_type,
            "device": {"model": self.device_model, "os": self.device_os},
            "lighting": self.lighting,
            "skinTone": self.skin_tone,
            "capturedAt": self.captured_at,
            "clips": [c.as_manifest() for c in self.clips],
        }
        d.update(self.extra)
        return d


_CHALLENGE_RESULTS = {"passed": True, "failed": False}
_KNOWN_TOP_KEYS = {
    "schemaVersion",
    "sessionId",
    "subjectId",
    "consentId",
    "type",
    "attackType",
    "device",
    "lighting",
    "skinTone",
    "capturedAt",
    "clips",
}


def _is_str(v) -> bool:
    return isinstance(v, str) and v.strip() != ""


def validate_manifest(data, *, session_dir: str | None = None) -> list[str]:
    """Return a list of human-readable validation errors ([] when valid).

    When ``session_dir`` is given, clip files must also exist on disk.
    """
    errors: list[str] = []
    if not isinstance(data, dict):
        return ["manifest must be a JSON object"]

    if data.get("schemaVersion") != SCHEMA_VERSION:
        errors.append(
            f"schemaVersion must be {SCHEMA_VERSION} "
            f"(got {data.get('schemaVersion')!r})"
        )
    for key in ("sessionId", "subjectId", "consentId"):
        if not _is_str(data.get(key)):
            errors.append(f"{key} must be a non-empty string")

    ptype = data.get("type")
    if ptype not in taxonomy.PRESENTATION_TYPES:
        errors.append(
            "type must be one of "
            f"{', '.join(taxonomy.PRESENTATION_TYPES)} (got {ptype!r})"
        )

    attack_type = data.get("attackType")
    if ptype == taxonomy.ATTACK:
        if not taxonomy.is_attack_type(attack_type or ""):
            errors.append(
                "attack sessions need attackType in "
                f"{{{', '.join(taxonomy.ATTACK_TYPES)}}} (got {attack_type!r})"
            )
    elif ptype == taxonomy.GENUINE and attack_type is not None:
        errors.append("genuine sessions must have attackType null")

    device = data.get("device")
    if not isinstance(device, dict):
        errors.append("device must be an object {model, os}")
    else:
        for key in ("model", "os"):
            if not _is_str(device.get(key)):
                errors.append(f"device.{key} must be a non-empty string")

    if data.get("lighting") not in taxonomy.LIGHTING:
        errors.append(
            f"lighting must be one of {', '.join(taxonomy.LIGHTING)} "
            f"(got {data.get('lighting')!r})"
        )

    skin_tone = data.get("skinTone")
    if skin_tone is not None and skin_tone not in taxonomy.SKIN_TONES:
        errors.append(
            "skinTone must be monk_01..monk_10 or null "
            f"(got {skin_tone!r})"
        )

    captured_at = data.get("capturedAt")
    if not _is_str(captured_at):
        errors.append("capturedAt must be a non-empty ISO-8601 string")
    else:
        try:
            datetime.fromisoformat(str(captured_at).replace("Z", "+00:00"))
        except ValueError:
            errors.append(f"capturedAt is not ISO-8601: {captured_at!r}")

    clips = data.get("clips")
    if not isinstance(clips, list) or not clips:
        errors.append("clips must be a non-empty array")
    else:
        for i, clip in enumerate(clips):
            where = f"clips[{i}]"
            if not isinstance(clip, dict):
                errors.append(f"{where} must be an object")
                continue
            file_name = clip.get("file")
            if not _is_str(file_name):
                errors.append(f"{where}.file must be a non-empty string")
            elif os.path.isabs(file_name) or ".." in file_name.split("/"):
                errors.append(
                    f"{where}.file must be a relative path inside the session "
                    f"directory (got {file_name!r})"
                )
            elif session_dir is not None and not os.path.isfile(
                os.path.join(session_dir, file_name)
            ):
                errors.append(f"{where}.file not found on disk: {file_name}")
            if not isinstance(clip.get("durationMs", 0), (int, float)):
                errors.append(f"{where}.durationMs must be a number")
            if not isinstance(clip.get("fps", 0), (int, float)):
                errors.append(f"{where}.fps must be a number")
            challenge = clip.get("challenge")
            if challenge is not None and challenge not in taxonomy.CHALLENGES:
                errors.append(
                    f"{where}.challenge must be null or one of "
                    f"{', '.join(taxonomy.CHALLENGES)} (got {challenge!r})"
                )
            result = clip.get("challengeResult")
            if result is not None and result not in _CHALLENGE_RESULTS:
                errors.append(
                    f"{where}.challengeResult must be null, 'passed' or "
                    f"'failed' (got {result!r})"
                )
    return errors


def session_from_manifest(data: dict, path: str = "") -> Session:
    """Build a Session from an already-validated manifest dict."""
    device = data.get("device") or {}
    clips = tuple(
        Clip(
            file=c["file"],
            duration_ms=int(c.get("durationMs") or 0),
            fps=float(c.get("fps") or 0.0),
            challenge=c.get("challenge"),
            challenge_result=_CHALLENGE_RESULTS.get(c.get("challengeResult")),
        )
        for c in data["clips"]
    )
    return Session(
        session_id=data["sessionId"],
        subject_id=data["subjectId"],
        consent_id=data["consentId"],
        type=data["type"],
        attack_type=data.get("attackType"),
        device_model=device.get("model", ""),
        device_os=device.get("os", ""),
        lighting=data["lighting"],
        skin_tone=data.get("skinTone"),
        captured_at=data["capturedAt"],
        clips=clips,
        path=path,
        schema_version=int(data["schemaVersion"]),
        extra={k: v for k, v in data.items() if k not in _KNOWN_TOP_KEYS},
    )


def load_session(session_dir: str, *, require_files: bool = True) -> Session:
    """Load + validate one session directory. Raises ManifestError."""
    manifest_path = os.path.join(session_dir, MANIFEST_NAME)
    if not os.path.isfile(manifest_path):
        raise ManifestError(f"no {MANIFEST_NAME} in {session_dir}")
    try:
        with open(manifest_path, encoding="utf-8") as fh:
            data = json.load(fh)
    except json.JSONDecodeError as e:
        raise ManifestError(f"{manifest_path}: invalid JSON: {e}") from e

    errors = validate_manifest(
        data, session_dir=session_dir if require_files else None
    )
    if errors:
        raise ManifestError(
            f"{manifest_path}: " + "; ".join(errors)
        )
    return session_from_manifest(data, path=session_dir)


def find_session_dirs(root: str) -> list[str]:
    """Every directory at or under ``root`` holding a manifest.json, sorted."""
    if os.path.isfile(os.path.join(root, MANIFEST_NAME)):
        return [root]
    found = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames.sort()
        if MANIFEST_NAME in filenames:
            found.append(dirpath)
            dirnames[:] = []  # sessions do not nest
    return sorted(found)


def load_sessions(
    root: str, *, require_files: bool = True
) -> tuple[list[Session], list[tuple[str, str]]]:
    """Load every session under ``root``.

    Returns ``(sessions, failures)`` where each failure is
    ``(session_dir, error message)`` — a bad manifest must not abort a
    500-presentation run.
    """
    sessions: list[Session] = []
    failures: list[tuple[str, str]] = []
    for session_dir in find_session_dirs(root):
        try:
            sessions.append(
                load_session(session_dir, require_files=require_files)
            )
        except ManifestError as e:
            failures.append((session_dir, str(e)))
    return sessions, failures


def write_manifest(session_dir: str, session: Session) -> str:
    """Write ``session`` as manifest.json into ``session_dir``."""
    os.makedirs(session_dir, exist_ok=True)
    path = os.path.join(session_dir, MANIFEST_NAME)
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(session.as_manifest(), fh, indent=2, sort_keys=False)
        fh.write("\n")
    return path
