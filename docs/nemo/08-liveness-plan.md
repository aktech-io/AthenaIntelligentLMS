# Liveness & 1:1 face verification — tiered plan

*2026-07-31. Decision + build plan for anti-spoofing in KYC selfie capture,
from a sourced build-vs-buy survey (research session, July 2026). Extends A2;
the 1:1 match itself (YuNet detect + SFace embed, ekyc-ml) already exists —
this adds "is it a live person", which v1 approximated as "a face was
detected". APPROVED to build: Tier 1 + Tier 2 (2026-07-31).*

## The tiers

**Tier 1 — on-device ACTIVE challenge (free, default).** ML Kit Face
Detection (`google_mlkit_face_detection`, classification mode) exposes
eye-open probability, smile probability and head Euler angles — enough for a
randomized 2–3 step challenge (blink → turn head → smile) rendered live over
the selfie camera. Frames are captured mid-challenge and become the selfie
set sent to the backend. Zero per-check cost, offline-capable, the most
widely deployed detector in mobile fintech. Limit: 2D only — a video replay
of a person doing the challenge passes; that's what Tier 2 is for.

**Tier 2 — server-side PASSIVE PAD (free, default).** MiniVision
Silent-Face-Anti-Spoofing (MiniFASNetV2, Apache-2.0, ~600 KB as ONNX) added
to ekyc-ml next to YuNet/SFace: takes an 80×80 face crop from the existing
YuNet detection and classifies live / print-attack / replay-attack. This is
the model DeepFace ships as its `anti_spoofing` backend — the most-deployed
OSS PAD. It catches print-outs and most screen replays (moiré/reflection
artifacts) that beat Tier 1.

**Calibration warning (must-do):** MiniFASNet's training data skews
East-Asian; documented false rejects in low light and a real risk of elevated
false rejects on darker skin tones. Ship it **shadow-mode first** (log
scores, decide nothing), calibrate thresholds on real Kenyan traffic, keep
retry-not-reject UX when enforcement turns on.

**Tier 3 — certified commercial, pluggable (per client/market).** The bank
bar is iBeta ISO/IEC 30107-3 PAD Level 2. Best fits:
- **ID R&D IDLive Face** — the only L1+L2 *single-image passive* product;
  ships as an on-prem Docker/C++ SDK that drops in server-side with zero
  Flutter changes; keeps biometrics on-prem (helps the ODPC/DPIA argument).
- **Smile ID SmartSelfie** — L2 (Fime 2025, 0% penetration), official
  Flutter SDK, Africa pricing (~$0.30–1.00/full-KYC check), plus Kenyan
  ID-registry lookups. Fastest regional end-to-end buy.
- AWS Rekognition Face Liveness — $0.015/check managed fallback; only
  iBeta-*benchmarked*, no official Flutter support. Budget option, not
  flagship.
Wire these behind a **LivenessProvider seam** ({frames} → {liveScore,
decision, provider, auditRef}) in the same registry style as the eKYC
vendors; escalate to Tier 3 on risk signals (large first deposit, device
anomaly, borderline Tier-2 score) instead of paying per-check on everything.

## Regulatory notes (from the survey)

- **Kenya**: no statute names a PAD certification; CBK guidance permits
  biometric remote onboarding. The binding constraint is the **Data
  Protection Act: a DPIA is required** before selfie/liveness/face-match at
  scale. Certified PAD is partner/auditor due-diligence, not law.
- **Ethiopia**: NBE has mandated **Fayda digital ID for banking** (phased
  2025→2026) with onboarding through **VeriFayda 2** (national eKYC via
  EthSwitch) — so ET liveness/1:1 likely routes through the national
  platform. Treat VeriFayda as a third provider behind the same seam; our
  own tiers still cover non-ET markets and fallback.

## Build plan (Tier 1+2, ~1 wk total)

1. **ekyc-ml `/v1/face/liveness`** (~2 d): multipart frames[] → YuNet crop →
   MiniFASNetV2 ONNX (model file an ops drop-in like the SFace models;
   deterministic low-confidence fallback when absent, same pattern) →
   {liveScore, label, perFrame[]}. Shadow mode via env
   `LIVENESS_ENFORCE=false` (default): score is logged into the provider
   result but never fails verification.
2. **inhouse provider** (~½ d): call /liveness with the selfie frames;
   `LivenessPassed = face detected` (unchanged) while shadow; when
   enforcement is on, `LivenessPassed = liveScore ≥ threshold` and referral
   (not rejection) on fail. Score + mode recorded in decision reasons.
3. **Flutter challenge screen** (~2–3 d): `google_mlkit_face_detection`
   (classification mode), randomized challenge sequence, live progress ring,
   frame capture per completed step (2–3 frames), frames uploaded as the
   selfie set (mediaType SELFIE, multiple). Fallback to today's single-shot
   selfie when the device can't run detection.
4. **Board/QA**: emulator can't do real camera liveness — challenge screen
   gets widget tests + a debug bypass flag; real testing is a device task.

## Open items
- Model provisioning: add MiniFASNetV2 ONNX to the ekyc-ml image build (same
  README drop-in section as YuNet/SFace) + offline bundle note (D2).
- DPIA artifact for ODPC before production enforcement in Kenya.
- VeriFayda 2 integration scoping for ET (separate design doc when ET goes
  live; NBE account-harmonization deadline 2026-04-08 already passed for
  existing banks — check the current onboarding requirement when ET pilot
  starts).
