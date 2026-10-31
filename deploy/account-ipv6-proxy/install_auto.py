"""Install only after validating the new adapter and isolated SQL tests.

Run from a private release directory containing config.json, prepare.py, and
automatic.sql. Secrets travel on psql stdin, never command-line arguments.
"""
import json
import urllib.request
from prepare import ROOT, sql

cfg = json.loads((ROOT / 'config.json').read_text())
secret = cfg['auto_secret']
assert len(secret) == 64 and all(c in '0123456789abcdef' for c in secret)
assert cfg['listen'] == '127.0.0.1:26445'
with urllib.request.urlopen('http://127.0.0.1:26445/health', timeout=5) as response:
    assert json.load(response)['status'] == 'ok'

backup = ROOT / 'automatic-before.jsonl'
assert not backup.exists(), 'already attempted; inspect state before retrying'
backup.write_text(sql("""SELECT json_build_object('id',a.id,'proxy_id',a.proxy_id)
FROM accounts a WHERE a.platform='openai' AND a.type='oauth' AND a.deleted_at IS NULL;"""))
backup.chmod(0o600)

ddl = (ROOT / 'automatic.sql').read_text()
statement = "BEGIN; SET LOCAL lock_timeout='2s'; SET LOCAL statement_timeout='20s';\n" + ddl
statement += f"""
INSERT INTO b_oauth_ipv6_policy(singleton,secret,port) VALUES(true,'{secret}',26445);
WITH moved AS (
 UPDATE proxies SET port=26445,updated_at=NOW()
 WHERE host='127.0.0.1' AND port=26443 AND username LIKE 'b-account-%'
 AND deleted_at IS NULL RETURNING id
) INSERT INTO scheduler_outbox(event_type,account_id)
SELECT 'account_changed',a.id FROM accounts a JOIN moved p ON p.id=a.proxy_id;
UPDATE accounts a SET proxy_id=a.proxy_id,updated_at=NOW()
WHERE a.platform='openai' AND a.type='oauth' AND a.deleted_at IS NULL
AND NOT EXISTS(SELECT 1 FROM proxies p WHERE p.id=a.proxy_id
 AND p.host='127.0.0.1' AND p.port=26445 AND p.username='b-account-'||a.id::text
 AND p.status='active' AND p.deleted_at IS NULL);
COMMIT;
"""
sql(statement)
print('Automatic assignment enabled; existing sources preserved; missing bindings backfilled.')
