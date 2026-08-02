"""Export the student to ONNX in the exact serving shape.

Graph contract (deployment.py): input ``(1, 3, 80, 80)`` float32, BGR raw
0..255; output ``(1, 2)`` softmax probabilities, index 1 = live. Opset 13 by
default — old enough that Hardswish/Hardsigmoid are emitted decomposed,
which is the safest dialect for cv2.dnn's ONNX importer; the parity test is
the actual gate, not the opset number. The legacy (non-dynamo) exporter is
forced for the same reason.

Usage::

    python -m liveness_training.export.onnx_export \
        --student runs/student/student_best.pt --out runs/export
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path

import torch

from liveness_training.deployment import INPUT_SIZE
from liveness_training.student.model import ExportStudent, StudentPAD, load_student

DEFAULT_OPSET = 13


def export_student_onnx(
    student: StudentPAD,
    out_path,
    opset: int = DEFAULT_OPSET,
    fixed_batch: bool = True,
) -> Path:
    """fixed_batch=True exports the literal (1,3,80,80) serving shape —
    cv2.dnn neither needs nor always tolerates dynamic axes."""
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    model = ExportStudent(student).eval()
    dummy = torch.zeros(1, 3, INPUT_SIZE, INPUT_SIZE, dtype=torch.float32)
    kwargs = dict(
        input_names=["input"],
        output_names=["probabilities"],
        opset_version=opset,
        do_constant_folding=True,
    )
    if not fixed_batch:
        kwargs["dynamic_axes"] = {"input": {0: "batch"}, "probabilities": {0: "batch"}}
    try:
        torch.onnx.export(model, dummy, str(out_path), dynamo=False, **kwargs)
    except TypeError:  # torch < 2.5: no dynamo kwarg, legacy is the default
        torch.onnx.export(model, dummy, str(out_path), **kwargs)

    import onnx

    onnx.checker.check_model(onnx.load(str(out_path)))
    return out_path


def main(argv=None) -> None:
    from liveness_training.export.manifest import write_model_manifest
    from liveness_training.export.parity import check_cv2_dnn_parity

    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--student", required=True, help="student checkpoint (.pt)")
    ap.add_argument("--out", default="runs/export")
    ap.add_argument("--name", default="nemo_pad_student")
    ap.add_argument("--opset", type=int, default=DEFAULT_OPSET)
    ap.add_argument("--skip-parity", action="store_true")
    args = ap.parse_args(argv)

    student = load_student(args.student)
    out_dir = Path(args.out)
    onnx_path = export_student_onnx(student, out_dir / f"{args.name}.onnx", args.opset)
    result = {"onnx": str(onnx_path)}
    if not args.skip_parity:
        parity = check_cv2_dnn_parity(student, onnx_path)
        result["parity_max_abs_diff"] = parity["max_abs_diff"]
    manifest = write_model_manifest(
        onnx_path, out_dir / "manifest.json",
        extra={"studentCheckpoint": str(args.student), "opset": args.opset},
    )
    result["manifest"] = str(out_dir / "manifest.json")
    result["sha256"] = manifest["files"][0]["sha256"]
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
