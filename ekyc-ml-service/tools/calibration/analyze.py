"""Distributions, threshold sweep, and recommendations over shadow records.

numpy + stdlib only (NO pandas — lean-image constraint). Two scores matter
and they are not the same thing:

* ``score``     — the FUSED score (fusion.score). This is what a calibrated
                  threshold will eventually gate on, so the sweep and the
                  distributions analyze it.
* ``liveScore`` — the top-level response score Go's enforcement gate reads
                  today (LivenessPassed = liveScore >= threshold,
                  go-services/internal/compliance/ekyc/inhouse.go). For the
                  minifasnet engine it equals the fused score; the fallback
                  engine hard-caps it at 0.5. The "what would
                  LIVENESS_ENFORCE=true have done" replay uses THIS one.

The old single-frame policy (min across frames) survives in ``padMin``, so
the old-vs-new A/B is: verdict(padMin >= t) vs verdict(score >= t) per
threshold t.
"""
from __future__ import annotations

import math
from dataclasses import dataclass, field

THRESHOLDS = [round(0.05 * i, 2) for i in range(1, 20)]  # 0.05 .. 0.95
DEFAULT_BPCER_TARGET = 0.05  # cert bar is <=15% (doc 06 §7); aim well under
MIN_SAMPLE_FOR_RECOMMENDATION = 30
_PARALLAX_AVAILABILITY_TARGET = 0.9

COMPONENTS = ["padMedian", "padMin", "parallax", "moire", "challenge", "score"]


def _values(records, fieldname, where=None):
    out = []
    for r in records:
        if where is not None and not where(r):
            continue
        v = r.get(fieldname)
        if v is not None and not (isinstance(v, float) and math.isnan(v)):
            out.append(float(v))
    return out


def summary_stats(values: list[float]) -> dict | None:
    import numpy as np

    if not values:
        return None
    a = np.asarray(values, dtype=float)
    return {
        "n": int(a.size),
        "mean": float(a.mean()),
        "median": float(np.median(a)),
        "p05": float(np.percentile(a, 5)),
        "p95": float(np.percentile(a, 95)),
        "min": float(a.min()),
        "max": float(a.max()),
    }


def ascii_hist(values: list[float], bins: int = 20, width: int = 40,
               lo: float | None = None, hi: float | None = None) -> str:
    """Fixed-width text histogram (monospace, for the markdown report)."""
    import numpy as np

    if not values:
        return "(no data)"
    a = np.asarray(values, dtype=float)
    lo = float(a.min()) if lo is None else lo
    hi = float(a.max()) if hi is None else hi
    if hi <= lo:
        hi = lo + 1e-9
    counts, edges = np.histogram(a, bins=bins, range=(lo, hi))
    peak = max(int(counts.max()), 1)
    lines = []
    for c, e0, e1 in zip(counts, edges, edges[1:]):
        bar = "#" * max(int(round(width * c / peak)), 1 if c else 0)
        lines.append(f"{e0:7.3f} - {e1:7.3f} | {bar:<{width}} {int(c)}")
    return "\n".join(lines)


def correlation_matrix(records: list[dict], fields=COMPONENTS) -> dict:
    """Pairwise-complete Pearson correlations between fusion components.

    Sanity check on the provisional fusion weights: every component should
    correlate positively with the fused score; a negative or ~zero
    correlation means that component is either constant in this traffic or
    pulling against the fusion.
    """
    import numpy as np

    matrix: dict[str, dict[str, float | None]] = {f: {} for f in fields}
    counts: dict[str, dict[str, int]] = {f: {} for f in fields}
    for f1 in fields:
        for f2 in fields:
            pairs = [
                (r.get(f1), r.get(f2))
                for r in records
                if r.get(f1) is not None and r.get(f2) is not None
            ]
            counts[f1][f2] = len(pairs)
            if len(pairs) < 3:
                matrix[f1][f2] = None
                continue
            a = np.asarray(pairs, dtype=float)
            if a[:, 0].std() < 1e-12 or a[:, 1].std() < 1e-12:
                matrix[f1][f2] = None  # constant column: correlation undefined
                continue
            matrix[f1][f2] = float(np.corrcoef(a[:, 0], a[:, 1])[0, 1])
    return {"fields": fields, "matrix": matrix, "counts": counts}


