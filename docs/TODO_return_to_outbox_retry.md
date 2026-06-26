# TODO (deferred): feed broker `basic.return` back to the outbox for retry

**Status:** deferred by decision (2026-06-26). Low ROI / high blast radius — see
the trade-off note at the bottom. This doc is the resumption spec.

## Problem

The shared publisher (`internal/common/event/publisher.go`) already publishes
with **`mandatory=true`** and logs any `basic.return` (an *unroutable* message —
accepted by the exchange but matching no bound queue) at ERROR via a
`NotifyReturn` goroutine. **But it does not fail the publish.** So when the outbox
relay calls `Publish` and the broker *returns* the message as unroutable, `Publish`
returns `nil`, the relay marks the outbox row **dispatched**, and the event is
silently lost even though it never reached a consumer.

This only happens when a routing key has **no bound queue** at publish time
(topology not yet declared, a missing/misconfigured binding, or a brief bootstrap
race). It is already mitigated in practice by `OnReady` re-declaring the full
topology on every (re)connect, so the realistic window is small — hence deferred.

## Fix (fail-safe design)

Make an unroutable publish **return an error** so the relay leaves the row
`pending` and retries (by which point the binding should exist). Use the
publisher confirms that are already enabled (`ch.Confirm(false)`) plus the
existing return listener:

1. In `ensureChannel`, keep the `NotifyReturn` listener but have it record
   returned `MessageId`s into a `sync.Map` (or a small mutex-guarded set) on the
   `Publisher`, instead of only logging. Key by `event.ID`.
2. In `Publish`:
   - set `Publishing.MessageId = event.ID` (already set) and `mandatory=true`
     (already set);
   - publish with `ch.PublishWithDeferredConfirmWithContext(...)` to get a
     `*amqp.DeferredConfirmation`;
   - `dc.WaitContext(ctx)` for the broker **ack**. For an unroutable mandatory
     message RabbitMQ sends `basic.return` **before** `basic.ack`, so by the time
     Wait returns the return has been (or is about to be) delivered to our
     listener;
   - after the ack, check the returned-set for `event.ID` with a tiny grace
     (e.g. poll up to ~50–100ms) to absorb goroutine-scheduling skew; if present,
     delete it and **return an error** (`fmt.Errorf("event %s returned unroutable", event.ID)`).
3. The relay already treats a `Publish` error as "leave pending + retry with
   backoff" (`internal/common/outbox/outbox.go` `dispatchOnce`), so no relay
   change is needed. Fire-and-forget callers already log the error.

### Why it's fail-safe (never worse than today)
- `basic.return` is only sent for messages that were **not** delivered to any
  queue, so a detected return can never cause a double-delivery.
- If the grace window misses a return (rare), the row is marked dispatched —
  i.e. **identical to today's behaviour** (no regression).
- Detected returns become retries — strictly an improvement.

## Rollout
Shared-lib change → **rebuild all 16 Go services** (see `CONTINUATION_STATUS.md`
build commands). No DB migration.

## Verification
1. Normal path unaffected: a fresh loan disburse still activates (~2s); outbox
   rows reach `status=1`.
2. Unroutable path: publish to a routing key with **no bound queue** (e.g. add a
   temporary event type that nothing binds, or delete a binding via
   `rabbitmqctl`), trigger an event of that type, and assert its outbox row stays
   `status=0` with `attempts` incrementing — then restore the binding and watch it
   dispatch.

## Files
- `internal/common/event/publisher.go` (the only code change)
- (test) a throwaway producer or `rabbitmqctl delete_binding` to force an unroutable case.

## Trade-off (why deferred)
Lowest effort/risk-to-value ratio of the remaining backlog: it needs careful
AMQP confirm+return correlation (subtle), forces a full-fleet rebuild, and the
gap it closes is already largely mitigated by topology re-declaration on
reconnect. Do it as a focused session with the unroutable test above, not as a
tail-end change.
