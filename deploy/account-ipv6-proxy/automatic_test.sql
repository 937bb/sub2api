-- Run against an isolated schema copy. Every synthetic account is rolled back.
BEGIN;
DO $$
DECLARE a bigint; p bigint; other bigint; initial_proxy bigint; pass text; expected text;
BEGIN
 INSERT INTO accounts(id,name,platform,type,status,schedulable)
 VALUES(281474976710651,'ipv6-test-new','openai','oauth','inactive',false) RETURNING id,proxy_id INTO a,p;
 IF p IS NULL THEN RAISE EXCEPTION 'new account was unbound'; END IF;
 SELECT password INTO pass FROM proxies WHERE id=p;
 SELECT encode(sha256(convert_to('b-ipv6-v1:'||a::text||':'||secret,'UTF8')),'hex') INTO expected FROM b_oauth_ipv6_policy;
 IF pass<>expected THEN RAISE EXCEPTION 'incorrect automatic credentials'; END IF;
 initial_proxy:=p;
 UPDATE accounts SET proxy_id=NULL WHERE id=a RETURNING proxy_id INTO p;
 IF p<>initial_proxy THEN RAISE EXCEPTION 'update changed fixed binding'; END IF;
 UPDATE accounts SET deleted_at=NOW() WHERE id=a;
 UPDATE accounts SET deleted_at=NULL WHERE id=a RETURNING proxy_id INTO p;
 IF p<>initial_proxy THEN RAISE EXCEPTION 'restore changed fixed binding'; END IF;
 INSERT INTO accounts(id,name,platform,type,status,schedulable,proxy_id)
 VALUES(281474976710652,'ipv6-test-duplicate','openai','oauth','inactive',false,p) RETURNING proxy_id INTO other;
 IF other=p THEN RAISE EXCEPTION 'duplicate account reused source binding'; END IF;
 INSERT INTO accounts(id,name,platform,type,status,schedulable)
 VALUES(281474976710653,'ipv6-test-key','openai','apikey','inactive',false) RETURNING proxy_id INTO p;
 IF p IS NOT NULL THEN RAISE EXCEPTION 'API key account modified'; END IF;
 INSERT INTO accounts(id,name,platform,type,status,schedulable)
 VALUES(281474976710654,'ipv6-test-other','anthropic','oauth','inactive',false) RETURNING proxy_id INTO p;
 IF p IS NOT NULL THEN RAISE EXCEPTION 'other platform modified'; END IF;
 UPDATE accounts SET platform='openai' WHERE id=281474976710654 RETURNING proxy_id INTO p;
 IF p IS NULL THEN RAISE EXCEPTION 'conversion not assigned'; END IF;
 IF NOT EXISTS(SELECT 1 FROM scheduler_outbox WHERE account_id=a AND event_type='account_changed') THEN
  RAISE EXCEPTION 'scheduler event missing';
 END IF;
 RAISE NOTICE 'new/import/duplicate/update/restore/convert/isolation/outbox assertions passed';
END $$;
ROLLBACK;
