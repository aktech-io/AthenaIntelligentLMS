"""ISO/IEC 30107-3 metrics and the certification gates.

Definitions used (30107-3 §terms, as the labs apply them):

* **APCER** — per **attack species**: the fraction of that species'
  presentations that the system classified as bona fide. The reported figure
  for a system is the **worst species**, never the pooled mean: pooling lets
  400 easy prints hide 5 accepted replays.
* **BPCER** — the fraction of genuine presentations rejected.
* **ACER** — ``(APCER + BPCER) / 2``, computed here against the worst-species
  APCER. Informative only; no lab gates on it.
* **BPCER @ APCER=1%** — the operating point a threshold sweep can reach: the
  lowest BPCER available while worst-species APCER stays <= 1%.

Decision rule: a presentation is *accepted* (classified bona fide) when
``live_score >= threshold``. Shadow mode gives us the score for every
presentation, so the whole ROC is computable offline and the rig can report
which threshold *would* pass each gate — that is the point of the sweep.

Gates:

* ``l1_gate`` — the internal bar before booking iBeta Level 1
  (docs/ekyc/06 §7, docs/nemo/09 §2): **0 accepted attacks** over
  ``min_attacks`` (default 500) presentations covering every L1 species, with
  BPCER <= 15%.
* ``l2_gate`` — worst-species APCER <= 1% including at least one 3D-mask
  species (docs/ekyc/06 §7b).
"""
from __future__ import annotations

from dataclasses import dataclass, field

from . import taxonomy

#: iBeta pass bars
L1_APCER_MAX = 0.0
L2_APCER_MAX = 0.01
BPCER_MAX = 0.15
L1_MIN_ATTACKS = 500  # internal sustain bar: "0/500" (docs/ekyc/06 §7)


@dataclass(frozen=True)
class Presentation:
    """One scored presentation — the unit of every metric here."""

    is_attack: bool
    species: str | None  # attack species key; None for genuine
    score: float
    label: str = ""
    session_id: str = ""
    clip: str = ""

    @property
    def level(self) -> str | None:
        spec = taxonomy.SPECIES.get(self.species or "")
        return spec.level if spec else None


@dataclass(frozen=True)
class SpeciesMetrics:
    species: str
    level: str | None
    presentations: int
    accepted: int

    @property
    def apcer(self) -> float:
        return self.accepted / self.presentations if self.presentations else 0.0


@dataclass
class Metrics:
    threshold: float
    attack_count: int
    genuine_count: int
    attacks_accepted: int
    genuine_rejected: int
    by_species: dict[str, SpeciesMetrics] = field(default_factory=dict)

    @property
    def apcer_pooled(self) -> float:
        """Pooled APCER. Reported for context only — gates use worst-species."""
        return (
            self.attacks_accepted / self.attack_count if self.attack_count else 0.0
        )

    @property
    def worst_species(self) -> str | None:
        """Species with the highest APCER; ties broken by name for stability."""
        if not self.by_species:
            return None
        worst = max(m.apcer for m in self.by_species.values())
        return sorted(
            s for s, m in self.by_species.items() if m.apcer == worst
        )[0]

    @property
    def apcer(self) -> float:
        """Worst-species APCER — the ISO reporting convention."""
        worst = self.worst_species
        return self.by_species[worst].apcer if worst else 0.0

    @property
    def bpcer(self) -> float:
        return (
            self.genuine_rejected / self.genuine_count if self.genuine_count else 0.0
        )

    @property
    def acer(self) -> float:
        return (self.apcer + self.bpcer) / 2.0

    @property
    def species_covered(self) -> tuple[str, ...]:
        return tuple(sorted(self.by_species))

    def missing_species(self, required: tuple[str, ...]) -> tuple[str, ...]:
        return tuple(s for s in required if s not in self.by_species)


def accepted(presentation: Presentation, threshold: float) -> bool:
    """Bona-fide classification: score at or above the decision threshold."""
    return presentation.score >= threshold


def compute(presentations, threshold: float) -> Metrics:
    """Full metric set at one threshold."""
    presentations = list(presentations)
    per_species: dict[str, list[int]] = {}
    attack_count = attacks_accepted = 0
    genuine_count = genuine_rejected = 0

    for p in presentations:
        if p.is_attack:
            key = p.species or "unspecified"
            attack_count += 1
            hit = 1 if accepted(p, threshold) else 0
            attacks_accepted += hit
            bucket = per_species.setdefault(key, [0, 0])
            bucket[0] += 1
            bucket[1] += hit
        else:
            genuine_count += 1
            if not accepted(p, threshold):
                genuine_rejected += 1

    by_species = {
        key: SpeciesMetrics(
            species=key,
            level=(
                taxonomy.SPECIES[key].level if key in taxonomy.SPECIES else None
            ),
            presentations=total,
            accepted=hits,
        )
        for key, (total, hits) in sorted(per_species.items())
    }
    return Metrics(
        threshold=threshold,
        attack_count=attack_count,
        genuine_count=genuine_count,
        attacks_accepted=attacks_accepted,
        genuine_rejected=genuine_rejected,
        by_species=by_species,
    )


# ─── threshold sweep ─────────────────────────────────────────────────────────


@dataclass(frozen=True)
class OperatingPoint:
    threshold: float
    apcer: float  # worst-species
    bpcer: float
    attacks_accepted: int
    genuine_rejected: int
    worst_species: str | None


def candidate_thresholds(presentations) -> list[float]:
    """Every threshold that produces a distinct classification.

    Each observed score is a candidate (accept >= score), plus one just above
    the maximum score — the operating point that accepts nothing, which is
    where a 0% APCER gate usually lives.
    """
    scores = sorted({round(float(p.score), 6) for p in presentations})
    if not scores:
        return []
    return scores + [scores[-1] + 1e-6]


