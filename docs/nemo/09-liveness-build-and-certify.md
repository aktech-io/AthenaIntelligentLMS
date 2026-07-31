# Liveness as a product — build-and-certify plan

*2026-08-01. Answer to "are there Chinese/open models that get us to Level 2,
and why not build this as a business?" — from a sourced landscape survey
(research session; all links in §5). Extends doc 08. Verdict up front:
**yes, and yes** — small teams demonstrably pass iBeta on open-lineage
stacks, and an owned, certified, East-Africa-calibrated liveness stack is a
defensible asset no vendor matches.*

## 1. The landscape in one paragraph

The strong PAD research is largely Chinese-published and open: our current
MiniFASNetV2 is MiniVision's *demo tier* (the high-precision model stayed
closed); CDCN/DeepPixBiS-era models are retrain-it-yourself research code;
the CelebA-Spoof dataset (625K images) is usable training data. The two
deployable modern candidates are **FLIP** (CLIP-based, best published
cross-dataset generalization of its generation) and **FoundPAD** (2025,
DINOv2+LoRA, built for unseen-domain generalization) — both public code,
both server-sized (distill to a mobile-class student for our shape).
Critically: several **micro-vendors hold iBeta Level 2 with stacks widely
believed to descend from this same open lineage** (MiniAiLive, KBY-AI —
sold as $2–8k/yr on-prem SDKs). A research lab is not required; a
disciplined attack-replication lab is.

## 2. What certification actually takes

- **iBeta Level 1**: ~900 presentations across 6 cheap attack species
  (print, replay, cutouts); pass = **0% APCER** (zero spoofs through) with
  BPCER ≤15% (generous — aggressive spoof-averse thresholds + retry UX are
  legal). Tested per app-build per device.
- **Level 2**: 3D attacks (silicone/latex/resin masks), APCER ≤1%.
- **Cost**: ~$10–50k per submission; realistic **Level-1 budget $25–40k**
  (lab fee + one retest reserve + rig/devices). 4–12 weeks. Retests are
  normal and billed.
- **Small-team proof**: iBeta's letter list includes 2-to-10-person shops
  at both levels.

## 3. The staged plan (2 engineers, 3–6 months)

**Stage 0 (now): buy the bridge, cheaply.** License a
MiniAiLive/KBY-AI-class iBeta-L2 on-prem SDK (~$2–8k/yr) wired behind the
existing LivenessProvider seam — sales can say "Level 2 certified" from
next week while we build. NOT per-check SaaS (data residency + unit
economics).

**Stage 1 (months 1–4): two model upgrades, in order.**
1. **Multi-frame fusion** — replace single-frame scoring with 10-frame
   aggregation (median PAD score + inter-frame parallax + moiré/frequency
   features) fused with the ML Kit challenge result into one decision
   score. Biggest APCER win per engineering hour; zero licensing risk.
   The Tier-1 challenge screen already captures multi-frame — the plumbing
   exists end-to-end.
2. **DG teacher → distilled student** — fine-tune FLIP or FoundPAD on
   CelebA-Spoof (license check first) + **≥5k locally captured
   genuine/attack videos** (Kenyan/Ethiopian faces, our-market phones —
   this data is the actual moat), distill to a MobileNetV3-class ONNX
   student in the exact deployment shape we run today. This kills
   MiniFASNet's skin-tone/lighting domain gap — in the field and in the
   lab.

**Stage 2 (months 4–6): certify Level 1 only.** Build the internal
red-team rig replicating the L1 attack repertoire on 3–4 handsets; book
iBeta only after sustaining 0/500 internal APCER. Defer Level 2 (~$30–50k
more + 3D mask fabrication skills) until liveness is revenue-attributable
— the bridge SDK covers L2 claims meanwhile. Get a competing quote from
BixeLab.

## 4. The business case

- **Cost avoided**: Smile ID-class pricing is $0.30–1.00/check forever;
  our L1 certification is a one-time ~$35k + the build we mostly have.
- **Product**: "Nemo KYC" — the doc-07+08 stack (scan-first onboarding,
  OCR, face match, screening, liveness, referral queue) as a white-label
  per-check API through the G2 platform, certified, calibrated on East
  African faces. Smile ID proves the demand; no Chinese SDK matches the
  regional calibration claim.
- **Skip condition**: only if liveness stays a pure internal checkbox.
  Given the neobank-in-a-box strategy, it doesn't.

## 5. Sources

Models: github.com/minivision-ai/Silent-Face-Anti-Spoofing ·
github.com/ZitongYu/DeepFAS · github.com/koushiksrivats/FLIP ·
github.com/gurayozgur/FoundPAD · github.com/ZhangYuanhan-AI/CelebA-Spoof ·
arxiv.org/abs/2305.03277 (FM-ViT) · arxiv.org/pdf/2604.19196 (VFM-PAD).
Certification: ibeta.com/iso-30107-3-presentation-attack-detection-confirmation-letters ·
axonlab.ai/ibeta-certification-requirements-overview · ibeta.com/fido ·
bixelab.com. Bridge/comparables: miniai.live/ibeta-level-2-liveness-detection ·
kby-ai.com/face-liveness-detection-sdk ·
github.com/MiniAiLive/FaceLivenessDetection-SDK-Linux ·
faceplusplus.com/v2/pricing · internat-zoloz-site.alipay.com/pricing ·
intl.cloud.tencent.com/products/faceid · iproov.com/certifications.
