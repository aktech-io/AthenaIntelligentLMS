from __future__ import annotations

import os
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from api.scoring import router as scoring_router
from api.training import router as training_router
from monitoring.metrics import metrics_router
from db.database import init_db


@asynccontextmanager
async def lifespan(app: FastAPI):
    await init_db()
    from feedback.loop import start_scheduler
    scheduler = start_scheduler()
    yield
    scheduler.shutdown(wait=False)


app = FastAPI(
    title="Athena Fraud ML Service",
    description="ML scoring sidecar for the fraud-detection-service: "
                "transaction anomaly detection (Isolation Forest), "
                "customer fraud scoring (LightGBM), and combined ensemble scoring.",
    version="1.0.0",
    lifespan=lifespan,
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=os.getenv("CORS_ORIGINS", "*").split(","),
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.include_router(scoring_router, prefix="/api/v1", tags=["Scoring"])
app.include_router(training_router, prefix="/api/v1", tags=["Training"])
app.include_router(metrics_router, tags=["Metrics"])


@app.get("/health", tags=["Health"])
async def health():
    # Report on already-initialised singletons only — constructing FraudScorer
    # here reaches out to MLflow, which hangs past the probe timeout when no
    # tracking server is deployed (k3s box) and crash-loops the pod.
    from models import anomaly_detector, fraud_scorer

    champion = fraud_scorer._champion
    anomaly_loaded = (anomaly_detector._detector is not None
                      and anomaly_detector._detector.model is not None)
    lgbm_loaded = champion is not None and champion.model is not None

    return {
        "status": "ok",
        "service": "fraud-ml-service",
        "models": {
            "anomaly_detector": "loaded" if anomaly_loaded else "not_loaded",
            "fraud_scorer": "loaded" if lgbm_loaded else "not_loaded",
        },
    }
