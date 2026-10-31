import socket,ssl,json,base64,pathlib,concurrent.futures
cfg=json.loads(pathlib.Path('config.json').read_text())
def trace(user,host):
 b=cfg['accounts'][user];auth=base64.b64encode((user+':'+b['password']).encode()).decode()
 s=socket.create_connection(('127.0.0.1',26443),15);s.sendall((f'CONNECT {host}:443 HTTP/1.1\r\nHost: {host}:443\r\nProxy-Authorization: Basic {auth}\r\n\r\n').encode());h=b''
 while b'\r\n\r\n' not in h:h+=s.recv(1)
 assert h.startswith(b'HTTP/1.1 200'),h
 s=ssl.create_default_context().wrap_socket(s,server_hostname=host);s.sendall((f'GET /cdn-cgi/trace HTTP/1.1\r\nHost: {host}\r\nConnection: close\r\n\r\n').encode());raw=b''
 while True:
  x=s.recv(4096)
  if not x:break
  raw+=x
 s.close();text=raw.decode();assert '\nip='+b['ipv6']+'\n' in text,(user,host,'source mismatch');return user,host,b['ipv6']
if __name__=='__main__':
 users=list(cfg['accounts'])[:2]
 with concurrent.futures.ThreadPoolExecutor(max_workers=3) as ex:
  fs=[ex.submit(trace,u,h) for u in users for h in ['chatgpt.com','auth.openai.com','api.openai.com'] for i in range(2)]
  for f in fs:print(*f.result())
 print('12 independent TLS connections verified; same account same source across all hosts')
