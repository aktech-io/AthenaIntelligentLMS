"""Tier-2 passive liveness tests — fallback mode only (no ONNX model files).

Engine tests need numpy + OpenCV; the endpoint tests additionally need
FastAPI. Missing dependencies skip (not fail), preserving the suite's
stdlib-only baseline (see README "Tests"). Model files are never required:
everything here exercises the deterministic UNKNOWN-capped fallback.
"""
import pytest

np = pytest.importorskip("numpy")
cv2 = pytest.importorskip("cv2")

from engine.liveness import (  # noqa: E402
    _FALLBACK_CAP,
    _scaled_crop,
    model_available,
    score_frames,
)


@pytest.fixture(autouse=True)
def no_model_files(monkeypatch, tmp_path):
    """Force fallback mode deterministically: point every model env at
    files that don't exist (dev boxes have no /app/models either, but be
    explicit rather than rely on that)."""
    monkeypatch.setenv("FACE_LIVENESS_MODEL", str(tmp_path / "absent_pad.onnx"))
    monkeypatch.setenv("FACE_DETECTOR_MODEL", str(tmp_path / "absent_det.onnx"))
    monkeypatch.setenv("FACE_EMBEDDER_MODEL", str(tmp_path / "absent_emb.onnx"))


def blank_frame(w=200, h=200):
    """A faceless BGR frame."""
    return np.zeros((h, w, 3), dtype=np.uint8)


def png_bytes(img) -> bytes:
    ok, buf = cv2.imencode(".png", img)
    assert ok
    return buf.tobytes()


class TestFallbackEngine:
    def test_no_model_returns_unknown_capped(self):
        assert not model_available()
        r = score_frames([blank_frame(), blank_frame()])
        assert r.model == "fallback"
        assert r.label == "UNKNOWN"
        assert r.live_score <= _FALLBACK_CAP  # never fabricates LIVE
        assert len(r.per_frame) == 2
        for f in r.per_frame:
            assert 0.0 <= f.score <= _FALLBACK_CAP

    def test_face_found_false_when_no_face(self):
        r = score_frames([blank_frame()])
        assert r.per_frame[0].face_found is False
        assert r.per_frame[0].score == 0.0
        assert r.live_score == 0.0
        assert r.label == "UNKNOWN"

    def test_zero_frames_raises(self):
        with pytest.raises(ValueError):
            score_frames([])

    def test_as_dict_is_camel_case_contract(self):
        d = score_frames([blank_frame()]).as_dict()
        # pre-fusion keys unchanged (Go reads liveScore/label/model); the
        # doc-09 Stage-1 fusion breakdown is additive
        assert set(d) == {"liveScore", "label", "perFrame", "model", "fusion"}
        assert set(d["perFrame"][0]) == {"score", "faceFound"}
        assert set(d["fusion"]) == {
            "score", "padMedian", "padMin", "parallax", "moire", "challenge",
            "motionPx", "nonRigidityPx", "moirePeakRatio", "frames", "version",
        }

    def test_scaled_crop_is_80x80_and_in_bounds(self):
        # box near the edge: the 2.7x widened box must clamp, not throw
        img = blank_frame(160, 120)
        crop = _scaled_crop(img, (130, 5, 25, 30))
        assert crop.shape == (80, 80, 3)


class TestLivenessEndpoint:
    @pytest.fixture()
    def client(self):
        pytest.importorskip("fastapi")
        from fastapi.testclient import TestClient

        from main import app

        return TestClient(app)

    @staticmethod
    def frames(n):
        data = png_bytes(blank_frame())
        return [("frame", (f"f{i}.png", data, "image/png")) for i in range(n)]

    def test_happy_path_contract_fallback_mode(self, client):
        r = client.post("/v1/face/liveness", files=self.frames(2))
        assert r.status_code == 200
        body = r.json()
        assert set(body) == {"liveScore", "label", "perFrame", "model", "fusion"}
        assert body["model"] == "fallback"
        assert body["label"] == "UNKNOWN"
        assert body["liveScore"] <= 0.5
        assert len(body["perFrame"]) == 2
        for f in body["perFrame"]:
            assert set(f) == {"score", "faceFound"}
            assert f["faceFound"] is False  # blank frames have no face

    def test_rejects_zero_frames(self, client):
        r = client.post("/v1/face/liveness", files=[])
        assert r.status_code == 422  # FastAPI: required field missing

    def test_rejects_more_than_ten_frames(self, client):
        # doc-09 Stage 1 widened the cap from 5 to the 10-frame aggregation
        r = client.post("/v1/face/liveness", files=self.frames(11))
        assert r.status_code == 400
        assert "10" in r.json()["detail"]

    def test_ten_frames_accepted(self, client):
        r = client.post("/v1/face/liveness", files=self.frames(10))
        assert r.status_code == 200
        assert len(r.json()["perFrame"]) == 10

    def test_undecodable_frame_is_422(self, client):
        files = [("frame", ("junk.png", b"not-an-image", "image/png"))]
        r = client.post("/v1/face/liveness", files=files)
        assert r.status_code == 422
