# HTTP request traceability

Usage records retain the local billing request ID and the ID returned by the
upstream. An account's explicit `upstream_request_id_header` remains authoritative.
When blank, the gateway detects common relay/provider headers, prioritizing
`X-Oneapi-Request-Id` and `X-Client-Request-ID` over forwarded provider headers.
IDs are never invented when an upstream supplies none. Existing historical rows
are not backfilled. WebSocket response IDs remain separate from HTTP request IDs.

In **Admin → Usage → Advanced filters → Request ID / upstream ID**, paste a full
upstream ID, local usage ID, or the UUID from `X-Client-Request-ID`. Other filters,
including the selected date range, continue to apply. Enable the Request ID and
Upstream ID table columns to copy the values.

The shared HTTP upstream transport emits structured events:

- `upstream_attempt_started`: fresh attempt UUID, account, host, local request ID,
  client request ID, and `usage_request_id` (the billing correlation key).
- `upstream_attempt_headers`: the same attempt UUID, HTTP status and the detected
  upstream response ID and its source header, before consuming any response body.
- `upstream_attempt_finished`: transport failure, cancellation, body read error,
  EOF, or early close. EOF denotes HTTP body completion, not semantic SSE success.

Retries remain separate attempts sharing the same billing correlation key.
These events use the existing application log sinks and their configured
rotation/retention, not the usage database or the Ops system-log index. Export
logs before their retention window expires for long-running investigations.
A process crash may leave a start/header event without a finish event. An absent
upstream ID may mean the connection failed before response headers arrived.

Only gateway calls with request correlation context are traced. Direct provider
SDKs and WebSocket connections that bypass the shared HTTP transport are not
covered by these attempt events. Automatic trace-header detection is independent
of an account-specific custom header; the usage record remains authoritative for
custom upstream ID overrides.

The trace logger does not read request/response bodies, modify session/cache keys,
or log authorization headers, URLs with query strings, raw transport errors, or
customer prompts. Request IDs identify individual calls and must never be added
to cacheable prompts as a tracing mechanism.
