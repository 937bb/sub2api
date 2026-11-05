-- Kiro credentials must always have one fixed proxy, including API/bulk writes.
-- A partial unique index serializes competing Kiro assignments in PostgreSQL.
ALTER TABLE accounts ADD CONSTRAINT accounts_kiro_proxy_required
CHECK (deleted_at IS NOT NULL OR platform <> 'kiro' OR proxy_id IS NOT NULL) NOT VALID;
ALTER TABLE accounts ADD CONSTRAINT accounts_kiro_no_proxy_fallback
CHECK (deleted_at IS NOT NULL OR platform <> 'kiro' OR (
    proxy_fallback_origin_id IS NULL AND
    COALESCE(extra->'proxy_fallback_ids', '[]'::jsonb) IN ('[]'::jsonb, 'null'::jsonb)
)) NOT VALID;
CREATE UNIQUE INDEX accounts_kiro_unique_proxy ON accounts (proxy_id)
WHERE platform = 'kiro' AND deleted_at IS NULL;

-- Lock the endpoint across all assignment paths, including concurrent API imports.
-- Different proxy rows pointing at the same static endpoint cannot defeat exclusivity.
CREATE FUNCTION enforce_kiro_static_proxy_binding() RETURNS trigger AS $$
DECLARE
    selected proxies%ROWTYPE;
BEGIN
    IF NEW.deleted_at IS NOT NULL OR NEW.proxy_id IS NULL THEN RETURN NEW; END IF;
    IF TG_OP = 'UPDATE' THEN
        IF OLD.proxy_id IS NOT DISTINCT FROM NEW.proxy_id AND OLD.platform = NEW.platform
           AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at THEN RETURN NEW; END IF;
    END IF;
    SELECT * INTO selected FROM proxies WHERE id = NEW.proxy_id;
    IF NOT FOUND THEN RETURN NEW; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(lower(selected.host) || ':' || selected.port::text, 250));
    IF NEW.platform = 'kiro' AND (selected.deleted_at IS NOT NULL OR selected.status <> 'active'
        OR selected.expires_at <= NOW() OR COALESCE(selected.fallback_mode, 'none') NOT IN ('', 'none')
        OR selected.backup_proxy_id IS NOT NULL) THEN
        RAISE EXCEPTION 'Kiro requires an active static proxy without fallback'
            USING ERRCODE = '23514', CONSTRAINT = 'accounts_kiro_proxy_required';
    END IF;
    IF EXISTS (
        SELECT 1 FROM accounts a JOIN proxies p ON p.id = a.proxy_id
        WHERE a.deleted_at IS NULL AND a.id <> NEW.id
          AND lower(p.host) = lower(selected.host) AND p.port = selected.port
          AND (NEW.platform = 'kiro' OR a.platform = 'kiro')
    ) THEN
        RAISE EXCEPTION 'Kiro static proxy endpoint is already assigned'
            USING ERRCODE = '23505', CONSTRAINT = 'accounts_kiro_unique_proxy';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER accounts_kiro_static_proxy_binding
BEFORE INSERT OR UPDATE OF platform, proxy_id, deleted_at ON accounts
FOR EACH ROW EXECUTE FUNCTION enforce_kiro_static_proxy_binding();
