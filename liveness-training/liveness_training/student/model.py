"""The deployable student: MobileNetV3-Small on 80x80 face crops.

Deployment-shape invariants (liveness_training.deployment):

* ``forward`` takes **BGR float32 raw 0..255** NCHW — exactly the blob
  serving builds — and normalizes INSIDE the graph via constant buffers, so
  the exported ONNX needs zero preprocessing beyond what
  ekyc-ml-service/engine/liveness.py already does for MiniFASNetV2.
* Two logits, index 0 = spoof, index 1 = live (serving reads probs[1]).
* :class:`ExportStudent` appends an in-graph Softmax for export, so
  serving's "already ends in softmax" detection short-circuits its own
  re-normalization.
"""
from __future__ import annotations

import torch
import torch.nn as nn

from liveness_training.deployment import INPUT_SIZE, NUM_CLASSES


class StudentPAD(nn.Module):
    input_size = INPUT_SIZE  # 80

    def __init__(self, width_mult: float = 1.0, dropout: float = 0.2) -> None:
        super().__init__()
        from torchvision.models import mobilenet_v3_small

        # never download weights implicitly: the student is always trained
        # from scratch or distilled — determinism over ImageNet priors
        net = mobilenet_v3_small(weights=None, width_mult=width_mult, dropout=dropout)
        in_features = net.classifier[-1].in_features
        net.classifier[-1] = nn.Linear(in_features, NUM_CLASSES)
        self.net = net
        # raw-range normalization baked into the graph: (x - 127.5) / 128
        self.register_buffer("input_mean", torch.full((1, 3, 1, 1), 127.5))
        self.register_buffer("input_scale", torch.full((1, 3, 1, 1), 128.0))

    def forward(self, x_bgr_raw: torch.Tensor) -> torch.Tensor:
        x = (x_bgr_raw - self.input_mean) / self.input_scale
        return self.net(x)  # logits (B, 2)


class ExportStudent(nn.Module):
    """Export wrapper: logits -> softmax probabilities in-graph."""

    def __init__(self, student: StudentPAD) -> None:
        super().__init__()
        self.student = student

    def forward(self, x_bgr_raw: torch.Tensor) -> torch.Tensor:
        return torch.softmax(self.student(x_bgr_raw), dim=1)


def save_student(model: StudentPAD, path, extra: dict | None = None) -> None:
    torch.save({"model_state": model.state_dict(), **(extra or {})}, path)


def load_student(path, map_location="cpu") -> StudentPAD:
    ckpt = torch.load(path, map_location=map_location, weights_only=False)
    model = StudentPAD()
    model.load_state_dict(ckpt["model_state"])
    model.eval()
    return model
