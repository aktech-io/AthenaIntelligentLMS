"""Teacher -> student knowledge distillation.

Loss = alpha * T^2 * KL(student/T || teacher/T)  +  (1 - alpha) * CE(student, y)

The dataloader serves frames at the TEACHER's resolution (raw BGR 0..255);
the student's 80x80 view is produced in-batch with antialiased bilinear
downsampling, so both models always see the same augmented pixels.
Augmentations are the standard PAD stack (transforms.py): color jitter,
blur, JPEG artifacts, moiré-ish overlays for replay simulation.

Usage::

    python -m liveness_training.student.distill --config configs/distill.yaml \
        --teacher runs/teacher/teacher_best.pt [--out runs/student]
"""
from __future__ import annotations

import argparse
import json
import time

import torch
import torch.nn as nn
import torch.nn.functional as F

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
from liveness_training.student.model import StudentPAD, save_student
from liveness_training.teacher.model import load_teacher


def kd_loss(student_logits, teacher_logits, labels, temperature: float, alpha: float):
    t = temperature
    soft = F.kl_div(
        F.log_softmax(student_logits / t, dim=1),
        F.softmax(teacher_logits / t, dim=1),
        reduction="batchmean",
    ) * (t * t)
    hard = F.cross_entropy(student_logits, labels)
    return alpha * soft + (1.0 - alpha) * hard


def distill_student(cfg: dict, teacher_ckpt, out_dir, device: str = "auto") -> dict:
    seed_all(int(cfg.get("seed", 42)))
    device = pick_device(device)
    out = ensure_out_dir(out_dir)

    dcfg = cfg["data"]
    ocfg = cfg.get("optim", {})
    kcfg = cfg.get("distill", {})
    temperature = float(kcfg.get("temperature", 4.0))
    alpha = float(kcfg.get("alpha", 0.7))

    teacher = load_teacher(teacher_ckpt, map_location=device).to(device)
    for p in teacher.parameters():
        p.requires_grad = False
    teacher.eval()

    student = StudentPAD(width_mult=float(cfg.get("student", {}).get("width_mult", 1.0)))
    student = student.to(device)

    train_pad = build_dataset(dcfg, "train")
    val_pad = build_dataset(dcfg, "val")
    # frames at teacher resolution; the student view is derived per-batch
    train_loader = make_loader(
        train_pad, size=teacher.input_size,
        batch_size=int(ocfg.get("batch_size", 32)), shuffle=True,
        transform=default_train_transform() if cfg.get("augment", True) else None,
        frames_per_sample=int(dcfg.get("frames_per_sample", 1)),
        num_workers=int(ocfg.get("num_workers", 0)),
    )
    val_loader = make_loader(
        val_pad, size=student.input_size,
        batch_size=int(ocfg.get("batch_size", 32)), shuffle=False,
        frames_per_sample=int(dcfg.get("frames_per_sample", 1)),
    )
    if len(train_loader.dataset) == 0 or len(val_loader.dataset) == 0:
        raise RuntimeError("empty train or val split — check the data config")

    optimizer = torch.optim.AdamW(
        student.parameters(), lr=float(ocfg.get("lr", 3e-4)),
        weight_decay=float(ocfg.get("weight_decay", 0.01)),
    )
    epochs = int(ocfg.get("epochs", 5))
    accum = max(1, int(ocfg.get("grad_accum", 1)))
    use_amp = bool(ocfg.get("amp", True)) and device == "cuda"
    scaler = torch.amp.GradScaler("cuda", enabled=use_amp)

    def student_view(x_teacher_res: torch.Tensor) -> torch.Tensor:
        if x_teacher_res.shape[-1] == student.input_size:
            return x_teacher_res
        return F.interpolate(
            x_teacher_res, size=(student.input_size, student.input_size),
            mode="bilinear", align_corners=False, antialias=True,
        )

    best_acer, best_path = float("inf"), out / "student_best.pt"
    history = []
    for epoch in range(epochs):
        student.train()
        t0, running, seen = time.time(), 0.0, 0
        optimizer.zero_grad(set_to_none=True)
        for step, batch in enumerate(train_loader):
            x = batch["image"].to(device, non_blocking=True)
            y = batch["label"].to(device, non_blocking=True)
            with torch.no_grad(), torch.autocast("cuda", torch.float16, enabled=use_amp):
                t_logits = teacher(x)
            with torch.autocast("cuda", torch.float16, enabled=use_amp):
                s_logits = student(student_view(x))
                loss = kd_loss(s_logits, t_logits.float(), y, temperature, alpha) / accum
            scaler.scale(loss).backward()
            if (step + 1) % accum == 0 or (step + 1) == len(train_loader):
                scaler.step(optimizer)
                scaler.update()
                optimizer.zero_grad(set_to_none=True)
            running += float(loss.detach()) * accum * x.size(0)
            seen += x.size(0)

        val = collect_scores(student, val_loader, device)
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
        print(f"[distill] epoch {epoch}: {history[-1]}")
        if metrics["acer"] <= best_acer:
            best_acer = metrics["acer"]
            save_student(student, best_path, extra={
                "val_metrics": metrics,
                "teacher_checkpoint": str(teacher_ckpt),
                "teacher_backbone": teacher.backbone_kind,
                "distill": {"temperature": temperature, "alpha": alpha},
            })

    (out / "student_history.json").write_text(json.dumps(history, indent=2))
    return {"checkpoint": str(best_path), "val": history[-1], "history": history}


def main(argv=None) -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--config", required=True)
    ap.add_argument("--teacher", required=True, help="teacher checkpoint (.pt)")
    ap.add_argument("--out", default="runs/student")
    ap.add_argument("--device", default="auto")
    args = ap.parse_args(argv)
    result = distill_student(load_config(args.config), args.teacher, args.out, args.device)
    print(json.dumps({"checkpoint": result["checkpoint"], "val": result["val"]}, indent=2))


if __name__ == "__main__":
    main()
