"""The red-team run: sessions in, per-presentation rows out.

One **clip = one presentation** (the ISO unit: one attempt of one attack
instrument against one capture subject/device). For each clip the runner
samples frames (default 5 — production parity with the Go provider's
``maxFrames``), calls the scorer, and writes a row with the verdict, the
fusion sub-score breakdown and the accept/reject decision at the run's
threshold.

Failures are contained: a bad manifest, an unreadable clip or a 503 from the
engine records an errored row and the run continues. A 500-presentation
sustain run must never be lost to one corrupt file — but errored rows are
excluded from APCER/BPCER (they are not classifications) and surfaced in the
report instead.
"""
from __future__ import annotations

import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone

from . import session as session_mod
from .frames import DEFAULT_FRAME_COUNT, ClipError, clamp_frame_count, sample_frames
from .scorers import DEFAULT_THRESHOLD, Scorer, ScorerError
from .storage import PresentationRow, ResultsDB, RunRecord, dumps, utcnow


def new_run_id(prefix: str = "redteam") -> str:
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    return f"{prefix}-{stamp}-{uuid.uuid4().hex[:6]}"


@dataclass
class RunConfig:
    sessions_root: str
    db_path: str = "results.db"
    frame_count: int = DEFAULT_FRAME_COUNT
    sample_fps: float | None = None
    threshold: float = DEFAULT_THRESHOLD
    run_id: str = ""
    notes: str = ""
    send_challenge: bool = True  # forward clip.challengeResult when recorded
    limit: int | None = None  # cap presentations (smoke runs)


@dataclass
class RunSummary:
    run_id: str
    model_version: str
    scorer: str
    target: str
    threshold: float
    sessions: int = 0
    presentations: int = 0
    errors: int = 0
    manifest_failures: list = field(default_factory=list)

    @property
    def ok(self) -> bool:
        return not self.manifest_failures and self.errors == 0


def _challenge_for(clip, config: RunConfig) -> bool | None:
    if not config.send_challenge:
        return None
    return clip.challenge_result


def run(
    config: RunConfig,
    scorer: Scorer,
    db: ResultsDB | None = None,
    progress=None,
) -> RunSummary:
    """Score every session under ``config.sessions_root`` into the results DB.

    ``progress`` is an optional ``callable(message: str)`` for CLI output.
    """
    clamp_frame_count(config.frame_count)
    db = db or ResultsDB(config.db_path)
    emit = progress or (lambda _msg: None)

    run_id = config.run_id or new_run_id()
    model_version = scorer.model_version()
    health = getattr(scorer, "health", None)
    health_json = dumps(health()) if callable(health) else None

    sessions, manifest_failures = session_mod.load_sessions(config.sessions_root)
    summary = RunSummary(
        run_id=run_id,
        model_version=model_version,
        scorer=scorer.name,
        target=scorer.target(),
        threshold=config.threshold,
        sessions=len(sessions),
        manifest_failures=manifest_failures,
    )
    for path, message in manifest_failures:
        emit(f"  ! skipping {path}: {message}")

    db.start_run(
        RunRecord(
            run_id=run_id,
            started_at=utcnow(),
            scorer=scorer.name,
            target=scorer.target(),
            model_version=model_version,
            threshold=config.threshold,
            frame_count=config.frame_count,
            sessions_root=config.sessions_root,
            notes=config.notes,
            health_json=health_json or "",
        )
    )
    emit(
        f"run {run_id} · scorer {scorer.name} ({scorer.target()}) · "
        f"model {model_version} · threshold {config.threshold} · "
        f"{config.frame_count} frames/presentation"
    )

    try:
        for sess in sessions:
            for clip in sess.clips:
                if config.limit is not None and summary.presentations >= config.limit:
                    emit(f"  limit {config.limit} reached — stopping")
                    db.commit()
                    db.finish_run(run_id)
                    return summary
                row = _score_clip(config, scorer, sess, clip, run_id)
                db.add_presentation(row)
                summary.presentations += 1
                if row.error:
                    summary.errors += 1
                    emit(f"  ERR {sess.session_id}/{clip.file}: {row.error}")
                else:
                    emit(
                        f"  {row.presentation_type:7s} "
                        f"{(row.species or 'genuine'):14s} "
                        f"{clip.file:16s} score={row.live_score:.4f} "
                        f"{row.label:7s} "
                        f"{'ACCEPTED' if row.accepted else 'rejected'}"
                    )
            db.commit()
        db.finish_run(run_id)
    finally:
        db.commit()
    return summary


def _score_clip(
    config: RunConfig,
    scorer: Scorer,
    sess: session_mod.Session,
    clip: session_mod.Clip,
    run_id: str,
) -> PresentationRow:
    row = PresentationRow(
        run_id=run_id,
        session_id=sess.session_id,
        subject_id=sess.subject_id,
        consent_id=sess.consent_id,
        presentation_type=sess.type,
        species=sess.species,
        level=sess.level,
        device_model=sess.device_model,
        device_os=sess.device_os,
        lighting=sess.lighting,
        skin_tone=sess.skin_tone,
        clip_file=clip.file,
        challenge=clip.challenge,
        challenge_result=(
            None
            if clip.challenge_result is None
            else ("passed" if clip.challenge_result else "failed")
        ),
        frames_used=0,
    )
    try:
        frames = sample_frames(
            clip.path(sess.path), config.frame_count, config.sample_fps
        )
    except Exception as e:  # noqa: BLE001 - contain per-clip faults
        row.error = f"frame sampling: {e}"
        return row

    row.frames_used = len(frames)
    try:
        result = scorer.score(frames, _challenge_for(clip, config))
    except ScorerError as e:
        row.error = str(e)
        return row
    except Exception as e:  # noqa: BLE001 - a crashed scorer loses one row only
        row.error = f"scorer: {e}"
        return row

    row.live_score = result.live_score
    row.label = result.label
    row.model = result.model
    row.accepted = int(result.live_score >= config.threshold)
    row.fusion_json = dumps(result.fusion)
    return row
