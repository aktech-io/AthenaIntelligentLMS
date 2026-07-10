#!/usr/bin/env bash
# Push project documents to Google Drive.
#
# One-time setup:
#   1. rclone config            → new remote, name: gdrive, type: drive (follow OAuth prompts)
#   2. ./scripts/drive-sync.sh  → first sync creates "AthenaLMS Docs/" in My Drive
#
# Runs are one-way (local → Drive) and incremental. Wire into a git post-commit
# hook or cron for fully automatic pushes.
set -euo pipefail

RCLONE="${RCLONE:-$(command -v rclone || echo "$HOME/.local/bin/rclone")}"
REMOTE="${DRIVE_REMOTE:-gdrive}"
DEST="${DRIVE_DEST:-AthenaLMS Docs}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if ! "$RCLONE" listremotes 2>/dev/null | grep -q "^${REMOTE}:"; then
  echo "rclone remote '${REMOTE}' not configured yet." >&2
  echo "Run: rclone config   (new remote → name: ${REMOTE} → type: drive)" >&2
  exit 1
fi

echo "Syncing docs/ → ${REMOTE}:${DEST}/docs"
"$RCLONE" copy --update --exclude "node_modules/**" \
  "$REPO_ROOT/docs" "${REMOTE}:${DEST}/docs"

for f in DEPLOYMENT.md CLAUDE.md; do
  [ -f "$REPO_ROOT/$f" ] && "$RCLONE" copyto --update "$REPO_ROOT/$f" "${REMOTE}:${DEST}/$f"
done

echo "Done. View at: https://drive.google.com → ${DEST}/"
