# NLD-EA session-manifest format (schemaVersion 1)

**This document is a contract** between the NLD-EA capture tooling and the
Stage-1 training pipeline (`liveness_training/datasets/nldea.py` implements it
verbatim — change both together or neither). Plan of record:
`docs/ekyc/06-level2-upgrade-plan.md` §5 and `docs/nemo/09-liveness-build-and-certify.md`.

## Layout

A dataset root contains session directories (nesting depth is free-form; the
loader discovers every `manifest.json` recursively). Each session directory
holds one `manifest.json` plus the clip files it references — clip `file`
entries are **bare filenames**, always resolved inside the session directory.

```
<root>/
  2026-08-04/station-a/session-0001/
    manifest.json
    clip_001.mp4
    clip_002.mp4
```

## manifest.json

```json
{
  "schemaVersion": 1,
  "sessionId": "6a1f0c9e-6e5b-4d0a-9d3e-2f9d1a7b4c10",
  "subjectId": "subj-7f3a91",
  "consentId": "consent-2026-0041",
  "type": "genuine",
  "attackType": null,
  "device": {"model": "Tecno Spark 10", "os": "Android 13"},
  "lighting": "low_light",
  "skinTone": "monk_08",
  "capturedAt": "2026-08-04T14:22:31Z",
  "clips": [
    {"file": "clip_001.mp4", "durationMs": 6100, "fps": 30, "challenge": null},
    {"file": "clip_002.mp4", "durationMs": 5900, "fps": 30, "challenge": "blink"}
  ]
}
```

### Fields

| Field | Type | Rules |
|---|---|---|
| `schemaVersion` | int | Must be exactly `1`. Bump only with a coordinated loader change. |
| `sessionId` | string | UUID of the session. Non-empty. |
| `subjectId` | string | **Pseudonymous** id (never a name/national id). Drives sharding — must be stable for a person across all of their sessions. |
| `consentId` | string | Links to the consent record (DPIA requirement, doc 06 §5). |
| `type` | string | `"genuine"` \| `"attack"`. |
| `attackType` | string\|null | `null` iff genuine. Else one of `print_flat`, `print_curved`, `replay_phone`, `replay_monitor`, `cutout_paper`, `mask_3d`. |
| `device.model`, `device.os` | string | Capture device, non-empty. |
| `lighting` | string | `daylight` \| `indoor` \| `low_light`. |
| `skinTone` | string\|null | Monk scale `monk_01`..`monk_10`, or `null` when not assessed. |
| `capturedAt` | string | ISO-8601 timestamp (`Z` or numeric offset). |
| `clips[]` | array | Non-empty. `file`: bare filename in the session dir. `durationMs`: int ≥ 0. `fps`: number ≥ 0. `challenge`: `null` \| `blink` \| `turn_left` \| `turn_right` \| `smile`. |

Unknown **extra** top-level keys are tolerated by the loader (forward
compatibility); every field above is validated strictly and any violation
rejects the whole session (`ManifestError`).

## Sharding — train / val / redteam = 70 / 15 / 15

Shard assignment is a **pure function of `subjectId`** (never of session,
clip, or time), so all of a subject's material lands in exactly one shard:

```
digest = SHA-256("nld-ea-v1:" + subjectId)
bucket = int(first 8 bytes of digest, big-endian) mod 100
bucket <  70          -> train
70 <= bucket < 85     -> val
85 <= bucket          -> redteam
```

Reference implementation: `liveness_training.datasets.base.subject_shard`.
The salt `nld-ea-v1` is frozen for the lifetime of NLD-EA v1 — changing it
(or any part of the recipe) silently reassigns every subject and invalidates
the certification holdout. A pinned regression test guards it.

### The redteam shard is radioactive

The redteam shard is the internal certification rig's holdout
(doc 06 §3: "red-team shard — never trained on"). Rules, enforced by the
loader:

* `NLDEADataset(root)` and `NLDEADataset(root, shard="val")` can never see
  redteam subjects.
* The only access path is the explicit `shard="redteam"`, which raises a
  `RuntimeWarning` stating the shard must never be used for training,
  distillation, threshold tuning, or model selection.
* There is deliberately no "all shards" mode.
