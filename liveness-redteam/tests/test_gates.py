"""L1 / L2 gate logic — the bars that decide whether we book the lab."""
from __future__ import annotations

import pytest

from liveness_redteam import metrics as M
from liveness_redteam import taxonomy


def attack(species: str, score: float) -> M.Presentation:
    return M.Presentation(is_attack=True, species=species, score=score)


def genuine(score: float) -> M.Presentation:
    return M.Presentation(is_attack=False, species=None, score=score)


def full_l1_battery(
    per_species: int = 100, attack_score: float = 0.1, genuine_score: float = 0.9
):
    """6 L1 species x per_species presentations + 100 genuine."""
    out = [
        attack(key, attack_score)
        for key in taxonomy.l1_species()
        for _ in range(per_species)
    ]
    out += [genuine(genuine_score) for _ in range(100)]
    return out


def check(gate: M.GateResult, needle: str) -> M.GateCheck:
    matches = [c for c in gate.checks if needle in c.name]
    assert matches, f"no check matching {needle!r} in {[c.name for c in gate.checks]}"
    return matches[0]


def test_l1_gate_passes_on_a_clean_600_presentation_battery():
    gate = M.l1_gate(full_l1_battery(), 0.5)
    assert gate.passed
    assert gate.metrics.attack_count == 600
    assert gate.metrics.apcer == 0.0
    assert gate.metrics.bpcer == 0.0
    assert gate.failures == []


def test_l1_gate_fails_on_a_single_accepted_attack():
    battery = full_l1_battery()
    battery[0] = attack("print_flat", 0.99)  # one spoof through
    gate = M.l1_gate(battery, 0.5)
    assert not gate.passed
    assert check(gate, "zero attacks accepted").passed is False
    assert gate.metrics.attacks_accepted == 1
    assert gate.metrics.worst_species == "print_flat"
    # 1/100 for that species — 1%, which L1 does not tolerate at all
    assert gate.metrics.apcer == pytest.approx(0.01)


def test_l1_gate_fails_below_500_attack_presentations():
    gate = M.l1_gate(full_l1_battery(per_species=10), 0.5)
    assert not gate.passed
    assert check(gate, "attack volume").passed is False
    # the acceptance check itself is still clean
    assert check(gate, "zero attacks accepted").passed is True


def test_l1_gate_fails_when_a_species_was_never_exercised():
    battery = [
        p
        for p in full_l1_battery()
        if p.species != "replay_monitor"
    ]
    battery += [attack("print_flat", 0.1) for _ in range(100)]  # keep N >= 500
    gate = M.l1_gate(battery, 0.5)
    assert not gate.passed
    coverage = check(gate, "species coverage")
    assert coverage.passed is False
    assert "replay_monitor" in coverage.detail


def test_l1_gate_fails_when_bpcer_exceeds_15_percent():
    battery = [
        attack(key, 0.1) for key in taxonomy.l1_species() for _ in range(100)
    ]
    # 20 of 100 genuine below threshold -> BPCER 20%
    battery += [genuine(0.9) for _ in range(80)]
    battery += [genuine(0.2) for _ in range(20)]
    gate = M.l1_gate(battery, 0.5)
    assert not gate.passed
    assert gate.metrics.bpcer == pytest.approx(0.2)
    assert check(gate, "BPCER").passed is False


def test_l1_gate_fails_with_no_genuine_presentations():
    battery = [
        attack(key, 0.1) for key in taxonomy.l1_species() for _ in range(100)
    ]
    gate = M.l1_gate(battery, 0.5)
    assert not gate.passed
    assert "unmeasured" in check(gate, "BPCER").detail


def test_l1_gate_recommends_a_threshold_that_would_pass():
    battery = full_l1_battery()
    battery[0] = attack("print_flat", 0.6)  # one acceptance at threshold 0.5
    gate = M.l1_gate(battery, 0.5)
    assert not gate.passed
    assert gate.recommended is not None
    assert gate.recommended.threshold > 0.6
    assert gate.recommended.attacks_accepted == 0
    # genuine sit at 0.9, so the recommended threshold keeps BPCER at 0
    assert gate.recommended.bpcer == pytest.approx(0.0)
    assert M.l1_gate(battery, gate.recommended.threshold).passed


def test_l2_gate_needs_mask_species_even_when_apcer_is_clean():
    gate = M.l2_gate(full_l1_battery(), 0.5)
    assert not gate.passed
    assert check(gate, "3D-mask").passed is False
    assert check(gate, "worst-species APCER").passed is True


def test_l2_gate_passes_at_1_percent_worst_species_apcer():
    battery = full_l1_battery()
    # 100 silicone-mask presentations, exactly 1 accepted -> 1.0% APCER
    battery += [attack("mask_silicone", 0.1) for _ in range(99)]
    battery += [attack("mask_silicone", 0.8)]
    gate = M.l2_gate(battery, 0.5)
    assert gate.passed
    assert gate.metrics.apcer == pytest.approx(0.01)
    # the same run fails L1, which tolerates zero acceptances
    assert not M.l1_gate(battery, 0.5).passed


def test_l2_gate_fails_just_above_1_percent():
    battery = full_l1_battery()
    battery += [attack("mask_latex", 0.1) for _ in range(98)]
    battery += [attack("mask_latex", 0.8) for _ in range(2)]  # 2/100 = 2%
    gate = M.l2_gate(battery, 0.5)
    assert not gate.passed
    assert gate.metrics.apcer == pytest.approx(0.02)
    assert check(gate, "worst-species APCER").passed is False


def test_gate_thresholds_match_the_documented_bars():
    assert M.L1_APCER_MAX == 0.0
    assert M.L2_APCER_MAX == 0.01
    assert M.BPCER_MAX == 0.15
    assert M.L1_MIN_ATTACKS == 500
