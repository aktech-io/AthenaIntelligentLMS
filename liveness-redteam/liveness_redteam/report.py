"""Markdown report for one red-team run.

Contains what a certification-readiness review actually needs: per-species
APCER with denominators, the BPCER side, gate verdicts with every failed
check named, the threshold sweep (what threshold *would* pass, since shadow
mode gives us scores for every presentation), and the trend against previous
runs of the same model version.

``badge_line()`` returns the one-liner for pasting into docs/ekyc/06.
"""
from __future__ import annotations

from dataclasses import dataclass

from . import metrics as M
from . import taxonomy
from .storage import PresentationRow, ResultsDB, RunRecord


def _pct(value: float | None) -> str:
    return "n/a" if value is None else f"{value * 100:.2f}%"


def _mark(passed: bool) -> str:
    return "PASS" if passed else "FAIL"


@dataclass
class RunView:
    """Everything the report needs about one run."""

    run: RunRecord
    rows: list[PresentationRow]
    presentations: list[M.Presentation]

    @property
    def errors(self) -> int:
        return sum(1 for r in self.rows if r.error)

    def metrics(self, threshold: float | None = None) -> M.Metrics:
        return M.compute(
            self.presentations,
            self.run.threshold if threshold is None else threshold,
        )


def load_run_view(db: ResultsDB, run_id: str) -> RunView:
    run = db.get_run(run_id)
    if run is None:
        raise KeyError(f"no such run: {run_id}")
    rows = db.presentations(run_id)
    presentations = [
        p for p in (r.to_presentation() for r in rows) if p is not None
    ]
    return RunView(run=run, rows=rows, presentations=presentations)


# ─── badge ───────────────────────────────────────────────────────────────────


def badge_line(view: RunView) -> str:
    """One-line status summary for the docs."""
    m = view.metrics()
    l1 = M.l1_gate(view.presentations, view.run.threshold)
    # naming a "worst" species is only meaningful once something got through
    worst = (
        (m.worst_species or "n/a") if m.attacks_accepted else "none accepted"
    )
    return (
        f"**PAD red-team {_mark(l1.passed)}** (L1 gate) — worst-species APCER "
        f"{_pct(m.apcer)} ({worst}) · BPCER {_pct(m.bpcer)} · "
        f"{m.attack_count} attack / {m.genuine_count} genuine presentations · "
        f"threshold {view.run.threshold:g} · model `{view.run.model_version}` · "
        f"run `{view.run.run_id}`"
    )


# ─── tables ──────────────────────────────────────────────────────────────────


def _species_table(m: M.Metrics) -> str:
    lines = [
        "| Species | Level | Presentations | Accepted | APCER |",
        "|---|---|---:|---:|---:|",
    ]
    for key in sorted(m.by_species):
        sm = m.by_species[key]
        spec = taxonomy.SPECIES.get(key)
        lines.append(
            f"| `{key}` | {sm.level or '?'} | {sm.presentations} | "
            f"{sm.accepted} | {_pct(sm.apcer)}{' **<- worst**' if key == m.worst_species else ''} |"
        )
        if spec is None:
            lines[-1] = lines[-1].replace("| ? |", "| (unknown species) |")
    if not m.by_species:
        lines.append("| _(no attack presentations)_ | | 0 | 0 | n/a |")
    lines.append(
        f"| **worst-species** | | {m.attack_count} | {m.attacks_accepted} | "
        f"**{_pct(m.apcer)}** |"
    )
    return "\n".join(lines)


def _coverage_note(m: M.Metrics) -> str:
    missing_l1 = m.missing_species(taxonomy.l1_species())
    masks = [s for s in taxonomy.l2_species() if s in m.by_species]
    parts = []
    if missing_l1:
        parts.append("L1 species not exercised: " + ", ".join(f"`{s}`" for s in missing_l1))
    else:
        parts.append("all L1 species exercised")
    parts.append(
        "3D-mask species present: " + ", ".join(f"`{s}`" for s in masks)
        if masks
        else "no 3D-mask species (L2 coverage outstanding)"
    )
    return " · ".join(parts)


def _gate_block(gate: M.GateResult) -> str:
    lines = [
        f"### {gate.gate} gate — **{_mark(gate.passed)}** "
        f"(threshold {gate.threshold:g})",
        "",
        "| Check | Result | Detail |",
        "|---|---|---|",
    ]
    for check in gate.checks:
        lines.append(f"| {check.name} | {_mark(check.passed)} | {check.detail} |")
    if gate.recommended is not None:
        rec = gate.recommended
        lines += [
            "",
            f"Recommended threshold for this gate: **{rec.threshold:.6f}** "
            f"-> APCER {_pct(rec.apcer)}, BPCER {_pct(rec.bpcer)} "
            f"({rec.genuine_rejected} genuine rejected, "
            f"{rec.attacks_accepted} attacks accepted).",
        ]
    else:
        lines += ["", "No threshold in this run's score range satisfies the gate."]
    return "\n".join(lines)


