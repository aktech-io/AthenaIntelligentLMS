"""POST /v1/face/match — document portrait vs selfie comparison.
POST /v1/face/liveness — Tier-2 passive PAD over 1..5 selfie frames."""
from __future__ import annotations

from fastapi import APIRouter, File, HTTPException, UploadFile

router = APIRouter()

_MAX_LIVENESS_FRAMES = 5


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


@router.post("/face/liveness")
async def face_liveness(frame: list[UploadFile] = File(...)):
    """Passive liveness: 1..5 'frame' multipart parts -> MiniFASNetV2 PAD
    score (deterministic UNKNOWN-capped fallback when the model is absent)."""
    from engine.facematch import _decode
    from engine.liveness import score_frames

    if len(frame) > _MAX_LIVENESS_FRAMES:
        raise HTTPException(
            400, f"at most {_MAX_LIVENESS_FRAMES} frames are accepted"
        )

    images = []
    for f in frame:
        data = await f.read()
        if not data:
            raise HTTPException(400, "empty frame upload")
        try:
            images.append(_decode(data))
        except ValueError as e:
            raise HTTPException(422, str(e)) from e

    try:
        result = score_frames(images)
    except Exception as e:  # model load failure etc. — fail loudly
        raise HTTPException(503, f"liveness engine error: {e}") from e

    return result.as_dict()
