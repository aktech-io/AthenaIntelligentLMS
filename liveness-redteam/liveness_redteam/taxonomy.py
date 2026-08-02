"""Presentation-attack taxonomy for the internal PAD red-team rig.

Species are the unit ISO/IEC 30107-3 reports APCER against: the standard's
pass/fail convention is **worst-species** APCER, not the pooled average, so
every species carries its own denominator through the whole rig
(metrics.py -> report.py).

Levels map onto the iBeta test we are buying:

* **Level 1** (docs/nemo/09 §2, docs/ekyc/06 §7) — ~900 presentations across
  the cheap 2D repertoire: prints, screen replays, paper cutouts. Pass bar
  **0% APCER** with BPCER <=15%. This is the repertoire the rig replicates
  today, and the internal gate is 0 accepted attacks over N>=500.
* **Level 2** (docs/ekyc/06 §7b) — adds 3D artefacts (silicone / latex /
  resin masks) with a pass bar of **APCER <=1%**. Mask fabrication starts
  during the L1 lab window, so the species are schema-ready here and marked
  ``status="future"`` until real mask sessions exist.

``mask_3d`` is the generic 3D-mask value carried by the shared NLD-EA
manifest contract; the three material-specific keys are the red-team
superset used once fabrication distinguishes them.
"""
from __future__ import annotations

from dataclasses import dataclass

LEVEL_1 = "L1"
LEVEL_2 = "L2"

#: manifest ``type`` values
GENUINE = "genuine"
ATTACK = "attack"
PRESENTATION_TYPES = (GENUINE, ATTACK)

STATUS_ACTIVE = "active"
STATUS_FUTURE = "future"  # schema-ready, no capture protocol running yet


@dataclass(frozen=True)
class AttackSpecies:
    """One attack species: what it is, how it is built, how it is captured."""

    key: str
    label: str
    level: str
    category: str  # "print" | "replay" | "cutout" | "mask"
    status: str
    description: str
    materials: str
    capture_notes: str

    @property
    def is_future(self) -> bool:
        return self.status == STATUS_FUTURE


