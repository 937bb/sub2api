import os,subprocess,json,pathlib,secrets,ipaddress
ROOT=pathlib.Path(__file__).resolve().parent
x=json.loads(subprocess.check_output(['docker','inspect','sub2api-b-019-http']))[0];e=dict(v.split('=',1) for v in x['Config']['Env'])
PG=['docker','run','--rm','-i','--network','host','-e','PGPASSWORD','postgres:18-alpine','psql','-h',e['DATABASE_HOST'],'-p',e['DATABASE_PORT'],'-U',e['DATABASE_USER'],'-d',e['DATABASE_DBNAME'],'-v','ON_ERROR_STOP=1','-At']
ENV=dict(os.environ,PGPASSWORD=e['DATABASE_PASSWORD'])
def sql(q):return subprocess.check_output(PG,input=q.encode(),env=ENV).decode()
if __name__=='__main__':
 rows=[json.loads(l) for l in sql("select json_build_object('id',id,'proxy_id',proxy_id,'status',status) from accounts where platform='openai' and type='oauth' and deleted_at is null order by id;").splitlines()]
 assert len(rows)>1000
 ROOT.chmod(0o700)
 backup=ROOT/'bindings-before.json';assert not backup.exists();backup.write_text(json.dumps(rows));backup.chmod(0o600)
 cfg={'listen':'127.0.0.1:26443','relay':'127.0.0.1:25443','prefix':'2a02:ae02:1a:2c00::/64','accounts':{}}
 for a in rows:
  cfg['accounts']['b-account-'+str(a['id'])]={'password':secrets.token_hex(24),'ipv6':str(ipaddress.IPv6Address(int(ipaddress.IPv6Address('2a02:ae02:1a:2c00:937b::'))+a['id']))}
 (ROOT/'config.json').write_text(json.dumps(cfg));(ROOT/'config.json').chmod(0o644)
 print('prepared stable bindings',len(rows),'no accounts changed')
