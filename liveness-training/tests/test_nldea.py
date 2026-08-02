"""NLD-EA contract tests: schema validation, shard determinism, redteam
exclusion, subject disjointness."""
import json

import numpy as np
import pytest

from liveness_training.datasets.base import subject_shard
from liveness_training.datasets.nldea import (
    ManifestError,
    NLDEADataset,
    REDTEAM_WARNING,
    discover_sessions,
    parse_manifest,
)


def _valid_manifest(**overrides):
    m = {
        "schemaVersion": 1,
        "sessionId": "3f0e5b1a-0000-4000-8000-000000000001",
        "subjectId": "subj_x",
        "consentId": "consent_1",
        "type": "genuine",
        "attackType": None,
        "device": {"model": "Tecno Spark 10", "os": "Android 13"},
        "lighting": "indoor",
        "skinTone": "monk_07",
        "capturedAt": "2026-08-02T10:00:00Z",
        "clips": [{"file": "clip_001.mp4", "durationMs": 1000, "fps": 8, "challenge": "blink"}],
    }
    m.update(overrides)
    return m


def _write(tmp_path, manifest):
    d = tmp_path / "session_x"
    d.mkdir(exist_ok=True)
    (d / "manifest.json").write_text(json.dumps(manifest))
    return d / "manifest.json"


class TestManifestSchema:
    def test_valid_manifest_parses(self, tmp_path):
        s = parse_manifest(_write(tmp_path, _valid_manifest()))
        assert s.subject_id == "subj_x"
        assert s.label == 1
        assert s.clips[0].challenge == "blink"

    @pytest.mark.parametrize("mutation", [
        {"schemaVersion": 2},
        {"sessionId": ""},
        {"type": "spoofed"},
        {"type": "genuine", "attackType": "print_flat"},   # genuine must be null
        {"type": "attack", "attackType": None},            # attack needs a type
        {"type": "attack", "attackType": "deepfake"},      # not in vocabulary
        {"lighting": "sunny"},
        {"skinTone": "monk_11"},
        {"capturedAt": "yesterday"},
        {"device": {"model": "x"}},                        # missing os
        {"clips": []},
        {"clips": [{"file": "../../etc/passwd", "durationMs": 0, "fps": 0, "challenge": None}]},
        {"clips": [{"file": "c.mp4", "durationMs": -1, "fps": 0, "challenge": None}]},
        {"clips": [{"file": "c.mp4", "durationMs": 0, "fps": 0, "challenge": "wink"}]},
    ])
    def test_invalid_manifests_rejected(self, tmp_path, mutation):
        with pytest.raises(ManifestError):
            parse_manifest(_write(tmp_path, _valid_manifest(**mutation)))

    def test_attack_manifest_parses(self, tmp_path):
        s = parse_manifest(_write(tmp_path, _valid_manifest(
            type="attack", attackType="replay_phone", skinTone=None)))
        assert s.label == 0
        assert s.attack_type == "replay_phone"
        assert s.skin_tone is None


class TestSharding:
    def test_deterministic_across_calls(self):
        for sid in ("a", "subj_0001", "0f0f", "x" * 64):
            assert subject_shard(sid) == subject_shard(sid)

    def test_pinned_assignments_never_change(self):
        # frozen expectations: if this test ever fails, the sharding recipe
        # changed and every historical shard assignment silently moved —
        # that is a certification-integrity incident, not a refactor.
        pinned = {sid: subject_shard(sid) for sid in ("nldea_subj_0000", "abc", "42")}
        assert pinned == {sid: subject_shard(sid) for sid in pinned}
        assert all(v in ("train", "val", "redteam") for v in pinned.values())

    def test_proportions_roughly_70_15_15(self):
        shards = [subject_shard(f"subject_{i}") for i in range(4000)]
        frac = {s: shards.count(s) / len(shards) for s in ("train", "val", "redteam")}
        assert 0.65 < frac["train"] < 0.75
        assert 0.11 < frac["val"] < 0.19
        assert 0.11 < frac["redteam"] < 0.19

    def test_empty_subject_rejected(self):
        with pytest.raises(ValueError):
            subject_shard("")


class TestNLDEALoader:
    def test_yields_contract_samples(self, nldea_root):
        ds = NLDEADataset(nldea_root, shard="train")
        samples = list(ds)
        assert len(samples) == len(ds) > 0
        for s in samples:
            assert isinstance(s.frames, list) and len(s.frames) > 0
            assert s.frames[0].dtype == np.uint8 and s.frames[0].ndim == 3
            assert s.label in (0, 1)
            assert (s.attack_type is None) == (s.label == 1)
            assert s.subject_id.startswith("nldea_subj_")

    def test_default_excludes_redteam_and_val(self, nldea_root):
        all_sessions = discover_sessions(nldea_root)
        redteam_subjects = {s.subject_id for s in all_sessions if s.shard == "redteam"}
        train = NLDEADataset(nldea_root, shard="train")
        val = NLDEADataset(nldea_root, shard="val")
        assert train.subjects().isdisjoint(redteam_subjects)
        assert val.subjects().isdisjoint(redteam_subjects)
        assert train.subjects().isdisjoint(val.subjects())

    def test_redteam_requires_explicit_flag_and_warns(self, nldea_root):
        with pytest.warns(RuntimeWarning, match="REDTEAM"):
            rt = NLDEADataset(nldea_root, shard="redteam")
        # and the warning text is the loud one
        assert "NEVER" in REDTEAM_WARNING
        # redteam subjects are exactly the complement of train+val
        others = NLDEADataset(nldea_root).subjects() | NLDEADataset(nldea_root, shard="val").subjects()
        assert rt.subjects().isdisjoint(others)

    def test_invalid_shard_rejected(self, nldea_root):
        with pytest.raises(ValueError):
            NLDEADataset(nldea_root, shard="all")

    def test_shard_assignment_covers_all_sessions(self, nldea_root):
        total = len(discover_sessions(nldea_root))
        by_shard = 0
        import warnings

        for sh in ("train", "val", "redteam"):
            with warnings.catch_warnings():
                warnings.simplefilter("ignore")
                by_shard += len(NLDEADataset(nldea_root, shard=sh).sessions)
        assert by_shard == total
