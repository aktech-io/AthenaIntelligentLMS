"""Markdown calibration report renderer (+ optional matplotlib PNGs).

Writes into tools/calibration/reports/ — deliberately NOT named ``data/`` or
``logs/``: the repo .gitignore swallows ``**/data/`` and ``**/logs/``
wholesale (it has eaten source twice already, per the comment in .gitignore),
so reports live in a directory those globs cannot touch and a .gitkeep pins
it.
"""
from __future__ import annotations

from datetime import datetime, timezone
from pathlib import Path

REPORTS_DIR = Path(__file__).resolve().parent / "reports"

# chart colors: categorical steps 1-2 of the validated default dataviz
# palette (light-mode surface #fcfcfb, primary/secondary ink)
_BLUE = "#2a78d6"
_ORANGE = "#eb6834"
_SURFACE = "#fcfcfb"
_INK = "#0b0b0b"
_MUTED = "#52514e"


def _fmt(v, digits=3):
    if v is None:
        return "—"
    if isinstance(v, float):
        return f"{v:.{digits}f}"
    return str(v)


def _pct(v):
    return "—" if v is None else f"{v:.1%}"


def _stats_row(name: str, stats: dict | None) -> str:
    if stats is None:
        return f"| {name} | 0 | — | — | — | — | — | — |"
    return (
        f"| {name} | {stats['n']} | {_fmt(stats['mean'])} | {_fmt(stats['median'])} "
        f"| {_fmt(stats['p05'])} | {_fmt(stats['p95'])} | {_fmt(stats['min'])} "
        f"| {_fmt(stats['max'])} |"
    )


def render_plots(records: list[dict], out_dir: Path, basename: str) -> list[str]:
    """Optional PNGs (fused-score histogram; padMin vs padMedian scatter).

    Returns relative image paths; empty when matplotlib is unavailable or
    there is nothing to plot.
    """
    try:
        import matplotlib

        matplotlib.use("Agg")
        import matplotlib.pyplot as plt
    except Exception:
        return []

    fused = [r["score"] for r in records if r.get("score") is not None]
    pairs = [
        (r["padMin"], r["padMedian"])
        for r in records
        if r.get("padMin") is not None and r.get("padMedian") is not None
    ]
    if not fused:
        return []

    def _style(ax, title):
        ax.set_facecolor(_SURFACE)
        ax.figure.set_facecolor(_SURFACE)
        ax.set_title(title, color=_INK, fontsize=11, loc="left")
        ax.tick_params(colors=_MUTED, labelsize=8)
        for spine in ("top", "right"):
            ax.spines[spine].set_visible(False)
        for spine in ("left", "bottom"):
            ax.spines[spine].set_color(_MUTED)
        ax.grid(axis="y", color=_MUTED, alpha=0.15, linewidth=0.6)
        ax.set_axisbelow(True)

    images = []
    fig, ax = plt.subplots(figsize=(6, 3.2), dpi=140)
    ax.hist(fused, bins=20, range=(0, 1), color=_BLUE, edgecolor=_SURFACE, linewidth=1)
    _style(ax, "Fused liveness score — all shadow sessions")
    ax.set_xlabel("fused score", color=_MUTED, fontsize=9)
    ax.set_ylabel("sessions", color=_MUTED, fontsize=9)
    name = f"{basename}-fused-hist.png"
    fig.tight_layout()
    fig.savefig(out_dir / name)
    plt.close(fig)
    images.append(name)

    if pairs:
        fig, ax = plt.subplots(figsize=(4.6, 4.4), dpi=140)
        xs, ys = zip(*pairs)
        ax.plot([0, 1], [0, 1], color=_MUTED, alpha=0.4, linewidth=1, linestyle="--")
        ax.scatter(xs, ys, s=26, color=_BLUE, alpha=0.75, edgecolors=_SURFACE, linewidths=0.8)
        _style(ax, "Old policy (padMin) vs new (padMedian)")
        ax.set_xlabel("padMin — old min-frame policy", color=_MUTED, fontsize=9)
        ax.set_ylabel("padMedian — fusion input", color=_MUTED, fontsize=9)
        ax.set_xlim(-0.02, 1.02)
        ax.set_ylim(-0.02, 1.02)
        name = f"{basename}-pad-ab.png"
        fig.tight_layout()
        fig.savefig(out_dir / name)
        plt.close(fig)
        images.append(name)
    return images


