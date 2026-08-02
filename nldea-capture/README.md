# NLD-EA Capture

Operator tooling for the **NLD-EA** (Nemo Liveness Dataset, East Africa) field
campaign — 400 subjects, ~3,200 genuine + ~2,000 attack clips
([docs/nemo/10-liveness-stage0-and-data-campaign.md](../docs/nemo/10-liveness-stage0-and-data-campaign.md),
[docs/ekyc/06-level2-upgrade-plan.md](../docs/ekyc/06-level2-upgrade-plan.md)).
This is internal operator tooling, not a customer app: a field agent guides a
paid subject through the capture protocol and the app emits session
directories in the exact format the training pipeline and red-team rig
consume.

## Session directory contract (`schemaVersion` 1)

One directory per session, named by `sessionId`:

```
<sessionId>/
  clip_001.mp4 …          ~3 s front-camera clips
  clip_001_thumb.jpg …    review stills (best-effort, not in the manifest)
  manifest.json
  checksums.json          {"algorithm":"sha256","files":{<name>:<hex>…}}
```

`manifest.json` (all keys always present; see `lib/core/models/manifest.dart`
— the single source of truth for wire values):

```json
{"schemaVersion":1,"sessionId":"<uuid>","subjectId":"NLD-XXXXXXXX",
 "consentId":"C-<uuid>","type":"genuine|attack",
 "attackType":null,"device":{"model":"…","os":"…"},
 "lighting":"daylight|indoor|low_light","skinTone":"monk_01…monk_10|null",
 "capturedAt":"<iso8601 UTC>",
 "clips":[{"file":"clip_001.mp4","durationMs":3012,"fps":30,
           "challenge":null}]}
```

`attackType` wire values: `print_flat`, `print_curved`, `replay_phone`,
`replay_monitor`, `cutout_paper`, `mask_3d` (schema-present, capture deferred
to the L2 phase). `challenge` wire values: `blink`, `turn_left`,
`turn_right`, `smile` (null = neutral hold).

## Flow

Home → **Consent** (hard gate: every capture route redirects here until the
subject agrees; pseudonymous `subjectId` + `consentId` minted on agree,
consent record stored under `consents/`, separate from media) → Session setup
(genuine/attack, species, lighting station, fleet device) → optional Monk
skin-tone self-report (genuine only) → guided capture (neutral → blink →
turn left → turn right → smile; attack mode records N neutral presentation
clips) → review/retake → finalize (writes manifest + checksums).

Also: **Withdrawal** (enter a subject ID → mark consent withdrawn → delete
that subject's sessions — the DPIA withdrawal-by-subject-ID commitment),
**Dashboard** (progress vs doc-10 targets incl. gate G2 = 200 subjects and
the ≥60% Monk 7–10 quota), **Sessions & export** (zip + share sheet).

## Compliance notes

- The consent copy in `lib/core/models/consent_record.dart` is a
  **PLACEHOLDER pending DPIA counsel review**. No field capture before gate
  G1 (DPIA filed with the ODPC). Bump `consentTextVersion` on any change.
- No PII anywhere: subjects exist only as generated `NLD-` pseudonyms. The
  pseudonym↔person mapping lives on the signed paper/tablet consent form
  held by the field lead, never in this app.

## Later phases (deliberate stubs)

- **Upload service** — `StubUploadService` in
  `lib/core/services/export_service.dart` (throws). v1 hand-off is zip +
  share sheet only; phase 2 implements direct upload to the encrypted
  on-prem store behind `uploadServiceProvider`. No cloud storage — DPIA
  commits to on-prem.
- **`mask_3d` attacks** — in the schema and dashboard, disabled in the UI
  until L2-phase mask fabrication (doc 06 §7b).

## Development

Flutter (stable). Camera is isolated behind `CaptureCamera`
(`lib/core/services/capture_camera.dart`); tests and camera-less machines
override `captureCameraProvider` with `FakeCaptureCamera`, so
`flutter analyze` and `flutter test` run without a device.
