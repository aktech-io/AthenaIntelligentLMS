"""Hand-computed checks for the ISO 30107-3 metric implementations."""
import math

import pytest

from liveness_training.eval.metrics import (
    apcer_per_type,
    bpcer,
    bpcer_at_apcer,
    compute_pad_metrics,
)
from liveness_training.eval.report import render_report

# 4 bona fide, 4 print attacks, 2 replay attacks
SCORES = [0.9, 0.8, 0.4, 0.95,   0.6, 0.2, 0.1, 0.3,   0.7, 0.05]
LABELS = [1, 1, 1, 1,            0, 0, 0, 0,           0, 0]
ATTACKS = ["", "", "", "",
           "print_flat", "print_flat", "print_flat", "print_flat",
           "replay_phone", "replay_phone"]
TONES = ["monk_02", "monk_02", "monk_09", "monk_09", "", "", "", "", "", ""]


def test_apcer_per_type_at_half():
    per = apcer_per_type(SCORES, LABELS, ATTACKS, threshold=0.5)
    assert per["print_flat"] == pytest.approx(0.25)   # only 0.6 accepted
    assert per["replay_phone"] == pytest.approx(0.5)  # 0.7 accepted, 0.05 not


def test_bpcer_at_half():
    assert bpcer(SCORES, LABELS, threshold=0.5) == pytest.approx(0.25)  # 0.4 rejected


def test_headline_metrics():
    m = compute_pad_metrics(SCORES, LABELS, ATTACKS, TONES, threshold=0.5)
    assert m["apcer_max"] == pytest.approx(0.5)
    assert m["acer"] == pytest.approx((0.5 + 0.25) / 2)
    assert m["n_bona_fide"] == 4 and m["n_attack"] == 6


def test_bpcer_at_apcer_target():
    # to push every attack below 1% APCER the threshold must exceed 0.7
    # (the highest-scoring attack) -> bona fides 0.4 and 0.7>0.4... scores
    # >= thr accepted: thr just above 0.7 rejects bona-fide 0.4 only -> BPCER 0.25
    b, thr = bpcer_at_apcer(SCORES, LABELS, ATTACKS, target_apcer=0.01)
    assert thr > 0.7
    assert b == pytest.approx(0.25)


def test_bpcer_at_apcer_unreachable():
    b, thr = bpcer_at_apcer([1.0, 1.0], [1, 0], ["", "print_flat"], 0.01)
    # attack scores exactly 1.0; threshold above max still qualifies -> reachable
    assert not math.isnan(b)
    # but with no bona fide at all it is nan
    b2, _ = bpcer_at_apcer([0.2], [0], ["print_flat"], 0.01)
    assert math.isnan(b2)


def test_skin_tone_breakdown_and_report():
    m = compute_pad_metrics(SCORES, LABELS, ATTACKS, TONES, threshold=0.5)
    # monk_02 bona fides: 0.9, 0.8 -> BPCER 0 ; monk_09: 0.4, 0.95 -> BPCER 0.5
    assert m["bpcer_per_skin_tone"]["monk_02"] == pytest.approx(0.0)
    assert m["bpcer_per_skin_tone"]["monk_09"] == pytest.approx(0.5)
    md = render_report(m, context={"model": "unit-test"})
    assert "APCER" in md and "monk_09" in md and "| print_flat |" in md


def test_report_without_tones():
    m = compute_pad_metrics(SCORES, LABELS, ATTACKS, None, threshold=0.5)
    md = render_report(m)
    assert "No skin-tone labels" in md
