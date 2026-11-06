# OpenAI SmartWS and fixed account egress

SmartWS converts eligible HTTP Responses requests into upstream WebSocket
messages while keeping the existing downstream SSE, response conversion, and
usage accounting. It is an opt-in connection optimization, not an upstream
concurrency entitlement.

## Enabling a group

Use **Groups → SmartWS** on an OpenAI group. Policies default to disabled and
use revision-based updates; concurrent edits return a conflict instead of
overwriting another administrator's setting. The API is:

- `GET /api/v1/admin/groups/:id/smart-ws`
- `PUT /api/v1/admin/groups/:id/smart-ws` with `enabled` and `revision`

The HTTP-serving process must enable `gateway.openai_ws.enabled`,
`gateway.openai_ws.oauth_enabled`, and
`gateway.openai_ws.responses_websockets_v2`, with
`gateway.openai_ws.force_http` disabled. Native downstream WS routing remains
separate. Each participating OpenAI OAuth account must explicitly enable a
WS-capable mode (`ctx_pool` or `passthrough`, including legacy enabled flags).
Account mode `off`, `http_bridge`, and account force-HTTP remain authoritative.
SIWC and API-key accounts are not eligible.

Only complete streaming POST requests to the existing ChatGPT Codex Responses
endpoint are eligible. Server-side continuations, unsupported payloads, and
unavailable pool capacity retain HTTP behavior before a WS request is sent.
Once a WS message may have been sent, neither internal compact/model retries
nor account failover may replay that generation.

## Account-bound IPv6

Bind each account to its own existing HTTP CONNECT or SOCKS proxy. A local
IPv6 adapter may expose one listener with different proxy credentials for each
fixed IPv6 address. The application preserves the complete proxy identity,
including those credentials, in connection compatibility checks.

- HTTP, SmartWS, native WS, account tests, and OAuth refresh validate the bound
  proxy. A missing, mismatched, inactive, or expired binding fails closed.
- SmartWS fallback uses the same resolved route as its HTTP request.
- Proxy rebindings cannot reuse an old native or SmartWS socket.
- Existing explicitly configured fallback pools retain their rotation policy.
  For one account / one IPv6, configure one primary proxy without fallbacks.
- This feature does not enable a deployment's external IPv6 allocator, change
  existing account assignments, or silently replace direct accounts' routes.

Changing a proxy or token while a conversation is using a native pinned WS
connection can require a new connection; it cannot migrate an active socket.
An already running generation drains on its original connection.

## Fingerprint and isolation

Production SmartWS uses the existing Codex WS dialer, TLS profile, header
ordering, compression, and framing. Its pool separates authenticated
group/user/API-key scope, upstream credentials, session, prepared handshake
headers, and proxy route. Without a stable session identifier a socket is not
retained for cross-request reuse. Idle connections retain a reader for control
frames; unsolicited/stale response events invalidate the connection.

Pool budgets are 256 connections per upstream account, 64 per exact identity,
and 4096 per process. Idle retention is 300 seconds with a 20-minute lifetime.
New handshakes are paced at 0.9/s with burst 2 and two concurrent dials per
capability. Existing scheduler account/user concurrency limits remain in force.
HTTP fallback does not increase an account's configured concurrency limit.

The group GET endpoint includes process-local pool counters. A separate HTTP
and WS deployment has separate pools and counters. Connection counts and
scheduler slots do not establish successful simultaneous generation capacity;
measure completed requests, errors, latency, and billing under real load before
claiming a supported concurrency level.

## Verification and provenance

The pool and settings baseline was adapted from the inspected
`sub2api-smartws` source at `b6d0ecdb64d9718406e22f54dcd4ebfae5c6bced`
(SmartWS integration commit `62ddf18`). This port preserves this project's
fingerprint transport, proxy pools, and protocol-specific forwarding paths.
It does not import latency-display changes from the reference deployment.

Run backend tests with and without the `unit` build tag. Race-test
`internal/pkg/smartws` and the service integration tests. The reference
retention matrix opens thousands of local sockets and should run on a Linux
test host with sufficient file descriptors and ephemeral ports. Test coverage
includes two-turn tool payloads, SSE usage preservation, certificate rejection
and TLS selection, no replay after dispatch, settings invalidation, fixed
proxy validation, route changes, and authenticated identity isolation.
