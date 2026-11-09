# Invoice Finance — progress and handoff

_Last updated 2026-11-09. Read this first, then SPEC.md and COREBANKING.md._

## Where things stand

**Backend (Go, `go-services/cmd/invoice-service`, port 8101, db `athena_invoice`) — done and live on Contabo.**
Full pipeline verified end to end on the live stack: upload → OCR (ekyc-ml `POST /v1/invoice/extract`) →
buyer confirmation by email link → scoring (ai-scoring-service → athena-python-service) → fraud checks →
policy `invoice.advance.ke.yaml` → REFER → manager approve → offer → seller accepts → funded via
loan-origination (loan + GL entries) → buyer payment → PAID. Ledger is hash-chained, HMAC-signed, hourly
Merkle roots (OpenTimestamps anchoring stubbed). Public confirm/verify endpoints work. Duplicate upload → 409.
Core banking is pluggable (`internal/invoice/corebanking`: `nemo` and `webhook` adapters).

**Portal (`lms-portal-ui`) — functional, visually rejected.** `APP_MODE` runtime switch builds three apps
from one image: `portal` (staff, Invoice Finance module), `invoice-admin`, `invoice-client` (seller).
Deployed on Contabo as `invoice-admin-ui` and `invoice-client-ui` (namespace `lms`). The user judged the
current UI "a high school project, not something you can demo". A redesign plan and approved-register
mockups exist (see Design below). **Step 2 (the rebuild) has not started.**

**NemoWallet (Flutter) — seller feature exists** (`lib/features/invoice_finance`), light theme is default
with an Appearance picker. Needs aligning to the new design system once the web rebuild lands.

## Design (what to build to)

- `design/mockups.html` — the approved direction: **Wise Business register** (friendly, clear, consumer-polished).
  Views: Overview, Invoice detail, **Payments** (added 2026-11-08), Upload, Admin workbench, System (tokens).
  Open the file in a browser; `#overview`, `#payments` etc. switch views; `?theme=dark` for dark.
- `design/redesign-plan.html` — the 5-step plan the user asked for and accepted.
- Reference screenshots: `design/overview-1440-light.png`, `design/pay-1440-light.png`, `design/pay-390-light.png`.
- Rules: light-first; white surfaces, hairlines; single coral accent `#E24F22`; Inter Tight, tabular numerals;
  radius 12/16, pill buttons; grey-dot statuses; no stripes/gradients; never show internal IDs to sellers;
  friendly copy ("Nudge them", "Ask my buyer to confirm", "You can still finance").
- Live artifacts (owner's claude.ai account): mockups `https://claude.ai/artifact/DgQT1RcGMaY42nbUjpTXVD`,
  plan `https://claude.ai/artifact/JkvDTjvVvCxQ4Zho6SUWtT`.

## Step 2 — next work (waiting on the user's "go")

1. **Seller web app rebuild** (`lms-portal-ui/src/pages/seller/**`) to `design/mockups.html`: top nav, dense
   table + side panel, PDF-hero invoice detail, Payments page, Upload with "What we read" panel.
2. **Payments for the seller** (gap found 2026-11-08). Backend already stores it per advance
   (`model.Advance`: principal, feesAccrued, repaid, feesRepaid, reserveAmount, reserveReleased;
   `model.Payment` rows). Missing:
   - seller-scoped `GET /invoices/mine/payments` (ledger: advances out, buyer payments, reserve releases, fees,
     upcoming due dates) and `GET /invoices/mine/statement?month=` (CSV/PDF);
   - seller repayment `POST /invoices/{id}/repay` {amount, source: wallet|mpesa|bank} for the
     "buyer paid me directly" case — today only `h.officer` can `POST /{id}/payments`;
   - payout account (bank / wallet) on the seller profile.
3. **Claude vision as primary invoice extractor.** Quiklink (`quiklink-tms/internal/extract`, model
   `claude-sonnet-4-6`, calibration in `quiklink-tms/docs/EXTRACTION.md`) proved the pattern. Add a Claude
   engine to ekyc-ml `POST /v1/invoice/extract` with structured JSON + per-field confidence + line items;
   keep PP-OCR/Tesseract/PyMuPDF as offline fallback. **Needs an `ANTHROPIC_API_KEY` secret on Contabo** —
   user has not yet said whether to reuse Quiklink's key or create a Nemo one.
4. **Session refresh** for the web client: bff-gateway mobile tokens expire in 15 min and the seller web app
   has no refresh (known defect).
5. **Demo tenant seed**: ~14 invoices across statuses, buyers with history, payments ledger, so the demo
   looks like the mockup.
6. Then Step 3 admin workbench, Step 4 wallet alignment, Step 5 public confirm/verify pages.

## Environment and access

- Contabo k3s `158.220.112.84`, `ssh deploy@…`, `sudo k3s kubectl -n lms`. CI: GitHub workflow "Deploy Contabo"
  (GHCR images, manifests from `deploy/k8s/contabo/gen-manifests.py`). Hot deploy:
  `scripts/hotdeploy-invoice-service.sh`. E2E: `scripts/demo-invoice-finance.sh`.
- Staff tenant on Contabo is `admin` (mobile default is `default`) — `TENANT_ID` runtime config in the UI,
  `NEMO_TENANT_ID` in the wallet.
- Test URLs until DNS exists: seller `http://158.220.112.84:30081`, admin `http://158.220.112.84:30082`.
  **User still has to add Namecheap A records** `invoice` and `invoice-admin` → `158.220.112.84`; then verify
  TLS via ingress and remove the NodePorts.
- Demo-only: `OTP_ECHO_ENABLED=true` on bff-gateway (OTP returned in the response). Disable once SMS exists.

## Known follow-ups

- Orphan APPROVED loan-origination application left when a fund attempt fails; clean up on retry.
- OpenTimestamps anchoring is a stub.
- Wallet shows a hard-coded `$128,430.50` balance somewhere — currency bug.
- No-data sellers score 433/Poor (placeholder model); band mapping Fair+Marginal→C, Poor→D; D/C → REFER.
