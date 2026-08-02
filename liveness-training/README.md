# liveness-training — Stage-1 PAD model pipeline

PyTorch pipeline for the Stage-1 liveness upgrade
(`docs/ekyc/06-level2-upgrade-plan.md` §3/§6, `docs/nemo/09-liveness-build-and-certify.md`):
fine-tune a domain-generalization **teacher** (FLIP-style CLIP), **distill**
it into a MobileNetV3-Small **student**, **export** to ONNX in the exact
deployment shape `ekyc-ml-service` runs today, and **evaluate** with
ISO/IEC 30107-3 certification metrics.

```
teacher (CLIP ViT-B/16, partial fine-tune)      liveness_training/teacher/
        │ KD (soft targets, T=4) + hard CE      liveness_training/student/
        ▼
student (MobileNetV3-Small, 80×80)
        │ opset-13 ONNX + in-graph softmax      liveness_training/export/
        ▼
cv2.dnn parity test + sha256 manifest  ──►  drop-in for engine/liveness.py
```

## Deployment contract (do not drift)

Single source of truth: `liveness_training/deployment.py`, mirroring
`ekyc-ml-service/engine/liveness.py`:

* input `(1,3,80,80)` float32, **BGR raw 0..255** (no /255, no mean/std at
  the boundary — normalization is baked into the graph);
* runtime `cv2.dnn.readNetFromONNX`;
* output = softmax probabilities, **index 1 = live** (serving reads
  `probs[1]`); the graph ends in a Softmax node so serving's sum-to-1
  detection skips re-normalization.

`export/parity.py` is the gate: it loads the exported ONNX **with cv2.dnn**
(not onnxruntime) and matches PyTorch to ≤1e-4 on serving-built blobs.
Provisioning follows the repo convention (Dockerfile SHA-256 pins, commit
4f95ab4): every export writes a checksum manifest with the ready-to-paste
`ARG`/`sha256sum -c` lines.

## Quick start

```bash
make smoke          # venv + CPU torch + full pytest suite (~2-3 min)
```

Real runs (need datasets — see `DATASETS.md` — and ideally the GPU):

```bash
python -m liveness_training.teacher.train   --config configs/teacher_clip.yaml --out runs/teacher
python -m liveness_training.student.distill --config configs/distill.yaml \
       --teacher runs/teacher/teacher_best.pt --out runs/student
python -m liveness_training.export.onnx_export --student runs/student/student_best.pt --out runs/export
```

## Layout

```
liveness_training/
  deployment.py       # serving-contract constants + blob builder
  common.py           # config/data/loop plumbing
  datasets/           # NOTE: named datasets/, never data/ (**/data/ is gitignored!)
    base.py           # PadSample contract, subject-disjoint splits, torch adapter
    nldea.py          # NLD-EA manifest loader (docs/NLDEA_FORMAT.md is the contract)
    celeba_spoof.py   # original archive + HF parquet mirror (pyarrow optional)
    cefa.py           # CASIA-SURF CeFA (cross-ethnicity; african filter)
    synthetic.py      # procedural fixtures — full pipeline runs with no real data
    transforms.py     # PAD augs: jitter, blur, JPEG, moiré overlays
  teacher/            # FLIP-style CLIP teacher (open_clip optional, tv fallback)
  student/            # MobileNetV3-Small student + KD distillation
  eval/               # APCER/BPCER/ACER, BPCER@APCER≤1%, skin-tone BPCER, md report
  export/             # ONNX export, cv2.dnn parity gate, checksum manifest
configs/              # real (teacher_clip, distill) + smoke configs
tests/                # pytest suite, synthetic-only, CPU, <3 min
```

Common loader contract: every dataset yields
`PadSample(frames, label, attack_type, skin_tone|None, subject_id)` with
label 1=live/0=spoof (== serving's class indices) and **subject-disjoint**
train/val splits everywhere — no identity leaks between shards. The NLD-EA
`redteam` shard (certification holdout) is excluded by default and only
reachable via an explicit flag that raises a RuntimeWarning.

## MLflow tracking (optional)

Every teacher/distill run can publish an audit trail (flattened config as
params, the per-epoch `train_loss`/`val_acer`/`val_apcer_max`/`val_bpcer`
series, the final metric set, VRAM peak, history JSON + exported ONNX with
its checksum manifest) to the platform's **nemo-mlflow** tracking server,
experiment **`liveness-training`**, run name = the `--out` dir basename.

It activates ONLY when `MLFLOW_TRACKING_URI` is set **and** `mlflow` is
installed (`pip install mlflow` — deliberately not in requirements.txt);
otherwise training is bit-identical to before, no extra deps needed.

The server is cluster-internal on the box (it has no auth — never exposed via
ingress). From the laptop, tunnel through SSH and port-forward the k3s
service in one go:

```bash
ssh -L 28115:localhost:28115 deploy@lms.athenafinance.cloud \
    'sudo k3s kubectl -n lms port-forward svc/nemo-mlflow 28115:5000'
# in another shell:
MLFLOW_TRACKING_URI=http://localhost:28115 \
    python -m liveness_training.teacher.train --config configs/teacher_clip.yaml --out runs/teacher
```

(Against the local docker-compose stack the server is already published on
the host: `MLFLOW_TRACKING_URI=http://localhost:28115`, no tunnel needed.)

## VRAM budget (RTX 4060 Laptop, 8 GB)

Measured on this machine (torch 2.13+cu130, fp16 autocast, ViT-B/16 @224,
peak `max_memory_allocated`): **1.1 GiB** for the default config
(`last_n:4`, batch 16 × accum 4), **2.6 GiB** full unfreeze @ batch 16,
**3.4 GiB** full unfreeze @ batch 32 — all comfortable on 8 GB. Every run
prints and stores its own peak (`runs/teacher/vram_peak_mib.txt`).
Distillation is teacher-forward-dominated and lighter still.

## Deliberately stubbed / deferred

* **LoRA** — v1 uses partial unfreeze (`last_n:K`) instead; same VRAM class,
  less machinery. Revisit if full-tower adaptation is ever needed on 8 GB.
* **FoundPAD (DINOv2) teacher** — config seam exists (`backbone:` spec);
  only CLIP + torchvision builders are implemented.
* **CeFA field order** — loader written to the published directory
  convention, validated on synthetic fixtures only; re-check the regex in
  `cefa.py` against the real (agreement-gated) download.
* **CelebA-Spoof parquet mirror has no subject identity** — verified on the
  real shards: paths are bare image ids, so the loader warns and degrades
  to per-image pseudo-subjects (deterministic, not subject-disjoint).
  Teacher bootstrap only; subject-safe evals need the original archive.
* **FairFace/RFW/BUPT loaders** — genuine-only fairness sets are documented
  (DATASETS.md) but not wired into loaders yet; NLD-EA carries the Monk-tone
  labels the fairness eval consumes today.
* **Multi-frame fusion training** — fusion is serving-side
  (`engine/liveness_fusion.py`); this pipeline trains the single-frame PAD
  scorer that feeds it.
