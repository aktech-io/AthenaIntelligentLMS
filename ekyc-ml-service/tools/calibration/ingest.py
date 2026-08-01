"""Shadow-log ingestion for liveness threshold calibration.

api/face.py logs one structured line per POST /v1/face/liveness call on the
``ekyc.liveness`` logger:

    logger.info("liveness fusion (shadow): %s", body)

``body`` is the response dict, so the rendered message carries a *Python
dict repr* (single quotes, True/False/None) — parsed here with
``ast.literal_eval`` (with a ``json.loads`` fallback should the format ever
switch to JSON). Everything before the marker is tolerated, which covers
every prefix seen in the wild:

    liveness fusion (shadow): {'liveScore': ...}                (bare)
    INFO:ekyc.liveness:liveness fusion (shadow): {...}          (basicConfig)
    2026-08-01T09:12:33.123456789Z <anything> (shadow): {...}   (kubectl --timestamps)
    2026-08-01 09:12:33,123 INFO ... (shadow): {...}            (asctime formatter)

Lines without the marker are silently irrelevant (uvicorn access logs etc.);
lines WITH the marker that fail to parse are counted as malformed.

Records are flattened (top-level liveScore/label/model/facesFound + the
``fusion`` sub-dict fields) and deduplicated: the key is the timestamp (when
one could be extracted) plus a hash of the canonicalized body, so
re-ingesting an overlapping ``kubectl logs`` pull is a no-op. Pull logs with
``--timestamps`` so distinct sessions with identical scores stay distinct.

A jsonl store (default ``tools/calibration/store/records.jsonl``) accumulates
records across runs — rerun the tool as traffic accumulates and the analysis
always covers the full history.

Runnable standalone:  python -m tools.calibration.ingest <file|->
"""
from __future__ import annotations

import ast
import hashlib
import json
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path

MARKER = "liveness fusion (shadow):"

# kubectl --timestamps (RFC3339Nano) or python asctime ("2026-08-01 09:12:33,123")
_TS_RE = re.compile(
    r"(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?)"
)

# fields copied verbatim from the fusion breakdown (engine/liveness_fusion.py)
FUSION_FIELDS = (
    "score",
    "padMedian",
    "padMin",
    "parallax",
    "moire",
    "challenge",
    "motionPx",
    "nonRigidityPx",
    "moirePeakRatio",
    "frames",
    "version",
)

DEFAULT_STORE = Path(__file__).resolve().parent / "store" / "records.jsonl"


@dataclass
class IngestResult:
    records: list[dict] = field(default_factory=list)
    malformed: int = 0  # lines with the marker that failed to parse
    scanned: int = 0  # total lines seen


def _extract_timestamp(prefix: str) -> str | None:
    m = _TS_RE.search(prefix)
    return m.group(1) if m else None


def _parse_body(text: str) -> dict | None:
    """Parse the ``{...}`` payload: Python dict repr first, JSON fallback."""
    try:
        body = ast.literal_eval(text)
    except (ValueError, SyntaxError):
        try:
            body = json.loads(text)
        except (ValueError, TypeError):
            return None
    return body if isinstance(body, dict) else None


def _dedupe_key(ts: str | None, body: dict) -> str:
    canonical = json.dumps(body, sort_keys=True, default=str)
    h = hashlib.sha1()
    h.update((ts or "").encode())
    h.update(canonical.encode())
    return h.hexdigest()


def normalize(body: dict, ts: str | None) -> dict:
    """Flatten one response body into a calibration record.

    Top-level: ts, key, model, label, liveScore (what Go's enforcement gate
    would read), frames actually uploaded, facesFound. Fusion sub-scores are
    copied under their log names — note ``score`` is the *fused* score, which
    for the fallback engine differs from the capped top-level liveScore.
    """
    fusion = body.get("fusion") or {}
    per_frame = body.get("perFrame") or []
    record = {
        "ts": ts,
        "key": _dedupe_key(ts, body),
        "model": body.get("model"),
        "label": body.get("label"),
        "liveScore": body.get("liveScore"),
        "facesFound": sum(1 for f in per_frame if f.get("faceFound")),
        "perFrameCount": len(per_frame),
    }
    for name in FUSION_FIELDS:
        record[name] = fusion.get(name)
    return record


def parse_line(line: str) -> dict | None | str:
    """One log line -> record dict, None (irrelevant), or "malformed"."""
    idx = line.find(MARKER)
    if idx < 0:
        return None
    payload_start = line.find("{", idx + len(MARKER))
    if payload_start < 0:
        return "malformed"
    body = _parse_body(line[payload_start:].strip())
    if body is None or "liveScore" not in body:
        return "malformed"
    return normalize(body, _extract_timestamp(line[:idx]))


def parse_stream(lines) -> IngestResult:
    result = IngestResult()
    seen: set[str] = set()
    for line in lines:
        result.scanned += 1
        parsed = parse_line(line)
        if parsed is None:
            continue
        if parsed == "malformed":
            result.malformed += 1
            continue
        if parsed["key"] in seen:
            continue
        seen.add(parsed["key"])
        result.records.append(parsed)
    return result


# ─── jsonl store ─────────────────────────────────────────────────────────────


def load_store(path: Path = DEFAULT_STORE) -> list[dict]:
    if not path.is_file():
        return []
    records = []
    with path.open() as fh:
        for line in fh:
            line = line.strip()
            if line:
                records.append(json.loads(line))
    return records


def merge_into_store(new_records: list[dict], path: Path = DEFAULT_STORE) -> tuple[list[dict], int]:
    """Append records whose key is not yet stored. Returns (all, added)."""
    existing = load_store(path)
    known = {r["key"] for r in existing}
    added = [r for r in new_records if r["key"] not in known]
    if added:
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("a") as fh:
            for r in added:
                fh.write(json.dumps(r) + "\n")
    return existing + added, len(added)


def ingest(source: str, store: Path | None = DEFAULT_STORE) -> tuple[IngestResult, list[dict], int]:
    """Parse ``source`` (file path or '-') and merge into the store.

    Returns (parse result, full record history, newly added count). Pass
    ``store=None`` to analyze a log file in isolation without persisting.
    """
    if source == "-":
        result = parse_stream(sys.stdin)
    else:
        with open(source, encoding="utf-8", errors="replace") as fh:
            result = parse_stream(fh)
    if store is None:
        return result, list(result.records), len(result.records)
    all_records, added = merge_into_store(result.records, store)
    return result, all_records, added


def main(argv: list[str] | None = None) -> int:
    import argparse

    ap = argparse.ArgumentParser(
        prog="python -m tools.calibration.ingest",
        description="Ingest ekyc.liveness shadow log lines into the jsonl store.",
    )
    ap.add_argument("logs", help="log file path, or '-' for stdin")
    ap.add_argument("--store", type=Path, default=DEFAULT_STORE)
    args = ap.parse_args(argv)

    result, all_records, added = ingest(args.logs, args.store)
    print(
        f"scanned {result.scanned} lines: {len(result.records)} fusion records "
        f"({result.malformed} malformed skipped), {added} new -> store now "
        f"holds {len(all_records)} ({args.store})"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
