#!/usr/bin/env python3
"""Capture successful local CLI WS framing with a dummy provider and credential.

Only frame shape and JSON field names are saved. No payload, credential, masking
key, raw identifier or child process output is retained. This fixture does not
measure production OAuth or TLS traffic, compressor byte equality, or thresholds.
"""
import argparse
import base64
import hashlib
import json
import os
import struct
import subprocess
import tempfile
import threading
import zlib
from pathlib import Path
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

def run(cli, extension):
    observations = []

    class Handler(BaseHTTPRequestHandler):
        protocol_version = 'HTTP/1.1'

        def log_message(self, *args):
            pass

        def do_GET(self):
            if self.headers.get('Upgrade', '').lower() != 'websocket':
                self.send_error(404)
                return
            offer = self.headers.get('Sec-WebSocket-Extensions', '')
            accept = base64.b64encode(hashlib.sha1((self.headers['Sec-WebSocket-Key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').encode()).digest()).decode()
            self.send_response(101)
            self.send_header('Upgrade', 'websocket')
            self.send_header('Connection', 'Upgrade')
            self.send_header('Sec-WebSocket-Accept', accept)
            if extension:
                self.send_header('Sec-WebSocket-Extensions', extension)
            self.end_headers()
            self.connection.settimeout(6)
            inflater = zlib.decompressobj(-15)

            def receive(n):
                b = self.rfile.read(n)
                if len(b) != n:
                    raise EOFError()
                return b

            def send(value, opcode=1):
                b = json.dumps(value, separators=(',', ':')).encode() if opcode == 1 else value
                h = bytes([128 | opcode])
                n = len(b)
                h += bytes([n]) if n < 126 else b'~' + struct.pack('!H', n) if n < 65536 else b'\x7f' + struct.pack('!Q', n)
                self.wfile.write(h + b)
                self.wfile.flush()
            try:
                for _ in range(12):
                    a, b = receive(2)
                    n = b & 127
                    if n == 126:
                        n = struct.unpack('!H', receive(2))[0]
                    elif n == 127:
                        n = struct.unpack('!Q', receive(8))[0]
                    if n > 8 * 1024 * 1024:
                        raise ValueError('oversized fixture frame')
                    mask = receive(4) if b & 128 else b''
                    payload = receive(n)
                    if mask:
                        payload = bytes((v ^ mask[i % 4] for i, v in enumerate(payload)))
                    row = {'offer': offer, 'accepted': extension, 'opcode': a & 15, 'fin': bool(a & 128), 'compressed': bool(a & 64), 'masked': bool(b & 128), 'wire_bytes': n}
                    if a & 15 == 8:
                        row['close_code'] = struct.unpack('!H', payload[:2])[0] if len(payload) >= 2 else None
                        observations.append(row)
                        send(payload, 8)
                        break
                    if a & 15 == 9:
                        observations.append(row)
                        send(payload, 10)
                        continue
                    if a & 64:
                        compressed = payload
                        payload = inflater.decompress(compressed + b'\x00\x00\xff\xff')
                        try:
                            independent = zlib.decompressobj(-15).decompress(compressed + b'\x00\x00\xff\xff')
                        except zlib.error:
                            independent = None
                        row['requires_previous_deflate_context'] = independent != payload
                    row['decoded_bytes'] = len(payload)
                    row['trailing_newline'] = payload.endswith(b'\n')
                    row['literal_html'] = b'<' in payload or b'>' in payload
                    row['escaped_html'] = b'\\u003c' in payload or b'\\u003e' in payload
                    try:
                        obj = json.loads(payload)
                    except ValueError:
                        obj = {}
                    row['type'] = obj.get('type')
                    row['generate'] = obj.get('generate')
                    row['keys'] = sorted(obj)
                    observations.append(row)
                    item = {'id': 'msg_fixture', 'type': 'message', 'role': 'assistant', 'status': 'completed', 'content': [{'type': 'output_text', 'text': 'ok', 'annotations': []}]}
                    response = {'id': 'resp_fixture', 'object': 'response', 'created_at': 1, 'status': 'completed', 'output': [] if obj.get('generate') is False else [item], 'usage': {'input_tokens': 1, 'output_tokens': 1, 'total_tokens': 2}}
                    send({'type': 'response.created', 'response': dict(response, status='in_progress', output=[])})
                    send({'type': 'response.completed', 'response': response})
            except (OSError, EOFError, ValueError) as e:
                observations.append({'fixture_end': type(e).__name__})

        def do_POST(self):
            self.send_error(400)
    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix='codex-frames-') as tmp:
            Path(tmp, 'config.toml').write_text(f'features.responses_websockets_v2=true\nmodel="gpt-5.4"\nmodel_provider="capture"\n[model_providers.capture]\nname="Local fixture"\nbase_url="http://127.0.0.1:{server.server_port}/v1"\nenv_key="CAPTURE_API_KEY"\nwire_api="responses"\nsupports_websockets=true\nrequest_max_retries=0\nstream_max_retries=0\n')
            env = {k: os.environ[k] for k in ('PATH', 'TMPDIR', 'LANG') if k in os.environ}
            env.update(CODEX_HOME=tmp, CAPTURE_API_KEY='local-fixture-only')
            try:
                r = subprocess.run([str(cli), 'exec', '--ephemeral', '--skip-git-repo-check', '--sandbox', 'read-only', 'Reply only ok.'], cwd=tmp, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=20)
                rc = r.returncode
            except subprocess.TimeoutExpired:
                rc = 'timeout'
    finally:
        server.shutdown()
        server.server_close()
        thread.join()
    return {'accepted': extension, 'exit_code': rc, 'frames': observations}

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--cli', type=Path, default=Path('/Applications/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex'))
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    results = [run(args.cli, ''), run(args.cli, 'permessage-deflate')]
    data = {'source_sha256': hashlib.sha256(args.cli.read_bytes()).hexdigest(), 'runs': results}
    args.output.write_text(json.dumps(data, indent=2) + '\n')
    print(json.dumps({'runs': len(results), 'exit_codes': [r['exit_code'] for r in results], 'output': str(args.output)}))
    if any(r['exit_code'] != 0 or sum(f.get('type') == 'response.create' for f in r['frames']) < 2 for r in results):
        raise SystemExit(1)
if __name__ == '__main__':
    main()
