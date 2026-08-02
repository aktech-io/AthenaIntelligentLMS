"""``python3 -m liveness_redteam <command>`` — the red-team rig CLI.

Commands:

    taxonomy   list the attack species the rig knows
    synth      generate synthetic smoke sessions (CI fixtures)
    validate   validate a tree of capture sessions against the manifest contract
    run        score a session tree into results.db (+ optional report)
    report     (re)generate the markdown report for a stored run
    gates      evaluate the L1/L2 gates for a stored run (exit 2 on failure)
    sweep      print the threshold sweep for a stored run
    runs       list stored runs

Exit codes: 0 ok · 1 usage/IO error · 2 a gate or validation failed.
"""
from __future__ import annotations

import argparse
import os
import sys

from . import metrics as M
from . import report as report_mod
from . import session as session_mod
from . import synth, taxonomy
from .frames import DEFAULT_FRAME_COUNT, MAX_FRAME_COUNT
from .runner import RunConfig, run as run_rig
from .scorers import DEFAULT_THRESHOLD, ScorerError, build_scorer
from .storage import ResultsDB

EXIT_OK, EXIT_ERROR, EXIT_GATE_FAILED = 0, 1, 2


def _add_db_arg(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "--db", default="results.db", help="sqlite results database (default: results.db)"
    )


def _resolve_run_id(db: ResultsDB, run_id: str | None) -> str:
    resolved = run_id or db.latest_run_id()
    if resolved is None:
        raise SystemExit("no runs in this database")
    return resolved


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="liveness_redteam",
        description="Internal PAD red-team rig for the Nemo eKYC liveness stack.",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    # taxonomy
    p = sub.add_parser("taxonomy", help="list known attack species")
    p.add_argument("--level", choices=[taxonomy.LEVEL_1, taxonomy.LEVEL_2])
    p.set_defaults(func=cmd_taxonomy)

    # synth
    p = sub.add_parser("synth", help="generate synthetic smoke sessions")
    p.add_argument("--out", required=True, help="output directory for sessions")
    p.add_argument("--genuine", type=int, default=3)
    p.add_argument("--per-species", type=int, default=2)
    p.add_argument("--clips", type=int, default=1, help="clips per session")
    p.add_argument("--seed", type=int, default=1)
    p.add_argument(
        "--species",
        nargs="*",
        help="species keys (default: all Level-1 species)",
    )
    p.set_defaults(func=cmd_synth)

    # validate
    p = sub.add_parser("validate", help="validate a session tree")
    p.add_argument("sessions", help="root directory of capture sessions")
    p.add_argument(
        "--no-file-check",
        action="store_true",
        help="validate manifests only, do not require clip files on disk",
    )
    p.set_defaults(func=cmd_validate)

    # run
    p = sub.add_parser("run", help="score a session tree")
    p.add_argument("sessions", help="root directory of capture sessions")
    _add_db_arg(p)
    p.add_argument("--scorer", choices=("http", "inprocess"), default="inprocess")
    p.add_argument("--url", help="ekyc-ml-service base URL for --scorer http")
    p.add_argument("--service-root", help="path to the ekyc-ml-service tree")
    p.add_argument(
        "--frames",
        type=int,
        default=DEFAULT_FRAME_COUNT,
        help=f"frames per presentation, 1..{MAX_FRAME_COUNT} "
        f"(default {DEFAULT_FRAME_COUNT} = the Go provider's cap)",
    )
    p.add_argument("--sample-fps", type=float, help="sample at a fixed fps instead")
    p.add_argument("--threshold", type=float, default=DEFAULT_THRESHOLD)
    p.add_argument("--run-id", help="explicit run id (default: generated)")
    p.add_argument("--notes", default="", help="free-text note stored with the run")
    p.add_argument("--limit", type=int, help="stop after N presentations")
    p.add_argument("--timeout", type=float, default=60.0)
    p.add_argument(
        "--no-challenge",
        action="store_true",
        help="never forward recorded challenge results to the engine",
    )
    p.add_argument("--report", help="write the markdown report to this path")
    p.add_argument(
        "--mlflow",
        action="store_true",
        help="publish the run to MLflow (experiment liveness-redteam; needs "
        "MLFLOW_TRACKING_URI set and the optional mlflow package installed)",
    )
    p.add_argument(
        "--gate",
        choices=("l1", "l2"),
        help="evaluate this gate after the run and exit 2 if it fails",
    )
    p.add_argument("--quiet", action="store_true")
    p.set_defaults(func=cmd_run)

    # report
    p = sub.add_parser("report", help="regenerate a run's markdown report")
    _add_db_arg(p)
    p.add_argument("--run-id", help="default: most recent run")
    p.add_argument("--out", help="write to this path instead of stdout")
    p.add_argument("--badge", action="store_true", help="print only the badge line")
    p.set_defaults(func=cmd_report)

    # gates
    p = sub.add_parser("gates", help="evaluate the certification gates")
    _add_db_arg(p)
    p.add_argument("--run-id", help="default: most recent run")
    p.add_argument("--threshold", type=float, help="override the run's threshold")
    p.add_argument("--min-attacks", type=int, default=M.L1_MIN_ATTACKS)
    p.set_defaults(func=cmd_gates)

    # sweep
    p = sub.add_parser("sweep", help="threshold sweep for a run")
    _add_db_arg(p)
    p.add_argument("--run-id", help="default: most recent run")
    p.set_defaults(func=cmd_sweep)

    # runs
    p = sub.add_parser("runs", help="list stored runs")
    _add_db_arg(p)
    p.add_argument("--limit", type=int, default=20)
    p.set_defaults(func=cmd_runs)

    return parser


