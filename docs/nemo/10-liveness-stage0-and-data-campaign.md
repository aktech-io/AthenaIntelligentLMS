# Liveness Stage 0 & NLD-EA data campaign — execution plan

*2026-08-01. Executes doc 09 Stage 0 (bridge SDK) and the field dataset.
Illustrated version (figures, timeline, budget tables) published as an
internal artifact — ask the session owner for the link. This file is the
repo-canonical summary.*

## Part 1 — Stage 0: bridge SDK

- **What**: license an iBeta-Level-2-certified on-prem passive liveness SDK
  and mount it behind the existing `LivenessProvider` seam as a second
  implementation (in-house scorer stays on 100% of traffic in shadow;
  Ethiopia routes to VeriFayda regardless).
- **Shortlist**: MiniAiLive (~$3–8k/yr, Linux/Docker), KBY-AI (~$2–6k/yr,
  server + mobile SDKs), Facia (fallback, quote). All hold current L2
  letters.
- **Evaluation (2 weeks)**: days 1–4 bench vs our held-out genuine/attack
  captures (APCER/BPCER/latency on box CPU); days 5–7 shadow side-by-side
  with ekyc-ml on staging (disagreement rate); parallel license diligence —
  on-prem terms, **white-label/resale rights** (required for the Nemo KYC
  product), letter matches shipped SDK version, no telemetry. Decision
  gates: APCER ≤1% on our attacks, BPCER ≤10% on our genuine set, ceiling
  $8k/yr.
- **Founder actions**: send outreach to MiniAiLive + KBY-AI (draft in the
  artifact §1.4); everything technical is ready on receipt of eval builds.

## Part 2 — NLD-EA (Nemo Liveness Dataset, East Africa)

Targets: **400 consenting subjects → ~3,200 genuine clips + ~2,000 attack
clips in 8 weeks, ≈ $8.6k all-in.**

- **Compliance gate (G1, end of week 2)**: DPIA filed with ODPC before any
  capture; tablet-signed consent (plain-language purpose, compensation,
  withdrawal-by-subject-ID with deletion, 36-month retention, encrypted
  on-prem storage, anti-fraud-model-development-only use); minors excluded.
- **Session protocol (~15 min/subject)**: 3 lighting stations (daylight
  >1,000 lux; office ~300 lux; dim <50 lux — deliberately over-sampled, it
  is MiniFASNet's failure zone) × 2 devices × 2 takes = 12 genuine clips,
  including a full blink→turn→smile challenge run so training matches
  production capture.
- **Attack desk** (from each subject's own media, mapped to the iBeta L1
  repertoire): matte print 400, glossy print 300, cutout mask 300,
  phone-screen replay 500, monitor replay 300, challenge-replay 200.
- **Quotas**: Monk skin-tone 1–10 with ≥60% in 7–10; age 45/40/15
  (18–30/31–50/51+); gender 50/50±5; device fleet = actual Kenyan market
  (2 budget Tecno/Redmi + 2 mid-range Samsung A/Camon), model recorded per
  clip; sites Nairobi CBD + university + peri-urban, Addis pilot (50
  subjects) weeks 7–8.
- **Pipeline**: capture app (consent + script) → encrypted on-prem store
  keyed by erasable subject-ID → label sheet
  (genuine|attack·species, device, lux, station, monk) → subject-disjoint
  train/val/red-team shards; the red-team shard never touches training and
  becomes the internal certification rig (doc 09 Stage 2).
- **Budget**: compensation $2,480 (400 × KES 800) · agents $2,400 · devices
  $900 · kit $450 · attack materials $350 · legal/DPIA $1,200 ·
  contingency $780 → **$8,560**.
- **Gates**: G1 DPIA filed (W2) · G2 200 subjects (W5, triggers first
  doc-09 fine-tune on half data) · G3 NLD-EA v1 frozen (W8).

## Founder decisions pending

1. Send Stage-0 outreach (two emails).
2. Approve $8.6k campaign budget + KES 800 compensation rate.
3. Name counsel for the DPIA.
