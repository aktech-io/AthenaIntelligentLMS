# Event-Driven Architecture Hardening — AthenaLMS

**Status:** living document · **Owner:** Platform · **Last updated:** 2026-06-25

This is the engineering plan to make AthenaLMS's event-driven core *foolproof*:
no silently-dropped events, no projections that drift from the system of record,
and a clear path to 10× volume. It is structured as a technical proposal (event
design → architecture → migration → failure modes → open questions) and folds in
the concrete fixes already shipped for **F27** (a loan disbursement that credited
the borrower but, because the `loan.disbursed` event was lost, never produced a
loan record).

---

## 0. What went wrong (the motivating incident)

On a cold start, two services exhausted a bounded RabbitMQ connect-retry, fell
back to a permanent **no-op publisher**, and then **logged "Published event" while
silently dropping every event**. A disbursement returned `HTTP 200` and moved
money, but loan-management never heard about it. Three independent weaknesses
lined up:

1. The broker connection **gave up** after 10 attempts and never recovered.
2. The publisher **latched the dead state** and reported false success.
3. There was a **dual write**: state in PostgreSQL + event to RabbitMQ as two
   non-atomic operations. Even a *perfect* publisher loses the event if the
   broker is down for the few milliseconds between commit and publish.

The fixes below address each layer, with the **transactional outbox** as the
structural cure for #3 — the one that makes the whole class of bug impossible.

---

## 1. Event Design

### 1.1 Envelope & schema

Every event is a `DomainEvent` envelope (`internal/common/event/domain_event.go`)
with a domain-specific JSON `payload`:

```jsonc
{
  "id":            "uuid",          // unique; consumer idempotency key
  "type":          "loan.disbursed",// routing key
  "version":       1,               // schema version of the payload
  "source":        "loan-origination-service",
  "tenantId":      "admin",         // multi-tenant routing/filtering
  "correlationId": "<aggregate id>",// trace a business action across services
  "timestamp":     "2026-06-25T10:59:43.45Z",
  "payload":       { /* type-specific, versioned */ }
}
```

**Naming:** `<aggregate>.<past-tense-fact>` — events are facts, not commands:
`loan.application.submitted`, `loan.application.approved`, `loan.disbursed`,
`payment.completed`, `account.credited`, `accounting.posted`, `overdraft.drawn`.
Routing key == `type`, so consumers bind with topic patterns (`loan.#`,
`payment.*`).

### 1.2 Versioning

- `version` is an integer on the envelope, owned by the producer.
- **Only ever make additive (backward-compatible) changes within a major
  version**: add optional fields, never remove/rename/retype. Consumers ignore
  unknown fields (Go `json.Unmarshal` already does).
- A **breaking** change publishes a *new* event type or `version: 2` *alongside*
  v1 until all consumers migrate (parallel-publish, then retire). Never mutate v1
  semantics in place.
- The payload is stored verbatim in the outbox, so replay always reproduces the
  exact bytes the producer emitted.

### 1.3 Example payloads

`loan.disbursed` (the F27 event):

```json
{
  "id": "405dc1bb-0865-4f2f-8385-a2b1ba7db03c",
  "type": "loan.disbursed", "version": 1,
  "source": "loan-origination-service", "tenantId": "admin",
  "correlationId": "c984e4f0-c041-4aaa-9ffd-cd18ddc73049",
  "timestamp": "2026-06-25T10:59:43.45Z",
  "payload": {
    "applicationId": "c984e4f0-...", "customerId": "JRN-B-7cffabfb",
    "productId": "077cba1e-...", "status": "DISBURSED",
    "amount": 40000, "currency": "KES", "tenorMonths": 6,
    "interestRate": 15.0, "disbursementAccount": "7b500cca-...",
    "scheduleType": "EMI", "repaymentFrequency": "MONTHLY"
  }
}
```

`payment.completed`:

