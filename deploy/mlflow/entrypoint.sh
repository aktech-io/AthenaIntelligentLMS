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

exec mlflow server \
    --backend-store-uri "$BACKEND_URI" \
    --artifacts-destination /mlflow/artifacts \
    --host 0.0.0.0 \
    --port "${PORT:-5000}"
