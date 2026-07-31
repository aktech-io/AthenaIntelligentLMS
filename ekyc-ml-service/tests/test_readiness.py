"""Readiness gating + screening production-guard tests.

The mismatch logic and the demo-list guard are pure stdlib and always run;
the /health and /v1/screen endpoint tests need FastAPI (+httpx for the test
client) and skip themselves when it's missing — same self-skip convention
as test_liveness.py, preserving the suite's stdlib-only baseline.
"""
import pytest

from engine.readiness import engine_mismatches, expected_engines
from engine.screening import demo_lists_allowed, demo_lists_only


class TestEngineMismatches:
    def test_no_expectations_is_always_healthy(self):
        assert engine_mismatches("fallback", "fallback", None, None) == []
        assert engine_mismatches("sface", "minifasnet_v2", None, None) == []

    def test_matching_expectations_are_healthy(self):
        assert engine_mismatches("sface", "minifasnet_v2", "sface", "minifasnet_v2") == []
        # Expecting fallback is legitimate (e.g. an air-gapped demo box).
        assert engine_mismatches("fallback", "fallback", "fallback", "fallback") == []

    def test_face_mismatch_is_reported(self):
        out = engine_mismatches("fallback", "minifasnet_v2", "sface", None)
        assert len(out) == 1
        assert "faceEngine" in out[0]
        assert "'fallback'" in out[0] and "'sface'" in out[0]

    def test_liveness_mismatch_is_reported(self):
        out = engine_mismatches("sface", "fallback", None, "minifasnet_v2")
        assert len(out) == 1
        assert "livenessEngine" in out[0]

    def test_both_mismatch_gives_two_reasons(self):
        out = engine_mismatches("fallback", "fallback", "sface", "minifasnet_v2")
        assert len(out) == 2

    def test_typo_in_expectation_gates_rather_than_matches(self):
        # An unknown expected value can never equal a real mode — the pod
        # stays gated out and the 503 body surfaces the bad value.
        out = engine_mismatches("sface", "minifasnet_v2", "sfaace", None)
        assert len(out) == 1 and "'sfaace'" in out[0]

    def test_env_reader_normalizes_empty_to_dont_care(self, monkeypatch):
        monkeypatch.delenv("EXPECTED_FACE_ENGINE", raising=False)
        monkeypatch.setenv("EXPECTED_LIVENESS_ENGINE", "  ")
        assert expected_engines() == (None, None)
        monkeypatch.setenv("EXPECTED_FACE_ENGINE", " SFace ")
        monkeypatch.setenv("EXPECTED_LIVENESS_ENGINE", "minifasnet_v2")
        assert expected_engines() == ("sface", "minifasnet_v2")


class TestDemoListGuard:
    def test_all_demo_files(self):
        assert demo_lists_only(["sanctions_demo.csv", "pep_demo.csv"])

    def test_mixed_real_and_demo_is_not_demo_only(self):
        assert not demo_lists_only(["sanctions_demo.csv", "sanctions_ofac.csv"])

    def test_all_real_files(self):
        assert not demo_lists_only(["sanctions_ofac.csv", "sanctions_un.csv"])

    def test_empty_load_is_not_demo_only(self):
        # empty is already fail-closed via the no-lists 503
        assert not demo_lists_only([])

    def test_allowed_by_default(self, monkeypatch):
        monkeypatch.delenv("EKYC_ALLOW_DEMO_LISTS", raising=False)
        assert demo_lists_allowed()

    @pytest.mark.parametrize("value", ["false", "FALSE", " False ", "0", "no"])
    def test_disallowed_values(self, monkeypatch, value):
        monkeypatch.setenv("EKYC_ALLOW_DEMO_LISTS", value)
        assert not demo_lists_allowed()

    @pytest.mark.parametrize("value", ["true", "1", "yes", ""])
    def test_allowed_values(self, monkeypatch, value):
        monkeypatch.setenv("EKYC_ALLOW_DEMO_LISTS", value)
        assert demo_lists_allowed()


class TestHealthEndpointGating:
    @pytest.fixture(autouse=True)
    def clean_env(self, monkeypatch, tmp_path):
        """Deterministic baseline: no model files (fallback engines), the
        packaged demo lists, no expectations, fresh screener singleton."""
        import engine.screening as screening

        monkeypatch.setenv("FACE_DETECTOR_MODEL", str(tmp_path / "absent_det.onnx"))
        monkeypatch.setenv("FACE_EMBEDDER_MODEL", str(tmp_path / "absent_emb.onnx"))
        monkeypatch.setenv("FACE_LIVENESS_MODEL", str(tmp_path / "absent_pad.onnx"))
        monkeypatch.delenv("EXPECTED_FACE_ENGINE", raising=False)
        monkeypatch.delenv("EXPECTED_LIVENESS_ENGINE", raising=False)
        monkeypatch.delenv("EKYC_ALLOW_DEMO_LISTS", raising=False)
        monkeypatch.delenv("EKYC_DATA_DIR", raising=False)
        monkeypatch.setattr(screening, "_screener", None)

    @pytest.fixture()
    def client(self):
        pytest.importorskip("fastapi")
        pytest.importorskip("httpx")  # TestClient transport
        from fastapi.testclient import TestClient

        from main import app

        return TestClient(app)

    def test_no_expectations_stays_200_with_unchanged_payload(self, client):
        r = client.get("/health")
        assert r.status_code == 200
        body = r.json()
        assert body["status"] == "ok"
        assert body["faceEngine"] == "fallback"
        assert body["livenessEngine"] == "fallback"
        # gating fields never leak into the healthy payload
        assert "degraded" not in body
        assert "screeningListsMode" not in body

    def test_expected_face_engine_mismatch_is_503(self, client, monkeypatch):
        monkeypatch.setenv("EXPECTED_FACE_ENGINE", "sface")
        r = client.get("/health")
        assert r.status_code == 503
        body = r.json()
        assert body["status"] == "degraded"
        assert any("faceEngine" in m for m in body["degraded"])

    def test_expected_liveness_engine_mismatch_is_503(self, client, monkeypatch):
        monkeypatch.setenv("EXPECTED_LIVENESS_ENGINE", "minifasnet_v2")
        r = client.get("/health")
        assert r.status_code == 503
        assert any("livenessEngine" in m for m in r.json()["degraded"])

    def test_expected_fallback_matches_unprovisioned_box(self, client, monkeypatch):
        monkeypatch.setenv("EXPECTED_FACE_ENGINE", "fallback")
        monkeypatch.setenv("EXPECTED_LIVENESS_ENGINE", "fallback")
        assert client.get("/health").status_code == 200

    def test_demo_only_guard_degrades_health(self, client, monkeypatch):
        monkeypatch.setenv("EKYC_ALLOW_DEMO_LISTS", "false")
        r = client.get("/health")
        assert r.status_code == 503
        body = r.json()
        assert body["status"] == "degraded"
        assert body["screeningListsMode"] == "demo-only"
        assert any("demo-only" in m for m in body["degraded"])

    def test_screen_fails_closed_on_demo_only(self, client, monkeypatch):
        monkeypatch.setenv("EKYC_ALLOW_DEMO_LISTS", "false")
        r = client.post("/v1/screen", json={"fullName": "Viktor Orlov"})
        assert r.status_code == 503
        assert "demo" in r.json()["detail"]

    def test_screen_serves_demo_lists_by_default(self, client):
        r = client.post("/v1/screen", json={"fullName": "Viktor Orlov"})
        assert r.status_code == 200
        assert r.json()["sanctionsHit"] is True
