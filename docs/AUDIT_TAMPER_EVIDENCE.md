# Tamper-evident audit trail

**Goal:** let an auditor *prove* the audit log was not edited, deleted, or
back-dated — not merely trust that it wasn't. Implemented for account-service as
the reference; this doc covers the design and the rollout to the other audit
tables.

## Design

Every audit row is linked into a per-tenant **hash chain**:

```
entry_hash = SHA256( prev_hash || '|' || canonical(row) )
prev_hash  = entry_hash of the previous row for that tenant ('' for the first)
canonical(row) = tenant|action|entityType|entityId|userId|userRole|before|after|details|createdAtUTC
```

Three guarantees, all enforced in the database (so they hold regardless of which
service writes, and can't be bypassed by application code):

1. **Chaining** — a `BEFORE INSERT` trigger (`audit_hash_chain`) computes
   `prev_hash`/`entry_hash`. An advisory lock per tenant serialises inserts so
   the chain can't fork under concurrency. Editing any past row changes its
   `entry_hash`, which no longer matches the next row's `prev_hash` → break is
   detectable. Deleting a row breaks the link the same way.
2. **Append-only** — `BEFORE UPDATE`/`BEFORE DELETE` triggers
   (`audit_block_modify`) raise an exception, so the ordinary path can only ever
   INSERT. (A superuser can still `DISABLE TRIGGER`, but then guarantee #3
   catches the edit.)
3. **Verification** — `audit_verify(tenant)` walks the chain in `seq` order,
   recomputes each hash, and returns `intact` or the `broken_seq` of the first
   tampered/missing entry. Exposed at `GET /api/v1/audit-log/verify`.

Existing rows are **back-filled** into the chain by the migration (before the
append-only triggers are installed), so history is verifiable too.

## Verified (account-service, 2026-06-26)

- Backfill chained 696 historical rows; `audit_verify('admin')` → `intact=t`.
- `UPDATE`/`DELETE` on `audit_log` → rejected ("append-only").
- Simulated privileged tamper (`DISABLE TRIGGER` → edit row seq 695 →
  `ENABLE`): `audit_verify` → `intact=f, broken_seq=695`. After restoring the
  value → `intact=t` again.
- `GET /api/v1/audit-log/verify` → `{"intact":true,"total":696}`; after a live
  deposit → `{"intact":true,"total":697}` (new entry auto-chained).

## Rollout to the other audit tables

The migration `migrations/account/000013_audit_tamper_evident.up.sql` is the
template. For each target, copy it and adjust the table/column names:

| DB | Table | Notes |
|----|-------|-------|
| `athena_loans` | `audit_log` | **Same schema as account** — the template applies almost verbatim (just confirm column names). Covers loan-origination + loan-management. |
| `athena_accounting` | `financial_audit_log` | Different columns (entry/action/etc.) — adapt `audit_canonical(...)` to that table's columns. |
| `athena_overdraft` | overdraft audit table | Adapt canonical to its columns. |
| `athena_fraud` | fraud audit/`audit_log` | Adapt as needed. |

Per target, also add a `VerifyAuditChain` repo method + `GET …/audit-log/verify`
route (mirroring `internal/account`).

**Migration note:** the live cluster's auto-migration paths are unreliable (see
the k3s deploy notes), so each migration is also applied directly to its DB
(idempotent `IF NOT EXISTS` / `OR REPLACE`).

## Follow-ups
- A UI "Verify integrity" action on the Audit Logs page that calls the endpoint
  and shows intact / broken-at-seq.
- Periodic automated verification + alert (e.g. in the relay/cron) so tampering
  is caught proactively, not only on demand.
- Optionally anchor the latest `entry_hash` externally (e.g. daily to an
  append-only store) for defence against full-DB rewrites.
