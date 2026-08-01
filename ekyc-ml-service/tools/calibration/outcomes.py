"""Weak-label join: shadow records x onboarding outcomes.

The shadow log line carries no application id (api/face.py logs only the
response body), so records are joined to compliance-DB outcomes by the two
signals both sides share:

1. **score match** — ``onboarding_applications.liveness_score`` is exactly
   the response ``liveScore`` the Go provider stored (rounded to 4 dp by the
   engine), so equality within 5e-5 is a strong key;
2. **timestamp proximity** — when both sides carry timestamps, the outcome's
   ``created_at`` must fall within ``--join-window`` seconds of the log line
   (the liveness call happens during Submit, so they are near-simultaneous).

Each outcome row is consumed at most once (greedy, nearest-in-time first).

Export the outcomes from the compliance DB (table + columns per
go-services/internal/compliance/repository/onboarding_repository.go):

    \\copy (
      SELECT id            AS application_id,
             created_at,
             decided_at,
             status,                -- AUTO_APPROVED|REFERRED|APPROVED|REJECTED
             risk_tier,
             decision_reasons,      -- free text, single column
             liveness_score,
             liveness_mode,         -- shadow|shadow-error|enforce
             liveness_provider
      FROM onboarding_applications
      WHERE liveness_mode IS NOT NULL
      ORDER BY created_at
    ) TO 'outcomes.csv' WITH CSV HEADER

WEAK LABELS — treat with appropriate suspicion (docs/nemo/08: calibrate on
real traffic, but officers are not a PAD ground truth):

* ``genuine``   — officer APPROVED a referral (a human vouched for the
  session), or AUTO_APPROVED straight-through (weaker still: nobody looked).
* ``suspicious`` — officer REJECTED and the decision reasons mention
  liveness/spoof/PAD/selfie/photo-of-photo. A rejection for e.g. a sanctions
  hit says nothing about the selfie, so those stay unlabeled.
* everything else (RECEIVED, REFERRED-pending, unrelated rejections) — None.

These are *proxies*: BPCER computed against them is a BPCER-proxy, not a
measured BPCER. The report says so.
"""
from __future__ import annotations

import csv
import json
import re
from datetime import datetime, timezone
from pathlib import Path

JOIN_WINDOW_S = 180.0  # default |log ts - created_at| tolerance
_SCORE_TOL = 5e-5  # liveness_score is the 4-dp-rounded liveScore

_LIVENESS_REASON_RE = re.compile(
    r"liveness|spoof|pad\b|presentation.attack|selfie|photo.of.photo|screen.replay",
    re.IGNORECASE,
)

# tolerated header spellings -> canonical name
_ALIASES = {
    "application_id": "applicationId",
    "applicationid": "applicationId",
    "id": "applicationId",
    "created_at": "createdAt",
    "createdat": "createdAt",
    "decided_at": "decidedAt",
    "decidedat": "decidedAt",
    "status": "status",
    "decision": "status",
    "risk_tier": "riskTier",
    "risktier": "riskTier",
    "decision_reasons": "decisionReasons",
    "decisionreasons": "decisionReasons",
    "officer_verdict": "decisionReasons",
    "liveness_score": "livenessScore",
    "livenessscore": "livenessScore",
    "liveness_mode": "livenessMode",
    "livenessmode": "livenessMode",
    "liveness_provider": "livenessProvider",
    "livenessprovider": "livenessProvider",
}


def _canonicalize(row: dict) -> dict:
    out = {}
    for k, v in row.items():
        if k is None:
            continue
        canon = _ALIASES.get(k.strip().lower().replace(" ", "_"))
        if canon:
            out[canon] = v.strip() if isinstance(v, str) else v
    return out


def parse_ts(value) -> float | None:
    """ISO-ish timestamp -> epoch seconds (UTC assumed when naive)."""
    if value in (None, ""):
        return None
    if isinstance(value, (int, float)):
        return float(value)
    text = str(value).strip().replace(",", ".")
    if " " in text and "T" not in text:
        text = text.replace(" ", "T", 1)
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    # RFC3339Nano from kubectl has 9 fractional digits; fromisoformat caps at 6
    m = re.match(r"(.*?\.\d{1,6})\d*([+-].*)?$", text)
    if m:
        text = m.group(1) + (m.group(2) or "")
    try:
        dt = datetime.fromisoformat(text)
    except ValueError:
        return None
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    return dt.timestamp()


def weak_label(outcome: dict) -> str | None:
    status = (outcome.get("status") or "").upper()
    reasons = outcome.get("decisionReasons") or ""
    if status == "APPROVED":
        return "genuine"  # officer approved a referral
    if status == "AUTO_APPROVED":
        return "genuine"  # straight-through; weaker evidence, same bucket
    if status == "REJECTED" and _LIVENESS_REASON_RE.search(str(reasons)):
        return "suspicious"
    return None


def load_outcomes(path: str | Path) -> list[dict]:
    """CSV (header row) or JSON (list of objects) -> canonical outcome dicts."""
    path = Path(path)
    text = path.read_text()
    if path.suffix.lower() == ".json" or text.lstrip().startswith(("[", "{")):
        raw = json.loads(text)
        rows = raw if isinstance(raw, list) else raw.get("outcomes", [])
    else:
        rows = list(csv.DictReader(text.splitlines()))
    outcomes = []
    for row in rows:
        o = _canonicalize(row)
        if not o:
            continue
        try:
            o["livenessScore"] = float(o["livenessScore"])
        except (KeyError, TypeError, ValueError):
            o["livenessScore"] = None
        o["createdAtEpoch"] = parse_ts(o.get("createdAt"))
        o["weakLabel"] = weak_label(o)
        outcomes.append(o)
    return outcomes


def join(records: list[dict], outcomes: list[dict], window_s: float = JOIN_WINDOW_S) -> int:
    """Annotate records in place with outcome fields. Returns joined count.

    Match = liveness_score equals the record's liveScore (4-dp tolerance);
    when both timestamps exist they must also be within ``window_s``. Among
    multiple candidates the nearest-in-time (or first unused) outcome wins,
    and every outcome row is consumed at most once.
    """
    used: set[int] = set()
    joined = 0
    for rec in records:
        rec.setdefault("weakLabel", None)
        score = rec.get("liveScore")
        if score is None:
            continue
        rec_ts = parse_ts(rec.get("ts"))
        best: tuple[float, int] | None = None
        for i, o in enumerate(outcomes):
            if i in used or o["livenessScore"] is None:
                continue
            if abs(o["livenessScore"] - score) > _SCORE_TOL:
                continue
            if rec_ts is not None and o["createdAtEpoch"] is not None:
                dt = abs(o["createdAtEpoch"] - rec_ts)
                if dt > window_s:
                    continue
            else:
                dt = float("inf")  # score-only match: allowed, ranked last
            if best is None or dt < best[0]:
                best = (dt, i)
        if best is None:
            continue
        used.add(best[1])
        o = outcomes[best[1]]
        rec["applicationId"] = o.get("applicationId")
        rec["outcomeStatus"] = o.get("status")
        rec["livenessMode"] = o.get("livenessMode")
        rec["livenessProvider"] = o.get("livenessProvider")
        rec["weakLabel"] = o["weakLabel"]
        joined += 1
    return joined
