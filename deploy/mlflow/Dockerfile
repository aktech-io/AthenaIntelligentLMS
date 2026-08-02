# MLflow tracking server — the audit backbone for liveness model training
# (experiment "liveness-training") and PAD red-team runs ("liveness-redteam").
#
# Compose service `mlflow` (host port 28115), k8s Deployment `nemo-mlflow`.
# Backend store: PostgreSQL database athena_mlflow on the platform PG;
# artifacts on the volume mounted at /mlflow/artifacts.
#
# SECURITY: MLflow ships NO authentication. Keep this cluster/compose-internal
# only — never route it through an Ingress or publish the port beyond
# localhost. Remote access is SSH tunnel / kubectl port-forward only.
FROM python:3.12-slim

# Pin: mlflow 3.15.0 is the version the training/red-team clients are smoke-
# tested against; psycopg2-binary is the PostgreSQL driver for the backend
# store URI.
RUN pip install --no-cache-dir mlflow==3.15.0 psycopg2-binary==2.9.10

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

VOLUME /mlflow/artifacts
EXPOSE 5000

ENTRYPOINT ["/entrypoint.sh"]
