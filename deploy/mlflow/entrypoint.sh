#!/bin/sh
# MLflow tracking server entrypoint.
#
# BACKEND_URI wins when set; otherwise the URI is assembled from the same
# DB_* env vars the Go services use (compose x-go-env / the lms-go-common
# ConfigMap on the k8s box), so the server needs no bespoke secret plumbing.
set -eu

if [ -z "${BACKEND_URI:-}" ]; then
    BACKEND_URI="postgresql://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT:-5432}/${DB_NAME:-athena_mlflow}"
fi

# MLflow 3 forks several uvicorn workers by default; each one loads the full
# server (~300 MB RSS), which OOM-kills a 1 Gi pod. One worker is plenty for
# the training/red-team clients.
exec mlflow server \
    --backend-store-uri "$BACKEND_URI" \
    --workers "${MLFLOW_WORKERS:-1}" \
    --artifacts-destination /mlflow/artifacts \
    --host 0.0.0.0 \
    --port "${PORT:-5000}"
