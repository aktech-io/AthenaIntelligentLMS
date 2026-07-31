# OCR-first onboarding — KE + ET documents, on-device ML Kit

*2026-07-31. Plan for collapsing registration to: scan document → OCR
pre-fills everything → confirm → phone OTP → selfie → done. Covers Kenya
(national ID, passport) and Ethiopia (Fayda national ID, passport) as the
test matrix. Extends A2; consumes market-pack kycRuleSet ids (C2).*

## 1. Goal & UX target

Today the user types name/ID/DOB manually, then captures the document —
the typed values exist only to be cross-checked against server OCR. Flip
it: **the document is the source of truth and the form is a confirmation
screen.** Target flow:

1. Choose document type (from the market pack's accepted list)
2. Scan it (camera, with live quality gating)
3. **On-device OCR** (Google ML Kit) extracts name / doc number / DOB /
   sex / expiry → pre-fills the details screen
4. User confirms (edits only if OCR got something wrong)
5. Phone number + OTP
6. Selfie → submit → server verifies as it does today

Registration effort drops to one scan + one OTP for the happy path.

## 2. Why ML Kit on-device (and its one hard limit)

- `google_mlkit_text_recognition` (Flutter): free, offline, on-device —
  no image leaves the phone before the user consents to submit; instant
  results for pre-fill; bundled Latin model ≈ +4 MB APK (bundled variant
  avoids the Play-Services dependency — matters for white-label OEM ships).
- **Limit: no Amharic/Ge'ez script support** (v2 scripts: Latin, Chinese,
  Devanagari, Japanese, Korean). Consequences per document:

| Market | Document | On-device strategy |
|--------|----------|--------------------|
| KE | National ID | English labels/values — full field extraction |
| KE | Passport | **MRZ (TD3)** — pure Latin, checksum-verifiable |
| ET | Fayda national ID | Bilingual card — extract the **English/Latin** half; Amharic ignored on-device (server keeps full image) |
| ET | Passport | **MRZ (TD3)** — same as KE |

MRZ is the great equalizer: for passports we don't scrape the visual zone
at all on-device — parse the two 44-char lines, verify check digits, done.
The server engine already has an ICAO 9303 parser (`engine/mrz.py`,
TD1+TD3) whose logic we port to Dart.

## 3. Workstreams

### WS-1 — Document taxonomy through the stack (backend, ~1–1.5 wk)
- `documentType` on the onboarding submission (BFF + compliance-service +
  provider interface): `NATIONAL_ID | PASSPORT` (extensible).
- Market pack `kycRuleSet` becomes real config: accepted doc types per
  market + per-doc validation (KE ID: 7–9 digit number; KE/ET passport:
  TD3 MRZ mandatory; ET Fayda: FIN 12 / FAN 16 digit), OCR profile id,
  and which fields are required vs optional.
- ekyc-ml-service: extraction **profiles keyed by (market, docType)** —
  KE ID profile exists (label-anchored); add passport profile (MRZ-only,
  visual zone as fallback) and ET Fayda profile (English labels;
  Tesseract `amh` traineddata optional, not required for v1).
- `inhouse.go` provider: pass docType, apply per-doc DocumentVerified
  rules (passport: MRZ check digits must pass + declared values match MRZ;
  IDs: confident fields + declared-value agreement as today).

### WS-2 — On-device OCR in the app (Flutter, ~1.5–2 wk)
- Add `google_mlkit_text_recognition` (bundled Latin).
- Doc-type picker screen fed from brand/market config (which comes from
  the pack via the BFF — one new tiny endpoint or folded into the
  existing brand payload).
- Dart extraction layer mirroring the server profiles:
  - `MrzParser` (TD3 + TD1, check-digit validation) — port of
    `engine/mrz.py`.
  - `KenyaIdProfile`, `FaydaProfile`: label-anchored line matching on ML
    Kit's text blocks (same anchor strategy as the Python engine).
- Live capture quality gates before OCR: blur (Laplacian variance),
  glare, minimum text density; retake prompts. MRZ retry loop: keep
  sampling frames until check digits validate (cheap and dramatic UX win).
- Details screen becomes a **pre-filled confirmation** (fields editable,
  OCR-confidence chips per field); manual-entry path stays as fallback
  when OCR confidence is low or the doc type has no profile.
- Reorder the flow: doc scan → confirm → phone/OTP → selfie → submit.

### WS-3 — Server stays authoritative (~0.5 wk)
On-device OCR is a UX assist, never a control: a tampered client can send
anything, so the server re-extracts from the uploaded image and
cross-checks the (OCR-pre-filled, user-confirmed) declared values exactly
as today — now with docType-aware rules. No verification decision moves
client-side. Selfie/face-match and screening unchanged.

### WS-4 — Test matrix & assets (~0.5 wk)
- Synthetic document generator (extend the emulator-test approach): KE ID
  (exists), KE passport w/ valid TD3 check digits, ET Fayda (bilingual
  layout), ET passport. Deterministic personas per market.
- ekyc-ml unit tests per profile; pytest API tests for docType paths;
  Flutter widget/unit tests for MrzParser + profiles; emulator E2E per
  document type (gallery-upload path, as proven 2026-07-31).

## 4. Sequencing & estimate

WS-1 and WS-2 run in parallel (backend vs app); WS-3 rides WS-1; WS-4
last. **~4 engineer-weeks total, ~2.5 weeks wall-clock with the two
tracks in parallel.** Ship order: KE passport (MRZ, cheapest + highest
confidence) → KE ID (profile exists server-side) → ET passport (free
once MRZ ships) → ET Fayda (only genuinely new extraction work).

## 5. Open decisions

1. **Fayda field set** — **DECIDED (2026-07-31): FAN, 16-digit** (the
   number printed on the card). FIN stays out of scope for v1.
2. **NFC later**: both KE/ET passports are e-passports; ML Kit MRZ gives
   us the BAC key (doc no + DOB + expiry) — an NFC chip-read (true
   authenticity check) is a natural Phase-2 upgrade, not in this scope.
3. **DOB becomes required for passports** (MRZ carries it; used for BAC
   later) — confirm product is fine making it mandatory there.
