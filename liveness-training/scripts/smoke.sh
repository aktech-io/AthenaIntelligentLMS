#!/usr/bin/env bash
# Smoke-test the whole Stage-1 liveness pipeline in a throwaway venv.
# No datasets, no GPU, no network beyond pip needed. ~2-3 min after the
# first (cached) install.
#
#   ./scripts/smoke.sh                 # CPU torch (small, default)
#   SMOKE_CUDA=1 ./scripts/smoke.sh    # full CUDA torch build
#   SMOKE_VENV=/path ./scripts/smoke.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VENV="${SMOKE_VENV:-$HERE/.venv}"
PY="$VENV/bin/python"

if [ ! -x "$PY" ]; then
  echo "[smoke] creating venv at $VENV"
  python3 -m venv "$VENV"
fi

"$PY" -m pip install --quiet --upgrade pip

if [ "${SMOKE_CUDA:-0}" = "1" ]; then
  echo "[smoke] installing requirements (CUDA torch)"
  "$PY" -m pip install --quiet -r "$HERE/requirements.txt"
else
  echo "[smoke] installing requirements (CPU torch)"
  "$PY" -m pip install --quiet torch torchvision --index-url https://download.pytorch.org/whl/cpu
  "$PY" -m pip install --quiet -r "$HERE/requirements.txt"
fi
# optional extras exercised by the suite when present:
#   pyarrow -> CelebA-Spoof HF parquet-mirror loader tests
"$PY" -m pip install --quiet pyarrow || echo "[smoke] pyarrow install failed — parquet tests will skip"

echo "[smoke] running pytest"
cd "$HERE"
exec "$PY" -m pytest tests/ -v --tb=short
