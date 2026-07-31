from __future__ import annotations

from fastapi import FastAPI

from api.document import router as document_router
from api.face import router as face_router
from api.screening import router as screening_router

app = FastAPI(
    title="Nemo eKYC ML Service",
    description="In-house eKYC engine sidecar for compliance-service: "
    "ID-document OCR + MRZ extraction (Tesseract), document-vs-selfie "
    "face match (YuNet + SFace ONNX, deterministic fallback), passive "
    "liveness / PAD (MiniFASNetV2 ONNX, deterministic fallback), and "
    "sanctions/PEP name screening.",
    version="1.0.0",
)

app.include_router(document_router, prefix="/v1", tags=["Document"])
app.include_router(face_router, prefix="/v1", tags=["Face"])
app.include_router(screening_router, prefix="/v1", tags=["Screening"])


@app.get("/health", tags=["Health"])
async def health():
    """Engine availability, plus readiness gating: when the deployment
    declares expectations (EXPECTED_FACE_ENGINE / EXPECTED_LIVENESS_ENGINE,
    or EKYC_ALLOW_DEMO_LISTS=false with only demo lists loaded) and reality
    differs, answer 503 so a k8s readinessProbe keeps the degraded pod out
    of service. With no expectations set the payload is unchanged."""
    from fastapi.responses import JSONResponse

    from engine.facematch import models_available
    from engine.liveness import model_available as liveness_model_available
    from engine.ocr import tesseract_available
    from engine.ppocr import models_available as ppocr_available
    from engine.readiness import engine_mismatches, expected_engines
    from engine.screening import demo_lists_allowed, get_screener

    import os

    mode = os.getenv("OCR_ENGINE", "auto").lower()
    if mode in ("auto", "ppocr") and ppocr_available():
        ocr_engine = "ppocr"
    elif mode != "ppocr" and tesseract_available():
        ocr_engine = "tesseract"
    else:
        ocr_engine = "unavailable"
    screener = get_screener()
    face_engine = "sface" if models_available() else "fallback"
    liveness_engine = (
        "minifasnet_v2" if liveness_model_available() else "fallback"
    )
    payload = {
        "status": "ok",
        "service": "ekyc-ml-service",
        "ocr": ocr_engine,
        "faceEngine": face_engine,
        "livenessEngine": liveness_engine,
        "screeningLists": screener.files,
        "screeningEntries": len(screener.entries),
    }

    degraded = engine_mismatches(
        face_engine, liveness_engine, *expected_engines()
    )
    if screener.demo_only and not demo_lists_allowed():
        payload["screeningListsMode"] = "demo-only"
        degraded.append(
            "screeningLists: demo-only (*_demo.csv) but "
            "EKYC_ALLOW_DEMO_LISTS=false — /v1/screen fails closed until "
            "real lists are provisioned"
        )
    if degraded:
        payload["status"] = "degraded"
        payload["degraded"] = degraded
        return JSONResponse(status_code=503, content=payload)
    return payload
