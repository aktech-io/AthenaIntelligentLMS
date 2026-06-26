-- Tamper-evident audit trail: hash-chain every audit_log entry and make the
-- table append-only, so an auditor can cryptographically prove the log was not
-- altered or back-dated. Each row's entry_hash = SHA256(prev_hash || canonical
-- content); deleting or editing any past row breaks the chain at verification.
--
-- Reference implementation (account-service). See docs/AUDIT_TAMPER_EVIDENCE.md
-- for the rollout to the other audit_log / financial_audit_log tables.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS seq        BIGSERIAL;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS prev_hash  TEXT;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS entry_hash TEXT;

-- Canonical, stable string representation of an audit row for hashing.
-- Trigger and verifier MUST use this identical expression.
CREATE OR REPLACE FUNCTION audit_canonical(
    p_tenant TEXT, p_action TEXT, p_etype TEXT, p_eid TEXT,
    p_uid TEXT, p_role TEXT, p_before TEXT, p_after TEXT, p_details TEXT,
    p_created TIMESTAMPTZ
) RETURNS TEXT AS $$
    SELECT concat_ws('|',
        p_tenant, p_action, p_etype, p_eid,
        coalesce(p_uid,''), coalesce(p_role,''),
        coalesce(p_before,''), coalesce(p_after,''), coalesce(p_details,''),
        to_char(p_created AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US'));
$$ LANGUAGE sql IMMUTABLE;

-- One-time backfill: chain existing rows (per tenant, in seq order) BEFORE the
-- append-only triggers exist, so the historical log is also verifiable.
DO $$
DECLARE
    rec        RECORD;
    v_tenant   TEXT := NULL;
    v_prev     TEXT := '';
    v_hash     TEXT;
BEGIN
    FOR rec IN
        SELECT * FROM audit_log WHERE entry_hash IS NULL ORDER BY tenant_id, seq
    LOOP
        IF v_tenant IS DISTINCT FROM rec.tenant_id THEN
            v_tenant := rec.tenant_id;
            v_prev := '';   -- new per-tenant chain
        END IF;
        v_hash := encode(digest(
            v_prev || '|' || audit_canonical(rec.tenant_id, rec.action, rec.entity_type,
                rec.entity_id, rec.user_id, rec.user_role,
                rec.before_data::text, rec.after_data::text, rec.details::text,
                rec.created_at),
            'sha256'), 'hex');
        UPDATE audit_log SET prev_hash = v_prev, entry_hash = v_hash WHERE id = rec.id;
        v_prev := v_hash;
    END LOOP;
END $$;

-- BEFORE INSERT: compute the chain link. Advisory lock serialises inserts per
-- tenant so concurrent writers can't fork the chain.
CREATE OR REPLACE FUNCTION audit_hash_chain() RETURNS TRIGGER AS $$
DECLARE
    v_prev TEXT;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('audit_log:' || NEW.tenant_id));
    SELECT entry_hash INTO v_prev
        FROM audit_log
        WHERE tenant_id = NEW.tenant_id AND entry_hash IS NOT NULL
        ORDER BY seq DESC LIMIT 1;
    v_prev := coalesce(v_prev, '');
    NEW.prev_hash  := v_prev;
    NEW.entry_hash := encode(digest(
        v_prev || '|' || audit_canonical(NEW.tenant_id, NEW.action, NEW.entity_type,
            NEW.entity_id, NEW.user_id, NEW.user_role,
            NEW.before_data::text, NEW.after_data::text, NEW.details::text,
            NEW.created_at),
        'sha256'), 'hex');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_audit_hash ON audit_log;
CREATE TRIGGER trg_audit_hash BEFORE INSERT ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_hash_chain();

-- Append-only enforcement: block UPDATE and DELETE outright.
CREATE OR REPLACE FUNCTION audit_block_modify() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only — % is not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_audit_no_update ON audit_log;
CREATE TRIGGER trg_audit_no_update BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_block_modify();
DROP TRIGGER IF EXISTS trg_audit_no_delete ON audit_log;
CREATE TRIGGER trg_audit_no_delete BEFORE DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_block_modify();

-- Verifier: walk the chain in seq order; return the first seq where a row's
-- stored prev_hash/entry_hash disagrees with recomputation (tamper detected),
-- or intact=true if the whole chain checks out.
CREATE OR REPLACE FUNCTION audit_verify(p_tenant TEXT)
RETURNS TABLE(intact BOOLEAN, broken_seq BIGINT, total BIGINT) AS $$
DECLARE
    rec     RECORD;
    v_prev  TEXT := '';
    v_calc  TEXT;
    v_total BIGINT := 0;
BEGIN
    FOR rec IN SELECT * FROM audit_log WHERE tenant_id = p_tenant ORDER BY seq LOOP
        v_total := v_total + 1;
        v_calc := encode(digest(
            v_prev || '|' || audit_canonical(rec.tenant_id, rec.action, rec.entity_type,
                rec.entity_id, rec.user_id, rec.user_role,
                rec.before_data::text, rec.after_data::text, rec.details::text,
                rec.created_at),
            'sha256'), 'hex');
        IF rec.prev_hash IS DISTINCT FROM v_prev OR rec.entry_hash IS DISTINCT FROM v_calc THEN
            intact := FALSE; broken_seq := rec.seq; total := v_total; RETURN NEXT; RETURN;
        END IF;
        v_prev := rec.entry_hash;
    END LOOP;
    intact := TRUE; broken_seq := NULL; total := v_total; RETURN NEXT;
END;
$$ LANGUAGE plpgsql STABLE;
