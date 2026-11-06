-- Usage classification is metadata; it does not alter account routing.
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS usage_type VARCHAR(20) NOT NULL DEFAULT 'general';
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'proxies_usage_type_check' AND conrelid = 'proxies'::regclass) THEN
  ALTER TABLE proxies ADD CONSTRAINT proxies_usage_type_check CHECK (usage_type IN ('general', 'kiro', 'pro'));
 END IF;
END $$;
CREATE INDEX IF NOT EXISTS proxy_usage_type ON proxies (usage_type);

-- Assign unambiguous Kiro-only routes using actual account associations.
UPDATE proxies p SET usage_type = 'kiro'
WHERE p.deleted_at IS NULL AND p.usage_type = 'general'
AND EXISTS (SELECT 1 FROM accounts a WHERE a.proxy_id = p.id AND a.platform = 'kiro' AND a.deleted_at IS NULL)
AND NOT EXISTS (SELECT 1 FROM accounts a WHERE a.proxy_id = p.id AND a.platform <> 'kiro' AND a.deleted_at IS NULL);

-- The B deployment uses these authenticated local listeners for its Pro pool.
UPDATE proxies SET usage_type = 'pro'
WHERE usage_type = 'general' AND host = '127.0.0.1' AND port IN (26443, 26445)
AND username ~ '^b-account-[0-9]+$';

-- The external IPv6 allocator inserts proxies directly, so classify new leases
-- at the database boundary as well. Later manual edits remain authoritative.
CREATE OR REPLACE FUNCTION classify_fixed_ipv6_proxy_usage() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.usage_type = 'general' AND NEW.host = '127.0.0.1'
 AND NEW.port IN (26443, 26445) AND NEW.username ~ '^b-account-[0-9]+$' THEN
  NEW.usage_type := 'pro';
 END IF;
 RETURN NEW;
END;
$$;
CREATE OR REPLACE TRIGGER proxy_fixed_ipv6_usage
 BEFORE INSERT ON proxies FOR EACH ROW EXECUTE FUNCTION classify_fixed_ipv6_proxy_usage();
