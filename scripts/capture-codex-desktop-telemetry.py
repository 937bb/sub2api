#!/usr/bin/env python3
"""Capture sanitized Desktop inference/OTLP wire shape on loopback only.

The bundled Codex app-server is initialized as ChatGPT Desktop and pointed at
one local HTTP server for both the dummy Responses provider and JSON OTLP
exporters. No OpenAI credential, user prompt, raw identifier, or telemetry
payload is written to the output fixture.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import plistlib
import queue
import re
import select
import struct
import subprocess
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


APP = Path("/Applications/ChatGPT.app")
CLI = APP / "Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex"
SAFE_VALUE = re.compile(r"^[A-Za-z0-9_.:/ -]{1,160}$")


def app_version() -> str:
    with (APP / "Contents/Info.plist").open("rb") as handle:
        return str(plistlib.load(handle)["CFBundleShortVersionString"])


def attribute_map(items: object) -> dict[str, object]:
    result: dict[str, object] = {}
    if not isinstance(items, list):
        return result
    for item in items:
        if not isinstance(item, dict) or not isinstance(item.get("key"), str):
            continue
        value = item.get("value")
        if not isinstance(value, dict):
            continue
        scalar = next((value[key] for key in ("stringValue", "boolValue", "intValue", "doubleValue") if key in value), None)
        result[item["key"]] = scalar
    return result


def safe_attributes(items: object) -> dict[str, object]:
    allowed = {
        "deployment.environment.name",
        "host.arch",
        "os.type",
        "service.name",
        "service.version",
        "telemetry.sdk.language",
        "telemetry.sdk.name",
        "telemetry.sdk.version",
    }
    values = attribute_map(items)
    return {key: values[key] for key in sorted(values) if key in allowed}


def safe_body(value: object) -> object:
    if not isinstance(value, dict):
        return type(value).__name__
    raw = value.get("stringValue")
    if isinstance(raw, str) and SAFE_VALUE.fullmatch(raw):
        return raw
    return next((key for key in sorted(value) if key.endswith("Value")), "unknown")


def sanitize_otlp(path: str, headers: dict[str, str], payload: object) -> dict:
    record: dict[str, object] = {
        "kind": "otlp",
        "path": path,
        "content_type": headers.get("content-type"),
        "top_level_fields": sorted(payload) if isinstance(payload, dict) else [],
        "resource_attributes": [],
        "scope_names": [],
        "span_names": [],
        "span_attribute_keys": [],
        "log_bodies": [],
        "log_attribute_keys": [],
        "metric_names": [],
    }
    resources: list[dict] = []
    if isinstance(payload, dict):
        for key in ("resourceSpans", "resourceLogs", "resourceMetrics"):
            value = payload.get(key)
            if isinstance(value, list):
                resources.extend(item for item in value if isinstance(item, dict))
    resource_attrs: list[dict[str, object]] = []
    scopes: set[str] = set()
    spans: set[str] = set()
    span_attrs: set[str] = set()
    log_bodies: set[object] = set()
    log_attrs: set[str] = set()
    metrics: set[str] = set()
    trace_ids: set[str] = set()
    for resource in resources:
        attrs = safe_attributes((resource.get("resource") or {}).get("attributes") if isinstance(resource.get("resource"), dict) else None)
        if attrs and attrs not in resource_attrs:
            resource_attrs.append(attrs)
        for scope_key, item_key in (("scopeSpans", "spans"), ("scopeLogs", "logRecords"), ("scopeMetrics", "metrics")):
            groups = resource.get(scope_key)
            if not isinstance(groups, list):
                continue
            for group in groups:
                if not isinstance(group, dict):
                    continue
                scope = group.get("scope")
                if isinstance(scope, dict) and isinstance(scope.get("name"), str):
                    scopes.add(scope["name"])
                items = group.get(item_key)
                if not isinstance(items, list):
                    continue
                for item in items:
                    if not isinstance(item, dict):
                        continue
                    if item_key == "spans":
                        if isinstance(item.get("name"), str):
                            spans.add(item["name"])
                        span_attrs.update(attribute_map(item.get("attributes")))
                        if isinstance(item.get("traceId"), str):
                            trace_ids.add(item["traceId"].lower())
                    elif item_key == "logRecords":
                        log_bodies.add(safe_body(item.get("body")))
                        log_attrs.update(attribute_map(item.get("attributes")))
                        if isinstance(item.get("traceId"), str):
                            trace_ids.add(item["traceId"].lower())
                    elif isinstance(item.get("name"), str):
                        metrics.add(item["name"])
    record.update(
        resource_attributes=resource_attrs,
        scope_names=sorted(scopes),
        span_names=sorted(spans),
        span_attribute_keys=sorted(span_attrs),
        log_bodies=sorted(log_bodies, key=str),
        log_attribute_keys=sorted(log_attrs),
        metric_names=sorted(metrics),
        _trace_ids=trace_ids,
    )
    return record


def sanitize_inference(kind: str, headers: dict[str, str], body: dict) -> dict:
    metadata = body.get("client_metadata")
    if not isinstance(metadata, dict):
        metadata = {}
    traceparent = headers.get("traceparent")
    if kind == "ws_frame":
        traceparent = metadata.get("ws_request_header_traceparent")
    trace_id = None
    if isinstance(traceparent, str) and re.fullmatch(r"[0-9a-fA-F]{2}-[0-9a-fA-F]{32}-[0-9a-fA-F]{16}-[0-9a-fA-F]{2}", traceparent):
        trace_id = traceparent.split("-", 3)[1].lower()
    return {
        "kind": kind,
        "path": "/v1/responses",
        "header_names": list(headers),
        "body_fields": sorted(body),
        "client_metadata_fields": sorted(metadata),
        "traceparent_present": isinstance(traceparent, str) and bool(traceparent),
        "tracestate_present": bool(headers.get("tracestate") or metadata.get("ws_request_header_tracestate")),
        "baggage_present": bool(headers.get("baggage")),
        "_trace_id": trace_id,
    }


def response_events(prewarm: bool):
    response = {
        "id": "resp_loopback_warm" if prewarm else "resp_loopback_turn",
        "object": "response",
        "created_at": int(time.time()),
        "status": "completed",
        "model": "gpt-5.4",
        "output": [],
        "usage": {
            "input_tokens": 1,
            "output_tokens": 0 if prewarm else 1,
            "total_tokens": 1 if prewarm else 2,
            "input_tokens_details": {"cached_tokens": 0},
            "output_tokens_details": {"reasoning_tokens": 0},
        },
    }
    yield {"type": "response.completed", "response": response}


class CaptureServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address, handler):
        super().__init__(address, handler)
        self.records: list[dict] = []


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_args):
        return

    def headers_lower(self) -> dict[str, str]:
        return {key.lower(): value for key, value in self.headers.items()}

    def read_json(self) -> object:
        length = int(self.headers.get("Content-Length", "0"))
        if length > 16 * 1024 * 1024:
            raise ValueError("payload too large")
        raw = self.rfile.read(length)
        return json.loads(raw or b"{}")

    def do_POST(self):
        headers = self.headers_lower()
        try:
            payload = self.read_json()
        except (ValueError, json.JSONDecodeError):
            self.send_error(400)
            return
        if self.path.rstrip("/").endswith("/responses"):
            body = payload if isinstance(payload, dict) else {}
            self.server.records.append(sanitize_inference("http_request", headers, body))
            data = b"".join(("data: " + json.dumps(event) + "\n\n").encode() for event in response_events(False))
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
            return
        if self.path.rstrip("/").endswith("/analytics/codex/turn-costs"):
            body = payload if isinstance(payload, dict) else {}
            turn_ids = body.get("turn_ids")
            self.server.records.append({
                "kind": "turn_costs",
                "path": self.path,
                "header_names": list(headers),
                "body_fields": sorted(body),
                "turn_id_count": len(turn_ids) if isinstance(turn_ids, list) else None,
                "traceparent_present": bool(headers.get("traceparent")),
                "tracestate_present": bool(headers.get("tracestate")),
                "baggage_present": bool(headers.get("baggage")),
            })
            data = b'{"turns":[]}'
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
            return
        self.server.records.append(sanitize_otlp(self.path, headers, payload))
        data = b"{}"
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def send_frame(self, opcode: int, payload: bytes):
        length = len(payload)
        header = bytes([0x80 | opcode, length if length < 126 else 126 if length < 65536 else 127])
        if length >= 126:
            header += struct.pack("!H" if length < 65536 else "!Q", length)
        self.wfile.write(header + payload)
        self.wfile.flush()

    def do_GET(self):
        if self.headers.get("Upgrade", "").lower() != "websocket":
            self.send_error(404)
            return
        headers = self.headers_lower()
        self.server.records.append({
            "kind": "ws_handshake",
            "header_names": list(headers),
            "traceparent_present": bool(headers.get("traceparent")),
            "tracestate_present": bool(headers.get("tracestate")),
            "baggage_present": bool(headers.get("baggage")),
        })
        accept = base64.b64encode(hashlib.sha1((self.headers["Sec-WebSocket-Key"] + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode()).digest()).decode()
        self.send_response(101)
        self.send_header("Upgrade", "websocket")
        self.send_header("Connection", "Upgrade")
        self.send_header("Sec-WebSocket-Accept", accept)
        self.end_headers()
        self.connection.settimeout(12)
        pending = b""
        try:
            for _ in range(32):
                first = self.rfile.read(2)
                if len(first) != 2:
                    return
                opcode = first[0] & 15
                length = first[1] & 127
                if length == 126:
                    length = struct.unpack("!H", self.rfile.read(2))[0]
                elif length == 127:
                    length = struct.unpack("!Q", self.rfile.read(8))[0]
                mask = self.rfile.read(4) if first[1] & 128 else None
                payload = self.rfile.read(length)
                if mask:
                    payload = bytes(value ^ mask[index % 4] for index, value in enumerate(payload))
                if opcode == 8:
                    self.send_frame(8, payload)
                    return
                if opcode == 9:
                    self.send_frame(10, payload)
                    continue
                if opcode not in (0, 1):
                    continue
                pending += payload
                if not first[0] & 128:
                    continue
                body = json.loads(pending)
                pending = b""
                self.server.records.append(sanitize_inference("ws_frame", {}, body))
                for sequence, event in enumerate(response_events(body.get("generate") is False)):
                    event["sequence_number"] = sequence
                    self.send_frame(1, json.dumps(event).encode())
        except (ConnectionError, OSError, TimeoutError, json.JSONDecodeError):
            return


def read_messages(process: subprocess.Popen, out: queue.Queue, stop: threading.Event):
    while not stop.is_set():
        ready, _, _ = select.select([process.stdout], [], [], 0.2)
        if not ready:
            if process.poll() is not None:
                return
            continue
        line = process.stdout.readline()
        if not line:
            return
        try:
            out.put(json.loads(line))
        except ValueError:
            continue


def wait_for(messages: queue.Queue, predicate, deadline: float):
    while time.monotonic() < deadline:
        try:
            message = messages.get(timeout=0.25)
        except queue.Empty:
            continue
        if predicate(message):
            return message
    raise TimeoutError("timed out waiting for app-server response")


def send(process: subprocess.Popen, message: dict):
    process.stdin.write(json.dumps(message) + "\n")
    process.stdin.flush()


def dummy_jwt() -> str:
    def encode(value: object) -> str:
        raw = json.dumps(value, separators=(",", ":")).encode()
        return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()

    now = int(time.time())
    return ".".join(
        (
            encode({"alg": "none", "typ": "JWT"}),
            encode({"email": "desktop-loopback@example.invalid", "iat": now, "exp": now + 3600}),
            "fixture",
        )
    )


def run(mode: str, auth_mode: str) -> dict:
    version = app_version()
    server = CaptureServer(("127.0.0.1", 0), Handler)
    server_thread = threading.Thread(target=server.serve_forever, daemon=True)
    server_thread.start()
    process = None
    stop = threading.Event()
    reader = None
    stderr_text = ""
    try:
        with tempfile.TemporaryDirectory(prefix="codex-desktop-telemetry-") as home:
            ws = "true" if mode == "ws" else "false"
            endpoint = f"http://127.0.0.1:{server.server_port}"
            auth_config = (
                'requires_openai_auth=true\n'
                if auth_mode == "chatgpt"
                else 'env_key="FIXTURE_API_KEY"\n'
            )
            Path(home, "config.toml").write_text(
                f'''model="gpt-5.4"
model_provider="fixture"
features.responses_websockets_v2={ws}
[model_providers.fixture]
name="Desktop telemetry loopback"
base_url="{endpoint}/v1"
{auth_config}wire_api="responses"
supports_websockets={ws}
request_max_retries=0
stream_max_retries=0
[analytics]
enabled=true
[otel]
environment="desktop-loopback"
exporter={{otlp-http={{endpoint="{endpoint}/v1/logs",protocol="json"}}}}
trace_exporter={{otlp-http={{endpoint="{endpoint}/v1/traces",protocol="json"}}}}
metrics_exporter={{otlp-http={{endpoint="{endpoint}/v1/metrics",protocol="json"}}}}
log_user_prompt=false
log_agent_responses=false
'''
            )
            env = {key: os.environ[key] for key in ("PATH", "TMPDIR", "LANG") if key in os.environ}
            env.update(CODEX_HOME=home, CODEX_INTERNAL_ORIGINATOR_OVERRIDE="Codex Desktop")
            if auth_mode == "apikey":
                env["FIXTURE_API_KEY"] = "loopback-only"
            process = subprocess.Popen(
                [str(CLI), "--strict-config", "app-server"],
                cwd=home,
                env=env,
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=1,
            )
            messages: queue.Queue = queue.Queue()
            reader = threading.Thread(target=read_messages, args=(process, messages, stop), daemon=True)
            reader.start()
            deadline = time.monotonic() + 20
            send(process, {"id": 1, "method": "initialize", "params": {"clientInfo": {"name": "Codex Desktop", "version": version}, "capabilities": {"experimentalApi": True}}})
            initialized = wait_for(messages, lambda msg: msg.get("id") == 1, deadline)
            if "error" in initialized:
                raise RuntimeError(initialized["error"])
            send(process, {"method": "initialized", "params": {}})
            if auth_mode == "chatgpt":
                send(process, {"id": 10, "method": "account/login/start", "params": {"type": "chatgptAuthTokens", "accessToken": dummy_jwt(), "chatgptAccountId": "desktop-loopback-account", "chatgptPlanType": "pro"}})
                login_reply = wait_for(messages, lambda msg: msg.get("id") == 10, deadline)
                if "error" in login_reply:
                    raise RuntimeError(login_reply["error"])
            send(process, {"id": 2, "method": "thread/start", "params": {"cwd": home, "ephemeral": True, "model": "gpt-5.4", "modelProvider": "fixture", "approvalPolicy": "never", "sandbox": "read-only", "dynamicTools": [], "threadSource": "app"}})
            thread_reply = wait_for(messages, lambda msg: msg.get("id") == 2, deadline)
            if "error" in thread_reply:
                raise RuntimeError(thread_reply["error"])
            thread_id = thread_reply["result"]["thread"]["id"]
            send(process, {"id": 3, "method": "turn/start", "params": {"threadId": thread_id, "input": [{"type": "text", "text": "Reply only OK."}]}})
            turn_reply = wait_for(messages, lambda msg: msg.get("id") == 3, deadline)
            if "error" in turn_reply:
                raise RuntimeError(turn_reply["error"])
            while time.monotonic() < deadline and not any(record["kind"] in ("http_request", "ws_frame") for record in server.records):
                time.sleep(0.05)
            time.sleep(8)
    finally:
        stop.set()
        if process is not None:
            process.terminate()
            try:
                process.wait(timeout=4)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
            if process.stderr is not None:
                stderr_text = process.stderr.read()[-2000:]
        if reader is not None:
            reader.join(timeout=1)
        time.sleep(1)
        server.shutdown()
        server.server_close()
        server_thread.join(timeout=2)
    inference_trace_ids = {record.pop("_trace_id") for record in server.records if record.get("_trace_id")}
    otlp_trace_ids: set[str] = set()
    for record in server.records:
        values = record.pop("_trace_ids", None)
        if isinstance(values, set):
            otlp_trace_ids.update(values)
    return {
        "source": "bundled Codex app-server initialized by ChatGPT Desktop; loopback inference and OTLP JSON exporters",
        "mode": mode,
        "auth_mode": auth_mode,
        "app_version": version,
        "native_cli_sha256": hashlib.sha256(CLI.read_bytes()).hexdigest(),
        "records": server.records,
        "relations": {
            "inference_trace_context_present": bool(inference_trace_ids),
            "inference_trace_matches_exported_telemetry": bool(inference_trace_ids & otlp_trace_ids),
        },
        "stderr_tail": [line for line in stderr_text.splitlines() if "error" in line.lower()][-5:],
        "limits": [
            "Loopback provider and dummy key; no OpenAI credential or user prompt sent",
            "OTLP JSON attribute keys and safe runtime identity values only",
            "Raw trace/span IDs, prompts, responses and arbitrary attribute values are not retained",
            "Short capture may not reach the periodic metrics export interval",
        ],
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=("http", "ws"), required=True)
    parser.add_argument("--auth-mode", choices=("apikey", "chatgpt"), default="apikey")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    result = run(args.mode, args.auth_mode)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({"mode": args.mode, "auth_mode": args.auth_mode, "records": len(result["records"]), "relations": result["relations"], "output": str(args.output)}))


if __name__ == "__main__":
    main()
