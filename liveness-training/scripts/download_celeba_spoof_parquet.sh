#!/usr/bin/env bash
# Resumable sync of the CelebA-Spoof HF parquet mirror (Ar4ikov/celebA_spoof,
# ~72 GB). RESEARCH-ONLY license — see ../DATASETS.md before using.
#
#   ./download_celeba_spoof_parquet.sh [dest]        (default /mnt/ml/datasets/celeba-spoof-parquet)
set -euo pipefail
DEST="${1:-/mnt/ml/datasets/celeba-spoof-parquet}"

if command -v hf >/dev/null 2>&1; then
  HF=hf
elif command -v huggingface-cli >/dev/null 2>&1; then
  HF=huggingface-cli
else
  echo "needs the huggingface CLI:  pip install -U huggingface_hub" >&2
  exit 1
fi

mkdir -p "$DEST"
exec "$HF" download Ar4ikov/celebA_spoof --repo-type dataset --local-dir "$DEST"
