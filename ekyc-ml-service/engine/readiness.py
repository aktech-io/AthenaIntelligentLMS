"""Readiness gating: declare which engine modes a deployment EXPECTS.

The engines already degrade safely when model files are missing (capped
fallback scores, UNKNOWN liveness), but a silently-degraded pod is still a
problem: fallback face scores can never auto-approve (every applicant lands
with a human) and shadow-mode liveness data collected from a fallback pod is
worthless for calibration. This module lets ops assert the intended mode:

    EXPECTED_FACE_ENGINE      sface | fallback   (empty/unset = don't care)
    EXPECTED_LIVENESS_ENGINE  minifasnet_v2 | fallback   (same semantics)

When set and the live engine mode differs, ``GET /health`` answers 503 with
a JSON body naming each mismatch — so a Kubernetes readinessProbe pointed at
/health keeps the degraded pod out of Service endpoints instead of letting
it quietly serve fallback verdicts. Unset vars preserve today's behavior
exactly (any mode is healthy). Pure stdlib; the env read and the comparison
are split so the comparison is unit-testable without FastAPI.
"""
from __future__ import annotations

import os

# Recognized modes per engine — a typo in the env var (e.g. "sfaace") should
# gate the pod out, not silently match nothing, so unknown values are kept
# and simply never equal the actual mode; they surface in the 503 body.
KNOWN_FACE_MODES = ("sface", "fallback")
KNOWN_LIVENESS_MODES = ("minifasnet_v2", "fallback")


def expected_engines() -> tuple[str | None, str | None]:
    """(expected_face, expected_liveness) from the environment; empty or
    unset values normalize to None ("don't care")."""
    face = os.getenv("EXPECTED_FACE_ENGINE", "").strip().lower() or None
    liveness = os.getenv("EXPECTED_LIVENESS_ENGINE", "").strip().lower() or None
    return face, liveness


def engine_mismatches(
    actual_face: str,
    actual_liveness: str,
    expected_face: str | None,
    expected_liveness: str | None,
) -> list[str]:
    """Human-readable mismatch descriptions, empty when the deployment is in
    its expected mode (or no expectation is declared)."""
    mismatches: list[str] = []
    if expected_face is not None and actual_face != expected_face:
        mismatches.append(
            f"faceEngine is '{actual_face}' but EXPECTED_FACE_ENGINE="
            f"'{expected_face}' — check FACE_DETECTOR_MODEL/"
            f"FACE_EMBEDDER_MODEL files (known modes: "
            f"{', '.join(KNOWN_FACE_MODES)})"
        )
    if expected_liveness is not None and actual_liveness != expected_liveness:
        mismatches.append(
            f"livenessEngine is '{actual_liveness}' but "
            f"EXPECTED_LIVENESS_ENGINE='{expected_liveness}' — check "
            f"FACE_LIVENESS_MODEL file (known modes: "
            f"{', '.join(KNOWN_LIVENESS_MODES)})"
        )
    return mismatches
