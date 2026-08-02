"""Checksum manifest for exported models — matches the repo's checksummed
model-provisioning convention (ekyc-ml-service/Dockerfile pins a SHA-256 ARG
per model and refuses to ship an unverified file; commit 4f95ab4).

The manifest carries everything the serving side needs to provision the
student the same way MiniFASNetV2 is provisioned today: the sha256 to paste
into a ``*_SHA256`` Dockerfile ARG (or to verify a models-PVC upload), plus
the deployment-contract facts a reviewer should re-check on swap.
"""
from __future__ import annotations

import datetime as _dt
import hashlib
import json
import subprocess
from pathlib import Path

from liveness_training.deployment import (
    CHANNEL_ORDER,
    INPUT_SIZE,
    LIVE_CLASS_INDEX,
    NUM_CLASSES,
    PIXEL_RANGE,
)


def sha256_of(path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def _git_commit() -> str | None:
    try:
        return subprocess.run(
            ["git", "rev-parse", "HEAD"], capture_output=True, text=True,
            timeout=10, check=True,
        ).stdout.strip()
    except Exception:
        return None


def write_model_manifest(onnx_path, manifest_path, extra: dict | None = None) -> dict:
    onnx_path = Path(onnx_path)
    digest = sha256_of(onnx_path)
    manifest = {
        "schemaVersion": 1,
        "createdAt": _dt.datetime.now(_dt.timezone.utc).isoformat(timespec="seconds"),
        "gitCommit": _git_commit(),
        "files": [
            {
                "file": onnx_path.name,
                "sha256": digest,
                "sizeBytes": onnx_path.stat().st_size,
            }
        ],
        "deploymentContract": {
            "runtime": "cv2.dnn.readNetFromONNX",
            "inputShape": [1, 3, INPUT_SIZE, INPUT_SIZE],
            "channelOrder": CHANNEL_ORDER,
            "pixelRange": list(PIXEL_RANGE),
            "normalization": "in-graph ((x-127.5)/128); feed raw 0..255",
            "output": f"softmax probabilities, {NUM_CLASSES} classes",
            "liveClassIndex": LIVE_CLASS_INDEX,
            "servingEnvVar": "FACE_LIVENESS_MODEL",
        },
        "provisioning": {
            "dockerfileArgExample": (
                f"ARG STUDENT_PAD_SHA256={digest}  "
                "# pin like MINIFASNET_SHA256 in ekyc-ml-service/Dockerfile"
            ),
            "verify": f"echo '{digest}  {onnx_path.name}' | sha256sum -c -",
        },
        **(extra or {}),
    }
    p = Path(manifest_path)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    return manifest


def verify_manifest(manifest_path) -> bool:
    """Re-hash every file listed; True iff all match (files resolved
    relative to the manifest's directory)."""
    p = Path(manifest_path)
    manifest = json.loads(p.read_text(encoding="utf-8"))
    for entry in manifest.get("files", []):
        f = p.parent / entry["file"]
        if not f.is_file() or sha256_of(f) != entry["sha256"]:
            return False
    return True
