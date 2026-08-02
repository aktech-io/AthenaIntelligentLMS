# Attack taxonomy

The species the rig scores APCER against. Generated from `liveness_redteam/taxonomy.py` — the source of truth.

APCER is reported per species and gated on the **worst** species (ISO/IEC 30107-3), never the pooled mean.

## Level 1 — 2D attacks (iBeta L1)

Pass bar: **0% APCER** over ~900 presentations, BPCER ≤15%. Internal gate: 0/500.

### `print_flat` — Flat print

- **Category**: print
- Matte or glossy photographic print of the subject's face held flat in the capture frame.
- **Materials**: A4/A5 photo paper, matte and glossy stock, >=300 dpi lab print of a frontal portrait at life size.
- **Capture**: Hold rigid (mounted on card) and fill the guide oval. Capture under all three lighting stations; glossy stock must also be captured with a deliberate specular highlight off-frame.

### `print_curved` — Curved print

- **Category**: print
- The same print bent around a cylinder to fake facial curvature and defeat naive flatness/parallax heuristics.
- **Materials**: Flat print wrapped on a 80-120 mm mug/tube former.
- **Capture**: Vary the curvature radius across takes and add a small hand wobble — a rigid curved print is the easiest curved variant.

### `replay_phone` — Phone-screen replay

- **Category**: replay
- Genuine selfie clip replayed on a handset screen, including challenge-replay takes (a recorded blink/turn played back).
- **Materials**: 2 budget (Tecno/Redmi) + 2 mid-range (Samsung A / Camon) handsets from the NLD-EA device fleet, brightness at max.
- **Capture**: Vary attacker-screen brightness and distance; the moire sub-score is distance-sensitive, so include a near-field take where the pixel grid is not resolvable.

### `replay_tablet` — Tablet-screen replay

- **Category**: replay
- Replay on a ~10" tablet: life-size face, lower pixel density than a phone, so moire and screen-bezel cues differ.
- **Materials**: 10-11" Android tablet or iPad, brightness at max.
- **Capture**: Life-size rendering is the point — scale the clip so the face matches real head size at the capture distance.

### `replay_monitor` — Monitor replay

- **Category**: replay
- Replay on a desktop monitor: largest, brightest and lowest-DPI replay surface, strongest moire signature.
- **Materials**: 24-27" IPS monitor, matte and glossy panels if available.
- **Capture**: Include an off-axis take (~20-30 deg) — panel viewing-angle colour shift is a strong cue we must not accidentally rely on.

### `cutout_paper` — Paper cutout

- **Category**: cutout
- Print with the eye and/or mouth regions cut out and a live attacker's eyes/mouth behind it — defeats blink and mouth-movement challenges while the rest of the face is flat.
- **Materials**: Flat print, scalpel, optional card former for curvature.
- **Capture**: Capture three variants: eyes-only, mouth-only, eyes+mouth. Always run the active challenge on these — they exist to beat it, so 'challenge passed' must not be sufficient on its own.

## Level 2 — 3D masks (iBeta L2)

Pass bar: **worst-species APCER ≤1%**. Schema-ready; exercised once mask fabrication produces sessions.

### `mask_silicone` — Silicone mask _(future)_

- **Category**: mask
- Custom-cast platinum silicone mask of a consented NLD-EA subject: realistic skin translucency and 3D geometry.
- **Materials**: Life-cast + platinum silicone, flocked hair, painted. Fabricate during the L1 lab window (docs/ekyc/06 §7b).
- **Capture**: Worn (not held) takes only — a worn mask moves non-rigidly and is the honest L2 threat model.

### `mask_latex` — Latex mask _(future)_

- **Category**: mask
- Latex/rubber mask: cheaper, matte, less translucent than silicone — different reflectance failure mode.
- **Materials**: Cast latex mask, optionally airbrushed for skin tone.
- **Capture**: Worn takes; include a commercial off-the-shelf mask.

### `mask_resin` — Resin / rigid mask _(future)_

- **Category**: mask
- 3D-printed or cast rigid resin mask: accurate geometry, rigid motion, no skin translucency.
- **Materials**: Photogrammetry scan -> resin print, painted.
- **Capture**: Rigid motion makes the parallax non-rigidity sub-score the primary defence — capture both worn and held-on-a-stand takes.

### `mask_3d` — 3D mask (unspecified material) _(future)_

- **Category**: mask
- Generic 3D-mask value carried by the shared NLD-EA manifest contract, for captures where the material is not recorded.
- **Materials**: Any of silicone / latex / resin.
- **Capture**: Prefer the material-specific keys when known — worst-species APCER is only meaningful at material granularity.

## Genuine presentations

`type: "genuine"`, `attackType: null`. Needed for BPCER (bona-fide presentation classification error rate). Capture the same device / lighting / Monk-tone spread as the attacks.

