#!/usr/bin/env bash
# FairFace (genuine-only, BPCER fairness evals). CC BY 4.0 — still see
# ../DATASETS.md licensing notes. Public Google Drive links from
# https://github.com/joojs/fairface (margin=1.25 variant + labels).
#
#   ./download_fairface.sh [dest]     (default /mnt/ml/datasets/fairface)
set -euo pipefail
DEST="${1:-/mnt/ml/datasets/fairface}"
mkdir -p "$DEST"

if ! command -v gdown >/dev/null 2>&1; then
  echo "needs gdown for Google Drive:  pip install -U gdown" >&2
  exit 1
fi

cd "$DEST"
# ids from the joojs/fairface README (stable since 2019; re-check there if a
# download 404s)
gdown 1g7qNOZz9wC7OfOhcPqH1EZ5bk1UFGmlL -O fairface-img-margin125-trainval.zip  # images, margin 1.25
gdown 1i1L3Yqwaio7YSOCj7ftgk8ZZchPG7dmH -O fairface_label_train.csv
gdown 1wOdja-ezstMEp81tX1a-EYkFebev4h7D -O fairface_label_val.csv
unzip -n fairface-img-margin125-trainval.zip
echo "done -> $DEST"
