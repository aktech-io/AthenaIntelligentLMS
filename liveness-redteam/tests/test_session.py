"""NLD-EA manifest contract: validation, loading, round-tripping."""
from __future__ import annotations

import json
import os

import pytest

from liveness_redteam import session as S


def valid_manifest(**overrides) -> dict:
    data = {
        "schemaVersion": 1,
        "sessionId": "3f8b1c2e-0000-4000-8000-000000000001",
        "subjectId": "subj-0007",
        "consentId": "consent-0007",
        "type": "attack",
        "attackType": "print_flat",
        "device": {"model": "Tecno Spark 10", "os": "Android 13"},
        "lighting": "low_light",
        "skinTone": "monk_08",
        "capturedAt": "2026-08-01T09:15:00+00:00",
        "clips": [
            {
                "file": "clip_001.mp4",
                "durationMs": 3000,
                "fps": 24,
                "challenge": "blink",
            }
        ],
    }
    data.update(overrides)
    return data


def test_valid_manifest_has_no_errors():
    assert S.validate_manifest(valid_manifest()) == []


def test_genuine_manifest_requires_null_attack_type():
    ok = valid_manifest(type="genuine", attackType=None)
    assert S.validate_manifest(ok) == []

    bad = valid_manifest(type="genuine", attackType="print_flat")
    errors = S.validate_manifest(bad)
    assert any("attackType null" in e for e in errors)


def test_attack_manifest_requires_a_known_species():
    errors = S.validate_manifest(valid_manifest(attackType=None))
    assert any("attackType" in e for e in errors)
    errors = S.validate_manifest(valid_manifest(attackType="hologram"))
    assert any("hologram" in e for e in errors)


@pytest.mark.parametrize(
    "attack_type",
    ["print_flat", "print_curved", "replay_phone", "replay_monitor",
     "cutout_paper", "mask_3d"],
)
def test_contract_attack_types_accepted(attack_type):
    assert S.validate_manifest(valid_manifest(attackType=attack_type)) == []


@pytest.mark.parametrize(
    "attack_type",
    ["replay_tablet", "mask_silicone", "mask_latex", "mask_resin"],
)
def test_redteam_superset_attack_types_accepted(attack_type):
    assert S.validate_manifest(valid_manifest(attackType=attack_type)) == []


def test_schema_version_must_be_one():
    errors = S.validate_manifest(valid_manifest(schemaVersion=2))
    assert any("schemaVersion" in e for e in errors)


def test_enumerations_are_enforced():
    assert any(
        "lighting" in e for e in S.validate_manifest(valid_manifest(lighting="dusk"))
    )
    assert any(
        "skinTone" in e for e in S.validate_manifest(valid_manifest(skinTone="monk_11"))
    )
    assert any(
        "type" in e for e in S.validate_manifest(valid_manifest(type="spoof"))
    )
    clips = [{"file": "c.mp4", "durationMs": 1, "fps": 1, "challenge": "wink"}]
    assert any(
        "challenge" in e for e in S.validate_manifest(valid_manifest(clips=clips))
    )


def test_null_skin_tone_is_allowed():
    assert S.validate_manifest(valid_manifest(skinTone=None)) == []


def test_captured_at_must_be_iso8601():
    assert any(
        "ISO-8601" in e
        for e in S.validate_manifest(valid_manifest(capturedAt="01/08/2026"))
    )
    # trailing-Z form is accepted
    assert S.validate_manifest(valid_manifest(capturedAt="2026-08-01T09:15:00Z")) == []


def test_clips_must_be_present_and_relative():
    assert any(
        "clips" in e for e in S.validate_manifest(valid_manifest(clips=[]))
    )
    escaping = [{"file": "../../etc/passwd", "durationMs": 0, "fps": 0,
                 "challenge": None}]
    assert any(
        "relative path" in e
        for e in S.validate_manifest(valid_manifest(clips=escaping))
    )


