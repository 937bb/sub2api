#!/usr/bin/env python3
"""Inspect installed ChatGPT artifacts and probe an isolated app-server identity.

Reads shipped files only. Does not read the user's credentials, start a turn,
or contact an inference endpoint. Requires Python 3 and the installed app.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import plistlib
import select
import struct
import subprocess
import tempfile
import time


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def read_asar_package(path):
    with path.open("rb") as stream:
        marker, header_size, _, json_size = struct.unpack("<IIII", stream.read(16))
        if marker != 4 or json_size > header_size or header_size > 32 * 1024 * 1024:
            raise ValueError("Unsupported ASAR header")
        index = json.loads(stream.read(json_size))
        entry = index["files"]["package.json"]
        if entry.get("unpacked") or entry.get("link") or entry["size"] > 1024 * 1024:
            raise ValueError("Unsupported package.json entry")
        stream.seek(8 + header_size + int(entry["offset"]))
        return json.loads(stream.read(entry["size"]))


def probe(cli, app_version):
    request = {
        "id": 1,
        "method": "initialize",
        "params": {
            "clientInfo": {"name": "Codex Desktop", "version": app_version},
            "capabilities": {"experimentalApi": True},
        },
    }
    with tempfile.TemporaryDirectory(prefix="chatgpt-identity-") as directory:
        # Avoid inherited credentials, proxy settings and terminal fingerprints.
        env = {key: os.environ[key] for key in ("PATH", "TMPDIR", "LANG") if key in os.environ}
        env["CODEX_HOME"] = directory
        env["CODEX_INTERNAL_ORIGINATOR_OVERRIDE"] = "Codex Desktop"
        process = subprocess.Popen(
            [str(cli), "app-server"], cwd=directory, env=env,
            stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL, text=True,
        )
        try:
            process.stdin.write(json.dumps(request) + "\n")
            process.stdin.flush()
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                if not select.select([process.stdout], [], [], 0.25)[0]:
                    continue
                line = process.stdout.readline()
                if not line:
                    raise RuntimeError("app-server exited before initialize")
                reply = json.loads(line)
                if reply.get("id") == 1:
                    if "error" in reply:
                        raise RuntimeError(f"initialize failed: {reply['error']}")
                    result = reply["result"]
                    return {key: result[key] for key in ("userAgent", "platformFamily", "platformOs") if key in result}
            raise TimeoutError("Timed out waiting for local initialize")
        finally:
            process.terminate()
            try:
                process.wait(timeout=3)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
            process.stdin.close()
            process.stdout.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--app", type=Path, default=Path("/Applications/ChatGPT.app"))
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    resources = args.app / "Contents/Resources"
    info = plistlib.loads((args.app / "Contents/Info.plist").read_bytes())
    package = read_asar_package(resources / "app.asar")
    cli_package = json.loads((resources / "codex-cli/codex-package.json").read_text())
    if package["version"] != info["CFBundleShortVersionString"]:
        raise ValueError("ASAR and bundle application versions disagree")
    result = {
        "bundle_id": info["CFBundleIdentifier"],
        "app_version": package["version"],
        "app_build": info["CFBundleVersion"],
        "engine_version": cli_package["version"],
        "chromium_version": info.get("ChromiumBaseVersion"),
        "asar_sha256": sha256(resources / "app.asar"),
        "native_cli_sha256": sha256(resources / "codex-cli/CodexCLI.app/Contents/MacOS/codex"),
        "probe_environment": "Isolated CODEX_HOME; no terminal variables; initialize only",
        "initialize": probe(resources / "codex-cli/bin/codex", package["version"]),
        "limits": ["No upstream capture", "No TLS/HTTP2 fingerprint measurement", "No device attestation recovery"],
    }
    text = json.dumps(result, indent=2) + "\n"
    if args.output:
        args.output.write_text(text)
    print(text, end="")


if __name__ == "__main__":
    main()
