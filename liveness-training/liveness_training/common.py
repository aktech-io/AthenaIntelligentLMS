"""Shared config/data/loop plumbing for teacher training and distillation."""
from __future__ import annotations

import random
from pathlib import Path
from typing import Optional

import numpy as np

from liveness_training.deployment import LIVE_CLASS_INDEX


def load_config(path) -> dict:
    import yaml

    with open(path, "r", encoding="utf-8") as f:
        cfg = yaml.safe_load(f)
    if not isinstance(cfg, dict):
        raise ValueError(f"{path}: config must be a YAML mapping")
    return cfg


def seed_all(seed: int) -> None:
    import torch

    random.seed(seed)
    np.random.seed(seed)
    torch.manual_seed(seed)


def pick_device(requested: str = "auto") -> str:
    import torch

    if requested != "auto":
        return requested
    return "cuda" if torch.cuda.is_available() else "cpu"


def build_dataset(data_cfg: dict, split: str):
    """data_cfg: {source: synthetic|nldea|celeba_spoof|cefa, root: ..., plus
    per-source options}. split: "train"|"val"."""
    source = data_cfg.get("source", "synthetic")
    if source == "synthetic":
        from liveness_training.datasets.synthetic import SyntheticPadDataset

        return SyntheticPadDataset(
            n_subjects=int(data_cfg.get("n_subjects", 20)),
            samples_per_subject=int(data_cfg.get("samples_per_subject", 4)),
            split=split,
            frames_per_sample=int(data_cfg.get("frames_per_sample", 2)),
            size=int(data_cfg.get("frame_size", 128)),
            seed=int(data_cfg.get("seed", 7)),
        )
    if source == "nldea":
        from liveness_training.datasets.nldea import NLDEADataset

        # NLD-EA has its own 70/15/15 subject sharding; train/val map directly.
        return NLDEADataset(
            data_cfg["root"], shard=split,
            max_frames_per_clip=int(data_cfg.get("max_frames_per_clip", 8)),
        )
    if source == "celeba_spoof":
        from liveness_training.datasets.celeba_spoof import CelebASpoofDataset

        return CelebASpoofDataset(
            data_cfg["root"], split=split,
            source_partition=data_cfg.get("source_partition", "train"),
            max_samples=data_cfg.get("max_samples"),
        )
    if source == "cefa":
        from liveness_training.datasets.cefa import CeFADataset

        eth = data_cfg.get("ethnicities")
        return CeFADataset(
            data_cfg["root"], split=split,
            ethnicities=tuple(eth) if eth else None,
            max_samples=data_cfg.get("max_samples"),
        )
    raise ValueError(f"unknown data source {source!r}")


def make_loader(
    pad_dataset,
    size: int,
    batch_size: int,
    shuffle: bool,
    transform=None,
    frames_per_sample: int = 1,
    num_workers: int = 0,
    seed: int = 0,
):
    import torch.utils.data as tud

    from liveness_training.datasets.base import FramePadDataset

    ds = FramePadDataset(
        pad_dataset, size=size, transform=transform,
        frames_per_sample=frames_per_sample, seed=seed,
    )
    return tud.DataLoader(ds, batch_size=batch_size, shuffle=shuffle,
                          num_workers=num_workers, drop_last=False)


def collect_scores(model, loader, device: str = "cpu") -> dict:
    """Run a model over a loader -> numpy arrays for the eval metrics:
    scores (P(live)), labels, attack_types, skin_tones, subject_ids."""
    import torch

    model.eval()
    scores, labels, attacks, tones, subjects = [], [], [], [], []
    with torch.no_grad():
        for batch in loader:
            x = batch["image"].to(device)
            logits = model(x)
            probs = torch.softmax(logits, dim=1)[:, LIVE_CLASS_INDEX]
            scores.extend(probs.cpu().numpy().tolist())
            labels.extend(int(v) for v in batch["label"])
            attacks.extend(batch["attack_type"])
            tones.extend(batch["skin_tone"])
            subjects.extend(batch["subject_id"])
    return {
        "scores": np.asarray(scores, dtype=np.float64),
        "labels": np.asarray(labels, dtype=np.int64),
        "attack_types": np.asarray(attacks, dtype=object),
        "skin_tones": np.asarray(tones, dtype=object),
        "subject_ids": np.asarray(subjects, dtype=object),
    }


def ensure_out_dir(path) -> Path:
    p = Path(path)
    p.mkdir(parents=True, exist_ok=True)
    return p
