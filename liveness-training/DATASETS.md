# Datasets for the Stage-1 liveness pipeline

## LICENSING — read this first

**Every public dataset below is research-only.** CelebA-Spoof, CASIA-SURF
CeFA, BUPT-BalancedFace, RFW and FairFace are all released under
non-commercial / research licenses (some additionally require a signed
agreement). They are used here for exactly two things:

1. **bootstrapping the teacher** before NLD-EA reaches useful size, and
2. **internal evaluation** (cross-dataset generalization checks, BPCER
   fairness sweeps).

**The commercial, certified model trains on NLD-EA** — our own consented,
DPIA-covered East-African dataset (`docs/NLDEA_FORMAT.md`, plan
`docs/ekyc/06-level2-upgrade-plan.md` §5). Do not ship, sell, or certify a
model whose weights were trained on the research sets; the teacher
bootstrapped on them is a development scaffold, and the shipping student's
final distillation runs must use NLD-EA-trained teachers. When in doubt:
if it goes to iBeta or a customer, its training data is NLD-EA.

Do not commit any dataset content to this repo. Keep local copies under
`/mnt/ml/datasets/`.

---

## PAD (live + attack) — teacher bootstrap

### CelebA-Spoof (~625K images, 10K subjects)

* Home: <https://github.com/ZhangYuanhan-AI/CelebA-Spoof> — download links
  (Google Drive / Baidu) are in the repo README. License: research-only.
* **Local mirror in use**: the HuggingFace parquet mirror
  [`Ar4ikov/celebA_spoof`](https://huggingface.co/datasets/Ar4ikov/celebA_spoof)
  (~72 GB, 93 train + 65 test shards) is being synced to
  `/mnt/ml/datasets/celeba-spoof-parquet`. The loader
  (`liveness_training/datasets/celeba_spoof.py`) auto-detects both the
  original archive layout and this parquet layout (parquet needs `pyarrow`).
* Helper: `scripts/download_celeba_spoof_parquet.sh` (resumable HF sync).

### CASIA-SURF CeFA (cross-ethnicity PAD, includes African subjects)

* Paper: "CASIA-SURF CeFA: A Benchmark for Multi-modal Cross-ethnicity Face
  Anti-spoofing" (Liu et al., 2020). Distribution requires a signed
  agreement — request via the CASIA-SURF/CeFA challenge organizers
  (see <https://sites.google.com/corp/view/face-anti-spoofing-challenge> or the
  paper's contact). No direct public link; **no helper script possible**.
* Why we care: the only large public PAD set with a dedicated African
  partition — the closest public proxy for our East-African calibration
  goal until NLD-EA lands. Loader supports an `ethnicities=("african",)`
  filter for partition-specific evals.

## Genuine-only — BPCER fairness evaluation

These have **no attack data**; they contribute bona-fide presentations for
per-demographic BPCER sweeps (`eval/metrics.py::bpcer_per_skin_tone` uses
NLD-EA Monk labels; for these sets group by their own demographic labels).

### FairFace (~108K faces, balanced across 7 race groups)

* <https://github.com/joojs/fairface> — public Google Drive links, CC BY 4.0
  (the most permissive of the lot, still: treat evals as internal).
* Helper: `scripts/download_fairface.sh`.

### RFW — Racial Faces in the Wild

* <http://www.whdeng.cn/RFW/index.html> — signed agreement + email request
  (research-only, no redistribution). No helper script possible.

### BUPT-BalancedFace / BUPT-GlobalFace

* Same portal as RFW (<http://www.whdeng.cn/RFW/index.html>), same
  agreement-gated process. Research-only. No helper script possible.

---

## NLD-EA (ours — the one that ships)

Captured by the parallel NLD-EA campaign (400 subjects, Kenya; doc 06 §5).
Format contract: `docs/NLDEA_FORMAT.md`. Expected location:
`/mnt/ml/datasets/nldea/`. The `redteam` shard is the certification holdout
and is excluded by every loader default — see the format doc.

## Synthetic fixtures (tests)

`liveness_training/datasets/synthetic.py` generates all of the above layouts
procedurally — the pytest suite and `make smoke` run with **zero** real
datasets present.
