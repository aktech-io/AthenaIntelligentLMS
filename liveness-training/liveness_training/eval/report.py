"""Markdown evaluation-report writer for compute_pad_metrics output."""
from __future__ import annotations

import datetime as _dt
import math
from pathlib import Path


def _pct(v: float) -> str:
    return "n/a" if v is None or (isinstance(v, float) and math.isnan(v)) else f"{v * 100:.2f}%"


def render_report(metrics: dict, title: str = "Liveness PAD evaluation",
                  context: dict | None = None) -> str:
    lines = [f"# {title}", ""]
    lines.append(f"Generated: {_dt.datetime.now(_dt.timezone.utc).isoformat(timespec='seconds')}")
    lines.append("")
    for k, v in (context or {}).items():
        lines.append(f"- **{k}**: {v}")
    if context:
        lines.append("")

    lines += [
        "## Headline (ISO/IEC 30107-3)",
        "",
        "| Metric | Value |",
        "|---|---|",
        f"| Bona-fide presentations | {metrics['n_bona_fide']} |",
        f"| Attack presentations | {metrics['n_attack']} |",
        f"| Decision threshold | {metrics['threshold']:.4f} |",
        f"| APCER (max over species) | {_pct(metrics['apcer_max'])} |",
        f"| BPCER | {_pct(metrics['bpcer'])} |",
        f"| ACER | {_pct(metrics['acer'])} |",
        (
            f"| **BPCER @ APCER≤{metrics['target_apcer'] * 100:.0f}%** "
            f"(L2 operating point) | **{_pct(metrics['bpcer_at_target_apcer'])}** "
            f"(thr {metrics['threshold_at_target_apcer']:.4f}) |"
            if not math.isnan(metrics["bpcer_at_target_apcer"])
            else f"| **BPCER @ APCER≤{metrics['target_apcer'] * 100:.0f}%** "
            "(L2 operating point) | **unreachable** — some attack species "
            "cannot be pushed under target at any threshold |"
        ),
        "",
        "## APCER per attack species",
        "",
        "| Attack type | APCER |",
        "|---|---|",
    ]
    per = metrics.get("apcer_per_type", {})
    if per:
        for at, v in sorted(per.items()):
            lines.append(f"| {at} | {_pct(v)} |")
    else:
        lines.append("| _no attack presentations in this split_ | — |")
    lines.append("")

    tones = metrics.get("bpcer_per_skin_tone", {})
    lines += ["## BPCER per skin tone (Monk scale)", ""]
    if tones:
        lines += ["| Skin tone | BPCER |", "|---|---|"]
        for tone, v in sorted(tones.items()):
            lines.append(f"| {tone} | {_pct(v)} |")
        lines += [
            "",
            "_The East-African calibration check: BPCER must stay flat across "
            "tones — a rising dark-tone BPCER is the MiniFASNet failure mode "
            "this pipeline exists to retire (doc 06 §5)._",
        ]
    else:
        lines.append("_No skin-tone labels in this split "
                     "(public bootstrap data carries none; NLD-EA does)._")
    lines.append("")
    return "\n".join(lines)


def write_report(metrics: dict, path, title: str = "Liveness PAD evaluation",
                 context: dict | None = None) -> Path:
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(render_report(metrics, title, context), encoding="utf-8")
    return p