```json
{
  "id": "…", "type": "payment.completed", "version": 1,
  "source": "payment-service", "tenantId": "admin",
  "correlationId": "<loanId>", "timestamp": "…",
  "payload": { "loanId": "…", "amount": 2707.75, "principal": 2332.75,
               "interest": 375.0, "method": "CASH", "reference": "RPY-…" }
}
```

---

## 2. Architecture Decisions

### 2.1 Broker: keep RabbitMQ

AthenaLMS already runs **RabbitMQ** with a topic exchange (`athena.lms.exchange`),
durable per-domain queues, and topic bindings. For this workload it is the right
tool and we are keeping it:

- **Routing fit:** topic exchange gives us content-based fan-out (`loan.#`) with
  zero app-side routing code — directly addresses "adding a provider touches 6
  services."
- **Operational weight:** a single statefulset; the team already operates it.
- **Throughput:** our volume (tens of thousands of events/day) is two-plus orders
  of magnitude below where RabbitMQ needs tuning.

**Explicitly *not* choosing (now):**
- **Kafka / Redpanda** — we don't need a replayable partitioned log, big-data
  retention, or its operational surface yet. The outbox already gives us
  *durable, replayable* emission from PostgreSQL, which covers the replay need.
  Revisit if we need long-retention streaming analytics or per-key ordering at
  high partition counts.
- **AWS SNS/SQS / EventBridge** — viable and low-ops on AWS, but it would mean
  running two messaging systems during migration and couples us harder to AWS.
  Keep as the natural option *if* we move messaging fully managed.
- **Postgres `LISTEN/NOTIFY` as the bus** — great as a *latency hint* for the
  outbox relay (below), not as the system bus (no fan-out semantics, no durable
  per-consumer queues).

### 2.2 Reliable emission: the transactional outbox

The cornerstone. Producers no longer publish directly in request handlers.
Instead they **write the event into an `event_outbox` row in the same DB
transaction as the state change**, and a background **relay** publishes it.

```
┌─ disburse() ────────── one DB transaction ─────────────┐
│  UPDATE loan_applications SET status='DISBURSED' ...    │
│  INSERT INTO event_outbox (loan.disbursed payload) ...  │
└──────────────────────── COMMIT ────────────────────────┘
                 │ (later, async)
        ┌────────▼─────────┐  publish (at-least-once)   ┌────────────┐
        │  Outbox Relay    │ ─────────────────────────► │  RabbitMQ  │
        │ SKIP LOCKED poll │ ◄── mark dispatched ──────  └─────┬──────┘
        └──────────────────┘                                  │ consume
                                                       ┌───────▼────────┐
                                                       │ loan-management│ (idempotent on event.id)
                                                       └────────────────┘
```

Because state and event commit atomically, the event can never be lost relative
to the state change. If the broker is down, rows simply accumulate as `pending`
and drain automatically on reconnect. **Consumers must be idempotent on
`event.id`** (we may deliver a row more than once if a broker ack is lost after a
successful publish — at-least-once, not exactly-once).

Code: `internal/common/outbox` (`Write`, `Relay`); reference integration in
loan-origination's disburse path; table migration `migrations/loans/9`.

### 2.2.1 Verified end-to-end (2026-06-25)

The full failure scenario was reproduced against the live cluster:

