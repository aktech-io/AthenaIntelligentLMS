"""CLI:  python -m tools.calibration run --logs <file|-> [--outcomes f] [--out r.md]"""
from __future__ import annotations

import argparse
import sys
from datetime import datetime, timezone
from pathlib import Path

from tools.calibration import analyze as analyze_mod
from tools.calibration import ingest as ingest_mod
from tools.calibration import outcomes as outcomes_mod
from tools.calibration import report as report_mod


def _default_out() -> Path:
    stamp = datetime.now(timezone.utc).strftime("%Y%m%d-%H%M")
    return report_mod.REPORTS_DIR / f"calibration-{stamp}.md"


def build_parser() -> argparse.ArgumentParser:
    ap = argparse.ArgumentParser(
        prog="python -m tools.calibration",
        description="Liveness shadow-log threshold calibration (docs/ekyc/06 §6).",
    )
    sub = ap.add_subparsers(dest="command", required=True)

    run = sub.add_parser("run", help="ingest logs, analyze, write markdown report")
    run.add_argument("--logs", required=False, default=None,
                     help="ekyc.liveness log file, or '-' for stdin; omit to "
                          "analyze the accumulated store only")
    run.add_argument("--outcomes", default=None,
                     help="CSV/JSON export of onboarding outcomes (weak labels)")
    run.add_argument("--out", type=Path, default=None,
                     help=f"report path (default {report_mod.REPORTS_DIR}/calibration-<stamp>.md)")
    run.add_argument("--store", type=Path, default=ingest_mod.DEFAULT_STORE,
                     help="jsonl history store; --no-store to skip persistence")
    run.add_argument("--no-store", action="store_true",
                     help="analyze this input only; do not read/write the store")
    run.add_argument("--bpcer-target", type=float,
                     default=analyze_mod.DEFAULT_BPCER_TARGET,
                     help="max tolerated weak-genuine rejection rate (default 0.05)")
    run.add_argument("--last", type=int, default=200,
                     help="sessions in the enforcement replay window (default 200)")
    run.add_argument("--join-window", type=float, default=outcomes_mod.JOIN_WINDOW_S,
                     help="outcome join timestamp tolerance, seconds (default 180)")
    run.add_argument("--no-plots", action="store_true",
                     help="skip matplotlib PNGs even if matplotlib is installed")

    ing = sub.add_parser("ingest", help="only ingest logs into the store")
    ing.add_argument("logs", help="log file path, or '-' for stdin")
    ing.add_argument("--store", type=Path, default=ingest_mod.DEFAULT_STORE)
    return ap


def cmd_run(args) -> int:
    provenance: dict = {"logs": args.logs or "(store only)"}
    store = None if args.no_store else args.store

    if args.logs:
        result, records, added = ingest_mod.ingest(args.logs, store)
        provenance.update(
            scanned=result.scanned,
            parsed=len(result.records),
            malformed=result.malformed,
            added=added,
        )
    elif store is not None:
        records = ingest_mod.load_store(store)
        provenance.update(scanned=0, parsed=0, malformed=0, added=0)
    else:
        print("error: --no-store requires --logs", file=sys.stderr)
        return 2
    provenance["store"] = str(store) if store is not None else "not persisted"

    if not records:
        print(
            "No fusion records found. If the deployment is fresh, confirm the "
            "ekyc.liveness logger is actually emitting (INFO level must be "
            "enabled) and that traffic hit POST /v1/face/liveness.",
            file=sys.stderr,
        )

    if args.outcomes:
        outcome_rows = outcomes_mod.load_outcomes(args.outcomes)
        joined = outcomes_mod.join(records, outcome_rows, args.join_window)
        provenance["outcomes"] = args.outcomes
        print(f"joined {joined}/{len(records)} records to "
              f"{len(outcome_rows)} outcome rows")

    analysis = analyze_mod.analyze(records, args.bpcer_target, args.last)
    out = args.out or _default_out()
    path = report_mod.render(analysis, provenance, records, out,
                             plots=not args.no_plots)
    rec = analysis["recommendation"]
    print(f"report written: {path}")
    print(f"sessions analyzed: {analysis['n']}  "
          f"(weak-genuine {analysis['labeled']['genuine']}, "
          f"weak-suspicious {analysis['labeled']['suspicious']})")
    if rec["threshold"] is not None:
        print(f"recommended fused-score threshold: {rec['threshold']:.2f} "
              f"({rec['basis']})")
    else:
        print(f"no threshold recommended ({rec['basis']}) — stay in shadow")
    return 0


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    if args.command == "run":
        return cmd_run(args)
    if args.command == "ingest":
        return ingest_mod.main([args.logs, "--store", str(args.store)])
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
