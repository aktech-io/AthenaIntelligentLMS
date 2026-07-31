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
    from engine.facematch import models_available
    from engine.liveness import model_available as liveness_model_available
    from engine.ocr import tesseract_available
    from engine.ppocr import models_available as ppocr_available
    from engine.screening import get_screener

    import os

    mode = os.getenv("OCR_ENGINE", "auto").lower()
    if mode in ("auto", "ppocr") and ppocr_available():
        ocr_engine = "ppocr"
    elif mode != "ppocr" and tesseract_available():
        ocr_engine = "tesseract"
    else:
        ocr_engine = "unavailable"
    screener = get_screener()
    return {
        "status": "ok",
        "service": "ekyc-ml-service",
        "ocr": ocr_engine,
        "faceEngine": "sface" if models_available() else "fallback",
        "livenessEngine": "minifasnet_v2"
        if liveness_model_available()
        else "fallback",
        "screeningLists": screener.files,
        "screeningEntries": len(screener.entries),
    }
