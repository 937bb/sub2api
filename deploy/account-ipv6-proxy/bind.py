from prepare import *
import sys,time
cfg=json.loads((ROOT/'config.json').read_text());before=json.loads((ROOT/'bindings-before.json').read_text())
selected=[a for a in before if a['id'] in [65172,65173]] if sys.argv[1]=='canary' else before
applied=ROOT/'applied-ids.json';done=json.loads(applied.read_text()) if applied.exists() else []
for start in range(0,len(selected),50):
 batch=[a for a in selected[start:start+50] if a['id'] not in done]
 if not batch:continue
 statements=['BEGIN;',"SET LOCAL lock_timeout='2s'; SET LOCAL statement_timeout='20s';"]
 for a in batch:
  ident=a['id'];user='b-account-'+str(ident);b=cfg['accounts'][user];old='NULL' if a['proxy_id'] is None else str(a['proxy_id'])
  statements.append(f"""DO $$ DECLARE p bigint; BEGIN
  PERFORM 1 FROM accounts WHERE id={ident} AND platform='openai' AND type='oauth' AND deleted_at IS NULL AND proxy_id IS NOT DISTINCT FROM {old}::bigint FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'Account {ident} changed; refusing to overwrite'; END IF;
  INSERT INTO proxies(name,protocol,host,port,username,password,status,created_at,updated_at,fallback_mode,expiry_warn_days)
  VALUES ('B fixed IPv6 {ident}','http','127.0.0.1',26443,'{user}','{b['password']}','active',NOW(),NOW(),'none',7) RETURNING id INTO p;
  UPDATE accounts SET proxy_id=p,updated_at=NOW() WHERE id={ident};
  INSERT INTO scheduler_outbox(event_type,account_id) VALUES ('account_changed',{ident});
  END $$;""")
 statements.append('COMMIT;');sql('\n'.join(statements));done.extend(a['id'] for a in batch);applied.write_text(json.dumps(done));applied.chmod(0o600);print('bound total',len(done),flush=True)
print(sql("select count(*),count(distinct a.proxy_id),count(distinct p.username) from accounts a join proxies p on p.id=a.proxy_id where a.platform='openai' and a.type='oauth' and a.deleted_at is null and p.host='127.0.0.1' and p.port=26443;"))
