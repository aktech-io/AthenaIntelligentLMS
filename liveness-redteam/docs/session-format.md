# Session format (NLD-EA manifest, schemaVersion 1)

Shared contract with the capture app and the dataset tooling — the red-team
rig implements it exactly and adds two backward-compatible extensions.

A session is a **directory**:

```
session-3f8b1c2e/
  manifest.json
  clip_001.mp4
  clip_002.mp4
```

## manifest.json

```json
{
  "schemaVersion": 1,
  "sessionId": "3f8b1c2e-0000-4000-8000-000000000001",
  "subjectId": "subj-0007",
  "consentId": "consent-0007",
  "type": "attack",
  "attackType": "replay_phone",
  "device": {"model": "Tecno Spark 10", "os": "Android 13"},
  "lighting": "low_light",
  "skinTone": "monk_08",
  "capturedAt": "2026-08-01T09:15:00+00:00",
  "clips": [
    {"file": "clip_001.mp4", "durationMs": 3000, "fps": 24, "challenge": "blink"}
  ]
}
```

## Fields

| Field | Type | Notes |
|---|---|---|
| `schemaVersion` | int | must be `1` |
| `sessionId` | string (uuid) | unique per session |
| `subjectId` | string | pseudonymous, erasable (withdrawal-by-subject-ID) |
| `consentId` | string | ties to the tablet-signed consent record |
| `type` | `"genuine"` \| `"attack"` | |
| `attackType` | null \| species key | `null` for genuine; a species for attack |
| `device` | `{model, os}` | recorded per clip fleet (doc 06 §5) |
| `lighting` | `"daylight"` \| `"indoor"` \| `"low_light"` | capture station |
| `skinTone` | `"monk_01"`..`"monk_10"` \| null | Monk scale |
| `capturedAt` | ISO-8601 string | trailing `Z` accepted |
| `clips` | array | at least one; see below |

### clips[]

| Field | Type | Notes |
|---|---|---|
| `file` | string | relative path inside the session dir (no `..`, not absolute) |
| `durationMs` | number | informational |
| `fps` | number | informational |
| `challenge` | null \| `"blink"` \| `"turn_left"` \| `"turn_right"` \| `"smile"` | active-challenge type run during capture |
| `challengeResult` | null \| `"passed"` \| `"failed"` | **red-team extension** — what the on-device challenge returned; forwarded to the engine's `challenge` form field. Absent = not run |

## attackType values

**Contract set** (shared NLD-EA manifest): `print_flat`, `print_curved`,
`replay_phone`, `replay_monitor`, `cutout_paper`, `mask_3d`.

**Red-team superset** (also accepted here): `replay_tablet`, `mask_silicone`,
`mask_latex`, `mask_resin` — material granularity matters because
worst-species APCER is only meaningful when masks are distinguished.

## Forward compatibility

Unknown top-level keys are **preserved, not rejected** (surfaced as
`Session.extra`) — the format is shared with two sibling tools and must stay
forward-compatible. A bad manifest in a tree is skipped with a recorded
failure, never aborting a 500-presentation run.

Validate a tree:

```bash
python3 -m liveness_redteam validate ./sessions
```
