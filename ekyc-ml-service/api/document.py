"""POST /v1/document/extract — OCR field extraction from an ID-document image."""
from __future__ import annotations

import os

from fastapi import APIRouter, File, Form, HTTPException, UploadFile

router = APIRouter()


def choose_ocr_engine() -> str:
    """Resolve OCR_ENGINE (auto|ppocr|tesseract) to a runnable engine name.

    auto prefers PP-OCR when its models are provisioned, falling back to
    Tesseract. A forced engine that cannot run raises 503 — fail loudly,
    never fabricate fields; the Go side fails closed on 5xx.
    """
    from engine import ocr, ppocr

    mode = os.getenv("OCR_ENGINE", "auto").lower()
    if mode not in ("auto", "ppocr", "tesseract"):
        raise HTTPException(503, f"unknown OCR_ENGINE {mode!r}")
    if mode in ("auto", "ppocr") and ppocr.models_available():
        return "ppocr"
    if mode == "ppocr":
        raise HTTPException(
            503, "OCR_ENGINE=ppocr but PP-OCR models/onnxruntime are not provisioned"
        )
    if not ocr.tesseract_available():
        raise HTTPException(503, "no OCR engine available (tesseract binary missing)")
    return "tesseract"


@router.post("/document/extract")
async def extract_document(
    file: UploadFile = File(...),
    doc_type: str | None = Form(None),  # NATIONAL_ID | PASSPORT (reserved)
    profile: str | None = Form(None),  # ke-national-id | passport-mrz | et-fayda
):
    from engine import ocr, ppocr
    from engine.profiles import extract_with_profile

    engine_name = choose_ocr_engine()

    image_bytes = await file.read()
    if not image_bytes:
        raise HTTPException(400, "empty file")

    try:
        run = ppocr.run_ocr if engine_name == "ppocr" else ocr.run_ocr
        out = run(image_bytes)
    except HTTPException:
        raise
    except Exception as e:  # unreadable image, engine crash, ...
        raise HTTPException(422, f"could not OCR image: {e}") from e

    # Dispatch is by profile id; missing/unknown -> default (pre-profile)
    # behaviour. doc_type is accepted for forward compatibility but the
    # profile id alone selects the strategy today.
    result = extract_with_profile(out.words, out.text, profile)
    mrz = result.mrz

    resp = {
        "engine": engine_name,
        "fields": {k: v.as_dict() for k, v in result.fields.items()},
        "mrz": None
        if mrz is None
        else {
            "format": mrz.format,
            "valid": mrz.valid,
            "documentNumber": mrz.document_number,
            "fullName": mrz.full_name,
            "dateOfBirth": mrz.date_of_birth,
            "expiryDate": mrz.expiry_date,
            "nationality": mrz.nationality,
            "checks": mrz.checks,
        },
        "wordCount": len(out.words),
    }
    # Additive only (the Go extractResponse ignores unknown JSON fields);
    # omitted entirely on the default path so existing callers see the
    # byte-identical pre-profile response.
    if result.document_type_detected:
        resp["documentTypeDetected"] = result.document_type_detected
    return resp
