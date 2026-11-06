# OpenAI proxy session affinity

An OpenAI account can use its primary proxy and configured fallback proxies for
separate conversations. HTTP/SSE, SmartWS, native Responses WS and WS passthrough
share one route selection rule. This does not raise an upstream account quota.

## Routing

The forwarding boundary captures the original client thread or explicit session
identity, namespaced by the authenticated API key. Claude Code session headers
and structured `metadata.user_id` session IDs are supported. Protocol conversion,
account fingerprint convergence, prompt text and generated cache keys cannot
replace the captured identity. An absent identity stays absent through nested
forwarders.

Rendezvous hashing uses the account ID, conversation scope and configured proxy
IDs. With the same configuration, different workers and restarts choose the same
proxy without a shared routing cache. Changing proxy credentials or list ordering
does not change the selected proxy ID. Distribution is statistical; it is not a
least-active-request scheduler.

A scoped conversation attempts only its selected proxy. Connect failures, HTTP
errors and 429 responses do not trigger alternate-IP retries within the account.
Missing, disabled and expired proxies remain part of the configured hash set and
fail closed if selected. Restore that proxy to resume those conversations. Adding
or removing a configured proxy ID can remap some conversations; changing a proxy's
actual endpoint can change its egress IP. These administrative changes are not
transparent session migrations.

Clients without an explicit conversation identity retain existing round-robin
selection and connection-only fallback. Do not infer identity from changing
message contents. A downstream WS connection retains the first frame's captured
identity for its lifetime, including subsequent frames that omit metadata.

## Connection reuse and limits

Each proxy uses separate compatible upstream sockets. One established WS cannot
change its source IP. A proxy may have multiple sockets for concurrent work.
Subsequent turns prefer compatible existing connections; a strict continuation
already pinned to a socket keeps that socket, including across this rollout.

The existing account scheduling, fingerprint policy and handshake compatibility
checks remain authoritative. Proxy affinity applies within the selected account;
it does not introduce a strict cross-account conversation lock or new 409 policy.
Native WS retains its account-wide pool cap across all configured proxies.
SmartWS retains its account admission budget across proxy-specific socket keys.
These pool limits are process-local, as before; this change does not introduce a
shared admission budget across HTTP and WS workers.

## Verification

Tests cover stable routing across snapshots, ordering and credential changes;
original identity preservation across Responses/Chat/Messages conversion;
concurrent distribution and repeated selection; failure and 429 non-replay;
aggregate native pool capacity; pinned continuation compatibility; and two-turn
local WebSocket exchanges through ctx_pool, passthrough and HTTP bridge.
These fixtures establish routing and reuse correctness, not production throughput
or a measured upstream concurrency ceiling.