# ─── threshold sweep ─────────────────────────────────────────────────────────


@dataclass
class SweepRow:
    threshold: float
    n: int
    pass_rate: float  # fused score >= t
    flips: int  # sessions where old (padMin) and new (fused) verdicts differ
    old_pass_rate: float | None  # padMin >= t
    bpcer_proxy: float | None  # weak-genuine rejected (fused < t)
    apcer_proxy: float | None  # weak-suspicious accepted (fused >= t)
    per_stratum_pass: dict = field(default_factory=dict)


def stratum_of(record: dict, kind: str) -> str:
    if kind == "model":
        return str(record.get("model"))
    if kind == "frames":
        f = record.get("frames") or 0
        return "1" if f <= 1 else ("2-4" if f <= 4 else "5+")
    if kind == "challenge":
        return "with-challenge" if record.get("challenge") is not None else "no-challenge"
    if kind == "provider":
        return str(record.get("livenessProvider") or "unjoined")
    if kind == "mode":
        return str(record.get("livenessMode") or "unjoined")
    raise ValueError(kind)


STRATA = ("model", "frames", "challenge", "provider", "mode")


def threshold_sweep(records: list[dict], thresholds=THRESHOLDS) -> list[SweepRow]:
    scored = [r for r in records if r.get("score") is not None]
    rows = []
    for t in thresholds:
        n = len(scored)
        if n == 0:
            rows.append(SweepRow(t, 0, 0.0, 0, None, None, None))
            continue
        new_pass = [r["score"] >= t for r in scored]
        with_min = [r for r in scored if r.get("padMin") is not None]
        flips = sum(
            (r["padMin"] >= t) != (r["score"] >= t) for r in with_min
        )
        genuine = [r for r in scored if r.get("weakLabel") == "genuine"]
        suspicious = [r for r in scored if r.get("weakLabel") == "suspicious"]
        per_stratum = {}
        for kind in STRATA:
            groups: dict[str, list[bool]] = {}
            for r, p in zip(scored, new_pass):
                groups.setdefault(stratum_of(r, kind), []).append(p)
            per_stratum[kind] = {
                name: (sum(v) / len(v), len(v)) for name, v in sorted(groups.items())
            }
        rows.append(
            SweepRow(
                threshold=t,
                n=n,
                pass_rate=sum(new_pass) / n,
                flips=flips,
                old_pass_rate=(
                    sum(r["padMin"] >= t for r in with_min) / len(with_min)
                    if with_min
                    else None
                ),
                bpcer_proxy=(
                    sum(r["score"] < t for r in genuine) / len(genuine)
                    if genuine
                    else None
                ),
                apcer_proxy=(
                    sum(r["score"] >= t for r in suspicious) / len(suspicious)
                    if suspicious
                    else None
                ),
                per_stratum_pass=per_stratum,
            )
        )
    return rows


def operating_range(sweep: list[SweepRow], bpcer_target: float) -> list[float]:
    """Thresholds whose weak-genuine rejection rate stays under target."""
    return [
        row.threshold
        for row in sweep
        if row.bpcer_proxy is not None and row.bpcer_proxy <= bpcer_target
    ]


# ─── recommendations ─────────────────────────────────────────────────────────


