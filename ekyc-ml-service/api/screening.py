"""POST /v1/screen — sanctions/PEP name screening."""
from __future__ import annotations

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field

from engine.screening import (
    DEFAULT_THRESHOLD,
    demo_lists_allowed,
    get_screener,
)

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
    if screener.demo_only and not demo_lists_allowed():
        # Only *_demo.csv (fictional) lists are loaded and the deployment
        # declared production posture — a real sanctioned name would screen
        # clean, so fail closed exactly like the missing-lists case.
        raise HTTPException(
            503,
            "only demo screening lists are loaded (*_demo.csv) and "
            "EKYC_ALLOW_DEMO_LISTS=false — provision real lists "
            "(scripts/sync-screening-lists.py) before serving screens",
        )

    threshold = req.threshold or DEFAULT_THRESHOLD
    matches = screener.screen(req.fullName, threshold)
    return {
        "sanctionsHit": any(m.list_name == "sanctions" for m in matches),
        "pepHit": any(m.list_name == "pep" for m in matches),
        "threshold": threshold,
        "matches": [m.as_dict() for m in matches],
        "listsLoaded": screener.files,
    }
