"""POST /v1/face/match — document portrait vs selfie comparison."""
from __future__ import annotations

from fastapi import APIRouter, File, HTTPException, UploadFile

router = APIRouter()


@router.post("/face/match")
async def face_match(
    document: UploadFile = File(...), selfie: UploadFile = File(...)
):
    from engine.facematch import match_faces

    doc_bytes = await document.read()
    selfie_bytes = await selfie.read()
    if not doc_bytes or not selfie_bytes:
        raise HTTPException(400, "both document and selfie images are required")

    try:
        result = match_faces(doc_bytes, selfie_bytes)
    except ValueError as e:
        raise HTTPException(422, str(e)) from e
    except Exception as e:  # model load failure etc. — fail loudly
        raise HTTPException(503, f"face engine error: {e}") from e

    return result.as_dict()
