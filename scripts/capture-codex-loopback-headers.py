#!/usr/bin/env python3
"""Capture sanitized CLI request shape using an isolated home and loopback fixture.

No existing credentials are loaded, no upstream inference endpoint is used, and
request bodies, bearer values, session IDs, and raw process output are not saved.
The local fixture rejects upgrades so WS prewarm, turn, and HTTP fallback can be
observed. This does not measure successful OAuth inference or TLS equivalence.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def capture(cli, mode):
    observations = []

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length", "0"))
            if length > 8 * 1024 * 1024:
                self.send_error(413)
                return
            self.rfile.read(length)
            headers = dict((k.lower(), v) for k, v in self.headers.items())
            metadata = json.loads(headers.get("x-codex-turn-metadata", "{}"))
            observations.append({
                "method": self.command,
                "path": self.path,
                "header_names": [key for key, _ in self.headers.items()],
                "user_agent": headers.get("user-agent"),
                "originator": headers.get("originator"),
                "beta": headers.get("openai-beta"),
                "websocket_extensions": headers.get("sec-websocket-extensions"),
                "metadata_fields": sorted(metadata),
                "request_kind": metadata.get("request_kind"),
                "root_equals_turn": bool(metadata.get("turn_id")) and metadata.get("turn_id") == metadata.get("root_turn_id"),
                "body_bytes": length,
            })
            body = b'{"error":{"message":"loopback capture complete","type":"invalid_request_error"}}'
            self.send_response(400)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        do_GET = do_POST

        def log_message(self, *_args):
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="codex-loopback-") as directory:
            enabled = "true" if mode == "ws" else "false"
            config = f'''features.responses_websockets_v2={enabled}
model="gpt-5.4"
model_provider="capture"
[model_providers.capture]
name="Loopback fixture"
base_url="http://127.0.0.1:{server.server_port}/v1"
env_key="CAPTURE_API_KEY"
wire_api="responses"
supports_websockets={enabled}
request_max_retries=0
stream_max_retries=0
'''
            Path(directory, "config.toml").write_text(config)
            env = {k: os.environ[k] for k in ("PATH", "TMPDIR", "LANG") if k in os.environ}
            env.update(CODEX_HOME=directory, CAPTURE_API_KEY="loopback-fixture-only")
            result = subprocess.run(
                [str(cli), "exec", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only", "Reply only ok."],
                cwd=directory, env=env, stdin=subprocess.DEVNULL,
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=25,
            )
            exit_code = result.returncode
    finally:
        server.shutdown()
        server.server_close()
        thread.join()
    if not observations:
        raise RuntimeError("No loopback request observed")
    return {"mode": mode, "cli_sha256": hashlib.sha256(cli.read_bytes()).hexdigest(),
            "cli_exit_code": exit_code, "requests": observations,
            "limits": ["Loopback custom provider; dummy credential", "Expected fixture rejection", "No TLS measurement"]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cli", type=Path, default=Path("/Applications/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex"))
    parser.add_argument("--mode", choices=("http", "ws"), default="http")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    result = capture(args.cli, args.mode)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({"mode": args.mode, "requests": len(result["requests"]), "output": str(args.output)}))


if __name__ == "__main__":
    main()
