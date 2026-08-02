"""`run --mlflow` publication — mlflow faked, no live server anywhere."""
from __future__ import annotations

import os
import sys
import types

import pytest

from liveness_redteam import cli, report, synth, tracking
from liveness_redteam.runner import RunConfig, run
from liveness_redteam.scorers import ScoreResult, Scorer
from liveness_redteam.storage import ResultsDB


class StubScorer(Scorer):
    """Deterministic scorer (mirrors test_runner): everything scores 0.2, so
    genuine sessions are rejected and every attack is blocked."""

    name = "stub"

    def model_version(self) -> str:
        return "minifasnet_v2@sha256:feedbeefcafe0123"

    def target(self) -> str:
        return "stub://local"

    def score(self, frames, challenge_passed=None) -> ScoreResult:
        return ScoreResult(
            live_score=0.2,
            label="SPOOF",
            model="stub",
            per_frame=[{"score": 0.2, "faceFound": True}] * len(frames),
            fusion={"score": 0.2, "padMedian": 0.2, "frames": len(frames)},
        )


class FakeMlflow(types.ModuleType):
    """Recording stand-in for the mlflow fluent API used by tracking.py."""

    def __init__(self):
        super().__init__("mlflow")
        self.experiments = []
        self.run_names = []
        self.params = {}
        self.metrics = {}
        self.tags = {}
        self.artifacts = []
        self.runs_ended = 0

    def set_experiment(self, name):
        self.experiments.append(name)

    def start_run(self, run_name=None):
        self.run_names.append(run_name)
        fake = self

        class _ActiveRun:
            info = types.SimpleNamespace(run_id="fake-run-id")

            def __enter__(self):
                return self

            def __exit__(self, *exc):
                fake.runs_ended += 1
                return False

        return _ActiveRun()

    def log_params(self, params):
        self.params.update(params)

    def log_metrics(self, metrics, step=None):
        self.metrics.update(metrics)

    def set_tags(self, tags):
        self.tags.update(tags)

    def log_artifact(self, path):
        # capture existence at call time — the temp dir is gone afterwards
        assert os.path.isfile(path)
        self.artifacts.append(os.path.basename(path))


@pytest.fixture()
def fake_mlflow(monkeypatch):
    fake = FakeMlflow()
    monkeypatch.setitem(sys.modules, "mlflow", fake)
    monkeypatch.setenv("MLFLOW_TRACKING_URI", "http://fake:28115")
    return fake


@pytest.fixture()
def stored_run(tmp_path):
    """A small scored battery in a tmp results.db -> (db_path, run_id)."""
    sessions = tmp_path / "sessions"
    synth.make_battery(str(sessions), genuine=2, per_species=1, seed=1)
    db_path = str(tmp_path / "results.db")
    config = RunConfig(sessions_root=str(sessions), db_path=db_path)
    with ResultsDB(db_path) as db:
        summary = run(config, StubScorer(), db=db, progress=lambda _m: None)
    return db_path, summary.run_id


# ─── availability gate ───────────────────────────────────────────────────────


def test_publish_requires_tracking_uri(monkeypatch, stored_run):
    monkeypatch.delenv("MLFLOW_TRACKING_URI", raising=False)
    db_path, run_id = stored_run
    with ResultsDB(db_path) as db:
        view = report.load_run_view(db, run_id)
    with pytest.raises(tracking.TrackingUnavailable, match="MLFLOW_TRACKING_URI"):
        tracking.publish_run(view, "# report")


def test_publish_requires_mlflow_package(monkeypatch, stored_run):
    monkeypatch.setenv("MLFLOW_TRACKING_URI", "http://fake:28115")
    # a None sys.modules entry makes `import mlflow` raise ImportError
    monkeypatch.setitem(sys.modules, "mlflow", None)
    db_path, run_id = stored_run
    with ResultsDB(db_path) as db:
        view = report.load_run_view(db, run_id)
    with pytest.raises(tracking.TrackingUnavailable, match="mlflow package"):
        tracking.publish_run(view, "# report")


# ─── publication payload ─────────────────────────────────────────────────────


def test_publish_run_logs_params_metrics_tags_artifact(fake_mlflow, stored_run):
    db_path, run_id = stored_run
    with ResultsDB(db_path) as db:
        view = report.load_run_view(db, run_id)
        text = report.build_report(db, run_id)

    mlflow_run_id = tracking.publish_run(view, text)

    assert mlflow_run_id == "fake-run-id"
    assert fake_mlflow.experiments == [tracking.EXPERIMENT]
    assert fake_mlflow.run_names == [run_id]
    assert fake_mlflow.runs_ended == 1
    # params: model version/checksum, threshold, frames
    assert fake_mlflow.params["model_version"] == "minifasnet_v2@sha256:feedbeefcafe0123"
    assert fake_mlflow.params["threshold"] == view.run.threshold
    assert fake_mlflow.params["frames"] == view.run.frame_count
    # metrics: apcer_<species> for every species, plus the summary set
    m = view.metrics()
    for species in m.by_species:
        assert fake_mlflow.metrics[f"apcer_{species}"] == m.by_species[species].apcer
    assert fake_mlflow.metrics["apcer_worst_species"] == m.apcer
    assert fake_mlflow.metrics["bpcer"] == m.bpcer
    assert fake_mlflow.metrics["acer"] == m.acer
    assert "bpcer_at_apcer_1pct" in fake_mlflow.metrics
    # tags: gate verdicts as pass/fail
    assert fake_mlflow.tags["l1_gate"] in ("pass", "fail")
    assert fake_mlflow.tags["l2_gate"] in ("pass", "fail")
    # artifact: the markdown report, named after the rig run id
    assert fake_mlflow.artifacts == [f"{run_id}.md"]


# ─── CLI wiring ──────────────────────────────────────────────────────────────


def _cli_run(tmp_path, extra_args=()):
    sessions = tmp_path / "sessions"
    synth.make_battery(str(sessions), genuine=2, per_species=1, seed=1)
    db_path = str(tmp_path / "results.db")
    return cli.main(
        ["run", str(sessions), "--db", db_path, "--quiet", *extra_args]
    )


def test_cli_run_mlflow_flag_publishes(monkeypatch, tmp_path, fake_mlflow):
    monkeypatch.setattr(cli, "build_scorer", lambda *a, **k: StubScorer())
    assert _cli_run(tmp_path, ["--mlflow"]) == cli.EXIT_OK
    assert fake_mlflow.experiments == [tracking.EXPERIMENT]
    assert fake_mlflow.runs_ended == 1


def test_cli_run_mlflow_flag_errors_cleanly_without_uri(monkeypatch, tmp_path):
    monkeypatch.setattr(cli, "build_scorer", lambda *a, **k: StubScorer())
    monkeypatch.delenv("MLFLOW_TRACKING_URI", raising=False)
    assert _cli_run(tmp_path, ["--mlflow"]) == cli.EXIT_ERROR


def test_cli_run_without_flag_never_imports_mlflow(monkeypatch, tmp_path):
    monkeypatch.setattr(cli, "build_scorer", lambda *a, **k: StubScorer())
    monkeypatch.setenv("MLFLOW_TRACKING_URI", "http://fake:28115")
    # if anything tried to import mlflow, this sentinel would blow up
    monkeypatch.setitem(sys.modules, "mlflow", None)
    assert _cli_run(tmp_path) == cli.EXIT_OK