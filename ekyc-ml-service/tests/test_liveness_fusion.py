"""doc-09 Stage-1 multi-frame fusion tests — model-free sub-scores only.

Everything here runs without ONNX model files: parallax and moiré are
pure OpenCV/numpy signals, and the fusion combiner is arithmetic. Synthetic
fixtures encode the physics being tested: identical frames carry zero
parallax, rigidly shifted crops carry motion without non-rigidity, split
shifts mimic depth-induced disagreement, and a sinusoidal "screen-door"
overlay mimics screen-replay moiré. Missing heavy deps skip (not fail),
matching the suite's stdlib-only baseline.
"""
import pytest

np = pytest.importorskip("numpy")
cv2 = pytest.importorskip("cv2")

from engine.liveness_fusion import (  # noqa: E402
    _MOIRE_RATIO_CLEAN,
    _WEIGHTS,
    compute_fusion,
    fuse,
    moire_analysis,
    parallax_analysis,
)

SIZE = 256


def textured_frame(seed=0):
    """Deterministic BGR frame with trackable mid-frequency texture."""
    rng = np.random.default_rng(seed)
    gray = rng.integers(0, 256, (SIZE, SIZE), dtype=np.uint8)
    gray = cv2.GaussianBlur(gray, (0, 0), 3)
    gray = cv2.normalize(gray, None, 0, 255, cv2.NORM_MINMAX).astype(np.uint8)
    return cv2.cvtColor(gray, cv2.COLOR_GRAY2BGR)


def shifted(frame, dx):
    """Rigid horizontal shift (circular, so phase correlation is exact)."""
    return np.roll(frame, dx, axis=1)


