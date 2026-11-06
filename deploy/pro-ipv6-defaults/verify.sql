-- Run after loading automatic-five.sql inside a transaction; always ROLLBACK.
DO $test$
DECLARE a accounts; b accounts; ids bigint[]; before_count bigint; after_count bigint;
BEGIN
 SELECT count(*) INTO before_count FROM proxies;
 INSERT INTO accounts(id,name,platform,type,credentials,schedulable)
 VALUES (900000000001,'fixture-five','openai','oauth','{"plan_type":"pro"}',false) RETURNING * INTO a;
 SELECT array_agg(value::bigint) INTO ids FROM jsonb_array_elements_text(a.extra->'proxy_fallback_ids');
 IF a.proxy_id IS NULL OR array_length(ids,1)<>4 OR a.proxy_id=ANY(ids) THEN RAISE EXCEPTION 'five routes not bound'; END IF;
 IF (SELECT count(DISTINCT username) FROM proxies WHERE id=a.proxy_id OR id=ANY(ids))<>5 THEN RAISE EXCEPTION 'duplicate exit'; END IF;
 IF (SELECT count(*) FROM proxies WHERE (id=a.proxy_id OR id=ANY(ids)) AND usage_type='pro' AND port=26448)<>5 THEN RAISE EXCEPTION 'wrong proxy type or listener'; END IF;
 SELECT count(*) INTO after_count FROM proxies;
 IF after_count-before_count<>5 THEN RAISE EXCEPTION 'unexpected proxy count'; END IF;
 UPDATE accounts SET name='fixture-renamed',credentials=credentials||'{"fixture":"update"}' WHERE id=a.id RETURNING * INTO b;
 IF b.proxy_id<>a.proxy_id OR b.extra->'proxy_fallback_ids'<>a.extra->'proxy_fallback_ids' THEN RAISE EXCEPTION 'update remapped account'; END IF;
 INSERT INTO accounts(id,name,platform,type,credentials,proxy_id,schedulable)
 VALUES (900000000002,'fixture-explicit','openai','oauth','{"plan_type":"pro"}',a.proxy_id,false) RETURNING * INTO b;
 IF b.proxy_id<>a.proxy_id OR jsonb_typeof(b.extra->'proxy_fallback_ids')='array' THEN RAISE EXCEPTION 'explicit binding replaced'; END IF;
 INSERT INTO accounts(id,name,platform,type,credentials,schedulable)
 VALUES (900000000003,'fixture-plus','openai','oauth','{"plan_type":"plus"}',false) RETURNING * INTO b;
 IF b.proxy_id IS NULL OR (SELECT port FROM proxies WHERE id=b.proxy_id)<>26445 THEN RAISE EXCEPTION 'legacy default changed'; END IF;
 INSERT INTO accounts(id,name,platform,type,credentials,schedulable)
 VALUES (900000000004,'fixture-other','anthropic','apikey','{}',false) RETURNING * INTO b;
 IF b.proxy_id IS NOT NULL THEN RAISE EXCEPTION 'other platform changed'; END IF;
 INSERT INTO accounts(id,name,platform,type,credentials,schedulable,parent_account_id,quota_dimension)
 VALUES (900000000005,'fixture-shadow','openai','oauth','{"plan_type":"pro"}',false,a.id,'spark') RETURNING * INTO b;
 IF jsonb_typeof(b.extra->'proxy_fallback_ids')='array' THEN RAISE EXCEPTION 'shadow allocated five exits'; END IF;
END $test$;
SELECT json_build_object('five_routes',true,'explicit_selection_preserved',true,'existing_updates_preserved',true,'non_pro_legacy_preserved',true,'other_platform_preserved',true,'shadow_excluded',true);