def recommend_threshold(records, sweep, bpcer_target: float) -> dict:
    """Threshold recommendation with an explicit confidence basis.

    * weak labels present and sample adequate -> highest threshold keeping
      BPCER-proxy <= target (spoof-averse: push as high as genuine traffic
      tolerates, per the 0%-APCER certification posture of doc 06 §7);
    * no labels but adequate sample -> distribution-only: highest threshold
      passing >= 98% of minifasnet traffic (shadow traffic is presumed
      overwhelmingly genuine), clearly flagged lower-confidence;
    * tiny sample -> no number at all; stay in shadow.
    """
    scored = [r for r in records if r.get("score") is not None]
    minifasnet = [r for r in scored if r.get("model") == "minifasnet_v2"]
    labeled_genuine = [r for r in scored if r.get("weakLabel") == "genuine"]
    notes: list[str] = []

    if len(scored) < MIN_SAMPLE_FOR_RECOMMENDATION:
        return {
            "threshold": None,
            "basis": "insufficient-data",
            "notes": [
                f"only {len(scored)} scored sessions (< {MIN_SAMPLE_FOR_RECOMMENDATION}); "
                "keep LIVENESS_ENFORCE=false and re-run as traffic accumulates"
            ],
        }

    if labeled_genuine:
        candidates = operating_range(sweep, bpcer_target)
        if candidates:
            t = max(candidates)
            notes.append(
                f"highest threshold keeping BPCER-proxy <= {bpcer_target:.0%} "
                f"over {len(labeled_genuine)} weak-genuine sessions"
            )
            apcer = next(
                (row.apcer_proxy for row in sweep if row.threshold == t), None
            )
            if apcer is not None:
                notes.append(f"APCER-proxy at this threshold: {apcer:.1%}")
            return {"threshold": t, "basis": "weak-labels", "notes": notes}
        notes.append(
            "no threshold satisfies the BPCER-proxy target — genuine and "
            "suspicious fused-score distributions overlap; do not enforce"
        )
        return {"threshold": None, "basis": "weak-labels", "notes": notes}

    population = minifasnet or scored
    if not minifasnet:
        notes.append(
            "no minifasnet_v2 sessions — every record is the capped fallback "
            "engine, so this calibrates model-free signals only"
        )
    best = None
    for t in THRESHOLDS:
        rate = sum(r["score"] >= t for r in population) / len(population)
        if rate >= 0.98:
            best = t
    if best is None:
        notes.append(
            "even the lowest candidate threshold rejects >2% of presumed-"
            "genuine traffic; distributions too low to enforce"
        )
        return {"threshold": None, "basis": "distribution-only", "notes": notes}
    notes.append(
        f"highest threshold passing >=98% of {len(population)} presumed-"
        "genuine sessions (no outcome labels; lower confidence)"
    )
    return {"threshold": best, "basis": "distribution-only", "notes": notes}


def weights_sanity(records: list[dict], corr: dict) -> list[str]:
    """Human-readable verdicts on the provisional fusion weights."""
    findings = []
    matrix = corr["matrix"]
    for comp in ("padMedian", "parallax", "moire", "challenge"):
        c = matrix.get(comp, {}).get("score")
        n = corr["counts"].get(comp, {}).get("score", 0)
        if c is None:
            findings.append(
                f"{comp}: correlation with fused score undefined "
                f"(n={n} or constant) — no evidence either way yet"
            )
        elif c < 0:
            findings.append(
                f"{comp}: NEGATIVE correlation with fused score ({c:+.2f}, n={n}) "
                "— component is pulling against the fusion; investigate before enforcing"
            )
        elif c < 0.1:
            findings.append(
                f"{comp}: near-zero correlation with fused score ({c:+.2f}, n={n}) "
                "— weight is effectively dead in this traffic"
            )
        else:
            findings.append(
                f"{comp}: correlates {c:+.2f} with fused score (n={n}) — weight direction OK"
            )
    pp = matrix.get("padMedian", {}).get("padMin")
    if pp is not None and pp < 0.5:
        findings.append(
            f"padMedian vs padMin correlation only {pp:+.2f}: frames disagree a lot "
            "— median-over-min is doing real work (expected), but check per-frame face detection"
        )
    return findings