def _point(presentations, threshold: float) -> OperatingPoint:
    m = compute(presentations, threshold)
    return OperatingPoint(
        threshold=threshold,
        apcer=m.apcer,
        bpcer=m.bpcer,
        attacks_accepted=m.attacks_accepted,
        genuine_rejected=m.genuine_rejected,
        worst_species=m.worst_species,
    )


def sweep(presentations) -> list[OperatingPoint]:
    """The full ROC over candidate thresholds, ascending."""
    presentations = list(presentations)
    return [_point(presentations, t) for t in candidate_thresholds(presentations)]


def operating_point_for_apcer(
    presentations, max_apcer: float
) -> OperatingPoint | None:
    """Lowest-BPCER threshold whose worst-species APCER <= ``max_apcer``.

    ``None`` when no threshold achieves it (only possible with max_apcer < 0,
    since the accept-nothing point always has APCER 0).
    """
    points = [p for p in sweep(presentations) if p.apcer <= max_apcer + 1e-12]
    if not points:
        return None
    return min(points, key=lambda p: (p.bpcer, -p.threshold))


def bpcer_at_apcer(presentations, max_apcer: float = 0.01) -> float | None:
    """BPCER @ APCER=1% (default) — None when unattainable."""
    point = operating_point_for_apcer(presentations, max_apcer)
    return None if point is None else point.bpcer


def threshold_for_zero_apcer(presentations) -> OperatingPoint | None:
    """Cheapest threshold that lets no attack through (the L1 shape)."""
    return operating_point_for_apcer(presentations, 0.0)


# ─── gates ───────────────────────────────────────────────────────────────────


@dataclass(frozen=True)
class GateCheck:
    name: str
    passed: bool
    detail: str


@dataclass
class GateResult:
    gate: str
    threshold: float
    passed: bool
    checks: list[GateCheck]
    metrics: Metrics
    recommended: OperatingPoint | None = None

    @property
    def failures(self) -> list[GateCheck]:
        return [c for c in self.checks if not c.passed]


def _pct(value: float) -> str:
    return f"{value * 100:.2f}%"


def _coverage_check(
    m: Metrics, required: tuple[str, ...], label: str
) -> GateCheck:
    missing = m.missing_species(required)
    return GateCheck(
        name=f"{label} species coverage",
        passed=not missing,
        detail=(
            f"all {len(required)} species present"
            if not missing
            else f"missing: {', '.join(missing)}"
        ),
    )


def _bpcer_check(m: Metrics, max_bpcer: float) -> GateCheck:
    if m.genuine_count == 0:
        return GateCheck(
            "BPCER <= " + _pct(max_bpcer),
            False,
            "no genuine presentations in the run — BPCER unmeasured",
        )
    return GateCheck(
        "BPCER <= " + _pct(max_bpcer),
        m.bpcer <= max_bpcer,
        f"{_pct(m.bpcer)} ({m.genuine_rejected}/{m.genuine_count} genuine rejected)",
    )


def l1_gate(
    presentations,
    threshold: float,
    *,
    min_attacks: int = L1_MIN_ATTACKS,
    max_bpcer: float = BPCER_MAX,
) -> GateResult:
    """Internal Level-1 bar: 0 accepted attacks over N>=500, all L1 species."""
    presentations = list(presentations)
    m = compute(presentations, threshold)
    checks = [
        GateCheck(
            f"attack volume >= {min_attacks}",
            m.attack_count >= min_attacks,
            f"{m.attack_count} attack presentations",
        ),
        GateCheck(
            "zero attacks accepted (APCER = 0%)",
            m.attacks_accepted == 0,
            f"{m.attacks_accepted} accepted; worst species "
            f"{m.worst_species or 'n/a'} at {_pct(m.apcer)}",
        ),
        _coverage_check(m, taxonomy.l1_species(), "L1"),
        _bpcer_check(m, max_bpcer),
    ]
    return GateResult(
        gate="L1",
        threshold=threshold,
        passed=all(c.passed for c in checks),
        checks=checks,
        metrics=m,
        recommended=threshold_for_zero_apcer(presentations),
    )


def l2_gate(
    presentations,
    threshold: float,
    *,
    min_attacks: int = L1_MIN_ATTACKS,
    max_apcer: float = L2_APCER_MAX,
    max_bpcer: float = BPCER_MAX,
) -> GateResult:
    """Internal Level-2 bar: worst-species APCER <= 1% including 3D masks."""
    presentations = list(presentations)
    m = compute(presentations, threshold)
    masks = [s for s in taxonomy.l2_species() if s in m.by_species]
    checks = [
        GateCheck(
            f"attack volume >= {min_attacks}",
            m.attack_count >= min_attacks,
            f"{m.attack_count} attack presentations",
        ),
        GateCheck(
            f"worst-species APCER <= {_pct(max_apcer)}",
            m.apcer <= max_apcer,
            f"{_pct(m.apcer)} (worst: {m.worst_species or 'n/a'}, "
            f"{m.attacks_accepted} accepted overall)",
        ),
        _coverage_check(m, taxonomy.l1_species(), "L1"),
        GateCheck(
            "3D-mask species present",
            bool(masks),
            ", ".join(masks) if masks else "no mask species in this run",
        ),
        _bpcer_check(m, max_bpcer),
    ]
    return GateResult(
        gate="L2",
        threshold=threshold,
        passed=all(c.passed for c in checks),
        checks=checks,
        metrics=m,
        recommended=operating_point_for_apcer(presentations, max_apcer),
    )
