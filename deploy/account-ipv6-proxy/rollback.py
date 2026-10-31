from prepare import *
before=json.loads((ROOT/'bindings-before.json').read_text());done=set(json.loads((ROOT/'applied-ids.json').read_text()))
for start in range(0,len(before),50):
 s=['BEGIN;',"SET LOCAL lock_timeout='2s'; SET LOCAL statement_timeout='20s';"]
 for a in before[start:start+50]:
  if a['id'] not in done:continue
  old='NULL' if a['proxy_id'] is None else str(a['proxy_id']);ident=a['id']
  s.append(f"WITH updated AS (UPDATE accounts a SET proxy_id={old},updated_at=NOW() FROM proxies p WHERE a.id={ident} AND a.proxy_id=p.id AND p.host='127.0.0.1' AND p.port=26443 AND p.username='b-account-{ident}' RETURNING a.id) INSERT INTO scheduler_outbox(event_type,account_id) SELECT 'account_changed',id FROM updated;")
 s.append('COMMIT;');sql('\n'.join(s))
print('original bindings restored where still owned by this release; leave proxy running for established tunnels')
