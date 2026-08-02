"""Optional MLflow run tracking for the training pipeline.

Tracking activates only when BOTH hold:

* the ``MLFLOW_TRACKING_URI`` environment variable is set, and
* the ``mlflow`` package is importable (it is deliberately NOT a
  requirement — see requirements.txt).

Otherwise every call is a silent no-op, so a box without mlflow (or a run
that doesn't want an audit trail) behaves exactly as before. Runs land in
the ``liveness-training`` experiment on the platform's nemo-mlflow server —
see the README "MLflow tracking" section for the tunnel incantation.
"""
from __future__ import annotations

import os
from contextlib import contextmanager
from numbers import Number
from pathlib import Path

EXPERIMENT = "liveness-training"


def flatten_config(cfg: dict, prefix: str = "") -> dict:
    """Nested config dict -> flat {"optim.lr": 0.0001, ...} for log_params."""
    out: dict = {}
    for key, value in cfg.items():
        name = f"{prefix}{key}"
        if isinstance(value, dict):
            out.update(flatten_config(value, prefix=f"{name}."))
        else:
            out[name] = value
    return out


def _numeric_only(metrics: dict, prefix: str = "") -> dict:
    """Flatten + keep finite numbers — mlflow metrics must be floats, and
    NaN (absent species / unattainable operating points) is dropped rather
    than logged as a bogus value."""
    out: dict = {}
    for key, value in metrics.items():
        name = f"{prefix}{key}"
        if isinstance(value, dict):
            out.update(_numeric_only(value, prefix=f"{name}."))
        elif isinstance(value, Number) and not isinstance(value, bool):
            v = float(value)
            if v == v and v not in (float("inf"), float("-inf")):
                out[name] = v
    return out


def _mlflow_or_none():
    if not os.environ.get("MLFLOW_TRACKING_URI"):
        return None
    try:
        import mlflow
    except ImportError:
        return None
    return mlflow


class NullTracker:
    """No-op stand-in when tracking is inactive — same surface, zero effect."""

    active = False

    def log_params(self, params: dict) -> None:
        pass

    def log_metrics(self, metrics: dict, step: int | None = None) -> None:
        pass

    def log_artifact(self, path) -> None:
        pass

    def set_tags(self, tags: dict) -> None:
        pass


class MlflowTracker:
    active = True

    def __init__(self, mlflow_module):
        self._mlflow = mlflow_module

    def log_params(self, params: dict) -> None:
        self._mlflow.log_params({k: str(v) for k, v in params.items()})

    def log_metrics(self, metrics: dict, step: int | None = None) -> None:
        cleaned = _numeric_only(metrics)
        if cleaned:
            self._mlflow.log_metrics(cleaned, step=step)

    def log_artifact(self, path) -> None:
        p = Path(path)
        if p.is_file():
            self._mlflow.log_artifact(str(p))

    def set_tags(self, tags: dict) -> None:
        self._mlflow.set_tags({k: str(v) for k, v in tags.items()})


@contextmanager
def track_run(run_name: str, experiment: str = EXPERIMENT):
    """Yield a tracker for one training run.

    Active (real MLflow run, ended FAILED on exception) only when
    MLFLOW_TRACKING_URI is set and mlflow imports; a NullTracker otherwise.
    """
    mlflow = _mlflow_or_none()
    if mlflow is None:
        yield NullTracker()
        return
    mlflow.set_experiment(experiment)
    with mlflow.start_run(run_name=run_name):
        yield MlflowTracker(mlflow)
