"""POST /v1/document/extract — OCR field extraction from an ID-document image."""
from __future__ import annotations

from fastapi import APIRouter, File, HTTPException, UploadFile

router = APIRouter()


@router.post("/document/extract")
async def extract_document(file: UploadFile = File(...)):
    from engine import ocr
    from engine.fields import extract_fields, merge_mrz
    from engine.mrz import parse_mrz

    if not ocr.tesseract_available():
        # Fail loudly, never fabricate fields — the Go side fails closed on 5xx.
        raise HTTPException(503, "tesseract binary not available in this image")

    image_bytes = await file.read()
    if not image_bytes:
        raise HTTPException(400, "empty file")

    try:
        out = ocr.run_ocr(image_bytes)
    except HTTPException:
        raise
    except Exception as e:  # unreadable image, tesseract crash, ...
        raise HTTPException(422, f"could not OCR image: {e}") from e

    mrz = parse_mrz(out.text)
    fields = merge_mrz(extract_fields(out.words), mrz)

    return {
        "engine": "tesseract",
        "fields": {k: v.as_dict() for k, v in fields.items()},
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
