"""Liveness threshold-calibration tooling (docs/ekyc/06 §6 Stage 1).

Turns the ``ekyc.liveness`` shadow-mode fusion logs emitted by
api/face.py into a re-runnable threshold recommendation report — the
evidence base for flipping LIVENESS_ENFORCE=true (Go-side,
go-services/internal/compliance/ekyc/inhouse.go) with a calibrated
threshold instead of the 0.5 placeholder.

Dependencies: stdlib + numpy (matplotlib optional for PNG plots).
Deliberately NO pandas — the service image is lean and this must run
anywhere the service runs.

Usage:
    python -m tools.calibration run --logs <file|-> [--outcomes <csv|json>] \
        [--out reports/calibration-<date>.md]
"""