def min_frames_policy(records: list[dict]) -> dict:
    """Parallax availability by uploaded frame count -> minimum-frames advice."""
    by_frames: dict[int, list[bool]] = {}
    for r in records:
        f = r.get("frames")
        if f is None:
            continue
        by_frames.setdefault(int(f), []).append(r.get("parallax") is not None)
    availability = {
        f: (sum(v) / len(v), len(v)) for f, v in sorted(by_frames.items())
    }
    recommended = None
    for f, (rate, n) in availability.items():
        if f >= 2 and rate >= _PARALLAX_AVAILABILITY_TARGET:
            recommended = f
            break
    return {
        "availability": availability,
        "recommended_min_frames": max(recommended or 3, 3),
        "note": (
            "parallax needs >=2 frames; recommendation is the smallest observed "
            f"frame count with >= {_PARALLAX_AVAILABILITY_TARGET:.0%} parallax availability, "
            "floored at 3 for one-bad-frame robustness"
        ),
    }


def enforcement_replay(records: list[dict], threshold: float | None, last_n: int) -> dict:
    """What LIVENESS_ENFORCE=true would have done to the last N sessions.

    Uses the top-level ``liveScore`` — exactly what inhouse.go compares
    (LivenessPassed = liveScore >= threshold). Fallback-engine sessions are
    capped at 0.5 by the engine and can therefore never pass a threshold
    above 0.5. Failing sessions would NOT have been auto-approved — they'd
    go to the officer referral queue (or be rejected, per Go-side policy).
    """
    scored = [r for r in records if r.get("liveScore") is not None]
    scored.sort(key=lambda r: (r.get("ts") or ""))
    window = scored[-last_n:]
    out = {"n": len(window), "at": {}}
    thresholds = {0.5: "current default (placeholder)"}
    if threshold is not None:
        thresholds[threshold] = "recommended"
    for t, label in sorted(thresholds.items()):
        fails = [r for r in window if r["liveScore"] < t]
        by_model: dict[str, int] = {}
        for r in fails:
            by_model[str(r.get("model"))] = by_model.get(str(r.get("model")), 0) + 1
        out["at"][t] = {
            "label": label,
            "would_fail": len(fails),
            "would_pass": len(window) - len(fails),
            "fails_by_model": by_model,
            "failed_sessions": [
                {
                    "ts": r.get("ts"),
                    "model": r.get("model"),
                    "liveScore": r.get("liveScore"),
                    "score": r.get("score"),
                    "padMin": r.get("padMin"),
                    "frames": r.get("frames"),
                    "weakLabel": r.get("weakLabel"),
                }
                for r in fails[:20]
            ],
        }
    return out


def analyze(records: list[dict], bpcer_target: float = DEFAULT_BPCER_TARGET,
            last_n: int = 200) -> dict:
    """Full analysis bundle consumed by report.render()."""
    sweep = threshold_sweep(records)
    corr = correlation_matrix(records)
    recommendation = recommend_threshold(records, sweep, bpcer_target)
    labeled = {
        "genuine": sum(1 for r in records if r.get("weakLabel") == "genuine"),
        "suspicious": sum(1 for r in records if r.get("weakLabel") == "suspicious"),
        "joined": sum(1 for r in records if r.get("outcomeStatus")),
    }
    distributions = {}
    for name in ("score", "padMedian", "padMin", "parallax", "moire"):
        vals = _values(records, name)
        distributions[name] = {
            "stats": summary_stats(vals),
            "hist": ascii_hist(vals, lo=0.0, hi=1.0),
        }
    for name in ("motionPx", "nonRigidityPx", "moirePeakRatio"):
        vals = _values(records, name)
        distributions[name] = {
            "stats": summary_stats(vals),
            "hist": ascii_hist(vals),
        }
    return {
        "n": len(records),
        "labeled": labeled,
        "distributions": distributions,
        "correlations": corr,
        "sweep": sweep,
        "operating_range": operating_range(sweep, bpcer_target),
        "bpcer_target": bpcer_target,
        "recommendation": recommendation,
        "weights_sanity": weights_sanity(records, corr),
        "min_frames": min_frames_policy(records),
        "enforcement": enforcement_replay(
            records, recommendation.get("threshold"), last_n
        ),
    }
