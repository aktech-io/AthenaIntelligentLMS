"""ISO 30107-3 metric math, against hand-computed cases."""
from __future__ import annotations

import pytest

from liveness_redteam import metrics as M


def attack(species: str, score: float) -> M.Presentation:
    return M.Presentation(is_attack=True, species=species, score=score)


def genuine(score: float) -> M.Presentation:
    return M.Presentation(is_attack=False, species=None, score=score)


# Hand-computed fixture, threshold 0.5 (accept when score >= 0.5):
#   print_flat   : 4 presentations, scores .10 .20 .30 .60 -> 1 accepted -> 25%
#   replay_phone : 2 presentations, scores .10 .40         -> 0 accepted ->  0%
#   cutout_paper : 4 presentations, scores .50 .55 .90 .95 -> 4 accepted -> 100%
#   genuine      : 5 presentations, scores .40 .60 .70 .80 .90 -> 1 rejected -> 20%
# worst species = cutout_paper at 100%; pooled APCER = 5/10 = 50%
HAND = [
    attack("print_flat", 0.10),
    attack("print_flat", 0.20),
    attack("print_flat", 0.30),
    attack("print_flat", 0.60),
    attack("replay_phone", 0.10),
    attack("replay_phone", 0.40),
    attack("cutout_paper", 0.50),
    attack("cutout_paper", 0.55),
    attack("cutout_paper", 0.90),
    attack("cutout_paper", 0.95),
    genuine(0.40),
    genuine(0.60),
    genuine(0.70),
    genuine(0.80),
    genuine(0.90),
]


def test_per_species_apcer_hand_computed():
    m = M.compute(HAND, 0.5)
    assert m.by_species["print_flat"].presentations == 4
    assert m.by_species["print_flat"].accepted == 1
    assert m.by_species["print_flat"].apcer == pytest.approx(0.25)
    assert m.by_species["replay_phone"].apcer == pytest.approx(0.0)
    assert m.by_species["cutout_paper"].apcer == pytest.approx(1.0)


def test_worst_species_is_the_reported_apcer_not_the_pooled_mean():
    m = M.compute(HAND, 0.5)
    assert m.worst_species == "cutout_paper"
    assert m.apcer == pytest.approx(1.0)
    assert m.apcer_pooled == pytest.approx(0.5)  # 5 accepted / 10 attacks


def test_bpcer_and_acer_hand_computed():
    m = M.compute(HAND, 0.5)
    assert m.genuine_count == 5
    assert m.genuine_rejected == 1  # only the 0.40 genuine falls below 0.5
    assert m.bpcer == pytest.approx(0.2)
    assert m.acer == pytest.approx((1.0 + 0.2) / 2)


def test_threshold_boundary_is_inclusive():
    # a score exactly at the threshold is ACCEPTED (matches the service's
    # `fusion.score >= _LIVE_THRESHOLD` labelling in engine/liveness.py)
    at_threshold = [attack("print_flat", 0.5)]
    assert M.compute(at_threshold, 0.5).attacks_accepted == 1
    assert M.compute(at_threshold, 0.5000001).attacks_accepted == 0


def test_raising_the_threshold_trades_apcer_for_bpcer():
    strict = M.compute(HAND, 0.96)
    assert strict.attacks_accepted == 0
    assert strict.apcer == pytest.approx(0.0)
    assert strict.genuine_rejected == 5
    assert strict.bpcer == pytest.approx(1.0)


def test_empty_inputs_do_not_divide_by_zero():
    m = M.compute([], 0.5)
    assert m.apcer == 0.0
    assert m.bpcer == 0.0
    assert m.worst_species is None


def test_genuine_only_run_has_no_species():
    m = M.compute([genuine(0.9), genuine(0.2)], 0.5)
    assert m.attack_count == 0
    assert m.by_species == {}
    assert m.bpcer == pytest.approx(0.5)


def test_sweep_covers_every_distinct_operating_point():
    points = M.sweep(HAND)
    thresholds = [p.threshold for p in points]
    # one per distinct score, plus the accept-nothing point above the max
    assert len(thresholds) == len({p.score for p in HAND}) + 1
    assert thresholds == sorted(thresholds)
    assert points[-1].attacks_accepted == 0
    assert points[-1].genuine_rejected == 5


def test_zero_apcer_threshold_is_just_above_the_best_attack_score():
    point = M.threshold_for_zero_apcer(HAND)
    assert point is not None
    assert point.apcer == pytest.approx(0.0)
    assert point.attacks_accepted == 0
    # best attack score is 0.95, so the cheapest zero-APCER threshold is the
    # accept-nothing point; every genuine at or below 0.95 is rejected too
    assert point.threshold > 0.95
    assert point.bpcer == pytest.approx(1.0)


def test_bpcer_at_apcer_1_percent():
    # allowing 1% APCER does not help here: cutout_paper has 4 presentations,
    # so its APCER is 0%, 25%, 50%, 75% or 100% — nothing between 0 and 25%
    assert M.bpcer_at_apcer(HAND, 0.01) == pytest.approx(1.0)

    # a species with 200 presentations and 1 acceptance is exactly 0.5% APCER
    wide = [attack("print_flat", 0.9)] + [
        attack("print_flat", 0.1) for _ in range(199)
    ] + [genuine(0.6) for _ in range(10)]
    point = M.operating_point_for_apcer(wide, 0.01)
    assert point.apcer == pytest.approx(0.005)
    assert point.bpcer == pytest.approx(0.0)  # all genuine at 0.6 still accepted


def test_missing_species_reports_uncovered_l1_repertoire():
    m = M.compute(HAND, 0.5)
    assert set(m.missing_species(("print_flat", "replay_monitor"))) == {
        "replay_monitor"
    }
    assert m.missing_species(("print_flat",)) == ()