1. **RabbitMQ scaled to 0** (total broker outage).
2. A loan was **disbursed during the outage** → `HTTP 200`, borrower credited,
   `loan.disbursed` written to the outbox (`status=0 pending`, relay retrying with
   backoff). Request latency stayed flat (publisher fails fast, doesn't block).
   loan-management correctly had **no loan yet** — nothing lost, nothing premature.
3. **RabbitMQ scaled back to 1.** With **no pod restarts**, the connection
   reconnected, `OnReady` re-declared topology, the consumer re-subscribed, and
   the relay published the parked event → the loan **activated (~48 s)** and every
   outbox row flipped to `dispatched`.

Steady-state disburse→active latency is ~2 s; the ~48 s is the capped
reconnect/resubscribe/relay backoff stack after a *total* outage, which is fine
for a recovery path.

### 2.3 Resilient connection & readiness (defense in depth)

- **Retry forever** with capped backoff + auto-reconnect on drop
  (`common/rabbitmq/connection.go`). Publishers *and* consumers self-heal after a
  broker restart with no pod restart.
- **Self-healing publisher** (`common/event/publisher.go`) lazily reopens its
  channel and surfaces real errors instead of false success.
- **Dependency-aware readiness** (`common/health`): `/actuator/health` now 503s
  when the DB is unreachable, so a broken pod leaves the gateway's rotation.

---

## 3. Transactional Outbox — design & high-volume optimization

The outbox table is on the **hot write path** of every money movement, so it is
engineered to stay small-and-fast regardless of history.

### 3.1 Schema (see `migrations/loans/9_event_outbox.up.sql`)

| column            | purpose                                              |
|-------------------|------------------------------------------------------|
| `id BIGSERIAL`    | FIFO dispatch order, purge cursor                    |
| `event_id UUID`   | unique → consumer idempotency + insert dedupe        |
| `event_type`      | routing key                                          |
| `payload JSONB`   | full serialized `DomainEvent` (verbatim replay)      |
| `status SMALLINT` | `0` pending · `1` dispatched · `2` dead              |
| `attempts`        | retry counter                                        |
| `next_attempt_at` | backoff schedule for retries                         |
| `created_at` / `dispatched_at` | observability + retention cursor        |

### 3.2 Why it stays fast when the table is "very busy"

1. **Partial index on the backlog only.**
   `CREATE INDEX ... ON event_outbox (next_attempt_at) WHERE status = 0`.
   The relay's poll (`WHERE status=0 AND next_attempt_at<=now()`) only ever scans
   this index, whose size equals the *undispatched backlog* (near-zero in steady
   state) — **not** the millions of retained dispatched rows. This is the single
   most important optimization: read cost is decoupled from history size.

2. **`FOR UPDATE SKIP LOCKED`** lets us run **N relay instances** (one per
   producer replica) that never contend on the same row — horizontal throughput
   with no distributed lock.

3. **Batched dispatch + drain loop.** Each tick claims up to `batchSize` (100)
   rows, publishes, and marks them dispatched in **one** `UPDATE ... WHERE id =
   ANY($sent)`. The relay keeps draining while batches come back full, so a
   backlog clears at line rate instead of one-per-tick.

4. **Bounded retention purge.** Dispatched rows are deleted in **5 000-row
   chunks** (`WHERE id IN (SELECT … LIMIT 5000)`) so the purge never holds a long
   lock on the hot table. Default retention 14 days (audit/replay window), then
   gone.

5. **Append-only insert pattern + HOT updates.** Inserts hit the table end (no
   index churn beyond the two small partials); the `status 0→1` update touches no
   indexed column except via the partial predicate, keeping write amplification
   low. Schedule **autovacuum aggressively** on this table (lower
   `autovacuum_vacuum_scale_factor`, e.g. 0.02) because the high
   insert+delete churn produces dead tuples fast.

### 3.3 Scaling further (when one table is the bottleneck)

- **Monthly partitioning by `created_at`** (`PARTITION BY RANGE`). Purge becomes
  an instant `DROP PARTITION` instead of a `DELETE` (no vacuum debt), and the
  pending-partial index lives only on the current partition. Adopt this once
  retained volume crosses ~tens of millions of rows.
- **`LISTEN/NOTIFY` latency hint.** Keep the 1 s poll as the floor, but have
  `Write` also `pg_notify('outbox')`; the relay wakes immediately on notify,
  cutting tail latency from ~1 s to ~ms without raising poll pressure.
- **Per-service outbox.** Each service DB owns its own `event_outbox`, so write
  load is naturally sharded across the fleet (no central hot table).
- **Separate the purge** into its own slow-tick goroutine (already hourly) or a
  cron, so retention never competes with dispatch.

### 3.4 Operability

- **Metrics to export:** `outbox_pending` (gauge — alert if it climbs and
  doesn't drain), `outbox_dead_total` (alert > 0), `outbox_dispatch_age_seconds`
  (oldest pending row), dispatch throughput.
- **Dead-letter handling:** rows that exhaust `maxAttempts` flip to `status=2`,
  are logged at ERROR, and stay queryable for an operator to fix-and-replay
  (`UPDATE … SET status=0, attempts=0`).

---

## 4. Top failure modes & mitigations

| # | Failure mode | Mitigation (status) |
|---|--------------|----------------------|
| 1 | **Dual write loses an event** (F27) | Transactional outbox — atomic state+event (**shipped**, origination; rollout below) |
| 2 | **Broker outage / cold-start race** | Retry-forever + auto-reconnect; outbox buffers in PG meanwhile (**shipped**) |
| 3 | **Duplicate delivery** (at-least-once) | Consumers idempotent on `event.id`; add a per-consumer `processed_events(event_id)` guard (**rollout**) |
| 4 | **Poison / unroutable message** | Producer-side: outbox dead-letters after capped retries; publish is now `mandatory=true` so unroutable messages are returned and logged at ERROR (not black-holed). Consumer-side DLQ/parking queue (**rollout**). Final follow-up: feed broker *returns* back to the outbox so an unroutable publish retries instead of being marked dispatched. |
| 4b | **Broker restart loses topology** | Every service re-declares the exchange/queues/bindings on each (re)connect via `OnReady`; consumers self-resubscribe (**shipped**) |
| 5 | **Silent consumer failure / lag** | Readiness on DB; export consumer lag + `outbox_pending`; alert (**partial — metrics pending**) |
| 6 | **Schema drift** breaks a consumer | Additive-only versioning + parallel-publish for breaking changes (**policy**) |
| 7 | **Projection drift undetected** | Reconciliation job: `DISBURSED` apps with no loan → re-emit/flag (**planned**) |

---

## 5. Migration / rollout strategy

**Tackle money paths first** (highest blast radius), defer cosmetic events.

1. **Done:** outbox infra + connection/publisher/readiness hardening; reference
   integration on `loan.disbursed`; full-fleet rebuild for the connection &
   readiness fixes.
2. **Next (money path):** route the remaining origination events
   (`submitted/approved/rejected`) and **payment**, **accounting.posted**, and
   **account credit/debit/transfer** emissions through their service's outbox,
   using the same `UpdateXWithEvent(tx, evt)` pattern. Each service already has a
   pgx pool; add an `event_outbox` migration + `outbox.NewRelay(pool, pub)` in
   `main.go`.
3. **Then (consumer idempotency):** add `processed_events(event_id PK)` insert-guard
   at the top of each consumer handler; on duplicate, ack and skip.
4. **Then (observability):** export the outbox + consumer-lag metrics; wire alerts.
5. **Then (reconciliation):** nightly sweep that compares producer state to
   consumer projections (e.g. disbursed apps ↔ active loans) and re-emits gaps.
6. **Defer:** Kafka/managed-bus migration; event sourcing; partitioning the
   outbox (only when volume demands it).

**No feature freeze required:** the outbox is additive — services keep working
while events move onto it one type at a time. Old fire-and-forget publishes and
new outbox publishes can coexist during migration (idempotency makes any overlap
safe).

---

## 6. Open questions

1. **Delivery SLA per event** — which events are money-critical (need the outbox
   now) vs. best-effort (notifications)? Drives rollout order.
2. **Consumer idempotency today** — which handlers are already idempotent, and
   which would double-apply on redelivery (e.g. does repayment posting dedupe)?
3. **Ordering needs** — any consumer that requires strict per-aggregate ordering?
   (Outbox `ORDER BY id` is per-producer FIFO; cross-service ordering is not
   guaranteed.)
4. **Multi-region / DR** — is the broker single-AZ? What's the RPO for in-flight
   events? (Outbox makes PG the source of truth, which helps.)
5. **Volume trajectory** — expected events/day at 10×, and retention/audit
   requirements? Determines when partitioning and metrics tiers kick in.
