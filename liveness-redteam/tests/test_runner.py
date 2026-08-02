"""End-to-end runner over synthetic sessions (in-process fallback engine)."""
from __future__ import annotations

import json

import pytest

from liveness_redteam import frames as F
from liveness_redteam import metrics as M
from liveness_redteam import session as S
from liveness_redteam import synth, taxonomy
from liveness_redteam.runner import RunConfig, new_run_id, run
from liveness_redteam.scorers import InProcessScorer, ScoreResult, Scorer
from liveness_redteam.storage import ResultsDB


class StubScorer(Scorer):
    """Deterministic scorer: species -> score, so the runner is testable
    without depending on what the real engine says about noise frames."""

    name = "stub"

    def __init__(self, scores: dict, fail_on: str | None = None):
        self.scores = scores
        self.fail_on = fail_on
        self.calls = []

    def model_version(self) -> str:
        return "stub@v1"

    def target(self) -> str:
        return "stub://local"

    def score(self, frames, challenge_passed=None) -> ScoreResult:
        self.calls.append((len(frames), challenge_passed))
        if self.fail_on and len(self.calls) == 1:
            from liveness_redteam.scorers import ScorerError

            raise ScorerError(self.fail_on)
        value = self.scores.get("_default", 0.2)
        return ScoreResult(
            live_score=value,
            label="LIVE" if value >= 0.5 else "SPOOF",
            model="stub",
            per_frame=[{"score": value, "faceFound": True}] * len(frames),
            fusion={"score": value, "padMedian": value, "frames": len(frames)},
        )


@pytest.fixture()
def battery(tmp_path):
    root = tmp_path / "sessions"
    synth.make_battery(str(root), genuine=2, per_species=1, seed=1)
    return str(root)


# ─── synthetic fixtures ──────────────────────────────────────────────────────


def test_synth_battery_is_valid_and_covers_every_l1_species(battery):
    sessions, failures = S.load_sessions(battery)
    assert failures == []
    assert len(sessions) == 2 + len(taxonomy.l1_species())
    species = {s.species for s in sessions if s.is_attack}
    assert species == set(taxonomy.l1_species())
    assert sum(1 for s in sessions if not s.is_attack) == 2


def test_synth_clips_decode_and_sample(battery):
    sessions, _ = S.load_sessions(battery)
    clip = sessions[0].clips[0]
    sampled = F.sample_frames(clip.path(sessions[0].path), 5)
    assert len(sampled) == 5
    assert sampled[0].shape == (synth.FRAME_SIZE, synth.FRAME_SIZE, 3)


def test_frame_count_contract():
    # api/face.py _MAX_LIVENESS_FRAMES = 10; inhouse.go maxFrames = 5
    assert F.DEFAULT_FRAME_COUNT == 5
    assert F.MAX_FRAME_COUNT == 10
    with pytest.raises(ValueError):
        F.clamp_frame_count(11)
    with pytest.raises(ValueError):
        F.clamp_frame_count(0)


def test_sample_indices_span_the_clip():
    assert F._sample_indices(12, 5) == [0, 3, 6, 8, 11]
    assert F._sample_indices(3, 5) == [0, 1, 2]
    assert F._sample_indices(9, 1) == [4]


# ─── runner ──────────────────────────────────────────────────────────────────


def test_runner_records_one_row_per_clip(battery, tmp_path):
    scorer = StubScorer({"_default": 0.2})
    db_path = str(tmp_path / "results.db")
    config = RunConfig(sessions_root=battery, db_path=db_path, threshold=0.5)
    with ResultsDB(db_path) as db:
        summary = run(config, scorer, db=db)
        rows = db.presentations(summary.run_id)

    assert summary.presentations == 8  # 2 genuine + 6 L1 species, 1 clip each
    assert summary.errors == 0
    assert summary.manifest_failures == []
    assert len(rows) == 8
    assert all(r.frames_used == 5 for r in rows)
    assert all(call[0] == 5 for call in scorer.calls)  # 5 frames per call
    assert {r.presentation_type for r in rows} == {"genuine", "attack"}


def test_runner_persists_metadata_verdicts_and_fusion(battery, tmp_path):
    db_path = str(tmp_path / "results.db")
    config = RunConfig(sessions_root=battery, db_path=db_path, threshold=0.5)
    with ResultsDB(db_path) as db:
        summary = run(config, StubScorer({"_default": 0.2}), db=db)
        rows = db.presentations(summary.run_id)
        record = db.get_run(summary.run_id)

    attacks = [r for r in rows if r.presentation_type == "attack"]
    assert {r.species for r in attacks} == set(taxonomy.l1_species())
    assert all(r.level == "L1" for r in attacks)
    assert all(r.accepted == 0 for r in rows)  # 0.2 < 0.5
    assert all(r.label == "SPOOF" for r in rows)
    assert all(r.device_model and r.lighting for r in rows)
    assert json.loads(attacks[0].fusion_json)["padMedian"] == 0.2

    assert record.model_version == "stub@v1"
    assert record.threshold == 0.5
    assert record.frame_count == 5
    assert record.finished_at


