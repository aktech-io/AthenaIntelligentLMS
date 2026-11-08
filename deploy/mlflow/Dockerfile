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
# tested against. The SQLAlchemy that mlflow 3.15 pulls in resolves a plain
# postgresql:// backend URI to the psycopg (v3) driver, so install that; keep
# psycopg2-binary too so an explicit postgresql+psycopg2:// URI still works.
RUN pip install --no-cache-dir mlflow==3.15.0 "psycopg[binary]==3.2.9" psycopg2-binary==2.9.10

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# MLflow 3 starts a background job runner (huey consumers for online scoring /
# trace archival / GenAI evals) alongside the web server — ~8 extra Python
# processes at ~230 MB each, which OOM-kills a 1 Gi pod. We only use plain
# experiment/run tracking, so keep it off. Override with "true" if ever needed.
ENV MLFLOW_SERVER_ENABLE_JOB_EXECUTION=false

VOLUME /mlflow/artifacts
EXPOSE 5000

ENTRYPOINT ["/entrypoint.sh"]