def test_device_block_required():
    errors = S.validate_manifest(valid_manifest(device={"model": "", "os": "iOS 17"}))
    assert any("device.model" in e for e in errors)


def test_load_session_checks_clip_files_exist(tmp_path):
    directory = tmp_path / "session-a"
    directory.mkdir()
    (directory / "manifest.json").write_text(json.dumps(valid_manifest()))

    with pytest.raises(S.ManifestError) as excinfo:
        S.load_session(str(directory))
    assert "not found on disk" in str(excinfo.value)

    (directory / "clip_001.mp4").write_bytes(b"\x00")
    sess = S.load_session(str(directory))
    assert sess.species == "print_flat"
    assert sess.level == "L1"
    assert sess.is_attack
    assert sess.clips[0].challenge == "blink"
    assert sess.clips[0].challenge_result is None


def test_genuine_session_has_no_species(tmp_path):
    directory = tmp_path / "session-b"
    directory.mkdir()
    (directory / "manifest.json").write_text(
        json.dumps(valid_manifest(type="genuine", attackType=None))
    )
    (directory / "clip_001.mp4").write_bytes(b"\x00")
    sess = S.load_session(str(directory))
    assert sess.species is None
    assert sess.level is None
    assert not sess.is_attack


def test_optional_challenge_result_extension(tmp_path):
    clips = [
        {
            "file": "clip_001.mp4",
            "durationMs": 3000,
            "fps": 24,
            "challenge": "blink",
            "challengeResult": "failed",
        }
    ]
    directory = tmp_path / "session-c"
    directory.mkdir()
    (directory / "manifest.json").write_text(json.dumps(valid_manifest(clips=clips)))
    (directory / "clip_001.mp4").write_bytes(b"\x00")
    sess = S.load_session(str(directory))
    assert sess.clips[0].challenge_result is False

    clips[0]["challengeResult"] = "maybe"
    assert any(
        "challengeResult" in e for e in S.validate_manifest(valid_manifest(clips=clips))
    )


def test_unknown_top_level_keys_are_preserved_not_rejected(tmp_path):
    data = valid_manifest(operatorId="agent-04", siteCode="NBO-CBD")
    assert S.validate_manifest(data) == []
    directory = tmp_path / "session-d"
    directory.mkdir()
    (directory / "manifest.json").write_text(json.dumps(data))
    (directory / "clip_001.mp4").write_bytes(b"\x00")
    sess = S.load_session(str(directory))
    assert sess.extra["operatorId"] == "agent-04"
    assert sess.as_manifest()["siteCode"] == "NBO-CBD"


def test_manifest_round_trip(tmp_path):
    original = valid_manifest()
    directory = tmp_path / "session-e"
    directory.mkdir()
    (directory / "manifest.json").write_text(json.dumps(original))
    (directory / "clip_001.mp4").write_bytes(b"\x00")

    sess = S.load_session(str(directory))
    out = tmp_path / "copy"
    S.write_manifest(str(out), sess)
    with open(out / "manifest.json", encoding="utf-8") as fh:
        written = json.load(fh)
    assert written == original


def test_find_and_load_sessions_skips_bad_manifests(tmp_path):
    good = tmp_path / "nested" / "good"
    good.mkdir(parents=True)
    (good / "manifest.json").write_text(json.dumps(valid_manifest()))
    (good / "clip_001.mp4").write_bytes(b"\x00")

    bad = tmp_path / "nested" / "bad"
    bad.mkdir()
    (bad / "manifest.json").write_text("{not json")

    invalid = tmp_path / "nested" / "invalid"
    invalid.mkdir()
    (invalid / "manifest.json").write_text(
        json.dumps(valid_manifest(lighting="dusk"))
    )
    (invalid / "clip_001.mp4").write_bytes(b"\x00")

    assert len(S.find_session_dirs(str(tmp_path))) == 3
    sessions, failures = S.load_sessions(str(tmp_path))
    assert len(sessions) == 1
    assert len(failures) == 2
    assert all(os.path.isdir(path) for path, _ in failures)
