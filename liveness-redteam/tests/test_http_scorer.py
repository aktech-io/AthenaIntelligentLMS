"""HTTP backend against a stub server — locks the endpoint contract.

The shape asserted here is the one confirmed from
``ekyc-ml-service/api/face.py`` and
``go-services/internal/compliance/liveness/inhouse.go``: POST
``/v1/face/liveness``, repeated multipart parts all named ``frame``, optional
``challenge`` form field of "passed"/"failed".
"""
from __future__ import annotations

import json
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

import numpy as np
import pytest

from liveness_redteam.scorers import HttpScorer, ScorerError

RECEIVED: dict = {}

LIVENESS_BODY = {
    "liveScore": 0.4231,
    "label": "SPOOF",
    "perFrame": [{"score": 0.4, "faceFound": True}],
    "model": "minifasnet_v2",
    "fusion": {"score": 0.4231, "padMedian": 0.4, "padMin": 0.2, "frames": 3},
}


class Handler(BaseHTTPRequestHandler):
    health_status = 200
    liveness_status = 200

    def log_message(self, *args):  # silence the stub server
        pass

    def do_GET(self):
        RECEIVED["health_path"] = self.path
        self._respond(
            self.health_status,
            {"status": "ok", "livenessEngine": "minifasnet_v2", "ocr": "ppocr"},
        )

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        RECEIVED["path"] = self.path
        RECEIVED["content_type"] = self.headers.get("Content-Type", "")
        RECEIVED["frame_parts"] = body.count(b'name="frame"')
        RECEIVED["has_challenge"] = b'name="challenge"' in body
        RECEIVED["challenge_value"] = (
            b"passed" if b"passed" in body else (b"failed" if b"failed" in body else None)
        )
        self._respond(self.liveness_status, LIVENESS_BODY)

    def _respond(self, status: int, payload: dict) -> None:
        data = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


@pytest.fixture()
def server():
    RECEIVED.clear()
    Handler.health_status = 200
    Handler.liveness_status = 200
    httpd = HTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=httpd.serve_forever, daemon=True)
    thread.start()
    yield f"http://127.0.0.1:{httpd.server_port}"
    httpd.shutdown()
    httpd.server_close()


def frames(n: int = 3):
    return [np.full((32, 32, 3), 90 + i, dtype=np.uint8) for i in range(n)]


def test_posts_repeated_frame_parts_to_the_liveness_endpoint(server):
    scorer = HttpScorer(server)
    result = scorer.score(frames(3))

    assert RECEIVED["path"] == "/v1/face/liveness"
    assert RECEIVED["content_type"].startswith("multipart/form-data")
    assert RECEIVED["frame_parts"] == 3
    assert RECEIVED["has_challenge"] is False

    assert result.live_score == pytest.approx(0.4231)
    assert result.label == "SPOOF"
    assert result.model == "minifasnet_v2"
    assert result.fusion["padMin"] == 0.2
    assert len(result.per_frame) == 1


def test_challenge_result_is_sent_as_the_form_field(server):
    HttpScorer(server).score(frames(1), challenge_passed=True)
    assert RECEIVED["has_challenge"] is True
    assert RECEIVED["challenge_value"] == b"passed"

    HttpScorer(server).score(frames(1), challenge_passed=False)
    assert RECEIVED["challenge_value"] == b"failed"


def test_model_version_comes_from_health(server):
    scorer = HttpScorer(server)
    assert scorer.model_version() == "minifasnet_v2@unpinned"
    assert RECEIVED["health_path"] == "/health"
    assert scorer.target() == server


def test_degraded_health_503_is_recorded_not_fatal(server):
    Handler.health_status = 503
    scorer = HttpScorer(server)
    assert scorer.model_version() == "minifasnet_v2@unpinned"
    assert scorer.health()["status"] == "ok"  # stub payload; 503 still parsed


def test_engine_error_becomes_a_scorer_error(server):
    Handler.liveness_status = 503
    with pytest.raises(ScorerError) as excinfo:
        HttpScorer(server).score(frames(1))
    assert "503" in str(excinfo.value)


def test_unreachable_service_is_reported_not_crashed():
    scorer = HttpScorer("http://127.0.0.1:1")  # nothing listens here
    assert scorer.health()["status"] == "unreachable"
    assert scorer.model_version() == "unknown@unpinned"
    with pytest.raises(ScorerError):
        scorer.score(frames(1))


def test_empty_frame_list_is_rejected_before_the_request(server):
    with pytest.raises(ScorerError):
        HttpScorer(server).score([])
    assert "path" not in RECEIVED