def _sweep_table(presentations, limit: int = 12) -> str:
    points = M.sweep(presentations)
    if not points:
        return "_No scored presentations — nothing to sweep._"
    if len(points) > limit:
        step = (len(points) - 1) / (limit - 1)
        picked_idx = sorted({int(round(i * step)) for i in range(limit)})
        points = [points[i] for i in picked_idx]
    lines = [
        "| Threshold | worst-species APCER | BPCER | Attacks accepted | Genuine rejected |",
        "|---:|---:|---:|---:|---:|",
    ]
    for p in points:
        lines.append(
            f"| {p.threshold:.6f} | {_pct(p.apcer)} | {_pct(p.bpcer)} | "
            f"{p.attacks_accepted} | {p.genuine_rejected} |"
        )
    return "\n".join(lines)


def _trend_table(db: ResultsDB, view: RunView, limit: int = 5) -> str:
    previous = db.runs_before(view.run.run_id, limit)
    lines = [
        "| Run | Started | Model version | Threshold | Attacks | worst APCER | BPCER | L1 |",
        "|---|---|---|---:|---:|---:|---:|---|",
    ]

    def row(v: RunView, marker: str = "") -> str:
        m = v.metrics()
        gate = M.l1_gate(v.presentations, v.run.threshold)
        return (
            f"| `{v.run.run_id}`{marker} | {v.run.started_at} | "
            f"`{v.run.model_version}` | {v.run.threshold:g} | {m.attack_count} | "
            f"{_pct(m.apcer)} | {_pct(m.bpcer)} | {_mark(gate.passed)} |"
        )

    lines.append(row(view, " **(this run)**"))
    for prev in previous:
        lines.append(row(load_run_view(db, prev.run_id)))
    if not previous:
        lines.append("| _(no earlier runs in this database)_ | | | | | | | |")
    note = ""
    changed = {p.model_version for p in previous} - {view.run.model_version}
    if changed:
        note = (
            "\n\n> Model version changed within this trend window "
            f"({', '.join(sorted(changed))}) — rows are **not** comparable as a "
            "single sustain series. The 0/500 sustain clock restarts on a model "
            "change (docs/ekyc/06 §2)."
        )
    return "\n".join(lines) + note


# ─── report ──────────────────────────────────────────────────────────────────


def build_report(db: ResultsDB, run_id: str, trend_limit: int = 5) -> str:
    view = load_run_view(db, run_id)
    run = view.run
    m = view.metrics()
    l1 = M.l1_gate(view.presentations, run.threshold)
    l2 = M.l2_gate(view.presentations, run.threshold)
    bpcer_at_1 = M.operating_point_for_apcer(view.presentations, M.L2_APCER_MAX)
    zero = M.threshold_for_zero_apcer(view.presentations)

    out = [
        f"# PAD red-team run `{run.run_id}`",
        "",
        badge_line(view),
        "",
        "## Run",
        "",
        "| | |",
        "|---|---|",
        f"| Started | {run.started_at} |",
        f"| Finished | {run.finished_at or '(incomplete)'} |",
        f"| Scorer | `{run.scorer}` -> `{run.target}` |",
        f"| Model version | `{run.model_version}` |",
        f"| Decision threshold | {run.threshold:g} (accept when liveScore >= threshold) |",
        f"| Frames per presentation | {run.frame_count} |",
        f"| Sessions root | `{run.sessions_root}` |",
        f"| Presentations scored | {len(view.presentations)} "
        f"({m.attack_count} attack / {m.genuine_count} genuine) |",
        f"| Errored presentations | {view.errors} (excluded from all rates) |",
    ]
    if run.notes:
        out.append(f"| Notes | {run.notes} |")
    out += [
        "",
        "## Attack presentations (APCER by species)",
        "",
        _species_table(m),
        "",
        _coverage_note(m),
        "",
        "## Genuine presentations (BPCER)",
        "",
        "| Genuine presentations | Rejected | BPCER | ACER (worst APCER + BPCER)/2 |",
        "|---:|---:|---:|---:|",
        f"| {m.genuine_count} | {m.genuine_rejected} | {_pct(m.bpcer)} | {_pct(m.acer)} |",
        "",
        "## Gates",
        "",
        _gate_block(l1),
        "",
        _gate_block(l2),
        "",
        "## Threshold sweep",
        "",
        "Shadow mode scores every presentation, so the full ROC is computable "
        "offline. Operating points of record:",
        "",
        f"- **Zero-APCER threshold**: "
        + (
            f"`{zero.threshold:.6f}` at BPCER {_pct(zero.bpcer)}"
            if zero
            else "unattainable in this run's score range"
        ),
        f"- **BPCER @ APCER<=1%**: "
        + (
            f"{_pct(bpcer_at_1.bpcer)} at threshold `{bpcer_at_1.threshold:.6f}`"
            if bpcer_at_1
            else "unattainable in this run's score range"
        ),
        "",
        _sweep_table(view.presentations),
        "",
        "## Trend",
        "",
        _trend_table(db, view, trend_limit),
        "",
        "---",
        "",
        "Generated by `liveness-redteam` "
        "(`python3 -m liveness_redteam report --run-id " + run.run_id + "`). "
        "APCER/BPCER follow ISO/IEC 30107-3: APCER is reported per attack "
        "species and gated on the worst species, never the pooled mean.",
        "",
    ]
    return "\n".join(out)


def write_report(db: ResultsDB, run_id: str, path: str) -> str:
    text = build_report(db, run_id)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(text)
    return text
