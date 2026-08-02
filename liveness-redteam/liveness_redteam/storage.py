"""sqlite result store: one row per presentation, keyed by run id.

Every run records the **model version** the scorer reported. Trend tables in
report.py group on it, so a model swap can never be mistaken for a threshold
improvement — which is the whole point of tracking progress toward 0/500.
"""
from __future__ import annotations

import json
import os
import sqlite3
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone

from .metrics import Presentation

SCHEMA = """
CREATE TABLE IF NOT EXISTS runs (
    run_id        TEXT PRIMARY KEY,
    started_at    TEXT NOT NULL,
    finished_at   TEXT,
    scorer        TEXT NOT NULL,
    target        TEXT NOT NULL,
    model_version TEXT NOT NULL,
    threshold     REAL NOT NULL,
    frame_count   INTEGER NOT NULL,
    sessions_root TEXT NOT NULL,
    notes         TEXT,
    health_json   TEXT
);

CREATE TABLE IF NOT EXISTS presentations (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id            TEXT NOT NULL REFERENCES runs(run_id),
    session_id        TEXT NOT NULL,
    subject_id        TEXT,
    consent_id        TEXT,
    presentation_type TEXT NOT NULL,
    species           TEXT,
    level             TEXT,
    device_model      TEXT,
    device_os         TEXT,
    lighting          TEXT,
    skin_tone         TEXT,
    clip_file         TEXT NOT NULL,
    challenge         TEXT,
    challenge_result  TEXT,
    frames_used       INTEGER NOT NULL,
    live_score        REAL,
    label             TEXT,
    model             TEXT,
    accepted          INTEGER,
    fusion_json       TEXT,
    error             TEXT,
    scored_at         TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_presentations_run ON presentations(run_id);
CREATE INDEX IF NOT EXISTS idx_presentations_species
    ON presentations(run_id, species);
"""


def utcnow() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


@dataclass
class RunRecord:
    run_id: str
    started_at: str
    scorer: str
    target: str
    model_version: str
    threshold: float
    frame_count: int
    sessions_root: str
    notes: str = ""
    health_json: str = ""
    finished_at: str | None = None


@dataclass
class PresentationRow:
    run_id: str
    session_id: str
    presentation_type: str
    clip_file: str
    frames_used: int
    scored_at: str = field(default_factory=utcnow)
    subject_id: str = ""
    consent_id: str = ""
    species: str | None = None
    level: str | None = None
    device_model: str = ""
    device_os: str = ""
    lighting: str = ""
    skin_tone: str | None = None
    challenge: str | None = None
    challenge_result: str | None = None
    live_score: float | None = None
    label: str | None = None
    model: str | None = None
    accepted: int | None = None
    fusion_json: str | None = None
    error: str | None = None

    @property
    def is_attack(self) -> bool:
        return self.presentation_type == "attack"

    def to_presentation(self) -> Presentation | None:
        """Metric view of this row; None for errored presentations."""
        if self.live_score is None:
            return None
        return Presentation(
            is_attack=self.is_attack,
            species=self.species,
            score=float(self.live_score),
            label=self.label or "",
            session_id=self.session_id,
            clip=self.clip_file,
        )


class ResultsDB:
    """Thin sqlite wrapper — stdlib only, one file, safe to commit-free copy."""

    def __init__(self, path: str):
        self.path = path
        parent = os.path.dirname(os.path.abspath(path))
        if parent:
            os.makedirs(parent, exist_ok=True)
        self.conn = sqlite3.connect(path)
        self.conn.row_factory = sqlite3.Row
        self.conn.executescript(SCHEMA)
        self.conn.commit()

    def close(self) -> None:
        self.conn.close()

    def __enter__(self) -> "ResultsDB":
        return self

    def __exit__(self, *exc) -> None:
        self.close()

    # ── runs ────────────────────────────────────────────────────────────────

    def start_run(self, run: RunRecord) -> None:
        self.conn.execute(
            "INSERT INTO runs (run_id, started_at, scorer, target, "
            "model_version, threshold, frame_count, sessions_root, notes, "
            "health_json) VALUES (?,?,?,?,?,?,?,?,?,?)",
            (
                run.run_id,
                run.started_at,
                run.scorer,
                run.target,
                run.model_version,
                run.threshold,
                run.frame_count,
                run.sessions_root,
                run.notes,
                run.health_json,
            ),
        )
        self.conn.commit()

    def finish_run(self, run_id: str, finished_at: str | None = None) -> None:
        self.conn.execute(
            "UPDATE runs SET finished_at = ? WHERE run_id = ?",
            (finished_at or utcnow(), run_id),
        )
        self.conn.commit()

    def get_run(self, run_id: str) -> RunRecord | None:
        row = self.conn.execute(
            "SELECT * FROM runs WHERE run_id = ?", (run_id,)
        ).fetchone()
        return None if row is None else RunRecord(**dict(row))

    def list_runs(self, limit: int = 20) -> list[RunRecord]:
        rows = self.conn.execute(
            "SELECT * FROM runs ORDER BY started_at DESC, rowid DESC LIMIT ?",
            (limit,),
        ).fetchall()
        return [RunRecord(**dict(r)) for r in rows]

    def latest_run_id(self) -> str | None:
        runs = self.list_runs(1)
        return runs[0].run_id if runs else None

    def runs_before(self, run_id: str, limit: int = 5) -> list[RunRecord]:
        """Previous runs (most recent first) for the trend table."""
        run = self.get_run(run_id)
        if run is None:
            return []
        rows = self.conn.execute(
            "SELECT * FROM runs WHERE started_at < ? AND run_id != ? "
            "ORDER BY started_at DESC LIMIT ?",
            (run.started_at, run_id, limit),
        ).fetchall()
        return [RunRecord(**dict(r)) for r in rows]

    # ── presentations ───────────────────────────────────────────────────────

    def add_presentation(self, row: PresentationRow) -> None:
        data = asdict(row)
        columns = ", ".join(data)
        placeholders = ", ".join("?" * len(data))
        self.conn.execute(
            f"INSERT INTO presentations ({columns}) VALUES ({placeholders})",
            tuple(data.values()),
        )

    def commit(self) -> None:
        self.conn.commit()

    def presentations(self, run_id: str) -> list[PresentationRow]:
        rows = self.conn.execute(
            "SELECT * FROM presentations WHERE run_id = ? ORDER BY id",
            (run_id,),
        ).fetchall()
        out = []
        for r in rows:
            data = {k: r[k] for k in r.keys() if k != "id"}
            out.append(PresentationRow(**data))
        return out

    def scored_presentations(self, run_id: str) -> list[Presentation]:
        """Metric view: errored presentations are excluded (and counted
        separately by the caller — a crashed scorer is not a rejection)."""
        return [
            p
            for p in (r.to_presentation() for r in self.presentations(run_id))
            if p is not None
        ]

    def error_count(self, run_id: str) -> int:
        return int(
            self.conn.execute(
                "SELECT COUNT(*) FROM presentations "
                "WHERE run_id = ? AND error IS NOT NULL",
                (run_id,),
            ).fetchone()[0]
        )


def dumps(value) -> str | None:
    return None if value is None else json.dumps(value, sort_keys=True)
