# liveness-redteam — internal PAD red-team rig

The harness that must sustain **0/500 APCER internally** before we book the
iBeta ISO/IEC 30107-3 test for the Nemo eKYC liveness stack. It runs
presentation-attack batteries against the liveness endpoint, computes the
ISO metrics, and evaluates the certification gates so we know we are ready
before spending lab money.

Where this sits in the plan: `docs/ekyc/06-level2-upgrade-plan.md` §2 (staged
timeline), §7 (Stage 2 — certify Level 1), §7b (Stage 3 — Level 2), and
`docs/nemo/09-liveness-build-and-certify.md` §2 (what certification takes).

* **Level 1 bar** — 0 accepted attacks over ~900 presentations across the six
  cheap 2D species (print, replay, cutout), BPCER ≤15%. The internal gate is
  **0/500** before booking.
* **Level 2 bar** — worst-species APCER ≤1% including 3D masks
  (silicone/latex/resin). Schema-ready now, exercised once mask fabrication
  produces sessions.

APCER is reported **per attack species and gated on the worst species**, never
the pooled mean — pooling lets 400 easy prints hide five accepted replays.

## The endpoint / frame-count contract

Confirmed from the service and Go provider code:

| What | Where | Value |
|---|---|---|
| Endpoint | `ekyc-ml-service/api/face.py` | `POST /v1/face/liveness` |
| Frame parts | `api/face.py` | repeated multipart parts **all named `frame`**, JPEG |
| Max frames | `api/face.py` `_MAX_LIVENESS_FRAMES` | **10** (more → HTTP 400) |
| App-reached frames | `go-services/internal/compliance/liveness/inhouse.go` `maxFrames` | **5** (provider truncates to the first 5) |
| Challenge field | `api/face.py` | optional form field `challenge` = `"passed"` \| `"failed"` (omitted = not run) |
| Response | `api/face.py` / `engine/liveness.py` | `{liveScore, label, perFrame[], model, fusion{…}}` |
| Health / engine | `main.py` `GET /health` | `{livenessEngine: "minifasnet_v2"\|"fallback", …}` (503 when degraded) |

The rig defaults to **5 frames per presentation** (production parity) and
allows up to 10 (doc-09 Stage-1 target aggregation) so a run can measure what
the extra frames buy before the Go cap is widened.

Decision rule (matches `engine/liveness.py`): a presentation is *accepted*
(classified bona fide) when `liveScore >= threshold`, threshold default 0.5.

## Install

```bash
cd liveness-redteam
pip install -r requirements.txt        # numpy, opencv-python-headless, requests, pytest
```

No ML framework of its own — same slim footprint as `ekyc-ml-service`.

## Scorer backends

* **`inprocess`** (default) — imports `engine.liveness.score_frames` from the
  `ekyc-ml-service` tree in the repo. No server; used in CI and for offline
  runs. With no MiniFASNet ONNX provisioned it runs the deterministic
  **fallback** engine (capped at 0.5, always `UNKNOWN`).
* **`http`** — `POST`s against a running `ekyc-ml-service` (local or the
  deployed box). Records the model version from `GET /health`.

## Quick start (CI smoke, no real attacks)

```bash
# 1. generate synthetic smoke sessions (solid-colour / noise clips)
python3 -m liveness_redteam synth --out /tmp/rt/sessions --genuine 4 --per-species 2

# 2. validate them against the NLD-EA manifest contract
python3 -m liveness_redteam validate /tmp/rt/sessions

# 3. score them with the in-process engine into results.db + a report
python3 -m liveness_redteam run /tmp/rt/sessions \
    --db /tmp/rt/results.db --scorer inprocess --report /tmp/rt/report.md

# 4. re-evaluate the gates any time (exit code 2 if the L1 gate fails)
python3 -m liveness_redteam gates --db /tmp/rt/results.db
```

## Running against a real box