# ─── commands ────────────────────────────────────────────────────────────────


def cmd_taxonomy(args) -> int:
    for spec in taxonomy.all_species(args.level):
        flag = "" if spec.status == taxonomy.STATUS_ACTIVE else "  [future]"
        print(f"{spec.key:16s} {spec.level}  {spec.category:7s} {spec.label}{flag}")
        print(f"                     {spec.description}")
    return EXIT_OK


def cmd_synth(args) -> int:
    species = tuple(args.species) if args.species else None
    if species:
        for key in species:
            taxonomy.species(key)  # raises on unknown
    sessions = synth.make_battery(
        args.out,
        genuine=args.genuine,
        per_species=args.per_species,
        species=species,
        clips_per_session=args.clips,
        seed=args.seed,
    )
    clips = sum(len(s.clips) for s in sessions)
    print(
        f"wrote {len(sessions)} synthetic sessions ({clips} clips) to {args.out}"
    )
    return EXIT_OK


def cmd_validate(args) -> int:
    dirs = session_mod.find_session_dirs(args.sessions)
    if not dirs:
        print(f"no sessions found under {args.sessions}", file=sys.stderr)
        return EXIT_ERROR
    sessions, failures = session_mod.load_sessions(
        args.sessions, require_files=not args.no_file_check
    )
    for path, message in failures:
        print(f"INVALID {path}\n        {message}", file=sys.stderr)
    counts: dict[str, int] = {}
    for sess in sessions:
        key = sess.species or "genuine"
        counts[key] = counts.get(key, 0) + len(sess.clips)
    print(f"{len(sessions)} valid sessions, {len(failures)} invalid")
    for key in sorted(counts):
        print(f"  {key:16s} {counts[key]} presentations")
    return EXIT_GATE_FAILED if failures else EXIT_OK


