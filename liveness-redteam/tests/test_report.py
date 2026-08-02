"""Report generation, badge line and the CLI wiring."""
from __future__ import annotations

import os

import pytest

from liveness_redteam import report as R
from liveness_redteam import synth, taxonomy
from liveness_redteam.cli import EXIT_GATE_FAILED, EXIT_OK, main
from liveness_redteam.storage import PresentationRow, ResultsDB, RunRecord, utcnow


def seed_run(
    db: ResultsDB,
    run_id: str,
    *,
    model_version: str = "minifasnet_v2@sha256:deadbeef",
    threshold: float = 0.5,
    started_at: str | None = None,
    attack_scores=((("print_flat", 0.1),) * 3),
    genuine_scores=(0.9, 0.9),
) -> None:
    db.start_run(
        RunRecord(
            run_id=run_id,
            started_at=started_at or utcnow(),
            scorer="stub",
            target="stub://local",
            model_version=model_version,
            threshold=threshold,
            frame_count=5,
            sessions_root="/sessions",
            notes="seeded",
        )
    )
    for i, (species, score) in enumerate(attack_scores):
        db.add_presentation(
            PresentationRow(
                run_id=run_id,
                session_id=f"attack-{i}",
                presentation_type="attack",
                species=species,
                level=taxonomy.SPECIES[species].level,
                clip_file=f"clip_{i:03d}.mp4",
                frames_used=5,
                live_score=score,
                label="LIVE" if score >= threshold else "SPOOF",
                model="minifasnet_v2",
                accepted=int(score >= threshold),
            )
        )
    for i, score in enumerate(genuine_scores):
        db.add_presentation(
            PresentationRow(
                run_id=run_id,
                session_id=f"genuine-{i}",
                presentation_type="genuine",
                clip_file=f"clip_{i:03d}.mp4",
                frames_used=5,
                live_score=score,
                label="LIVE" if score >= threshold else "SPOOF",
                model="minifasnet_v2",
                accepted=int(score >= threshold),
            )
        )
    db.commit()
    db.finish_run(run_id)


@pytest.fixture()
def db(tmp_path):
    with ResultsDB(str(tmp_path / "results.db")) as handle:
        yield handle


def test_badge_line_reports_gate_species_and_model(db):
    seed_run(
        db,
        "run-1",
        attack_scores=(("print_flat", 0.1), ("replay_phone", 0.9)),
    )
    view = R.load_run_view(db, "run-1")
    badge = R.badge_line(view)
    assert badge.count("\n") == 0
    assert "FAIL" in badge
    assert "replay_phone" in badge  # worst species named
    assert "APCER 100.00%" in badge
    assert "minifasnet_v2@sha256:deadbeef" in badge
    assert "run-1" in badge


def test_badge_line_passes_when_nothing_is_accepted(db):
    seed_run(db, "clean", attack_scores=(("print_flat", 0.1),) * 4)
    badge = R.badge_line(R.load_run_view(db, "clean"))
    # zero acceptances, but the L1 gate still fails on volume/coverage
    assert "APCER 0.00%" in badge
    assert "BPCER 0.00%" in badge
    assert "none accepted" in badge  # no misleading "worst species" name


def test_report_contains_every_required_section(db):
    seed_run(
        db,
        "run-1",
        attack_scores=(
            ("print_flat", 0.1),
            ("print_flat", 0.7),
            ("replay_monitor", 0.2),
        ),
        genuine_scores=(0.9, 0.3),
    )
    text = R.build_report(db, "run-1")

    for heading in (
        "# PAD red-team run `run-1`",
        "## Run",
        "## Attack presentations (APCER by species)",
        "## Genuine presentations (BPCER)",
        "## Gates",
        "### L1 gate",
        "### L2 gate",
        "## Threshold sweep",
        "## Trend",
    ):
        assert heading in text

    # per-species table with denominators
    assert "| `print_flat` | L1 | 2 | 1 | 50.00%" in text
    assert "| `replay_monitor` | L1 | 1 | 0 | 0.00% |" in text
    # BPCER: one of two genuine below 0.5
    assert "| 2 | 1 | 50.00% |" in text
    # uncovered species are called out
    assert "L1 species not exercised" in text
    assert "no 3D-mask species" in text
    # threshold recommendation
    assert "Recommended threshold for this gate" in text


