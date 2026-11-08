#!/usr/bin/env bash
# Fast iteration loop for invoice-service on the Contabo k3s box: build the
# image locally, stream it into containerd over SSH, and roll the deployment.
# CI (push to master) remains the source of truth; this just avoids the ~20 min
# full-matrix build while debugging the live integration.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOST="${HOST:-deploy@158.220.112.84}"
SVC="${1:-invoice-service}"
TAG="hot-$(date +%Y%m%d-%H%M%S)"
IMG="ghcr.io/aktech-io/nemo-$SVC:$TAG"

echo "==> build $IMG"
docker build -q -f "$ROOT/go-services/deploy/docker/Dockerfile.service" --build-arg "SERVICE=$SVC" -t "$IMG" "$ROOT/go-services"
echo "==> import into k3s"
docker save "$IMG" | gzip -1 | ssh -o BatchMode=yes "$HOST" 'sudo k3s ctr -n k8s.io images import --digests=false - >/dev/null'
echo "==> roll $SVC"
ssh -o BatchMode=yes "$HOST" "K='sudo k3s kubectl -n lms'; \$K set image deploy/$SVC $SVC=$IMG && \$K patch deploy $SVC -p '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"$SVC\",\"imagePullPolicy\":\"IfNotPresent\"}]}}}}' >/dev/null; \$K rollout status deploy/$SVC --timeout=180s"