_SPECIES: tuple[AttackSpecies, ...] = (
    AttackSpecies(
        key="print_flat",
        label="Flat print",
        level=LEVEL_1,
        category="print",
        status=STATUS_ACTIVE,
        description=(
            "Matte or glossy photographic print of the subject's face held "
            "flat in the capture frame."
        ),
        materials=(
            "A4/A5 photo paper, matte and glossy stock, >=300 dpi lab print "
            "of a frontal portrait at life size."
        ),
        capture_notes=(
            "Hold rigid (mounted on card) and fill the guide oval. Capture "
            "under all three lighting stations; glossy stock must also be "
            "captured with a deliberate specular highlight off-frame."
        ),
    ),
    AttackSpecies(
        key="print_curved",
        label="Curved print",
        level=LEVEL_1,
        category="print",
        status=STATUS_ACTIVE,
        description=(
            "The same print bent around a cylinder to fake facial curvature "
            "and defeat naive flatness/parallax heuristics."
        ),
        materials="Flat print wrapped on a 80-120 mm mug/tube former.",
        capture_notes=(
            "Vary the curvature radius across takes and add a small hand "
            "wobble — a rigid curved print is the easiest curved variant."
        ),
    ),
    AttackSpecies(
        key="replay_phone",
        label="Phone-screen replay",
        level=LEVEL_1,
        category="replay",
        status=STATUS_ACTIVE,
        description=(
            "Genuine selfie clip replayed on a handset screen, including "
            "challenge-replay takes (a recorded blink/turn played back)."
        ),
        materials=(
            "2 budget (Tecno/Redmi) + 2 mid-range (Samsung A / Camon) "
            "handsets from the NLD-EA device fleet, brightness at max."
        ),
        capture_notes=(
            "Vary attacker-screen brightness and distance; the moire "
            "sub-score is distance-sensitive, so include a near-field take "
            "where the pixel grid is not resolvable."
        ),
    ),
    AttackSpecies(
        key="replay_tablet",
        label="Tablet-screen replay",
        level=LEVEL_1,
        category="replay",
        status=STATUS_ACTIVE,
        description=(
            "Replay on a ~10\" tablet: life-size face, lower pixel density "
            "than a phone, so moire and screen-bezel cues differ."
        ),
        materials="10-11\" Android tablet or iPad, brightness at max.",
        capture_notes=(
            "Life-size rendering is the point — scale the clip so the face "
            "matches real head size at the capture distance."
        ),
    ),
    AttackSpecies(
        key="replay_monitor",
        label="Monitor replay",
        level=LEVEL_1,
        category="replay",
        status=STATUS_ACTIVE,
        description=(
            "Replay on a desktop monitor: largest, brightest and lowest-DPI "
            "replay surface, strongest moire signature."
        ),
        materials="24-27\" IPS monitor, matte and glossy panels if available.",
        capture_notes=(
            "Include an off-axis take (~20-30 deg) — panel viewing-angle "
            "colour shift is a strong cue we must not accidentally rely on."
        ),
    ),
    AttackSpecies(
        key="cutout_paper",
        label="Paper cutout",
        level=LEVEL_1,
        category="cutout",
        status=STATUS_ACTIVE,
        description=(
            "Print with the eye and/or mouth regions cut out and a live "
            "attacker's eyes/mouth behind it — defeats blink and "
            "mouth-movement challenges while the rest of the face is flat."
        ),
        materials="Flat print, scalpel, optional card former for curvature.",
        capture_notes=(
            "Capture three variants: eyes-only, mouth-only, eyes+mouth. "
            "Always run the active challenge on these — they exist to beat "
            "it, so 'challenge passed' must not be sufficient on its own."
        ),
    ),
    AttackSpecies(
        key="mask_silicone",
        label="Silicone mask",
        level=LEVEL_2,
        category="mask",
        status=STATUS_FUTURE,
        description=(
            "Custom-cast platinum silicone mask of a consented NLD-EA "
            "subject: realistic skin translucency and 3D geometry."
        ),
        materials=(
            "Life-cast + platinum silicone, flocked hair, painted. Fabricate "
            "during the L1 lab window (docs/ekyc/06 §7b)."
        ),
        capture_notes=(
            "Worn (not held) takes only — a worn mask moves non-rigidly and "
            "is the honest L2 threat model."
        ),
    ),
    AttackSpecies(
        key="mask_latex",
        label="Latex mask",
        level=LEVEL_2,
        category="mask",
        status=STATUS_FUTURE,
        description=(
            "Latex/rubber mask: cheaper, matte, less translucent than "
            "silicone — different reflectance failure mode."
        ),
        materials="Cast latex mask, optionally airbrushed for skin tone.",
        capture_notes="Worn takes; include a commercial off-the-shelf mask.",
    ),
    AttackSpecies(
        key="mask_resin",
        label="Resin / rigid mask",
        level=LEVEL_2,
        category="mask",
        status=STATUS_FUTURE,
        description=(
            "3D-printed or cast rigid resin mask: accurate geometry, rigid "
            "motion, no skin translucency."
        ),
        materials="Photogrammetry scan -> resin print, painted.",
        capture_notes=(
            "Rigid motion makes the parallax non-rigidity sub-score the "
            "primary defence — capture both worn and held-on-a-stand takes."
        ),
    ),
    AttackSpecies(
        key="mask_3d",
        label="3D mask (unspecified material)",
        level=LEVEL_2,
        category="mask",
        status=STATUS_FUTURE,
        description=(
            "Generic 3D-mask value carried by the shared NLD-EA manifest "
            "contract, for captures where the material is not recorded."
        ),
        materials="Any of silicone / latex / resin.",
        capture_notes=(
            "Prefer the material-specific keys when known — worst-species "
            "APCER is only meaningful at material granularity."
        ),
    ),
)

SPECIES: dict[str, AttackSpecies] = {s.key: s for s in _SPECIES}

#: Values the shared NLD-EA manifest contract enumerates for ``attackType``.
CONTRACT_ATTACK_TYPES: tuple[str, ...] = (
    "print_flat",
    "print_curved",
    "replay_phone",
    "replay_monitor",
    "cutout_paper",
    "mask_3d",
)

#: Superset the red-team rig also accepts (documented in the README).
EXTRA_ATTACK_TYPES: tuple[str, ...] = (
    "replay_tablet",
    "mask_silicone",
    "mask_latex",
    "mask_resin",
)

ATTACK_TYPES: tuple[str, ...] = CONTRACT_ATTACK_TYPES + EXTRA_ATTACK_TYPES

LIGHTING = ("daylight", "indoor", "low_light")
CHALLENGES = ("blink", "turn_left", "turn_right", "smile")
SKIN_TONES = tuple(f"monk_{i:02d}" for i in range(1, 11))


def species(key: str) -> AttackSpecies:
    try:
        return SPECIES[key]
    except KeyError:
        raise KeyError(
            f"unknown attack species {key!r}; known: {', '.join(sorted(SPECIES))}"
        ) from None


def all_species(level: str | None = None) -> tuple[AttackSpecies, ...]:
    out = tuple(SPECIES.values())
    if level is not None:
        out = tuple(s for s in out if s.level == level)
    return out


def species_keys(level: str | None = None) -> tuple[str, ...]:
    return tuple(s.key for s in all_species(level))


def l1_species() -> tuple[str, ...]:
    """Species the Level-1 gate demands coverage of."""
    return species_keys(LEVEL_1)


def l2_species() -> tuple[str, ...]:
    """3D-mask species (Level 2). Any one of these satisfies L2 coverage."""
    return species_keys(LEVEL_2)


def is_attack_type(value: str) -> bool:
    return value in SPECIES


def level_of(key: str) -> str:
    return species(key).level
