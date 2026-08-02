"""End-to-end smoke: synthetic data -> teacher train -> distill -> ONNX
export -> cv2.dnn parity -> metrics + markdown report + checksum manifest.

A few dozen samples, 2 epochs each, CPU-only — pipeline mechanics, not model
quality. The parity check IS a quality gate though: the exported graph must
match PyTorch through OpenCV's dnn to <= 1e-4 absolute.
"""
import json
from pathlib import Path

import pytest

CONFIG_DIR = Path(__file__).resolve().parents[1] / "configs"


@pytest.fixture(scope="module")
def out_dir(tmp_path_factory):
    return tmp_path_factory.mktemp("e2e")


@pytest.fixture(scope="module")
def teacher_result(out_dir):
    from liveness_training.common import load_config
    from liveness_training.teacher.train import train_teacher

    cfg = load_config(CONFIG_DIR / "smoke_teacher.yaml")
    with pytest.warns(RuntimeWarning) if _no_open_clip_and_clip_cfg(cfg) else _null_ctx():
        return train_teacher(cfg, out_dir / "teacher", device="cpu")


def _no_open_clip_and_clip_cfg(cfg):
    from liveness_training.teacher.model import open_clip_available

    return cfg["teacher"]["backbone"].startswith("clip:") and not open_clip_available()


def _null_ctx():
    import contextlib

    return contextlib.nullcontext()


@pytest.fixture(scope="module")
def student_result(out_dir, teacher_result):
    from liveness_training.common import load_config
    from liveness_training.student.distill import distill_student

    cfg = load_config(CONFIG_DIR / "smoke_distill.yaml")
    return distill_student(cfg, teacher_result["checkpoint"], out_dir / "student",
                           device="cpu")


def test_teacher_trains_and_checkpoints(teacher_result):
    assert Path(teacher_result["checkpoint"]).is_file()
    assert teacher_result["val"]["train_loss"] > 0


def test_teacher_fallback_degradation_smoke(out_dir):
    """clip: backbone with open_clip absent must degrade, warn, and still train."""
    from liveness_training.teacher.model import build_teacher, open_clip_available

    if open_clip_available():
        pytest.skip("open_clip installed — degradation path not exercised here")
    with pytest.warns(RuntimeWarning, match="degrading"):
        model = build_teacher({
            "backbone": "clip:ViT-B-16",
            "fallback_backbone": "mobilenet_v3_small",
            "pretrained": False,
            "freeze": "head_only",
            "input_size": 96,
        })
    assert model.backbone_kind == "torchvision:mobilenet_v3_small"
    # head_only really froze the tower
    assert all(not p.requires_grad for p in model.backbone.parameters())
    assert all(p.requires_grad for p in model.head.parameters())


def test_distillation_produces_student(student_result):
    assert Path(student_result["checkpoint"]).is_file()


def test_export_parity_and_manifest(out_dir, student_result):
    """The deployment-compatibility gate: cv2.dnn == PyTorch on the exported
    graph, plus the checksummed provisioning manifest."""
    from liveness_training.export.manifest import verify_manifest, write_model_manifest
    from liveness_training.export.onnx_export import export_student_onnx
    from liveness_training.export.parity import check_cv2_dnn_parity
    from liveness_training.student.model import load_student

    student = load_student(student_result["checkpoint"])
    onnx_path = export_student_onnx(student, out_dir / "export" / "nemo_pad_student.onnx")
    assert onnx_path.is_file() and onnx_path.stat().st_size > 10_000

    parity = check_cv2_dnn_parity(student, onnx_path, n_inputs=12, atol=1e-4)
    assert parity["max_abs_diff"] <= 1e-4

    manifest = write_model_manifest(onnx_path, out_dir / "export" / "manifest.json")
    assert manifest["deploymentContract"]["liveClassIndex"] == 1
    assert manifest["deploymentContract"]["inputShape"] == [1, 3, 80, 80]
    assert len(manifest["files"][0]["sha256"]) == 64
    assert verify_manifest(out_dir / "export" / "manifest.json")
    # tampering must fail verification
    onnx_path.write_bytes(onnx_path.read_bytes() + b"\x00")
    assert not verify_manifest(out_dir / "export" / "manifest.json")


def test_eval_report_from_student(out_dir, student_result):
    from liveness_training.common import collect_scores, load_config, make_loader
    from liveness_training.datasets.synthetic import SyntheticPadDataset
    from liveness_training.eval.metrics import compute_pad_metrics
    from liveness_training.eval.report import write_report
    from liveness_training.student.model import load_student

    student = load_student(student_result["checkpoint"])
    val = SyntheticPadDataset(n_subjects=14, samples_per_subject=3, split="val",
                              frames_per_sample=1, size=96)
    loader = make_loader(val, size=80, batch_size=16, shuffle=False)
    scored = collect_scores(student, loader)
    metrics = compute_pad_metrics(scored["scores"], scored["labels"],
                                  scored["attack_types"], scored["skin_tones"])
    path = write_report(metrics, out_dir / "report.md",
                        context={"model": "smoke student", "data": "synthetic val"})
    text = path.read_text()
    assert "BPCER" in text and "APCER per attack species" in text
    assert metrics["n_bona_fide"] > 0 and metrics["n_attack"] > 0
    # smoke-config loader check only — no accuracy bar on 2 CPU epochs

    # metrics JSON alongside, like real runs will keep
    (out_dir / "metrics.json").write_text(json.dumps(metrics, indent=2, default=float))


def test_nldea_end_to_end_trainable(nldea_root, out_dir):
    """The NLD-EA loader plugs straight into the training plumbing."""
    from liveness_training.common import make_loader
    from liveness_training.datasets.nldea import NLDEADataset

    loader = make_loader(NLDEADataset(nldea_root, shard="train", max_frames_per_clip=2),
                         size=80, batch_size=8, shuffle=True, frames_per_sample=2)
    batch = next(iter(loader))
    assert batch["image"].shape[1:] == (3, 80, 80)
    assert float(batch["image"].max()) > 1.5  # raw-range convention held
