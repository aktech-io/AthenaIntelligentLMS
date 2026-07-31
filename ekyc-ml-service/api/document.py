"""POST /v1/document/extract — OCR field extraction from an ID-document image."""
from __future__ import annotations

from fastapi import APIRouter, File, Form, HTTPException, UploadFile

router = APIRouter()


@router.post("/document/extract")
async def extract_document(
    file: UploadFile = File(...),
    doc_type: str | None = Form(None),  # NATIONAL_ID | PASSPORT (reserved)
    profile: str | None = Form(None),  # ke-national-id | passport-mrz | et-fayda
):
    from engine import ocr
    from engine.profiles import extract_with_profile

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

    # Dispatch is by profile id; missing/unknown -> default (pre-profile)
    # behaviour. doc_type is accepted for forward compatibility but the
    # profile id alone selects the strategy today.
    result = extract_with_profile(out.words, out.text, profile)
    mrz = result.mrz

    resp = {
        "engine": "tesseract",
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
