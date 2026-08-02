"""Optional MLflow publication of a red-team run (the ``run --mlflow`` flag).

Publishes one stored run to the platform's nemo-mlflow tracking server,
experiment ``liveness-redteam``: params (model version/checksum, threshold,
frames), metrics (per-species APCER as ``apcer_<species>``, worst-species
APCER, BPCER, ACER, BPCER@APCER<=1%), gate verdict tags and the markdown
report as the run artifact.

``mlflow`` is deliberately NOT a dependency of this rig (requirements.txt —
it must stay runnable on the slim box). Publication requires BOTH the
``MLFLOW_TRACKING_URI`` environment variable and an importable ``mlflow``;
anything else raises :class:`TrackingUnavailable`, which the CLI reports as
a plain error — scoring results are already safe in results.db either way.
"""
from __future__ import annotations

import os
import tempfile

from . import metrics as M

EXPERIMENT = "liveness-redteam"


class TrackingUnavailable(RuntimeError):
    """--mlflow was requested but tracking cannot run."""


def _require_mlflow():
    if not os.environ.get("MLFLOW_TRACKING_URI"):
        raise TrackingUnavailable(
            "--mlflow requires the MLFLOW_TRACKING_URI environment variable "
            "(e.g. http://localhost:28115 over the SSH tunnel — see README)"
        )
    try:
        import mlflow
    except ImportError:
        raise TrackingUnavailable(
            "--mlflow requires the mlflow package (pip install mlflow); "
            "it is intentionally not part of this rig's requirements"
        ) from None
    return mlflow


def _gate_tag(gate: M.GateResult) -> str:
    return "pass" if gate.passed else "fail"


def publish_run(view, report_text: str) -> str:
    """Publish one run (a ``report.RunView``) to MLflow; returns the MLflow
    run id. Raises TrackingUnavailable when tracking cannot run."""
    mlflow = _require_mlflow()

    run = view.run
    m = view.metrics()
    l1 = M.l1_gate(view.presentations, run.threshold)
    l2 = M.l2_gate(view.presentations, run.threshold)
    bpcer_at_1 = M.bpcer_at_apcer(view.presentations, M.L2_APCER_MAX)

    metric_values = {
        "apcer_worst_species": m.apcer,
        "apcer_pooled": m.apcer_pooled,
        "bpcer": m.bpcer,
        "acer": m.acer,
        "attack_count": float(m.attack_count),
        "genuine_count": float(m.genuine_count),
        "attacks_accepted": float(m.attacks_accepted),
        "genuine_rejected": float(m.genuine_rejected),
    }
    for species, sm in m.by_species.items():
        metric_values[f"apcer_{species}"] = sm.apcer
    if bpcer_at_1 is not None:  # None = unattainable in this score range
        metric_values["bpcer_at_apcer_1pct"] = bpcer_at_1

    mlflow.set_experiment(EXPERIMENT)
    with mlflow.start_run(run_name=run.run_id) as active:
        mlflow.log_params({
            # model_version is <engine>@<checksum> (scorers.py) — the
            # version AND checksum of what was attacked, in one param.
            "model_version": run.model_version,
            "threshold": run.threshold,
            "frames": run.frame_count,
            "scorer": run.scorer,
            "target": run.target,
            "sessions_root": run.sessions_root,
        })
        mlflow.log_metrics(metric_values)
        mlflow.set_tags({
            "l1_gate": _gate_tag(l1),
            "l2_gate": _gate_tag(l2),
            "worst_species": m.worst_species or "",
            "rig_run_id": run.run_id,
        })
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, f"{run.run_id}.md")
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(report_text)
            mlflow.log_artifact(path)
        return active.info.run_id
