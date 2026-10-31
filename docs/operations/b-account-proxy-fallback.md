# Account proxy failover

OpenAI accounts can configure a primary proxy and up to seven ordered fallback
proxies in **Accounts → Edit → Fallback egress proxies**. The existing primary
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

Core OpenAI HTTP forwarding, connection tests, WebSocket connection setup and
OAuth refresh try another eligible proxy only after a connection failure
before sending the upstream request. They do not replay requests on upstream
HTTP errors (including 429), stream interruption or cancellation. Established
WebSockets remain on their existing connection. Each new connection tries the
primary first. Missing, disabled and expired pool members are skipped; an
unusable configured pool fails without silently connecting directly.

Fallback use is logged as `account_proxy_fallback` with account ID, proxy ID
and transport, without proxy credentials. Custom transport plugins retain
control of their own routing. Auxiliary metadata/privacy probes and initial
OAuth authorization retain their existing explicit proxy behavior.

Release uses a new HTTP/WS pair and graceful nginx reload, preserving old
assets and active connections. Account/proxy data is backed up before cutover.
Rolling back the application leaves the additive list in place; older code
continues using the unchanged primary proxy.
