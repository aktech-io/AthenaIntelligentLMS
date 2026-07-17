"""POST /v1/screen — sanctions/PEP name screening."""
from __future__ import annotations

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field

from engine.screening import DEFAULT_THRESHOLD, get_screener

router = APIRouter()


class ScreenRequest(BaseModel):
    fullName: str = Field(min_length=1)
    threshold: float | None = Field(default=None, ge=0.5, le=1.0)


@router.post("/screen")
async def screen(req: ScreenRequest):
    screener = get_screener()
    if not screener.entries:
        # No lists loaded means screening cannot run — 503 so the caller
        # fails closed instead of treating "no data" as "no hit".
        raise HTTPException(503, "no screening lists loaded")

    threshold = req.threshold or DEFAULT_THRESHOLD
    matches = screener.screen(req.fullName, threshold)
    return {
        "sanctionsHit": any(m.list_name == "sanctions" for m in matches),
        "pepHit": any(m.list_name == "pep" for m in matches),
        "threshold": threshold,
        "matches": [m.as_dict() for m in matches],
        "listsLoaded": screener.files,
    }
