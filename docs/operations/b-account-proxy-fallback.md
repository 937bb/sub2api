# Account proxy rotation

OpenAI accounts can configure an original proxy and up to seven additional
proxies in **Accounts → Edit → Rotating egress IPs**. The existing primary
selector and `proxy_id` API field remain unchanged. No existing account is
automatically assigned additional proxies.

`POST /api/v1/admin/accounts` and `PUT /api/v1/admin/accounts/:id` accept an
optional `proxy_fallback_ids` array. Omission preserves the existing list; `[]`
clears it. IDs must refer to active, unexpired proxies, cannot be repeated, and
cannot include the primary. A primary proxy is required. The ordered list is
stored in `accounts.extra.proxy_fallback_ids`, so no schema migration or
backfill is required. Ordinary extra edits cannot overwrite this managed key.
Clearing the primary clears its backups. Bulk primary replacement resets the
backup list. Spark shadow accounts inherit their parent's list.

Core OpenAI HTTP forwarding, connection tests and new WebSocket requests rotate
through all eligible proxies in order. Counters are per account and per process;
process restarts reset them. HTTP and WS instances have independent counters.
WebSocket pools only reuse a connection on the selected route. Established
WebSocket sessions and continuations requiring a specific connection retain that
connection's route. OAuth refresh retains primary-first behavior.

These transports and OAuth refresh try another eligible proxy after a connection failure
before sending the upstream request. They do not replay requests on upstream
HTTP errors (including 429), stream interruption or cancellation. Established
WebSockets remain on their existing connection. Missing, disabled and expired pool members are skipped; an
unusable configured pool fails without silently connecting directly.

Multi-proxy route use is logged as `account_proxy_selected` with account ID, proxy ID
and transport, without proxy credentials. Custom transport plugins retain
control of their own routing. Auxiliary metadata/privacy probes and initial
OAuth authorization retain their existing explicit proxy behavior.

Release uses a new HTTP/WS pair and graceful nginx reload, preserving old
assets and active connections. Account/proxy data is backed up before cutover.
Rolling back the application leaves the additive list in place; older code
continues using the unchanged primary proxy.