def split_shifted(frame, dx):
    """Top half shifted +dx, bottom half -dx: motion WITHOUT a rigid
    explanation — the synthetic stand-in for depth-induced parallax."""
    out = frame.copy()
    out[: SIZE // 2] = np.roll(frame[: SIZE // 2], dx, axis=1)
    out[SIZE // 2 :] = np.roll(frame[SIZE // 2 :], -dx, axis=1)
    return out


def screendoor(frame, amplitude=20, period=3.5):
    """Overlay a fine sinusoidal grid — the classic screen-replay artifact."""
    yy, xx = np.mgrid[:SIZE, :SIZE]
    grid = amplitude * np.sin(2 * np.pi * xx / period) * np.sin(
        2 * np.pi * yy / period
    )
    return np.clip(frame.astype(np.float64) + grid[..., None], 0, 255).astype(
        np.uint8
    )


class TestParallax:
    def test_identical_frames_zero_parallax(self):
        f = textured_frame()
        score, motion, nonrigidity = parallax_analysis([f, f.copy(), f.copy()])
        assert score == 0.0
        assert motion == pytest.approx(0.0, abs=0.05)
        assert nonrigidity == pytest.approx(0.0, abs=0.05)

    def test_single_frame_is_absent_not_zero(self):
        assert parallax_analysis([textured_frame()]) == (None, None, None)

    def test_untrackable_frames_are_absent(self):
        blank = np.zeros((SIZE, SIZE, 3), dtype=np.uint8)
        assert parallax_analysis([blank, blank]) == (None, None, None)

    def test_rigid_shift_registers_motion_but_little_nonrigidity(self):
        f = textured_frame()
        score, motion, nonrigidity = parallax_analysis([f, shifted(f, 5)])
        assert score > 0.0  # parallax signal present
        assert motion == pytest.approx(5.0, abs=0.5)
        assert nonrigidity < 0.5  # a flat surface moves rigidly

    def test_nonrigid_motion_scores_higher_than_rigid(self):
        f = textured_frame()
        rigid, _, _ = parallax_analysis([f, shifted(f, 6)])
        nonrigid, motion, deviation = parallax_analysis([f, split_shifted(f, 6)])
        assert deviation > 3.0  # blocks disagree
        assert nonrigid > rigid  # 3D-like motion beats photo-like motion


class TestMoire:
    def test_clean_frame_scores_clean(self):
        score, ratio = moire_analysis([textured_frame()])
        assert score >= 0.9
        assert ratio < _MOIRE_RATIO_CLEAN

    def test_screendoor_overlay_scores_spoof(self):
        f = textured_frame()
        clean_score, clean_ratio = moire_analysis([f])
        spoof_score, spoof_ratio = moire_analysis([screendoor(f)])
        assert spoof_ratio > clean_ratio * 5
        assert spoof_score <= 0.2
        assert spoof_score < clean_score

    def test_median_across_frames_tolerates_one_bad_frame(self):
        f = textured_frame()
        score, _ = moire_analysis([f, f.copy(), screendoor(f)])
        assert score >= 0.9  # one moiré frame does not decide alone


class TestFuse:
    def test_weights_renormalize_when_components_absent(self):
        got = fuse(0.8, None, 1.0, None)
        want = (_WEIGHTS["pad"] * 0.8 + _WEIGHTS["moire"] * 1.0) / (
            _WEIGHTS["pad"] + _WEIGHTS["moire"]
        )
        assert got == pytest.approx(want)

    def test_all_components_weighted_mean(self):
        got = fuse(1.0, 1.0, 1.0, 1.0)
        assert got == pytest.approx(1.0)
        assert fuse(0.0, 0.0, 0.0, 0.0) == pytest.approx(0.0)

    def test_challenge_failed_lowers_score(self):
        passed = fuse(0.9, 0.5, 0.9, 1.0)
        failed = fuse(0.9, 0.5, 0.9, 0.0)
        absent = fuse(0.9, 0.5, 0.9, None)
        assert failed < absent < passed

    def test_result_stays_in_unit_interval(self):
        assert 0.0 <= fuse(1.0, 1.0, 1.0, None) <= 1.0
        assert 0.0 <= fuse(0.0, None, 0.0, 0.0) <= 1.0


class TestComputeFusion:
    def test_identical_frames_score_below_shifted_frames(self):
        f = textured_frame()
        pads = [0.9, 0.9, 0.9]
        still = compute_fusion([f, f.copy(), f.copy()], pads)
        moving = compute_fusion([f, shifted(f, 4), shifted(f, 8)], pads)
        assert still.parallax == 0.0  # zero parallax measured, not absent
        assert moving.parallax > 0.0
        assert still.score < moving.score  # frozen feed earns less

    def test_median_pad_robust_to_one_bad_frame_unlike_min(self):
        f = textured_frame()
        frames = [f, f.copy(), f.copy()]
        b = compute_fusion(frames, [0.9, 0.05, 0.9])
        assert b.pad_median == pytest.approx(0.9)  # median absorbs the outlier
        assert b.pad_min == pytest.approx(0.05)  # old policy kept for A/B

    def test_challenge_signal_reaches_breakdown(self):
        f = textured_frame()
        assert compute_fusion([f], [0.5], True).challenge == 1.0
        assert compute_fusion([f], [0.5], False).challenge == 0.0
        assert compute_fusion([f], [0.5], None).challenge is None

    def test_as_dict_camel_case_and_version(self):
        d = compute_fusion([textured_frame()], [0.5]).as_dict()
        assert d["version"] == 1
        assert d["frames"] == 1
        assert d["parallax"] is None  # single frame: absent, JSON null
        assert 0.0 <= d["score"] <= 1.0


class TestPrimaryEngineFused:
    """The minifasnet path with the ONNX net faked out: verifies the fused
    score (not the old min) now drives the LIVE/SPOOF label, without needing
    provisioned model files."""

    @pytest.fixture()
    def primary(self, monkeypatch, tmp_path):
        from engine import liveness

        model = tmp_path / "fake_pad.onnx"
        model.write_bytes(b"onnx")  # exists -> model_available() is True
        monkeypatch.setenv("FACE_LIVENESS_MODEL", str(model))
        monkeypatch.setattr(
            liveness, "_detect_largest_face", lambda img: (60, 60, 120, 120)
        )

        class FakeNet:
            """Yields one 3-class softmax row per forward(), cycling."""

            def __init__(self, rows):
                self.rows = rows
                self.calls = 0

            def setInput(self, blob):
                assert blob.shape == (1, 3, 80, 80)

            def forward(self):
                row = self.rows[self.calls % len(self.rows)]
                self.calls += 1
                return np.array([row], dtype=np.float32)

        def with_probs(*rows):
            net = FakeNet(list(rows))
            monkeypatch.setattr(cv2.dnn, "readNetFromONNX", lambda path: net)
            return liveness

        return with_probs

    def test_live_probs_fuse_to_live(self, primary):
        liveness = primary([0.05, 0.90, 0.05])  # class 1 = genuine
        f = textured_frame()
        r = liveness.score_frames([f, f.copy()])
        assert r.model == "minifasnet_v2"
        assert r.label == "LIVE"
        assert r.live_score == pytest.approx(r.fusion.score)
        assert r.fusion.pad_median == pytest.approx(0.90, abs=1e-4)

    def test_spoof_probs_fuse_to_spoof(self, primary):
        liveness = primary([0.90, 0.05, 0.05])
        f = textured_frame()
        r = liveness.score_frames([f, f.copy()])
        assert r.label == "SPOOF"
        assert r.live_score < 0.5

    def test_one_bad_frame_no_longer_decides(self, primary):
        """The doc-06 §3 point of median fusion: with min-score policy one
        weak frame forced SPOOF; the median absorbs it."""
        liveness = primary(
            [0.05, 0.90, 0.05],
            [0.90, 0.02, 0.08],  # frame 2 of 3: near-zero live probability
            [0.05, 0.90, 0.05],
        )
        f = textured_frame()
        r = liveness.score_frames([f, f.copy(), f.copy()])
        assert r.fusion.pad_min == pytest.approx(0.02, abs=1e-4)
        assert r.fusion.pad_median == pytest.approx(0.9, abs=1e-4)
        assert r.live_score > r.fusion.pad_min  # min no longer decides


class TestEndpointChallengeField:
    @pytest.fixture()
    def client(self, monkeypatch, tmp_path):
        pytest.importorskip("fastapi")
        from fastapi.testclient import TestClient

        from main import app

        # force deterministic fallback mode (no model files)
        monkeypatch.setenv("FACE_LIVENESS_MODEL", str(tmp_path / "no_pad.onnx"))
        monkeypatch.setenv("FACE_DETECTOR_MODEL", str(tmp_path / "no_det.onnx"))
        monkeypatch.setenv("FACE_EMBEDDER_MODEL", str(tmp_path / "no_emb.onnx"))
        return TestClient(app)

    @staticmethod
    def frames(n=2):
        ok, buf = cv2.imencode(".png", textured_frame())
        assert ok
        data = buf.tobytes()
        return [("frame", (f"f{i}.png", data, "image/png")) for i in range(n)]

    def test_challenge_passed_lands_in_fusion(self, client):
        r = client.post(
            "/v1/face/liveness",
            files=self.frames(),
            data={"challenge": "passed"},
        )
        assert r.status_code == 200
        assert r.json()["fusion"]["challenge"] == 1.0

    def test_challenge_failed_lands_in_fusion(self, client):
        r = client.post(
            "/v1/face/liveness",
            files=self.frames(),
            data={"challenge": "FAILED"},  # case-insensitive
        )
        assert r.status_code == 200
        assert r.json()["fusion"]["challenge"] == 0.0

    def test_challenge_omitted_is_null(self, client):
        r = client.post("/v1/face/liveness", files=self.frames())
        assert r.status_code == 200
        assert r.json()["fusion"]["challenge"] is None

    def test_invalid_challenge_is_400(self, client):
        r = client.post(
            "/v1/face/liveness",
            files=self.frames(),
            data={"challenge": "maybe"},
        )
        assert r.status_code == 400
        assert "challenge" in r.json()["detail"]

    def test_fallback_fused_score_stays_capped(self, client):
        # even with clean moiré + passed challenge, an unprovisioned
        # deployment must never look confidently live
        r = client.post(
            "/v1/face/liveness",
            files=self.frames(3),
            data={"challenge": "passed"},
        )
        assert r.status_code == 200
        body = r.json()
        assert body["model"] == "fallback"
        assert body["label"] == "UNKNOWN"
        assert body["liveScore"] <= 0.5
