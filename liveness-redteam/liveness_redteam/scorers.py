"""Two scorer backends for the same liveness contract.

Endpoint contract, confirmed from ``ekyc-ml-service/api/face.py`` and
``go-services/internal/compliance/liveness/inhouse.go``:

    POST {base}/v1/face/liveness
      multipart/form-data
        frame     repeated part, 1..10 image uploads (the Go provider sends
                  JPEG named frame0.jpg..frame4.jpg and caps at 5)
        challenge optional form field, "passed" | "failed" (omitted = the
                  active challenge was not run)
      200 -> {"liveScore": 0..1, "label": "LIVE"|"SPOOF"|"UNKNOWN",
              "perFrame": [{"score": .., "faceFound": bool}, ...],
              "model": "minifasnet_v2"|"fallback",
              "fusion": {"score","padMedian","padMin","parallax","moire",
                         "challenge","motionPx","nonRigidityPx",
                         "moirePeakRatio","frames","version"}}
      400 -> >10 frames, empty frame, bad challenge value
      422 -> undecodable frame
      503 -> engine error

    GET {base}/health -> {"livenessEngine": "minifasnet_v2"|"fallback", ...}
      503 when the deployment is degraded (readiness gating) — the payload is
      still JSON and still names the engine, so the rig records the version
      and flags the run.

The in-process backend imports ``engine.liveness.score_frames`` directly from
the ekyc-ml-service tree: same code path, no server, usable in CI. When the
MiniFASNet ONNX file is absent both backends run the deterministic fallback
engine, which is capped at 0.5 and always labels UNKNOWN.
"""
from __future__ import annotations

import hashlib
import os
import sys
from dataclasses import dataclass, field

from .frames import encode_jpeg

LIVENESS_PATH = "/v1/face/liveness"
HEALTH_PATH = "/health"

#: engine.liveness._LIVE_THRESHOLD — the provisional decision threshold the
#: service itself uses to label LIVE vs SPOOF. The rig's default too, so a
#: run with no --threshold reproduces service behaviour exactly.
DEFAULT_THRESHOLD = 0.5


@dataclass
class ScoreResult:
    live_score: float
    label: str
    model: str
    per_frame: list = field(default_factory=list)
    fusion: dict | None = None
    raw: dict = field(default_factory=dict)

    @classmethod
    def from_payload(cls, payload: dict) -> "ScoreResult":
        return cls(
            live_score=float(payload.get("liveScore", 0.0)),
            label=str(payload.get("label", "UNKNOWN")),
            model=str(payload.get("model", "unknown")),
            per_frame=list(payload.get("perFrame") or []),
            fusion=payload.get("fusion"),
            raw=payload,
        )


class ScorerError(RuntimeError):
    """The scorer could not produce a verdict for this presentation."""


class Scorer:
    """Common interface: ``score(frames, challenge_passed) -> ScoreResult``."""

    name = "scorer"

    def score(self, frames, challenge_passed: bool | None = None) -> ScoreResult:
        raise NotImplementedError

    def model_version(self) -> str:
        """Stable identity of the scored model, recorded on every run.

        Format ``<engine>@<checksum-or-marker>`` so results from different
        model generations can never be silently pooled in a trend table.
        """
        raise NotImplementedError

    def target(self) -> str:
        raise NotImplementedError


# ─── HTTP backend ────────────────────────────────────────────────────────────


