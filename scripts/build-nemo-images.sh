#!/usr/bin/env bash
# =============================================================================
# Build the image set the Nemo Helm chart (deploy/helm/nemo) installs:
#   nemo-<service>:<TAG> for the 16 Go services, nemo-fraud-ml, nemo-ekyc-ml,
#   nemo-portal.
#
# Usage:
#   ./scripts/build-nemo-images.sh                # docker-build all, tag latest
#   TAG=2026.07 ./scripts/build-nemo-images.sh    # explicit tag
#   ONLY=payment-service ./scripts/build-nemo-images.sh
#   REGISTRY=registry.example.com/nemo ./scripts/build-nemo-images.sh  # prefix + push
#   K3S_IMPORT=1 ./scripts/build-nemo-images.sh   # also import into local k3s
#     (uses the isolated buildx builder from build-k3s.sh — the host image
#      store workaround documented there)
# =============================================================================
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO_ROOT="${ROOT}/go-services"
DF="${GO_ROOT}/deploy/docker/Dockerfile.vendor"
TAG="${TAG:-latest}"
ONLY="${ONLY:-}"
REGISTRY="${REGISTRY:-}"
BUILDER="lmsbuilder"

GO_SERVICES=(
  account-service product-service loan-origination-service loan-management-service
  payment-service accounting-service float-service collections-service
  compliance-service reporting-service ai-scoring-service overdraft-service
  media-service notification-service fraud-detection-service lms-api-gateway
  card-service
)

img() { # img <name> -> full image ref
  local n="nemo-$1:${TAG}"
  [[ -n "$REGISTRY" ]] && n="${REGISTRY}/${n}"
  echo "$n"
}

build() { # build <image> <dockerfile|-> <context> [extra args...]
  local image="$1" df="$2" ctx="$3"; shift 3
  if [[ "${K3S_IMPORT:-0}" == "1" ]]; then
    docker buildx inspect "$BUILDER" >/dev/null 2>&1 \
      || docker buildx create --name "$BUILDER" --driver docker-container >/dev/null
    local tar; tar="$(mktemp --suffix=.tar)"
    docker buildx build --builder "$BUILDER" -t "$image" ${df:+-f "$df"} "$@" \
      --output "type=docker,dest=${tar}" "$ctx" || { rm -f "$tar"; return 1; }
    sudo k3s ctr images import "$tar"; local rc=$?; rm -f "$tar"; return $rc
  fi
  docker build -t "$image" ${df:+-f "$df"} "$@" "$ctx" || return 1
  [[ -n "$REGISTRY" ]] && docker push "$image"
}

echo "==> vendoring Go dependencies (offline build)"
( cd "$GO_ROOT" && go mod vendor )

FAILED=(); BUILT=()
for name in "${GO_SERVICES[@]}"; do
  [[ -n "$ONLY" && "$ONLY" != "$name" ]] && continue
  echo "==> [$(date +%H:%M:%S)] $(img "$name")"
  if build "$(img "$name")" "$DF" "$GO_ROOT" --build-arg "SERVICE=${name}" >/dev/null 2>&1; then
    BUILT+=("$name")
  else
    echo "    BUILD FAILED: ${name}"; FAILED+=("$name")
  fi
done

if [[ -z "$ONLY" || "$ONLY" == "fraud-ml" ]]; then
  echo "==> [$(date +%H:%M:%S)] $(img fraud-ml)"
  build "$(img fraud-ml)" "" "${ROOT}/fraud-ml-service" >/dev/null 2>&1 \
    && BUILT+=(fraud-ml) || { echo "    BUILD FAILED: fraud-ml"; FAILED+=(fraud-ml); }
fi

if [[ -z "$ONLY" || "$ONLY" == "ekyc-ml" ]]; then
  echo "==> [$(date +%H:%M:%S)] $(img ekyc-ml)"
  build "$(img ekyc-ml)" "" "${ROOT}/ekyc-ml-service" >/dev/null 2>&1 \
    && BUILT+=(ekyc-ml) || { echo "    BUILD FAILED: ekyc-ml"; FAILED+=(ekyc-ml); }
fi

if [[ -z "$ONLY" || "$ONLY" == "portal" ]]; then
  echo "==> [$(date +%H:%M:%S)] $(img portal)"
  build "$(img portal)" "${ROOT}/lms-portal-ui/Dockerfile" "${ROOT}/lms-portal-ui" >/dev/null 2>&1 \
    && BUILT+=(portal) || { echo "    BUILD FAILED: portal"; FAILED+=(portal); }
fi

echo "=============================="
if [ ${#FAILED[@]} -eq 0 ]; then
  echo "OK: built ${#BUILT[@]} image(s) tagged ${TAG}${REGISTRY:+ in ${REGISTRY}}"
else
  echo "FAILED: ${FAILED[*]}"; exit 1
fi
