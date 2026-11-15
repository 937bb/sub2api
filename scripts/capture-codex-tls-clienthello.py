#!/usr/bin/env python3
"""Capture a sanitized ClientHello from the bundled Codex native engine.

The harness uses an isolated CODEX_HOME, a loopback custom provider and a dummy
API key. It records TLS structure only; no credentials or request bodies leave
the machine.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import socket
import struct
import subprocess
import tempfile
import threading
import time


def read_exact(conn, size):
    data = bytearray()
    while len(data) < size:
        chunk = conn.recv(size - len(data))
        if not chunk:
            raise EOFError("connection closed during ClientHello")
        data.extend(chunk)
    return bytes(data)


def decode_u16_list(data):
    if len(data) % 2:
        raise ValueError("odd u16 vector")
    return list(struct.unpack(f"!{len(data) // 2}H", data)) if data else []


def parse_client_hello(record_header, payload):
    if record_header[0] != 22 or not payload or payload[0] != 1:
        raise ValueError("first TLS record is not a ClientHello")
    handshake_size = int.from_bytes(payload[1:4], "big")
    body = payload[4 : 4 + handshake_size]
    pos = 0
    legacy_version = body[pos : pos + 2]
    pos += 2 + 32
    session_size = body[pos]
    pos += 1 + session_size
    cipher_size = int.from_bytes(body[pos : pos + 2], "big")
    pos += 2
    cipher_suites = decode_u16_list(body[pos : pos + cipher_size])
    pos += cipher_size
    compression_size = body[pos]
    pos += 1 + compression_size
    extension_size = int.from_bytes(body[pos : pos + 2], "big")
    pos += 2
    extension_end = pos + extension_size
    extension_ids = []
    extensions = {}
    while pos < extension_end:
        extension_id, size = struct.unpack("!HH", body[pos : pos + 4])
        pos += 4
        value = body[pos : pos + size]
        pos += size
        extension_ids.append(extension_id)
        extensions[extension_id] = value

    details = {}
    for extension_id in (10, 13):
        value = extensions.get(extension_id)
        if value is not None and len(value) >= 2:
            details[str(extension_id)] = decode_u16_list(value[2:])
    for extension_id in (5, 11, 18, 23, 35, 45):
        if extension_id in extensions:
            details[str(extension_id)] = extensions[extension_id].hex()
    if 43 in extensions and extensions[43]:
        details["43"] = decode_u16_list(extensions[43][1:])
    if 51 in extensions and len(extensions[51]) >= 2:
        shares = extensions[51][2:]
        parsed = []
        while len(shares) >= 4:
            group, size = struct.unpack("!HH", shares[:4])
            shares = shares[4:]
            parsed.append([group, size])
            shares = shares[size:]
        details["key_shares"] = parsed
    if 16 in extensions and len(extensions[16]) >= 2:
        protocols = []
        alpn = extensions[16][2:]
        while alpn:
            size = alpn[0]
            protocols.append(alpn[1 : 1 + size].decode("ascii", "replace"))
            alpn = alpn[1 + size :]
        details["alpn"] = protocols

    return {
        "record_version": record_header[1:3].hex(),
        "legacy_version": legacy_version.hex(),
        "cipher_suites": cipher_suites,
        "extensions": extension_ids,
        "details": details,
    }


def capture(cli, mode):
    listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    listener.bind(("127.0.0.1", 0))
    listener.listen(4)
    listener.settimeout(20)
    port = listener.getsockname()[1]
    result = {}
    failure = []

    def serve():
        try:
            conn, _ = listener.accept()
            with conn:
                conn.settimeout(10)
                if mode == "auth":
                    connect = bytearray()
                    while b"\r\n\r\n" not in connect:
                        chunk = conn.recv(4096)
                        if not chunk:
                            raise EOFError("connection closed during proxy CONNECT")
                        connect.extend(chunk)
                        if len(connect) > 65536:
                            raise ValueError("proxy CONNECT headers too large")
                    first_line = bytes(connect).split(b"\r\n", 1)[0].decode("ascii", "replace")
                    if not first_line.startswith("CONNECT auth.openai.com:"):
                        raise ValueError(f"unexpected auth proxy target: {first_line}")
                    result["proxy_connect"] = first_line
                    conn.sendall(b"HTTP/1.1 200 Connection Established\r\n\r\n")
                header = read_exact(conn, 5)
                payload = read_exact(conn, int.from_bytes(header[3:5], "big"))
                result.update(parse_client_hello(header, payload))
        except Exception as exc:  # pragma: no cover - diagnostic harness
            failure.append(str(exc))

    worker = threading.Thread(target=serve, daemon=True)
    worker.start()
    with tempfile.TemporaryDirectory(prefix="codex-tls-capture-") as directory:
        websocket = "true" if mode == "ws" else "false"
        Path(directory, "config.toml").write_text(
            f'''model="gpt-5.4"
model_provider="capture"
features.responses_websockets_v2={websocket}
[model_providers.capture]
name="Loopback TLS capture"
base_url="https://localhost:{port}/v1"
env_key="CAPTURE_API_KEY"
wire_api="responses"
supports_websockets={websocket}
request_max_retries=0
stream_max_retries=0
[analytics]
enabled=false
[otel]
exporter="none"
trace_exporter="none"
metrics_exporter="none"
log_user_prompt=false
'''
        )
        env = {key: os.environ[key] for key in ("PATH", "TMPDIR", "LANG") if key in os.environ}
        env.update(CODEX_HOME=directory, CAPTURE_API_KEY="loopback-only")
        if mode == "auth":
            proxy = f"http://127.0.0.1:{port}"
            env.update(HTTPS_PROXY=proxy, HTTP_PROXY=proxy, ALL_PROXY=proxy, NO_PROXY="")
        elif mode == "custom-ca":
            env["SSL_CERT_FILE"] = "/etc/ssl/cert.pem"
        command = [str(cli), "login", "--device-auth"] if mode == "auth" else [
            str(cli), "exec", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only", "Reply only OK."
        ]
        proc = subprocess.Popen(
            command,
            cwd=directory,
            env=env,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        deadline = time.monotonic() + 20
        while worker.is_alive() and proc.poll() is None and time.monotonic() < deadline:
            time.sleep(0.05)
        if worker.is_alive() and proc.poll() is None:
            proc.terminate()
        try:
            exit_code = proc.wait(timeout=3)
        except subprocess.TimeoutExpired:
            proc.kill()
            exit_code = proc.wait()
    worker.join(timeout=5)
    listener.close()
    if failure:
        raise RuntimeError(failure[0])
    if not result:
        raise RuntimeError("no ClientHello captured")
    result.update(
        mode=mode,
        cli_exit_code=exit_code,
        native_cli_sha256=hashlib.sha256(cli.read_bytes()).hexdigest(),
        source=(
            "Bundled Codex native engine; isolated home, local CONNECT proxy, device-auth bootstrap, no credentials"
            if mode == "auth"
            else "Bundled Codex native engine; isolated home, loopback provider, dummy API key"
        ),
        limits=["First ClientHello only", "No TLS session resumption measurement"],
    )
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=("http", "ws", "custom-ca", "auth"), required=True)
    parser.add_argument(
        "--cli",
        type=Path,
        default=Path("/Applications/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex"),
    )
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    observed = capture(args.cli, args.mode)
    args.output.write_text(json.dumps(observed, indent=2) + "\n")
    print(json.dumps({"mode": args.mode, "output": str(args.output), "sha256": observed["native_cli_sha256"]}))


if __name__ == "__main__":
    main()
