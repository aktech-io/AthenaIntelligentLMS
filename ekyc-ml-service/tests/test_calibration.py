"""tools/calibration tests — synthetic shadow-log fixtures.

Fixture lines replicate api/face.py exactly: ``"liveness fusion (shadow): "``
followed by the *Python repr* of the response body, under the prefixes seen
in the wild (bare, basicConfig, kubectl --timestamps). Two synthetic
populations: genuine-like (high PAD, parallax present, clean moiré) and
spoof-like (low PAD, rigid/zero parallax, screen-door moiré ratios).

Ingest/outcomes/report tests are stdlib-only; analysis tests importorskip
numpy, matching the suite's bare-box baseline (matplotlib is optional
everywhere — report generation must succeed without it).
"""
from __future__ import annotations

import json
import random
from pathlib import Path

import pytest

from tools.calibration import ingest, outcomes
from tools.calibration.ingest import parse_line, parse_stream

MARKER = "liveness fusion (shadow): "


# ─── fixture generation ──────────────────────────────────────────────────────


def make_body(live=True, model="minifasnet_v2", frames=5, challenge=None,
              rng=None, score=None):
    rng = rng or random.Random(0)
    if live:
        pad = [round(min(1.0, rng.uniform(0.75, 0.98)), 4) for _ in range(frames)]
        parallax = round(rng.uniform(0.4, 0.9), 4) if frames >= 2 else None
        moire, ratio = round(rng.uniform(0.85, 1.0), 4), round(rng.uniform(5, 20), 2)
    else:
        pad = [round(rng.uniform(0.02, 0.3), 4) for _ in range(frames)]
        parallax = round(rng.uniform(0.0, 0.15), 4) if frames >= 2 else None
        moire, ratio = round(rng.uniform(0.0, 0.3), 4), round(rng.uniform(60, 150), 2)
    pad_sorted = sorted(pad)
    pad_median = pad_sorted[frames // 2]
    weights = {"pad": 0.55, "parallax": 0.15, "moire": 0.15, "challenge": 0.15}
    comps = {"pad": pad_median, "parallax": parallax, "moire": moire,
             "challenge": None if challenge is None else float(challenge)}
    present = {k: v for k, v in comps.items() if v is not None}
    fused = sum(weights[k] * v for k, v in present.items()) / sum(
        weights[k] for k in present
    )
    fused = round(fused, 4) if score is None else score
    live_score = fused if model == "minifasnet_v2" else min(fused, 0.5)
    return {
        "liveScore": live_score,
        "label": "LIVE" if live_score >= 0.5 else "SPOOF",
        "perFrame": [{"score": p, "faceFound": p > 0.0} for p in pad],
        "model": model,
        "fusion": {
            "score": fused,
            "padMedian": pad_median,
            "padMin": min(pad),
            "parallax": parallax,
            "moire": moire,
            "challenge": comps["challenge"],
            "motionPx": None if parallax is None else round(rng.uniform(0.5, 5.0), 4),
            "nonRigidityPx": None if parallax is None else round(rng.uniform(0.1, 2.0), 4),
            "moirePeakRatio": ratio,
            "frames": frames,
            "version": 1,
        },
    }


def make_line(body, prefix="kubectl", ts="2026-08-01T09:00:00.123456789Z"):
    msg = MARKER + str(body)  # str(dict): exactly what %s renders
    if prefix == "bare":
        return msg
    if prefix == "basicconfig":
        return f"INFO:ekyc.liveness:{msg}"
    if prefix == "kubectl":
        return f"{ts} {msg}"
    raise ValueError(prefix)


def synth_lines(n_genuine=40, n_spoof=10, seed=7):
    rng = random.Random(seed)
    lines = []
    t0 = 0
    for i in range(n_genuine + n_spoof):
        live = i < n_genuine
        frames = rng.choice([1, 3, 5, 5, 5, 8])
        challenge = rng.choice([None, True, True]) if live else rng.choice([None, False])
        body = make_body(live=live, frames=frames, challenge=challenge, rng=rng)
        ts = f"2026-08-01T{9 + (t0 // 3600):02d}:{(t0 % 3600) // 60:02d}:{t0 % 60:02d}.000000000Z"
        t0 += 137
        lines.append(make_line(body, ts=ts))
    return lines


# ─── parser robustness ───────────────────────────────────────────────────────


def test_parse_line_prefixes():
    body = make_body()
    for prefix in ("bare", "basicconfig", "kubectl"):
        rec = parse_line(make_line(body, prefix=prefix))
        assert isinstance(rec, dict), prefix
        assert rec["liveScore"] == body["liveScore"]
        assert rec["padMin"] == body["fusion"]["padMin"]
        assert rec["frames"] == body["fusion"]["frames"]
    # only the kubectl prefix carries a timestamp
    assert parse_line(make_line(body, prefix="kubectl"))["ts"].startswith("2026-08-01T")
    assert parse_line(make_line(body, prefix="bare"))["ts"] is None


def test_parse_line_json_variant():
    body = make_body()
    line = MARKER + json.dumps(body)  # future-proofing: JSON payload
    rec = parse_line(line)
    assert rec["score"] == body["fusion"]["score"]


def test_parse_stream_skips_noise_and_counts_malformed():
    good = make_line(make_body())
    lines = [
        'INFO:     172.16.0.9 - "POST /v1/face/liveness HTTP/1.1" 200 OK',
        good,
        MARKER + "{'liveScore': 0.5, truncated...",  # malformed: marker, bad payload
        MARKER + "not even braces",  # malformed: no payload
        "totally unrelated line",
        good,  # duplicate: same ts + body
    ]
    result = parse_stream(lines)
    assert result.scanned == 6
    assert len(result.records) == 1  # dedupe collapsed the repeat
    assert result.malformed == 2


def test_parse_line_none_fields_survive():
    body = make_body(frames=1)  # parallax None
    rec = parse_line(make_line(body))
    assert rec["parallax"] is None
    assert rec["motionPx"] is None
    assert rec["challenge"] is None


def test_store_accumulates_and_dedupes(tmp_path):
    store = tmp_path / "records.jsonl"
    r1 = parse_stream(synth_lines(5, 2))
    all1, added1 = ingest.merge_into_store(r1.records, store)
    assert added1 == 7 and len(all1) == 7
    # overlapping second pull: 7 old + 3 new
    r2 = parse_stream(synth_lines(5, 2) + synth_lines(2, 1, seed=99))
    all2, added2 = ingest.merge_into_store(r2.records, store)
    assert added2 == 3 and len(all2) == 10
    assert len(ingest.load_store(store)) == 10


# ─── outcomes / weak labels ──────────────────────────────────────────────────


def test_weak_labels():
    assert outcomes.weak_label({"status": "APPROVED"}) == "genuine"
    assert outcomes.weak_label({"status": "AUTO_APPROVED"}) == "genuine"
    assert outcomes.weak_label(
        {"status": "REJECTED", "decisionReasons": "liveness spoof suspected"}
    ) == "suspicious"
    assert outcomes.weak_label(
        {"status": "REJECTED", "decisionReasons": "sanctions hit"}
    ) is None
    assert outcomes.weak_label({"status": "REFERRED"}) is None


def test_outcome_join_by_score_and_time(tmp_path):
    lines = synth_lines(4, 2)
    records = parse_stream(lines).records
    rows = []
    for i, rec in enumerate(records[:4]):
        rows.append({
            "application_id": f"app-{i}",
            "created_at": rec["ts"].replace("Z", "+00:00") if rec["ts"] else "",
            "decided_at": "",
            "status": "APPROVED" if rec["label"] == "LIVE" else "REJECTED",
            "risk_tier": "LOW",
            "decision_reasons": "" if rec["label"] == "LIVE" else "liveness concerns",
            "liveness_score": rec["liveScore"],
            "liveness_mode": "shadow",
            "liveness_provider": "inhouse",
        })
    csv_path = tmp_path / "outcomes.csv"
    header = list(rows[0])
    csv_path.write_text(
        ",".join(header) + "\n"
        + "\n".join(",".join(str(r[h]) for h in header) for r in rows)
    )
    loaded = outcomes.load_outcomes(csv_path)
    assert len(loaded) == 4
    joined = outcomes.join(records, loaded)
    assert joined == 4
    labeled = [r for r in records if r.get("weakLabel")]
    assert len(labeled) == 4
    assert records[0]["applicationId"] == "app-0"
    assert records[0]["livenessMode"] == "shadow"
    # outcome rows are consumed at most once
    assert sum(1 for r in records if r.get("applicationId") == "app-0") == 1


def test_outcome_join_respects_time_window():
    records = parse_stream(synth_lines(1, 0)).records
    far = [{
        "applicationId": "far",
        "createdAt": "2026-07-01T00:00:00+00:00",
        "createdAtEpoch": outcomes.parse_ts("2026-07-01T00:00:00+00:00"),
        "status": "APPROVED",
        "livenessScore": records[0]["liveScore"],
        "weakLabel": "genuine",
    }]
    assert outcomes.join(records, far, window_s=60) == 0


# ─── analysis math (numpy) ───────────────────────────────────────────────────


@pytest.fixture
def np():
    return pytest.importorskip("numpy")


def test_sweep_math_exact(np):
    from tools.calibration.analyze import threshold_sweep

    # hand-built: fused scores 0.2/0.4/0.8, padMin 0.1/0.6/0.7
    records = [
        {"score": 0.2, "padMin": 0.1, "model": "m", "frames": 5, "liveScore": 0.2},
        {"score": 0.4, "padMin": 0.6, "model": "m", "frames": 5, "liveScore": 0.4},
        {"score": 0.8, "padMin": 0.7, "model": "m", "frames": 5, "liveScore": 0.8},
    ]
    rows = {r.threshold: r for r in threshold_sweep(records, thresholds=[0.5])}
    row = rows[0.5]
    assert row.n == 3
    assert row.pass_rate == pytest.approx(1 / 3)  # only 0.8 >= 0.5
    assert row.old_pass_rate == pytest.approx(2 / 3)  # 0.6, 0.7 >= 0.5
    assert row.flips == 1  # the 0.4/0.6 record flips verdict


def test_sweep_bpcer_apcer_proxies(np):
    from tools.calibration.analyze import threshold_sweep

    records = (
        [{"score": 0.9, "padMin": 0.9, "weakLabel": "genuine", "model": "m",
          "frames": 5, "liveScore": 0.9} for _ in range(8)]
        + [{"score": 0.3, "padMin": 0.3, "weakLabel": "genuine", "model": "m",
            "frames": 5, "liveScore": 0.3} for _ in range(2)]
        + [{"score": 0.1, "padMin": 0.1, "weakLabel": "suspicious", "model": "m",
            "frames": 5, "liveScore": 0.1} for _ in range(4)]
        + [{"score": 0.7, "padMin": 0.7, "weakLabel": "suspicious", "model": "m",
            "frames": 5, "liveScore": 0.7}]
    )
    row = threshold_sweep(records, thresholds=[0.5])[0]
    assert row.bpcer_proxy == pytest.approx(2 / 10)  # 2 genuine below 0.5
    assert row.apcer_proxy == pytest.approx(1 / 5)  # 1 suspicious above 0.5


def test_verdict_flip_ab_on_populations(np):
    from tools.calibration.analyze import threshold_sweep

    records = parse_stream(synth_lines(30, 10)).records
    rows = threshold_sweep(records)
    # flips must match an independent recount at every threshold
    for row in rows:
        expected = sum(
            (r["padMin"] >= row.threshold) != (r["score"] >= row.threshold)
            for r in records
        )
        assert row.flips == expected
    # pass-rate is monotonically non-increasing in the threshold
    rates = [r.pass_rate for r in rows]
    assert all(a >= b for a, b in zip(rates, rates[1:]))


def test_recommendation_paths(np):
    from tools.calibration.analyze import analyze

    # tiny sample -> no recommendation
    few = parse_stream(synth_lines(3, 1)).records
    a = analyze(few)
    assert a["recommendation"]["threshold"] is None
    assert a["recommendation"]["basis"] == "insufficient-data"

    # adequate sample, no labels -> distribution-only
    many = parse_stream(synth_lines(60, 0, seed=3)).records
    a = analyze(many)
    assert a["recommendation"]["basis"] == "distribution-only"
    assert a["recommendation"]["threshold"] is not None

    # weak labels -> label-based, respecting the BPCER target
    labeled = parse_stream(synth_lines(50, 12, seed=5)).records
    for r in labeled:
        if r["score"] >= 0.5:
            r["weakLabel"] = "genuine"
        elif r["score"] <= 0.3:
            r["weakLabel"] = "suspicious"
    a = analyze(labeled)
    assert a["recommendation"]["basis"] == "weak-labels"
    t = a["recommendation"]["threshold"]
    assert t is not None
    genuine = [r for r in labeled if r.get("weakLabel") == "genuine"]
    bpcer = sum(r["score"] < t for r in genuine) / len(genuine)
    assert bpcer <= a["bpcer_target"]


def test_min_frames_policy(np):
    from tools.calibration.analyze import min_frames_policy

    records = parse_stream(synth_lines(40, 5, seed=11)).records
    policy = min_frames_policy(records)
    assert policy["recommended_min_frames"] >= 3
    single = policy["availability"].get(1)
    if single:
        assert single[0] == 0.0  # parallax impossible with one frame


def test_enforcement_replay_uses_livescore_not_fused(np):
    from tools.calibration.analyze import enforcement_replay

    # fallback engine: fused high but liveScore capped at 0.5
    records = [{
        "ts": f"2026-08-01T09:00:{i:02d}Z", "model": "fallback",
        "liveScore": 0.5, "score": 0.9, "padMin": 0.5, "frames": 5,
    } for i in range(5)]
    replay = enforcement_replay(records, threshold=0.6, last_n=200)
    assert replay["at"][0.5]["would_pass"] == 5  # 0.5 >= 0.5
    assert replay["at"][0.6]["would_fail"] == 5  # cap can never reach 0.6
    assert replay["at"][0.6]["fails_by_model"] == {"fallback": 5}


# ─── end-to-end report ───────────────────────────────────────────────────────


def test_cli_end_to_end(np, tmp_path):
    from tools.calibration.__main__ import main

    log = tmp_path / "shadow.log"
    log.write_text("\n".join(
        synth_lines(45, 10)
        + [MARKER + "{'liveScore': broken"]  # one malformed line
    ))
    out = tmp_path / "report.md"
    store = tmp_path / "store.jsonl"
    rc = main(["run", "--logs", str(log), "--out", str(out),
               "--store", str(store), "--no-plots"])
    assert rc == 0
    text = out.read_text()
    assert "# Liveness threshold calibration report" in text
    assert "malformed lines skipped: **1**" in text
    assert "What would LIVENESS_ENFORCE=true have done?" in text
    assert "Old vs new policy A/B" in text
    assert "Threshold sweep" in text
    assert "| 0.50 |" in text
    assert store.is_file()

    # rerun on the same logs: dedupe means no new records
    rc = main(["run", "--logs", str(log), "--out", str(out),
               "--store", str(store), "--no-plots"])
    assert rc == 0
    assert "New records this run: 0" in out.read_text()


def test_report_on_empty_input(np, tmp_path):
    from tools.calibration.__main__ import main

    log = tmp_path / "empty.log"
    log.write_text("nothing relevant\n")
    out = tmp_path / "report.md"
    rc = main(["run", "--logs", str(log), "--out", str(out), "--no-store",
               "--no-plots"])
    assert rc == 0
    assert "**0 sessions**" in out.read_text()
