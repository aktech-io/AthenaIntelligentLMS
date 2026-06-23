#!/usr/bin/env bash
# =============================================================================
# Build LMS Go services + UI and deploy to local k3s — resilient path.
#
# Works around two host issues (see docs / project memory):
#   * Flaky in-container `go mod download`  -> vendor deps first, build offline.
#   * Corrupted host Docker image store     -> build via isolated `lmsbuilder`
#                                              buildx builder, output to a tar,
#                                              import straight into k3s containerd.
#
# Usage:
#   ./scripts/build-k3s.sh                 # all Go services + UI, build + rollout
#   ./scripts/build-k3s.sh account-service # one service
#   NO_ROLLOUT=1 ./scripts/build-k3s.sh    # build + import only, skip rollout
# =============================================================================
set -uo pipefail

LMS_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO_ROOT="${LMS_ROOT}/go-services"
DF="${GO_ROOT}/deploy/docker/Dockerfile.vendor"
BUILDER="lmsbuilder"
NS="lms"
ONLY="${1:-}"
TARDIR="$(mktemp -d)"
trap 'rm -rf "$TARDIR"' EXIT

GO_SERVICES=(
  account-service product-service loan-origination-service loan-management-service
  payment-service accounting-service float-service collections-service
  compliance-service reporting-service ai-scoring-service overdraft-service
  media-service notification-service fraud-detection-service lms-api-gateway
)

# Ensure the isolated builder exists (host default image store is corrupt).
if ! docker buildx inspect "$BUILDER" >/dev/null 2>&1; then
  echo "==> creating buildx builder '$BUILDER'"
  docker buildx create --name "$BUILDER" --driver docker-container >/dev/null
fi

echo "==> vendoring Go dependencies (offline build)"
( cd "$GO_ROOT" && go mod vendor )

FAILED=()
rolled=()

for name in "${GO_SERVICES[@]}"; do
  [[ -n "$ONLY" && "$ONLY" != "$name" ]] && continue
  tar="${TARDIR}/${name}.tar"
  echo "==> [$(date +%H:%M:%S)] build lms/${name}:latest"
  if docker buildx build --builder "$BUILDER" -t "lms/${name}:latest" \
       --build-arg "SERVICE=${name}" -f "$DF" \
       --output "type=docker,dest=${tar}" "$GO_ROOT" >/dev/null 2>&1; then
    sudo k3s ctr images import "$tar" >/dev/null && echo "    imported lms/${name}:latest" && rolled+=("$name")
  else
    echo "    BUILD FAILED: ${name}"; FAILED+=("$name")
  fi
done

# UI
if [[ -z "$ONLY" || "$ONLY" == "lms-portal-ui" ]]; then
  tar="${TARDIR}/lms-portal-ui.tar"
  echo "==> [$(date +%H:%M:%S)] build lms/lms-portal-ui:latest"
  if docker buildx build --builder "$BUILDER" -t "lms/lms-portal-ui:latest" \
       -f "${LMS_ROOT}/lms-portal-ui/Dockerfile" \
       --output "type=docker,dest=${tar}" "${LMS_ROOT}/lms-portal-ui" >/dev/null 2>&1; then
    sudo k3s ctr images import "$tar" >/dev/null && echo "    imported lms/lms-portal-ui:latest" && rolled+=("lms-portal-ui")
  else
    echo "    BUILD FAILED: lms-portal-ui"; FAILED+=("lms-portal-ui")
  fi
fi

if [[ "${NO_ROLLOUT:-0}" != "1" ]]; then
  for name in "${rolled[@]}"; do
    sudo k3s kubectl rollout restart "deploy/${name}" -n "$NS" >/dev/null 2>&1 && echo "    rolled out ${name}"
  done
fi

echo "=============================="
if [ ${#FAILED[@]} -eq 0 ]; then echo "OK: built ${#rolled[@]} image(s)"; else echo "FAILED: ${FAILED[*]}"; exit 1; fi
