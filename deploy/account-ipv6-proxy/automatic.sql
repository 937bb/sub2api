-- Deployment-specific policy for the B database only. Applied transactionally
-- after the new adapter is healthy. No upstream migration file is modified.
CREATE TABLE IF NOT EXISTS public.b_oauth_ipv6_policy (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 secret text NOT NULL CHECK(length(secret)=64),
 port integer NOT NULL CHECK(port BETWEEN 1024 AND 65535)
);
REVOKE ALL ON public.b_oauth_ipv6_policy FROM PUBLIC;

CREATE OR REPLACE FUNCTION public.b_bind_oauth_ipv6() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE target_port integer; signing_secret text; own_proxy bigint; proxy_user text;
BEGIN
 IF NEW.platform <> 'openai' OR NEW.type <> 'oauth' OR NEW.deleted_at IS NOT NULL THEN
  RETURN NEW;
 END IF;
 IF NEW.id <= 0 OR NEW.id > 281474976710655 THEN
  RAISE EXCEPTION 'Account ID outside fixed IPv6 allocation range';
 END IF;
 SELECT port,secret INTO STRICT target_port,signing_secret FROM public.b_oauth_ipv6_policy WHERE singleton;
 proxy_user := 'b-account-' || NEW.id::text;
 -- Serializes simultaneous conversion/restoration of the same account.
 PERFORM pg_advisory_xact_lock(hashtextextended('b-oauth-ipv6:' || NEW.id::text,0));
 SELECT id INTO own_proxy FROM public.proxies
 WHERE host='127.0.0.1' AND port=target_port AND username=proxy_user
 AND deleted_at IS NULL AND status='active' ORDER BY id LIMIT 1;
 IF own_proxy IS NULL THEN
  INSERT INTO public.proxies(name,protocol,host,port,username,password,status,created_at,updated_at,fallback_mode,expiry_warn_days)
  VALUES ('B fixed IPv6 ' || NEW.id::text,'http','127.0.0.1',target_port,proxy_user,
   encode(sha256(convert_to('b-ipv6-v1:' || NEW.id::text || ':' || signing_secret,'UTF8')),'hex'),
   'active',NOW(),NOW(),'none',7) RETURNING id INTO own_proxy;
 END IF;
 NEW.proxy_id := own_proxy;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION public.b_bind_oauth_ipv6() FROM PUBLIC;

CREATE OR REPLACE FUNCTION public.b_publish_oauth_ipv6() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
 IF NEW.platform='openai' AND NEW.type='oauth' AND NEW.deleted_at IS NULL
 AND (TG_OP='INSERT' OR NEW.proxy_id IS DISTINCT FROM OLD.proxy_id) THEN
  INSERT INTO public.scheduler_outbox(event_type,account_id) VALUES('account_changed',NEW.id);
 END IF;
 RETURN NULL;
END $$;
REVOKE ALL ON FUNCTION public.b_publish_oauth_ipv6() FROM PUBLIC;

CREATE OR REPLACE TRIGGER b_oauth_ipv6_bind
 BEFORE INSERT OR UPDATE OF proxy_id,platform,type,deleted_at ON public.accounts
 FOR EACH ROW EXECUTE FUNCTION public.b_bind_oauth_ipv6();
CREATE OR REPLACE TRIGGER b_oauth_ipv6_publish
 AFTER INSERT OR UPDATE OF proxy_id,platform,type,deleted_at ON public.accounts
 FOR EACH ROW EXECUTE FUNCTION public.b_publish_oauth_ipv6();
