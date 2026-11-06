-- B-only deployment policy. The adapter and local IPv6 route must be healthy
-- before installation. Existing accounts and explicit proxy choices are preserved.
CREATE OR REPLACE FUNCTION public.b_bind_new_pro_ipv6_five() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE
 signing_secret text;
 own_proxy bigint;
 route_ids bigint[] := ARRAY[]::bigint[];
 virtual_id bigint;
 slot integer;
 proxy_user text;
BEGIN
 IF NEW.platform <> 'openai' OR NEW.type <> 'oauth' OR NEW.deleted_at IS NOT NULL
 OR NEW.parent_account_id IS NOT NULL
 OR lower(coalesce(NEW.credentials->>'plan_type','')) <> 'pro'
 OR NEW.proxy_id IS NOT NULL
 OR (jsonb_typeof(NEW.extra->'proxy_fallback_ids')='array'
     AND NEW.extra->'proxy_fallback_ids' <> '[]'::jsonb) THEN
  RETURN NEW;
 END IF;
 IF NEW.id <= 0 OR NEW.id > 35184372088831 THEN
  RAISE EXCEPTION 'Account ID outside five-exit IPv6 allocation range';
 END IF;
 SELECT secret INTO STRICT signing_secret FROM public.b_oauth_ipv6_policy WHERE singleton;
 PERFORM pg_advisory_xact_lock(hashtextextended('b-pro-ipv6-five:' || NEW.id::text,0));
 FOR slot IN 1..5 LOOP
  -- Reserve eight addresses per account; five are used. The adapter's /80 is
  -- separate from the legacy single-exit /80 and the static hundred-address pool.
  virtual_id := NEW.id * 8 + slot;
  proxy_user := 'b-account-' || virtual_id::text;
  SELECT id INTO own_proxy FROM public.proxies
   WHERE host='127.0.0.1' AND port=26448 AND username=proxy_user
   AND deleted_at IS NULL AND status='active'
   AND (expires_at IS NULL OR expires_at>now()) ORDER BY id LIMIT 1;
  IF own_proxy IS NULL THEN
   INSERT INTO public.proxies(name,protocol,host,port,username,password,status,
    created_at,updated_at,fallback_mode,expiry_warn_days,usage_type)
   VALUES ('B Pro IPv6 '||NEW.id::text||' / '||slot::text,'http','127.0.0.1',26448,proxy_user,
    encode(sha256(convert_to('b-ipv6-v1:'||virtual_id::text||':'||signing_secret,'UTF8')),'hex'),
    'active',now(),now(),'none',7,'pro') RETURNING id INTO own_proxy;
  END IF;
  route_ids := array_append(route_ids,own_proxy);
 END LOOP;
 NEW.proxy_id := route_ids[1];
 NEW.extra := jsonb_set(coalesce(NEW.extra,'{}'::jsonb),'{proxy_fallback_ids}',to_jsonb(route_ids[2:5]),true);
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION public.b_bind_new_pro_ipv6_five() FROM PUBLIC;

-- PostgreSQL runs BEFORE triggers in name order: set the five-exit default
-- before the existing b_oauth_ipv6_bind sees an unassigned primary proxy.
CREATE OR REPLACE TRIGGER b_oauth_00_new_pro_ipv6_five
 BEFORE INSERT ON public.accounts
 FOR EACH ROW EXECUTE FUNCTION public.b_bind_new_pro_ipv6_five();