```bash
# local ekyc-ml-service
python3 -m liveness_redteam run ./sessions \
    --scorer http --url http://localhost:8080 \
    --db runs/results.db --frames 5 --threshold 0.5 \
    --report runs/latest.md --gate l1

# deployed box (inside the cluster network — the ML sidecar has no auth,
# never expose its port publicly)
python3 -m liveness_redteam run ./sessions \
    --scorer http --url http://ekyc-ml-service.compliance.svc.cluster.local:8080 \
    --db runs/results.db
```

Other commands: `sweep` (full ROC of thresholds), `runs` (list stored runs),
`report --badge` (one-line status for the docs), `taxonomy` (list species).

## How an operator captures sessions

A **session** is a directory with a `manifest.json` and its clips. One clip =
one presentation = one attempt of one attack instrument. See
[docs/session-format.md](docs/session-format.md) for the full schema and
[docs/taxonomy.md](docs/taxonomy.md) for per-species capture guidance.

Per iBeta Level-1 guidance and the NLD-EA protocol (doc 06 §5):

* **Devices** — the actual Kenyan fleet: 2 budget (Tecno/Redmi) + 2 mid-range
  (Samsung A / Camon). Test **per app-build, per device** — the letter only
  covers the build/device pairs submitted.
* **Prints** — matte and glossy, ≥300 dpi, life-size; flat and curved
  variants. Cutouts with eye/mouth holes over a live attacker, run *with* the
  active challenge (they exist to beat it).
* **Replay screens** — phone, tablet, monitor at max brightness; include a
  near-field take (pixel grid not resolvable) and an off-axis take. Include
  challenge-replay (a recorded blink/turn played back).
* **Lighting** — all three stations: daylight (>1,000 lux), indoor (~300 lux),
  low light (<50 lux, over-sampled — MiniFASNet's failure zone).
* **Skin tone** — full Monk 1–10, ≥60% in 7–10 (the regional-calibration
  claim). Record `skinTone` per session.
* **Genuine presentations** — needed for BPCER; capture the same
  device/lighting/tone spread as the attacks.

Label each session honestly: `type`, `attackType` species, device, lighting,
Monk tone, and the challenge result if one was run.

## The 0/500 sustain protocol (doc 06 §2)

1. **Rig phase (28 days)** — build out the L1 attack battery on 3–4 handsets,
   iterate the model/threshold until a full battery run shows **0 accepted
   attacks** with all six L1 species covered and BPCER ≤15%.
2. **Sustain phase (21 days)** — keep running fresh batteries; the internal
   bar is **0 accepted over N≥500** attack presentations, held across the
   window. `gates` returns exit 2 the moment a run breaks it.
3. Only then **book iBeta** (get a competing BixeLab quote first). L2 mask
   fabrication + rig extension start during the L1 lab window.

**The sustain clock restarts on any model change.** Every run records the
model version (ONNX checksum, or `fallback@none`); the report's trend table
flags when the version changed within the window, because a 0/500 streak is
only meaningful for one fixed model.

## NLD-EA red-team shard — never trained on

The internal certification rig is fed by the **NLD-EA red-team shard**: a
subject-disjoint slice of the dataset that never touches training (doc 06 §5).
Scoring attacks the model has effectively memorised would inflate APCER
confidence and invalidate the gate. Keep the shard separate; this rig only
ever *reads* it.

> Synthetic smoke fixtures (`synth`) exercise the rig end-to-end in CI. They
> are **not** attack material and no synthetic run may be cited as evidence
> toward any certification gate — the rig is what they test, not the model.

## Tests

```bash
cd liveness-redteam && python3 -m pytest
```

Covers taxonomy + manifest validation, frame sampling and the frame-count
contract, the runner on synthetic sessions (stub + real in-process engine),
the HTTP backend against a stub server (endpoint contract), hand-computed
APCER/BPCER/sweep math, gate logic, and report generation.