def test_report_trend_lists_previous_runs_and_flags_model_changes(db):
    seed_run(db, "older", model_version="old@v0", started_at="2026-07-01T00:00:00+00:00")
    seed_run(db, "newer", model_version="new@v1", started_at="2026-07-02T00:00:00+00:00")
    text = R.build_report(db, "newer")
    assert "`newer` **(this run)**" in text
    assert "`older`" in text
    assert "sustain clock restarts" in text


def test_report_trend_without_previous_runs(db):
    seed_run(db, "solo")
    text = R.build_report(db, "solo")
    assert "no earlier runs in this database" in text
    assert "sustain clock restarts" not in text


def test_report_counts_errored_presentations_separately(db):
    seed_run(db, "run-1")
    db.add_presentation(
        PresentationRow(
            run_id="run-1",
            session_id="broken",
            presentation_type="attack",
            species="print_flat",
            clip_file="clip_x.mp4",
            frames_used=0,
            error="engine 503",
        )
    )
    db.commit()
    view = R.load_run_view(db, "run-1")
    assert view.errors == 1
    assert len(view.presentations) == 5  # 3 attacks + 2 genuine
    assert "Errored presentations | 1 (excluded from all rates)" in R.build_report(
        db, "run-1"
    )


def test_write_report_creates_the_file(db, tmp_path):
    seed_run(db, "run-1")
    out = tmp_path / "report.md"
    R.write_report(db, "run-1", str(out))
    assert out.read_text().startswith("# PAD red-team run `run-1`")


def test_unknown_run_raises(db):
    with pytest.raises(KeyError):
        R.load_run_view(db, "nope")


# ─── CLI ─────────────────────────────────────────────────────────────────────


def test_cli_smoke_end_to_end(tmp_path, capsys):
    sessions = tmp_path / "sessions"
    db_path = tmp_path / "results.db"
    report_path = tmp_path / "report.md"

    assert main(["synth", "--out", str(sessions), "--genuine", "2",
                 "--per-species", "1"]) == EXIT_OK
    assert main(["validate", str(sessions)]) == EXIT_OK
    assert main([
        "run", str(sessions),
        "--db", str(db_path),
        "--scorer", "inprocess",
        "--threshold", "0.5",
        "--report", str(report_path),
        "--quiet",
    ]) == EXIT_OK
    assert os.path.isfile(report_path)
    assert "# PAD red-team run" in report_path.read_text()

    # the smoke battery is nowhere near 500 presentations, so the gate fails
    assert main(["gates", "--db", str(db_path)]) == EXIT_GATE_FAILED
    assert main(["sweep", "--db", str(db_path)]) == EXIT_OK
    assert main(["runs", "--db", str(db_path)]) == EXIT_OK
    assert main(["report", "--db", str(db_path), "--badge"]) == EXIT_OK
    assert main(["taxonomy"]) == EXIT_OK

    output = capsys.readouterr().out
    assert "PAD red-team" in output
    assert "attack volume" in output


def test_cli_validate_reports_invalid_sessions(tmp_path):
    sessions = tmp_path / "sessions"
    synth.make_battery(str(sessions), genuine=1, per_species=0)
    (sessions / "broken").mkdir()
    (sessions / "broken" / "manifest.json").write_text("{}")
    assert main(["validate", str(sessions)]) == EXIT_GATE_FAILED


def test_cli_run_gate_flag_sets_exit_code(tmp_path):
    sessions = tmp_path / "sessions"
    synth.make_battery(str(sessions), genuine=1, per_species=1)
    assert main([
        "run", str(sessions),
        "--db", str(tmp_path / "results.db"),
        "--gate", "l1",
        "--quiet",
    ]) == EXIT_GATE_FAILED
