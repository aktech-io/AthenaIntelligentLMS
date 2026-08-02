"""Optional-MLflow tracking: no-op without MLFLOW_TRACKING_URI/mlflow, full
param/metric/artifact logging with them (mlflow faked — no live server)."""
import os
import sys
import types
from pathlib import Path

import pytest

from liveness_training.tracking import (
    EXPERIMENT,
    MlflowTracker,
    NullTracker,
    flatten_config,
    track_run,
)

TEACHER_CFG = {
    "seed": 7,
    "augment": False,
    "teacher": {
        "backbone": "torchvision:mobilenet_v3_small",
        "pretrained": False,
        "freeze": "full",
        "input_size": 96,
    },
    "data": {
        "source": "synthetic",
        "n_subjects": 8,
        "samples_per_subject": 2,
        "frames_per_sample": 1,
        "frame_size": 96,
    },
    "optim": {
        "epochs": 1, "batch_size": 16, "grad_accum": 1,
        "lr": 2.0e-3, "amp": False, "num_workers": 0,
    },
}

DISTILL_CFG = {
    "seed": 7,
    "augment": False,
    "student": {"width_mult": 1.0},
    "distill": {"temperature": 4.0, "alpha": 0.7},
    "data": dict(TEACHER_CFG["data"]),
    "optim": dict(TEACHER_CFG["optim"]),
}


class FakeMlflow(types.ModuleType):
    """Recording stand-in for the mlflow fluent API used by tracking.py."""

    def __init__(self):
        super().__init__("mlflow")
        self.experiments = []
        self.run_names = []
        self.params = {}
        self.metrics = []  # list of (dict, step)
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
        self.metrics.append((dict(metrics), step))

    def log_artifact(self, path):
        self.artifacts.append(os.path.basename(path))

    def set_tags(self, tags):
        self.tags.update(tags)

    def all_metric_keys(self):
        return {k for batch, _step in self.metrics for k in batch}


@pytest.fixture()
def fake_mlflow(monkeypatch):
    fake = FakeMlflow()
    monkeypatch.setitem(sys.modules, "mlflow", fake)
    monkeypatch.setenv("MLFLOW_TRACKING_URI", "http://fake:28115")
    return fake


# ─── unit: helpers and activation gate ───────────────────────────────────────


def test_flatten_config_nests_with_dots():
    flat = flatten_config({"a": 1, "b": {"c": "x", "d": {"e": 2.5}}})
    assert flat == {"a": 1, "b.c": "x", "b.d.e": 2.5}


def test_track_run_is_noop_without_uri(monkeypatch):
    monkeypatch.delenv("MLFLOW_TRACKING_URI", raising=False)
    with track_run("anything") as track:
        assert isinstance(track, NullTracker)
        assert not track.active
        track.log_params({"k": "v"})  # all no-ops, nothing to assert but no crash
        track.log_metrics({"m": 1.0})


def test_track_run_is_noop_when_mlflow_missing(monkeypatch):
    monkeypatch.setenv("MLFLOW_TRACKING_URI", "http://fake:28115")
    # a None sys.modules entry makes `import mlflow` raise ImportError
    monkeypatch.setitem(sys.modules, "mlflow", None)
    with track_run("anything") as track:
        assert isinstance(track, NullTracker)


def test_track_run_active_sets_experiment_and_run_name(fake_mlflow):
    with track_run("run-7") as track:
        assert isinstance(track, MlflowTracker)
        assert track.active
    assert fake_mlflow.experiments == [EXPERIMENT]
    assert fake_mlflow.run_names == ["run-7"]
    assert fake_mlflow.runs_ended == 1


def test_tracker_drops_nan_and_non_numeric_metrics(fake_mlflow):
    with track_run("run-nan") as track:
        track.log_metrics(
            {"ok": 0.5, "bad": float("nan"), "text": "no", "flag": True,
             "nested": {"deep": 2.0, "worse": float("nan")}},
            step=3,
        )
    assert fake_mlflow.metrics == [({"ok": 0.5, "nested.deep": 2.0}, 3)]


