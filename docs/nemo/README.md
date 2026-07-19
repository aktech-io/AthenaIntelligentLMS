# Nemo — Neobank in a Box

Nemo is the product identity of this platform: a full-stack, multi-tenant digital banking
platform that lets a partner (bank, MFI, fintech, telco) launch and operate a neobank —
deployable on cloud or on-premise, configured per market, with AI at the core of
decisioning and operations.

The platform was built out of the Athena Intelligent LMS codebase; the 16 Go services,
double-entry GL, compliance stack and AI scoring are the existing foundation. These
documents define the concept and the path from "lending platform" to "neobank in a box".

| Doc | Contents |
|-----|----------|
| [01-vision.md](01-vision.md) | Concept, positioning, tenant model, business model, competitive landscape |
| [02-gap-analysis-and-roadmap.md](02-gap-analysis-and-roadmap.md) | What exists today, every capability gap to close, and the phased roadmap |
| [03-engineering-execution-plan.md](03-engineering-execution-plan.md) | EM status board: issue list by track, business blockers, immediate queue |
| [04-wallet-app-reuse-audit.md](04-wallet-app-reuse-audit.md) | NemoWallet (ex-AthenaMobileWallet, Flutter) reuse audit for the A1 customer app |
| [05-decision-engine-design.md](05-decision-engine-design.md) | E1 unified decision engine: policy spine, decision log, explainability, governance |
| [Pitch deck v1](https://claude.ai/code/artifact/0e8de1e3-2879-4840-8313-f3d6b6754911) | 12-slide investor/partner deck (Claude artifact, private until shared) |
| [Pitch deck v2](https://claude.ai/code/artifact/2da0adb6-9eca-4173-85e7-8223bd700023) | v2 with mobile-app mockup imagery — the preferred version |
| [App concept](https://claude.ai/code/artifact/143549e3-548b-4613-be57-947778abb3d6) | 12 high-fidelity customer-app screens across 7 feature areas incl. crypto & AI banker (source: deck/nemo-app-concept.html) |
| [deck/](deck/) | Deck HTML sources + `build_nemo_pptx.py` (regenerates the .pptx) |
| [exports/](exports/) | `nemo-pitch-deck.pptx` (editable PowerPoint) and `nemo-pitch-deck.pdf` (pixel-faithful render of v2) |

## Google Drive

Working copies live in the Drive folder [Nemo — Neobank in a Box](https://drive.google.com/drive/folders/1m3k0cls3G4akNdQooPgqhGfmJVqfLS_3):
[Vision & Positioning](https://docs.google.com/document/d/1igVb_qo_pnq3W0zrWhMNTXYgST5pJWUna1q2NwQyYXs/edit) ·
[Gap Analysis & Roadmap](https://docs.google.com/document/d/1mJlX9R35rFv3PuGlxt0zCU4qAXtcRUffqbqmRjeTLl8/edit).
`scripts/drive-sync.sh` pushes `docs/` (including the deck exports) to that folder via rclone
(one-time setup: `rclone config` → create a remote named `gdrive`).

Naming note: code, repos and service names still say "LMS"/"loan-*". The rebrand to Nemo
is deliberate-but-gradual — docs and new work use Nemo naming; existing identifiers are
renamed opportunistically (tracked in the roadmap, Phase 1).