def test_runner_forwards_recorded_challenge_results(battery, tmp_path):
    scorer = StubScorer({"_default": 0.2})
    db_path = str(tmp_path / "results.db")
    run(RunConfig(sessions_root=battery, db_path=db_path), scorer, ResultsDB(db_path))
    forwarded = [c[1] for c in scorer.calls]
    # make_battery marks the cutout_paper and one genuine session as
    # challenge-passed; every other clip must send nothing at all
    assert True in forwarded
    assert None in forwarded
    assert False not in forwarded


def test_runner_can_suppress_challenge_forwarding(battery, tmp_path):
    scorer = StubScorer({"_default": 0.2})
    db_path = str(tmp_path / "results.db")
    run(
        RunConfig(sessions_root=battery, db_path=db_path, send_challenge=False),
        scorer,
        ResultsDB(db_path),
    )
    assert all(c[1] is None for c in scorer.calls)


def test_runner_contains_scorer_failures(battery, tmp_path):
    scorer = StubScorer({"_default": 0.2}, fail_on="engine 503")
    db_path = str(tmp_path / "results.db")
    with ResultsDB(db_path) as db:
        summary = run(RunConfig(sessions_root=battery, db_path=db_path), scorer, db)
        rows = db.presentations(summary.run_id)
        scored = db.scored_presentations(summary.run_id)

    assert summary.errors == 1
    assert summary.presentations == 8  # the run continued
    assert db_errors(rows) == ["engine 503"]
    # errored presentations are not classifications and must not skew rates
    assert len(scored) == 7


def db_errors(rows) -> list:
    return [r.error for r in rows if r.error]


def test_missing_clip_file_skips_the_session(tmp_path):
    root = tmp_path / "sessions"
    sess = synth.make_battery(str(root), genuine=1, per_species=0, seed=5)[0]
    (root / "genuine-001" / sess.clips[0].file).unlink()
    db_path = str(tmp_path / "results.db")
    with ResultsDB(db_path) as db:
        summary = run(RunConfig(sessions_root=str(root), db_path=db_path), StubScorer({}), db)
    # the manifest now points at a missing file: caught at load time, so the
    # session is skipped rather than scored
    assert summary.presentations == 0
    assert len(summary.manifest_failures) == 1


def test_runner_limit_stops_early(battery, tmp_path):
    db_path = str(tmp_path / "results.db")
    with ResultsDB(db_path) as db:
        summary = run(
            RunConfig(sessions_root=battery, db_path=db_path, limit=3),
            StubScorer({"_default": 0.2}),
            db,
        )
    assert summary.presentations == 3


def test_run_ids_are_unique():
    assert new_run_id() != new_run_id()
    assert new_run_id("smoke").startswith("smoke-")


# ─── real engine, in-process ─────────────────────────────────────────────────


def test_inprocess_scorer_runs_the_real_engine(battery, tmp_path):
    """No ONNX model in CI -> the deterministic fallback engine, capped at
    0.5 and always UNKNOWN (engine/liveness.py). Synthetic frames contain no
    face, so every presentation scores 0.0 and no attack is accepted."""
    scorer = InProcessScorer()
    assert scorer.model_version() == "fallback@none"

    db_path = str(tmp_path / "results.db")
    with ResultsDB(db_path) as db:
        summary = run(
            RunConfig(sessions_root=battery, db_path=db_path, threshold=0.5),
            scorer,
            db,
        )
        rows = db.presentations(summary.run_id)
        presentations = db.scored_presentations(summary.run_id)

    assert summary.errors == 0
    assert len(rows) == 8
    assert all(r.model == "fallback" for r in rows)
    assert all(0.0 <= r.live_score <= 0.5 for r in rows)
    assert all(r.label == "UNKNOWN" for r in rows)
    # the fusion breakdown from engine/liveness_fusion.py rides along
    fusion = json.loads(rows[0].fusion_json)
    assert {"score", "padMedian", "padMin", "moire", "frames"} <= set(fusion)
    assert fusion["frames"] == 5

    m = M.compute(presentations, 0.5)
    assert m.attacks_accepted == 0
    assert m.attack_count == 6
    assert m.genuine_count == 2
