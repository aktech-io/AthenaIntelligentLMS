"""POST /v1/face/match — document portrait vs selfie comparison.
POST /v1/face/liveness — Tier-2 passive PAD over 1..10 selfie frames with
doc-09 Stage-1 multi-frame fusion (shadow mode: scores only, never decides)."""
from __future__ import annotations

import logging

from fastapi import APIRouter, File, Form, HTTPException, UploadFile

router = APIRouter()

logger = logging.getLogger("ekyc.liveness")

# doc-09 Stage 1 targets 10-frame aggregation; the Go inhouse provider still
# caps at 5 today, so accepting up to 10 is a backward-compatible widening.
_MAX_LIVENESS_FRAMES = 10

_CHALLENGE_VALUES = {"passed": True, "failed": False}


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
async def face_liveness(
    frame: list[UploadFile] = File(...),
    challenge: str | None = Form(None),
):
    """Passive liveness: 1..10 'frame' multipart parts -> doc-09 Stage-1
    multi-frame fused PAD score (MiniFASNetV2 median + parallax + moiré,
    deterministic UNKNOWN-capped fallback when the model is absent). The
    optional 'challenge' form field ("passed" | "failed") folds the active
    ML Kit challenge result into the fusion; omitted = not run."""
    from engine.facematch import _decode
    from engine.liveness import score_frames

    if len(frame) > _MAX_LIVENESS_FRAMES:
        raise HTTPException(
            400, f"at most {_MAX_LIVENESS_FRAMES} frames are accepted"
        )
    challenge_passed: bool | None = None
    if challenge is not None:
        try:
            challenge_passed = _CHALLENGE_VALUES[challenge.strip().lower()]
        except KeyError:
            raise HTTPException(
                400, "challenge must be 'passed' or 'failed'"
            ) from None

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
        result = score_frames(images, challenge_passed)
    except Exception as e:  # model load failure etc. — fail loudly
        raise HTTPException(503, f"liveness engine error: {e}") from e

    body = result.as_dict()
    # shadow-mode calibration trail: one structured line per scoring call so
    # sub-score distributions can be mined for thresholds even before the Go
    # side persists the fusion breakdown (it only reads liveScore/label today)
    logger.info("liveness fusion (shadow): %s", body)
    return body
