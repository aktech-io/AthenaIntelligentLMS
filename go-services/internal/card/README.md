# card-service — Nemo B1 card issuing

Issues and manages payment cards (virtual-first debit) through a pluggable
issuer-processor adapter. Port 8107 in-cluster, 28107 on the compose host,
DB `athena_cards`, gateway prefix `/lms/api/v1/cards/` (routeKey
`CARD_SERVICE`).

## Processor adapters (`internal/card/processor`)

Selected with `CARD_PROCESSOR` (default `sandbox`), mirroring the eKYC
provider registry idiom:

- **`sandbox`** — deterministic, no-vendor, **test/demo only**. No PAN ever
  exists; a stable `processorRef` + `last4` are derived from the request so
  demos and tests reproduce. Cardholder name `DECLINE ME` exercises the
  processor-decline path.
- **`paymentology`** — the real adapter (founder decision 2026-07-18:
  Paymentology, Kenya BIN sponsorship via Diamond Trust Bank). Currently a
  faithful-but-stub implementation: the full config surface exists
  (`PAYMENTOLOGY_BASE_URL`, `PAYMENTOLOGY_PROGRAM_ID`, `PAYMENTOLOGY_API_KEY`,
  `PAYMENTOLOGY_API_SECRET`, `PAYMENTOLOGY_WEBHOOK_SECRET`) but every call
  fails fast with a "not configured" error until API credentials arrive from
  the commercial deal. Each method carries a `TODO(paymentology)` documenting
  the call to wire and `⚠ VERIFY` markers where the shape is inferred from
  public knowledge of their API family and must be checked against partner
  docs.

## PCI-DSS posture (important)

**This service never stores, logs, or transports full PANs, CVVs, PINs,
expiry dates, or track data.** The processor is the PCI card-data
environment; Nemo keeps exactly two card identifiers:

- `processor_ref` — the processor's opaque card id (the handle for all
  lifecycle calls and webhook correlation),
- `pan_last4` — the last 4 digits, permitted for display under PCI-DSS.

The schema (`migrations/card`) has no column such data could live in, the
processor seam (`IssueResult`) has no field it could travel in, and domain
event payloads carry `processorRef` + `panLast4` only. Real PAN display in
the customer app will use **processor-side tokenized reveal** — the device
fetches the PAN directly from the processor's PCI-scoped widget/SDK with a
short-lived token, so the PAN never transits Nemo. That flow is deferred
until the app card screens land. Any change that widens this surface is a
PCI scope change and requires a compliance review (see F2 certification
roadmap in `docs/nemo/02-gap-analysis-and-roadmap.md`).

## Lifecycle

`REQUESTED → ACTIVE ⇄ FROZEN`, with `BLOCKED` (terminal, lost/stolen/fraud)
and `CLOSED` (terminal) from any live state. Freeze/unfreeze/block are
idempotent; blocked and closed cards are immutable. Every change is pushed to
the processor first, then persisted atomically with a `card_events` audit row
and a transactional-outbox domain event (`card.issued`, `card.frozen`,
`card.unfrozen`, `card.blocked`, `card.limits.changed`).

## API (staff, JWT via LMS gateway)

| Method | Path | Roles |
|---|---|---|
| POST | `/api/v1/cards` | ADMIN, MANAGER, OFFICER |
| GET | `/api/v1/cards?customerId=` | ADMIN, MANAGER, OFFICER |
| GET | `/api/v1/cards/{id}` | ADMIN, MANAGER, OFFICER |
| GET | `/api/v1/cards/{id}/events` | ADMIN, MANAGER, OFFICER |
| POST | `/api/v1/cards/{id}/freeze` | ADMIN, MANAGER, OFFICER |
| POST | `/api/v1/cards/{id}/unfreeze` | ADMIN, MANAGER, OFFICER |
| POST | `/api/v1/cards/{id}/block` | ADMIN, MANAGER |
| PUT | `/api/v1/cards/{id}/limits` | ADMIN, MANAGER |

The mobile BFF will proxy these with `X-Service-Key` (SERVICE role) and
customer scoping when the app card screens are built.

## Deferred (tracked, not in the skeleton)

- Paymentology HTTP client + webhook endpoint (needs credentials/docs).
- Physical-card activation flow, card close API, PIN set/change, 3-D Secure,
  disputes & chargebacks (pairs with H6 support tooling).
- Tokenized PAN reveal for the app; BFF card endpoints.
