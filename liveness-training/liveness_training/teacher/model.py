"""FLIP-style PAD teacher: an open-CLIP vision tower (frozen or partially
unfrozen) + a small binary head.

Why CLIP: FLIP (Srivatsan et al., ICCV'23) showed CLIP's vision tower carries
the best published cross-dataset PAD generalization of its generation — the
property we need to bridge public bootstrap data -> NLD-EA -> the
certification lab (docs/ekyc/06-level2-upgrade-plan.md §6.2).

Degradation contract: if ``open_clip`` is not importable the builder falls
back to a torchvision backbone (cfg ``fallback_backbone``) so the whole
pipeline — and the smoke suite — runs without it. The checkpoint records
which backbone actually ran.

Fine-tune policy (cfg ``freeze``):
  * ``"head_only"``  — tower fully frozen, train the head (default; fits
    anywhere, weakest adaptation).
  * ``"last_n:K"``   — additionally unfreeze the last K transformer blocks
    (CLIP) / last K children (torchvision). The 8GB-VRAM sweet spot.
  * ``"full"``       — everything trainable (needs more than 8GB for ViT-B
    at any useful batch size; not the default on this hardware).
LoRA is a deliberate non-goal for v1 — last_n partial unfreeze covers the
8GB budget with less machinery; see README "Deliberately stubbed".

Input convention: like every model in this pipeline the teacher's
``forward`` takes **BGR float raw 0..255** batches (the FramePadDataset
output) and does its own BGR->RGB flip + resize + CLIP/ImageNet
normalization internally, so teacher and student always see the same
upstream tensors.
"""
from __future__ import annotations

import warnings
from typing import Optional

import torch
import torch.nn as nn
import torch.nn.functional as F

_CLIP_MEAN = (0.48145466, 0.4578275, 0.40821073)
_CLIP_STD = (0.26862954, 0.26130258, 0.27577711)
_IMAGENET_MEAN = (0.485, 0.456, 0.406)
_IMAGENET_STD = (0.229, 0.224, 0.225)

TORCHVISION_FEATURE_DIMS = {
    "resnet18": 512,
    "resnet50": 2048,
    "mobilenet_v3_small": 576,
    "mobilenet_v3_large": 960,
}


def open_clip_available() -> bool:
    try:
        import open_clip  # noqa: F401

        return True
    except ImportError:
        return False


class TeacherPAD(nn.Module):
    def __init__(
        self,
        backbone: nn.Module,
        embed_dim: int,
        input_size: int,
        mean: tuple,
        std: tuple,
        backbone_kind: str,
        num_classes: int = 2,
    ) -> None:
        super().__init__()
        self.backbone = backbone
        self.head = nn.Sequential(
            nn.LayerNorm(embed_dim),
            nn.Linear(embed_dim, num_classes),
        )
        self.input_size = input_size
        self.backbone_kind = backbone_kind  # e.g. "open_clip:ViT-B-16" / "torchvision:resnet18"
        # normalization constants as buffers (move with .to(device))
        self.register_buffer("_mean", torch.tensor(mean).view(1, 3, 1, 1), persistent=False)
        self.register_buffer("_std", torch.tensor(std).view(1, 3, 1, 1), persistent=False)

    def preprocess(self, x_bgr_raw: torch.Tensor) -> torch.Tensor:
        """(B,3,H,W) BGR float 0..255 -> resized, RGB, normalized."""
        x = x_bgr_raw.flip(1)  # BGR -> RGB
        if x.shape[-1] != self.input_size or x.shape[-2] != self.input_size:
            x = F.interpolate(
                x, size=(self.input_size, self.input_size),
                mode="bilinear", align_corners=False, antialias=True,
            )
        x = x / 255.0
        return (x - self._mean) / self._std

    def forward(self, x_bgr_raw: torch.Tensor) -> torch.Tensor:
        feats = self.backbone(self.preprocess(x_bgr_raw))
        if feats.dim() > 2:
            feats = feats.flatten(1)
        return self.head(feats)  # logits, index 1 = live


def _apply_freeze(model: TeacherPAD, freeze: str) -> None:
    for p in model.backbone.parameters():
        p.requires_grad = False
    if freeze == "head_only":
        return
    if freeze == "full":
        for p in model.backbone.parameters():
            p.requires_grad = True
        return
    if freeze.startswith("last_n:"):
        k = int(freeze.split(":", 1)[1])
        blocks = _trailing_blocks(model.backbone, k)
        for b in blocks:
            for p in b.parameters():
                p.requires_grad = True
        return
    raise ValueError(f"unknown freeze policy {freeze!r} (head_only|last_n:K|full)")