def render(analysis: dict, provenance: dict, records: list[dict],
           out_path: Path, plots: bool = True) -> Path:
    """Write the markdown report (and PNGs beside it). Returns the path."""
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    now = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    images = (
        render_plots(records, out_path.parent, out_path.stem) if plots else []
    )

    L: list[str] = []
    add = L.append
    add("# Liveness threshold calibration report")
    add("")
    add(f"Generated {now} by `tools/calibration` — re-run as shadow traffic accumulates.")
    add("")
    add("## 1. Data")
    add("")
    add(f"- Log source: `{provenance.get('logs', '—')}`")
    add(f"- Lines scanned: {provenance.get('scanned', 0)}, "
        f"fusion records parsed: {provenance.get('parsed', 0)}, "
        f"malformed lines skipped: **{provenance.get('malformed', 0)}**")
    add(f"- New records this run: {provenance.get('added', 0)}; "
        f"analysis covers **{analysis['n']} sessions** "
        f"(store: `{provenance.get('store', 'not persisted')}`)")
    lab = analysis["labeled"]
    add(f"- Outcome join: {lab['joined']} joined "
        f"({lab['genuine']} weak-genuine, {lab['suspicious']} weak-suspicious)"
        + (f" from `{provenance['outcomes']}`" if provenance.get("outcomes") else
           " — no outcomes file supplied"))
    add("")
    add("### Caveats")
    add("")
    add("- **Shadow mode**: nothing here changed any verdict; this report is the "
        "calibration evidence docs/nemo/08-liveness-plan.md requires before "
        "LIVENESS_ENFORCE=true.")
    add("- **Weak labels are proxies**: officer-approved referrals ~ genuine, "
        "officer-rejected-with-liveness-reasons ~ suspicious. Officers are not a "
        "PAD ground truth — BPCER/APCER figures below are *proxies*, not lab "
        "measurements (the certification bar of docs/ekyc/06 §7 is measured "
        "differently).")
    if analysis["n"] < 30:
        add(f"- **Tiny sample ({analysis['n']} sessions)**: every number below is "
            "indicative only. Do not act on this report; re-run later.")
    add("")

    add("## 2. Distributions")
    add("")
    add("| metric | n | mean | median | p05 | p95 | min | max |")
    add("|---|---|---|---|---|---|---|---|")
    for name, d in analysis["distributions"].items():
        add(_stats_row(name, d["stats"]))
    add("")
    for name, d in analysis["distributions"].items():
        if d["stats"] is None:
            continue
        add(f"<details><summary>{name} histogram</summary>")
        add("")
        add("```")
        add(d["hist"])
        add("```")
        add("</details>")
        add("")
    for img in images:
        add(f"![{img}]({img})")
        add("")

    add("## 3. Old vs new policy A/B (padMin vs fused)")
    add("")
    add("The old policy decided on the *minimum* per-frame PAD score; the fusion "
        "decides on the fused score (median PAD + parallax + moiré + challenge). "
        "`flips` counts sessions whose verdict differs between the two at each "
        "threshold.")
    add("")
    add("| threshold | old pass-rate (padMin) | new pass-rate (fused) | verdict flips |")
    add("|---|---|---|---|")
    for row in analysis["sweep"]:
        add(f"| {row.threshold:.2f} | {_pct(row.old_pass_rate)} | "
            f"{_pct(row.pass_rate if row.n else None)} | {row.flips} |")
    add("")

    add("## 4. Component correlations (fusion-weight sanity)")
    add("")
    corr = analysis["correlations"]
    fields = corr["fields"]
    add("| | " + " | ".join(fields) + " |")
    add("|---" * (len(fields) + 1) + "|")
    for f1 in fields:
        cells = [
            "—" if corr["matrix"][f1][f2] is None else f"{corr['matrix'][f1][f2]:+.2f}"
            for f2 in fields
        ]
        add(f"| **{f1}** | " + " | ".join(cells) + " |")
    add("")
    for finding in analysis["weights_sanity"]:
        add(f"- {finding}")
    add("")

    add("## 5. Threshold sweep")
    add("")
    add(f"BPCER-proxy target: **{analysis['bpcer_target']:.0%}** "
        "(share of weak-genuine sessions a threshold would reject).")
    add("")
    add("| threshold | pass-rate | BPCER-proxy | APCER-proxy | minifasnet pass | fallback pass |")
    add("|---|---|---|---|---|---|")
    for row in analysis["sweep"]:
        by_model = row.per_stratum_pass.get("model", {})
        mf = by_model.get("minifasnet_v2")
        fb = by_model.get("fallback")
        add(f"| {row.threshold:.2f} | {_pct(row.pass_rate if row.n else None)} "
            f"| {_pct(row.bpcer_proxy)} | {_pct(row.apcer_proxy)} "
            f"| {_pct(mf[0]) if mf else '—'} | {_pct(fb[0]) if fb else '—'} |")
    add("")
    rng = analysis["operating_range"]
    if rng:
        add(f"**Operating range** (BPCER-proxy <= target): "
            f"{min(rng):.2f} – {max(rng):.2f}")
    else:
        add("**Operating range**: none established "
            "(no weak-genuine labels, or every threshold exceeds the target).")
    add("")
    add("<details><summary>Per-stratum pass-rates at each threshold</summary>")
    add("")
    for row in analysis["sweep"]:
        parts = []
        for kind in ("frames", "challenge", "provider", "mode"):
            for name, (rate, n) in row.per_stratum_pass.get(kind, {}).items():
                parts.append(f"{kind}={name}: {rate:.0%} (n={n})")
        add(f"- t={row.threshold:.2f}: " + ("; ".join(parts) if parts else "no data"))
    add("")
    add("</details>")
    add("")

    add("## 6. Recommendations")
    add("")
    rec = analysis["recommendation"]
    if rec["threshold"] is not None:
        add(f"- **Fused-score threshold: {rec['threshold']:.2f}** "
            f"(basis: {rec['basis']})")
    else:
        add(f"- **No threshold recommended** (basis: {rec['basis']}) — "
            "keep LIVENESS_ENFORCE=false.")
    for note in rec["notes"]:
        add(f"  - {note}")
    mf = analysis["min_frames"]
    add(f"- **Minimum frames: {mf['recommended_min_frames']}** — {mf['note']}")
    add("  - parallax availability by frame count: "
        + (", ".join(
            f"{f} frames: {rate:.0%} (n={n})"
            for f, (rate, n) in mf["availability"].items()
        ) or "no data"))
    add("- **Fusion weights**: see §4 — components with dead or negative "
        "correlations need investigation before their weights are trusted.")
    add("")

    add("## 7. What would LIVENESS_ENFORCE=true have done?")
    add("")
    enf = analysis["enforcement"]
    add(f"Replay over the last **{enf['n']}** sessions, gating on the top-level "
        "`liveScore` exactly as `inhouse.go` does (`LivenessPassed = liveScore >= "
        "threshold`). Note the fallback engine caps liveScore at 0.5, so fallback "
        "sessions can never pass a threshold above 0.5; failing sessions would "
        "have been referred to an officer, not silently dropped.")
    add("")
    add("| threshold | | would pass | would fail | fails by engine |")
    add("|---|---|---|---|---|")
    for t, d in enf["at"].items():
        by_model = ", ".join(f"{k}: {v}" for k, v in d["fails_by_model"].items()) or "—"
        add(f"| {t:.2f} | {d['label']} | {d['would_pass']} | {d['would_fail']} | {by_model} |")
    add("")
    for t, d in enf["at"].items():
        if not d["failed_sessions"]:
            continue
        add(f"<details><summary>Sessions failing at t={t:.2f} "
            f"(first {len(d['failed_sessions'])})</summary>")
        add("")
        add("| ts | engine | liveScore | fused | padMin | frames | weak label |")
        add("|---|---|---|---|---|---|---|")
        for s in d["failed_sessions"]:
            add(f"| {s['ts'] or '—'} | {s['model']} | {_fmt(s['liveScore'], 4)} "
                f"| {_fmt(s['score'], 4)} | {_fmt(s['padMin'], 4)} "
                f"| {s['frames']} | {s['weakLabel'] or '—'} |")
        add("")
        add("</details>")
        add("")

    add("## Appendix: exporting outcomes")
    add("")
    add("See the SQL in `tools/calibration/outcomes.py` (columns match "
        "`go-services/internal/compliance/repository/onboarding_repository.go`); "
        "run it with `\\copy ... TO 'outcomes.csv' WITH CSV HEADER` against the "
        "compliance DB and pass `--outcomes outcomes.csv`.")
    add("")

    out_path.write_text("\n".join(L))
    return out_path
