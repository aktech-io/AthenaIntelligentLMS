"""Teacher fine-tune loop (config-driven, fp16 + gradient accumulation).

Usage::

    python -m liveness_training.teacher.train --config configs/teacher_clip.yaml \
        [--out runs/teacher] [--device cuda|cpu|auto]

VRAM: sized for an 8GB RTX 4060 — see configs/teacher_clip.yaml for the
budget breakdown (fp16 autocast + micro-batch 16 + grad-accum 4 with
last_n:4 unfreeze keeps ViT-B-16 under ~6GB).
"""
from __future__ import annotations

import argparse
import json
import time
from pathlib import Path

import torch
import torch.nn as nn

from liveness_training.common import (
    build_dataset,
    collect_scores,
    ensure_out_dir,
    load_config,
    make_loader,
    pick_device,
    seed_all,
)
from liveness_training.datasets.transforms import default_train_transform
from liveness_training.eval.metrics import compute_pad_metrics
from liveness_training.teacher.model import build_teacher, save_teacher


def train_teacher(cfg: dict, out_dir, device: str = "auto") -> dict:
    """Returns {"checkpoint": path, "val": metrics dict}."""
    seed_all(int(cfg.get("seed", 42)))
    device = pick_device(device)
    out = ensure_out_dir(out_dir)

    tcfg = cfg["teacher"]
    dcfg = cfg["data"]
    ocfg = cfg.get("optim", {})

    model = build_teacher(tcfg).to(device)
    train_pad = build_dataset(dcfg, "train")
    val_pad = build_dataset(dcfg, "val")
    train_loader = make_loader(
        train_pad, size=model.input_size,
        batch_size=int(ocfg.get("batch_size", 16)), shuffle=True,
        transform=default_train_transform() if cfg.get("augment", True) else None,
        frames_per_sample=int(dcfg.get("frames_per_sample", 1)),
        num_workers=int(ocfg.get("num_workers", 0)),
    )
    val_loader = make_loader(
        val_pad, size=model.input_size,
        batch_size=int(ocfg.get("batch_size", 16)), shuffle=False,
        frames_per_sample=int(dcfg.get("frames_per_sample", 1)),
    )
    if len(train_loader.dataset) == 0 or len(val_loader.dataset) == 0:
        raise RuntimeError("empty train or val split — check the data config")

    params = [p for p in model.parameters() if p.requires_grad]
    optimizer = torch.optim.AdamW(
        params, lr=float(ocfg.get("lr", 1e-4)),
        weight_decay=float(ocfg.get("weight_decay", 0.01)),
    )
    epochs = int(ocfg.get("epochs", 3))
    accum = max(1, int(ocfg.get("grad_accum", 1)))
    use_amp = bool(ocfg.get("amp", True)) and device == "cuda"
    scaler = torch.amp.GradScaler("cuda", enabled=use_amp)
    criterion = nn.CrossEntropyLoss()
    scheduler = torch.optim.lr_scheduler.CosineAnnealingLR(
        optimizer, T_max=max(1, epochs * len(train_loader) // accum)
    )

    best_acer, best_path = float("inf"), out / "teacher_best.pt"
    history = []
    for epoch in range(epochs):
        model.train()
        t0, running, seen = time.time(), 0.0, 0
        optimizer.zero_grad(set_to_none=True)
        for step, batch in enumerate(train_loader):
            x = batch["image"].to(device, non_blocking=True)
            y = batch["label"].to(device, non_blocking=True)
            with torch.autocast("cuda", dtype=torch.float16, enabled=use_amp):
                loss = criterion(model(x), y) / accum
            scaler.scale(loss).backward()
            if (step + 1) % accum == 0 or (step + 1) == len(train_loader):
                scaler.step(optimizer)
                scaler.update()
                optimizer.zero_grad(set_to_none=True)
                scheduler.step()
            running += float(loss.detach()) * accum * x.size(0)
            seen += x.size(0)

        val = collect_scores(model, val_loader, device)
        metrics = compute_pad_metrics(
            val["scores"], val["labels"], val["attack_types"], val["skin_tones"]
        )
        history.append({
            "epoch": epoch,
            "train_loss": running / max(1, seen),
            "val_acer": metrics["acer"],
            "val_apcer_max": metrics["apcer_max"],
            "val_bpcer": metrics["bpcer"],
            "seconds": round(time.time() - t0, 1),
        })
        print(f"[teacher] epoch {epoch}: {history[-1]}")
        if metrics["acer"] <= best_acer:
            best_acer = metrics["acer"]
            save_teacher(model, tcfg, best_path, extra={"val_metrics": metrics})

    (out / "teacher_history.json").write_text(json.dumps(history, indent=2))
    if device == "cuda":
        peak = torch.cuda.max_memory_allocated() / 2**20
        print(f"[teacher] peak CUDA memory allocated: {peak:.0f} MiB")
        (out / "vram_peak_mib.txt").write_text(f"{peak:.0f}\n")
    return {"checkpoint": str(best_path), "val": history[-1], "history": history}


def main(argv=None) -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--config", required=True)
    ap.add_argument("--out", default="runs/teacher")
    ap.add_argument("--device", default="auto")
    args = ap.parse_args(argv)
    result = train_teacher(load_config(args.config), args.out, args.device)
    print(json.dumps({"checkpoint": result["checkpoint"], "val": result["val"]}, indent=2))


if __name__ == "__main__":
    main()