class HttpScorer(Scorer):
    """Scores against a running ekyc-ml-service (local or deployed box)."""

    name = "http"

    def __init__(self, base_url: str, timeout: float = 60.0, quality: int = 92):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.quality = quality
        self._session = None
        self._health: dict | None = None

    def _http(self):
        if self._session is None:
            import requests

            self._session = requests.Session()
        return self._session

    def health(self, refresh: bool = False) -> dict:
        """/health payload. A 503 (degraded readiness) is still parsed —
        the rig wants to *record* degradation, not crash on it."""
        if self._health is not None and not refresh:
            return self._health
        import requests

        try:
            resp = self._http().get(
                self.base_url + HEALTH_PATH, timeout=self.timeout
            )
            payload = resp.json()
            if resp.status_code not in (200, 503):
                payload = {"status": f"http {resp.status_code}"}
        except (requests.RequestException, ValueError) as e:
            payload = {"status": "unreachable", "error": str(e)}
        self._health = payload if isinstance(payload, dict) else {}
        return self._health

    def model_version(self) -> str:
        health = self.health()
        engine = health.get("livenessEngine", "unknown")
        # the service exposes no model checksum today; honour one if a future
        # build adds it rather than needing a rig change to notice
        checksum = (
            health.get("livenessModelChecksum")
            or health.get("modelChecksum")
            or ("none" if engine == "fallback" else "unpinned")
        )
        return f"{engine}@{checksum}"

    def target(self) -> str:
        return self.base_url

    def score(self, frames, challenge_passed: bool | None = None) -> ScoreResult:
        import requests

        if not frames:
            raise ScorerError("no frames to score")
        files = [
            ("frame", (f"frame{i}.jpg", encode_jpeg(f, self.quality), "image/jpeg"))
            for i, f in enumerate(frames)
        ]
        data = {}
        if challenge_passed is not None:
            data["challenge"] = "passed" if challenge_passed else "failed"
        try:
            resp = self._http().post(
                self.base_url + LIVENESS_PATH,
                files=files,
                data=data,
                timeout=self.timeout,
            )
        except requests.RequestException as e:
            raise ScorerError(f"POST {LIVENESS_PATH}: {e}") from e
        if resp.status_code != 200:
            raise ScorerError(
                f"POST {LIVENESS_PATH}: status {resp.status_code}: "
                f"{resp.text[:200]}"
            )
        try:
            payload = resp.json()
        except ValueError as e:
            raise ScorerError(f"POST {LIVENESS_PATH}: bad JSON: {e}") from e
        return ScoreResult.from_payload(payload)


# ─── in-process backend ──────────────────────────────────────────────────────


def default_service_root() -> str:
    """``<repo>/ekyc-ml-service`` relative to this package."""
    here = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.dirname(os.path.dirname(here))
    return os.path.join(repo_root, "ekyc-ml-service")


class InProcessScorer(Scorer):
    """Imports the engine directly — offline runs and CI, no server needed."""

    name = "inprocess"

    def __init__(self, service_root: str | None = None):
        self.service_root = os.path.abspath(service_root or default_service_root())
        if not os.path.isdir(self.service_root):
            raise ScorerError(
                f"ekyc-ml-service tree not found at {self.service_root} "
                "(pass --service-root)"
            )
        if self.service_root not in sys.path:
            sys.path.insert(0, self.service_root)
        try:
            from engine import liveness  # noqa: F401
        except ImportError as e:  # pragma: no cover - environment problem
            raise ScorerError(
                f"cannot import engine.liveness from {self.service_root}: {e}"
            ) from e
        self._liveness = liveness

    def model_version(self) -> str:
        path = self._liveness.liveness_model_path()
        if not self._liveness.model_available():
            return "fallback@none"
        digest = hashlib.sha256()
        with open(path, "rb") as fh:
            for chunk in iter(lambda: fh.read(1 << 20), b""):
                digest.update(chunk)
        return f"minifasnet_v2@sha256:{digest.hexdigest()[:16]}"

    def target(self) -> str:
        return f"inprocess:{self.service_root}"

    def score(self, frames, challenge_passed: bool | None = None) -> ScoreResult:
        if not frames:
            raise ScorerError("no frames to score")
        try:
            result = self._liveness.score_frames(list(frames), challenge_passed)
        except Exception as e:  # engine failure: fail the presentation, not the run
            raise ScorerError(f"engine.score_frames: {e}") from e
        return ScoreResult.from_payload(result.as_dict())


def build_scorer(
    backend: str,
    *,
    url: str | None = None,
    service_root: str | None = None,
    timeout: float = 60.0,
) -> Scorer:
    if backend == "http":
        if not url:
            raise ScorerError("the http scorer needs --url")
        return HttpScorer(url, timeout=timeout)
    if backend == "inprocess":
        return InProcessScorer(service_root)
    raise ScorerError(f"unknown scorer backend {backend!r} (http|inprocess)")