# ─── integration: teacher / distill log through the fake module ─────────────


@pytest.fixture(scope="module")
def teacher_checkpoint(tmp_path_factory):
    """One quiet (untracked) teacher run shared by the tests below."""
    from liveness_training.teacher.train import train_teacher

    out = tmp_path_factory.mktemp("teacher-base")
    result = train_teacher(TEACHER_CFG, out, device="cpu")
    return result["checkpoint"]


def test_teacher_untracked_never_touches_mlflow(monkeypatch, tmp_path):
    """URI unset -> zero mlflow traffic even with the package importable."""
    from liveness_training.teacher.train import train_teacher

    fake = FakeMlflow()
    monkeypatch.setitem(sys.modules, "mlflow", fake)
    monkeypatch.delenv("MLFLOW_TRACKING_URI", raising=False)
    train_teacher(TEACHER_CFG, tmp_path / "t", device="cpu")
    assert fake.experiments == []
    assert fake.metrics == []


def test_teacher_tracked_logs_params_metrics_artifacts(fake_mlflow, tmp_path):
    from liveness_training.teacher.train import train_teacher

    train_teacher(TEACHER_CFG, tmp_path / "teacher-run", device="cpu")

    assert fake_mlflow.experiments == [EXPERIMENT]
    assert fake_mlflow.run_names == ["teacher-run"]  # out-dir basename
    # flattened config as params (stringified)
    assert fake_mlflow.params["optim.epochs"] == "1"
    assert fake_mlflow.params["teacher.backbone"] == "torchvision:mobilenet_v3_small"
    assert fake_mlflow.params["device"] == "cpu"
    # per-epoch series uses the history's exact metric names at step=epoch
    epoch0 = next(batch for batch, step in fake_mlflow.metrics if step == 0)
    assert {"train_loss", "val_acer", "val_apcer_max", "val_bpcer"} <= set(epoch0)
    # final metric set, flattened
    keys = fake_mlflow.all_metric_keys()
    assert {"final_apcer_max", "final_bpcer", "final_bpcer_at_target_apcer"} <= keys
    assert "teacher_history.json" in fake_mlflow.artifacts


def test_distill_tracked_logs_onnx_and_manifest(fake_mlflow, tmp_path,
                                                teacher_checkpoint):
    from liveness_training.student.distill import distill_student

    out = tmp_path / "student-run"
    distill_student(DISTILL_CFG, teacher_checkpoint, out, device="cpu")

    assert fake_mlflow.experiments == [EXPERIMENT]
    assert fake_mlflow.run_names == ["student-run"]
    assert fake_mlflow.params["distill.alpha"] == "0.7"
    epoch0 = next(batch for batch, step in fake_mlflow.metrics if step == 0)
    assert {"train_loss", "val_acer", "val_apcer_max", "val_bpcer"} <= set(epoch0)
    # tracked runs attach the serving-shape export + checksum manifest
    assert {"student_history.json", "student_best.onnx", "manifest.json"} <= set(
        fake_mlflow.artifacts
    )
    assert fake_mlflow.tags["onnx_sha256"]
    assert (out / "student_best.onnx").is_file()
    assert (out / "manifest.json").is_file()


def test_distill_untracked_exports_nothing_extra(monkeypatch, tmp_path,
                                                 teacher_checkpoint):
    from liveness_training.student.distill import distill_student

    monkeypatch.delenv("MLFLOW_TRACKING_URI", raising=False)
    out = tmp_path / "student-plain"
    result = distill_student(DISTILL_CFG, teacher_checkpoint, out, device="cpu")
    assert Path(result["checkpoint"]).is_file()
    # zero behaviour change: no ONNX/manifest side products without tracking
    assert not (out / "student_best.onnx").exists()
    assert not (out / "manifest.json").exists()