def cmd_run(args) -> int:
    try:
        scorer = build_scorer(
            args.scorer,
            url=args.url,
            service_root=args.service_root,
            timeout=args.timeout,
        )
    except ScorerError as e:
        print(str(e), file=sys.stderr)
        return EXIT_ERROR

    config = RunConfig(
        sessions_root=args.sessions,
        db_path=args.db,
        frame_count=args.frames,
        sample_fps=args.sample_fps,
        threshold=args.threshold,
        run_id=args.run_id or "",
        notes=args.notes,
        send_challenge=not args.no_challenge,
        limit=args.limit,
    )
    emit = (lambda _m: None) if args.quiet else print
    with ResultsDB(args.db) as db:
        summary = run_rig(config, scorer, db=db, progress=emit)
        view = report_mod.load_run_view(db, summary.run_id)
        print("")
        print(report_mod.badge_line(view))
        if args.report:
            report_mod.write_report(db, summary.run_id, args.report)
            print(f"report written to {args.report}")
        if args.mlflow:
            from . import tracking

            try:
                mlflow_run_id = tracking.publish_run(
                    view, report_mod.build_report(db, summary.run_id)
                )
            except tracking.TrackingUnavailable as e:
                print(str(e), file=sys.stderr)
                return EXIT_ERROR
            print(
                f"published to MLflow experiment {tracking.EXPERIMENT!r} "
                f"as run {mlflow_run_id}"
            )
        status = EXIT_OK
        if args.gate:
            gate = (
                M.l1_gate(view.presentations, view.run.threshold)
                if args.gate == "l1"
                else M.l2_gate(view.presentations, view.run.threshold)
            )
            _print_gate(gate)
            status = EXIT_OK if gate.passed else EXIT_GATE_FAILED
    return status


def cmd_report(args) -> int:
    with ResultsDB(args.db) as db:
        run_id = _resolve_run_id(db, args.run_id)
        view = report_mod.load_run_view(db, run_id)
        if args.badge:
            print(report_mod.badge_line(view))
            return EXIT_OK
        text = report_mod.build_report(db, run_id)
        if args.out:
            with open(args.out, "w", encoding="utf-8") as fh:
                fh.write(text)
            print(f"report written to {args.out}")
        else:
            print(text)
    return EXIT_OK


def _print_gate(gate: M.GateResult) -> None:
    verdict = "PASS" if gate.passed else "FAIL"
    print(f"\n{gate.gate} gate: {verdict} (threshold {gate.threshold:g})")
    for check in gate.checks:
        print(f"  [{'ok' if check.passed else 'XX'}] {check.name}: {check.detail}")
    if gate.recommended is not None:
        rec = gate.recommended
        print(
            f"  recommended threshold {rec.threshold:.6f} -> "
            f"APCER {rec.apcer * 100:.2f}%, BPCER {rec.bpcer * 100:.2f}%"
        )


def cmd_gates(args) -> int:
    with ResultsDB(args.db) as db:
        run_id = _resolve_run_id(db, args.run_id)
        view = report_mod.load_run_view(db, run_id)
        threshold = (
            args.threshold if args.threshold is not None else view.run.threshold
        )
        print(f"run {run_id} · model {view.run.model_version}")
        l1 = M.l1_gate(
            view.presentations, threshold, min_attacks=args.min_attacks
        )
        l2 = M.l2_gate(
            view.presentations, threshold, min_attacks=args.min_attacks
        )
        _print_gate(l1)
        _print_gate(l2)
    return EXIT_OK if l1.passed else EXIT_GATE_FAILED


def cmd_sweep(args) -> int:
    with ResultsDB(args.db) as db:
        run_id = _resolve_run_id(db, args.run_id)
        view = report_mod.load_run_view(db, run_id)
        print(f"{'threshold':>12} {'APCER':>9} {'BPCER':>9} {'accepted':>9} {'rejected':>9}")
        for point in M.sweep(view.presentations):
            print(
                f"{point.threshold:12.6f} {point.apcer * 100:8.2f}% "
                f"{point.bpcer * 100:8.2f}% {point.attacks_accepted:9d} "
                f"{point.genuine_rejected:9d}"
            )
    return EXIT_OK


def cmd_runs(args) -> int:
    with ResultsDB(args.db) as db:
        runs = db.list_runs(args.limit)
        if not runs:
            print("no runs recorded")
            return EXIT_OK
        for record in runs:
            n = len(db.presentations(record.run_id))
            print(
                f"{record.run_id}  {record.started_at}  {record.scorer:9s} "
                f"{record.model_version:34s} thr={record.threshold:g}  "
                f"{n} presentations"
            )
    return EXIT_OK


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    if getattr(args, "db", None):
        parent = os.path.dirname(os.path.abspath(args.db))
        os.makedirs(parent, exist_ok=True)
    return args.func(args)


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