def _trailing_blocks(backbone: nn.Module, k: int) -> list[nn.Module]:
    """Last-k unfreeze targets: CLIP ViT resblocks when present, else the
    trailing top-level children."""
    # open_clip VisionTransformer: .transformer.resblocks (ModuleList-ish)
    tr = getattr(getattr(backbone, "transformer", None), "resblocks", None)
    if tr is not None:
        blocks = list(tr)[-k:]
        # the final LN + projection should train along with the last blocks
        for name in ("ln_post",):
            if hasattr(backbone, name):
                blocks.append(getattr(backbone, name))
        return blocks
    children = [c for c in backbone.children() if sum(1 for _ in c.parameters())]
    return children[-k:]


def _build_open_clip(model_name: str, pretrained: str) -> tuple[nn.Module, int, int]:
    import open_clip

    clip_model, _, _ = open_clip.create_model_and_transforms(
        model_name, pretrained=pretrained or None
    )
    visual = clip_model.visual
    embed_dim = getattr(visual, "output_dim", None) or clip_model.text_projection.shape[1]
    input_size = visual.image_size[0] if isinstance(visual.image_size, (tuple, list)) \
        else int(visual.image_size)
    return visual, int(embed_dim), int(input_size)


def _build_torchvision(name: str, pretrained: bool) -> tuple[nn.Module, int, int]:
    import torchvision.models as tvm

    if name not in TORCHVISION_FEATURE_DIMS:
        raise ValueError(
            f"unsupported fallback backbone {name!r}; add it to TORCHVISION_FEATURE_DIMS"
        )
    weights = "DEFAULT" if pretrained else None
    net = getattr(tvm, name)(weights=weights)
    if name.startswith("resnet"):
        net.fc = nn.Identity()
    else:  # mobilenet_v3_*
        net.classifier = nn.Identity()
    return net, TORCHVISION_FEATURE_DIMS[name], 224


def build_teacher(cfg: dict) -> TeacherPAD:
    """cfg keys (teacher section of the yaml):
    backbone: "clip:ViT-B-16" | "torchvision:<name>"
    clip_pretrained: open_clip tag, e.g. "openai" ("" = random init)
    fallback_backbone: torchvision name used when open_clip is missing
    pretrained: bool (torchvision path)
    freeze: "head_only" | "last_n:K" | "full"
    input_size: optional override (torchvision path; smoke uses 96)
    """
    backbone_spec = cfg.get("backbone", "clip:ViT-B-16")
    freeze = cfg.get("freeze", "head_only")

    if backbone_spec.startswith("clip:"):
        clip_name = backbone_spec.split(":", 1)[1]
        if open_clip_available():
            visual, embed_dim, input_size = _build_open_clip(
                clip_name, cfg.get("clip_pretrained", "openai")
            )
            model = TeacherPAD(visual, embed_dim, input_size, _CLIP_MEAN, _CLIP_STD,
                               backbone_kind=f"open_clip:{clip_name}")
            _apply_freeze(model, freeze)
            return model
        fallback = cfg.get("fallback_backbone", "mobilenet_v3_small")
        warnings.warn(
            f"open_clip not installed — teacher degrading to torchvision "
            f"backbone {fallback!r}. Fine for smoke tests; install "
            "open-clip-torch for the real DG teacher.",
            RuntimeWarning,
        )
        backbone_spec = f"torchvision:{fallback}"

    if backbone_spec.startswith("torchvision:"):
        name = backbone_spec.split(":", 1)[1]
        net, embed_dim, input_size = _build_torchvision(name, bool(cfg.get("pretrained", False)))
        input_size = int(cfg.get("input_size", input_size))
        model = TeacherPAD(net, embed_dim, input_size, _IMAGENET_MEAN, _IMAGENET_STD,
                           backbone_kind=f"torchvision:{name}")
        _apply_freeze(model, freeze)
        return model

    raise ValueError(f"unknown backbone spec {backbone_spec!r}")


def save_teacher(model: TeacherPAD, cfg: dict, path, extra: Optional[dict] = None) -> None:
    torch.save(
        {
            "model_state": model.state_dict(),
            "config": cfg,
            "backbone_kind": model.backbone_kind,
            "input_size": model.input_size,
            **(extra or {}),
        },
        path,
    )


def load_teacher(path, map_location="cpu") -> TeacherPAD:
    ckpt = torch.load(path, map_location=map_location, weights_only=False)
    model = build_teacher(ckpt["config"])
    if model.backbone_kind != ckpt["backbone_kind"]:
        raise RuntimeError(
            f"checkpoint was trained with {ckpt['backbone_kind']} but this "
            f"environment builds {model.backbone_kind} (open_clip availability "
            "changed?) — refusing to load mismatched weights"
        )
    model.load_state_dict(ckpt["model_state"])
    model.eval()
    return model
