# OpenAI request audit

Enabled only when `OPENAI_REQUEST_AUDIT_DIR` and `OPENAI_REQUEST_AUDIT_KEY_FILE`
are configured. The key file contains 32 random bytes (not a hex string). Keep
both the directory and key accessible only to the application UID and operators;
use directory mode 0700 and key mode 0600. Never commit or print the key.

The HTTP and WS instances must share the directory **and key**. Records are
compressed then encrypted with AES-256-GCM, with an independent random nonce
and authentication tag per line. A process lock plus filesystem flock serializes
segment appends and pruning across both instances. The implementation targets
local POSIX filesystems, not network storage.

Retention is at most 72 hours, or 5 GiB of encrypted JSONL files, whichever is
reached first. Segments rotate at one hour or 64 MiB and expire by their creation
time, not their latest append. Whole-segment expiry can discard up to an hour
of newer records early. Active processes prune every ten seconds and before
writes. Old service instances must drain normally; requests still served by an
older image are not covered until they reconnect.

## Evidence and scope

`account_attempt_input` records the input at the account forwarding boundary.
`outbound_http_or_smartws`, `outbound_ws`, and `ws_pre_role` record protocol
payloads at their respective stages. These are attempts, not proof of upstream
acceptance; retries and conversions can produce multiple records.
`upstream_http_headers` records the received status and upstream request ID
without reading response streams. `upstream_ws_result` associates a completed
or failed turn with its response ID. Use `request_id`, `client_request_id`,
`attempt_id`, user/key/account IDs, timestamps, phase, body hash, and upstream
request ID together. HTTP usage logs may use `client:<client_request_id>`;
WS usage logs may use the upstream response ID.

Coverage includes account-selected OpenAI Responses, Chat Completions,
Anthropic compatibility, embeddings, count-token, image requests, HTTP/SmartWS,
and Responses WS forwarding. Authentication failures or requests blocked before
account selection do not have an upstream account and remain in existing Ops
logs. This is not audio/video capture or an HTTP header recorder.

JSON bodies are normalized copies, with original byte count and SHA256.
Credential fields and common token strings are redacted on the copy only.
Redaction is best effort, not a guarantee of removing every secret in arbitrary
prose. Multipart text fields are recorded with file names, sizes and SHA256;
binary attachments are explicitly omitted. Non-JSON payloads are omitted with
an explicit reason. No silent content-length truncation is performed. Recording
errors are counted and logged; requests continue if auditing fails. Writes are
synchronous but are not fsynced per record and can be lost in a host crash.

`status-<host>-<pid>.json` provides timestamped write/error counters for each
process. Check both current instances, not stale status files from past runs.
Audit records alone do not establish whether a request violated policy or
caused an account ban. Combine original instructions, conversation context,
upstream errors, and account events. Do not equate `token_revoked` with a content
violation or attribute an account ban to the caller that first observed it.

## Operator access

Build the local-only reader: `go build -o request-audit ./cmd/request-audit`.
It makes no network calls and outputs metadata by default:

```sh
request-audit --dir /path/to/audit --key-file /path/to/key \
  --account 123 --since 2026-11-09T00:00:00Z --limit 100
```

Add `--body` only in a restricted session to inspect decrypted content. Filters
also include `--user` and `--request`. `--limit 0` reads all matching records.
Do not write plaintext exports to normal logs, shared reports, or Git.
The original encrypted files remain authoritative; the reader tolerates an
incomplete final line during an active append.

To disable, remove the audit environment variables during a rolling deployment.
Keep the key until retained records expire or authorized deletion is complete.
No database migrations or fingerprint/scheduling changes are included.
