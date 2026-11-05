# Kiro accounts

Kiro is an account provider, not a group type. Its accounts can join existing
Anthropic, OpenAI, Gemini and composite groups. The native Go adapter uses
hank9999/kiro.rs as its MIT-licensed protocol reference; it is not a Rust sidecar.

## Import and static proxies

Create a Kiro OAuth account using a kiro.rs JSON object/array or a Kiro Account
Manager export. Nested credentials and millisecond expiry values are supported.
Machine IDs from exports are retained; missing machine IDs are generated once.
Refresh tokens and other secrets are excluded from account API responses.

Every account requires a dedicated static proxy with automatic fallback disabled.
The single-account form uses its proxy selector. Each entry in a batch must
contain its own positive `proxy_id`, referencing an existing proxy. Repeated proxy
IDs are rejected before the batch is submitted. PostgreSQL additionally enforces
exclusive assignment, including concurrent imports and duplicate proxy rows
that refer to the same host and port. Failed batch imports retain the remaining
entries and their proxy IDs for retry.

Credential refresh, model discovery, account tests and inference use the same
bound proxy. Missing, disabled or expired proxies stop outbound requests. The
expiry worker does not move Kiro accounts to a backup or direct connection.
A proxy's static egress must also be guaranteed by the proxy provider.

## Protocols and models

The gateway supports Anthropic Messages, OpenAI Chat Completions and Responses,
and Gemini generateContent/streamGenerateContent,
with buffered and streaming responses. Text, base64 images, function calls and
text/image tool results are translated to Kiro conversation history. Account-qualified
session IDs prevent reusing an upstream conversation across accounts or tenants.
The existing scheduler supplies account balancing and sticky selection.

GPT/Gemini aliases require explicit account model mappings to actual Kiro models;
joining a group does not grant access to its native provider models. OpenAI WS,
compact, image generation, embeddings and live endpoints exclude Kiro accounts.

The account model list is fetched through its proxy from Kiro's management API.
Unsupported models and unsupported request controls are rejected. Legacy models
without an additional-fields schema use upstream generation defaults; their
`max_tokens` cannot be enforced by this endpoint. Stop sequences, forced tool
selection, disabling parallel tool use, assistant prefill, server tools and
non-text/non-image tool results are currently unsupported. Signed Anthropic thinking blocks cannot retain an Anthropic
signature contract when sent to Kiro.

## Response caching and accounting

Caching is configured in the group editor and is off by default. Account-level
cache metadata is ignored. All Kiro accounts in a group share its hit target,
TTL and aggregate capacity. When explicitly enabled, it reuses completed identical
text responses within the configured TTL. Keys include the authenticated user,
API key, group, account, model, account identity and full request. Tools, images,
thinking and nonzero-temperature requests bypass reuse. This is full-response
reuse, not simulated upstream prompt-cache tokens or prefix KV caching.

The target percentage bounds actual hits over eligible requests within a UTC
hour. Availability of repeated requests limits achievable hit rate. Zero disables
reuse; TTL and entry capacity are bounded per group and policy. Policy changes
start a new namespace, with old entries expiring naturally. Redis failure falls
back to inference rather than claiming a cache hit. Response headers expose
`X-Sub2api-Cache` and `X-Sub2api-Usage-Source`.

Local response-cache writes and reads now use Sub's existing cache-creation and
cache-read billing buckets and configured model/group/channel prices. A successful
write classifies the prompt footprint as cache creation (5-minute bucket for TTL
up to 300 seconds, 1-hour bucket for longer TTLs), retaining generated output usage.
A hit classifies the stored prompt footprint as cache reads, with zero new ordinary
input and zero generated output. Buckets are mutually exclusive; writes that fail,
interrupted responses and bypassed requests do not incur local creation fees.
`usage.cache_source=local` identifies these gateway operations; they are not
upstream prefix-cache claims. Kiro token counts remain estimates unless the
upstream provides exact counts. Credits are currently exposed on buffered native
responses, not stored as a separate durable billing unit.

Claude Code's empty system `output_config` control messages are folded into the
current request's effective output configuration. Nonempty system messages are
rejected; instructions belong in the top-level `system`. Requested output limits
are clamped to the selected account's advertised minimum and maximum; for current
Opus 5.5 this is 1,024–64,000. A smaller requested cap cannot be enforced by that
upstream API and is raised to its minimum. No model is silently substituted.

## Release validation

Run backend tests, vet and production build; frontend tests, typecheck and build;
and PostgreSQL/Redis integration tests for proxy exclusivity, expiry behavior,
cache isolation and real hit targeting. Use an authorized account for two turns
and a tool-result round trip through its bound proxy. Verify existing OpenAI
HTTP/WS and C-side routing before switching production. Migration
`250_kiro_static_proxy.sql` and `251_kiro_group_cache.sql` require a verified production backup before startup.


## Account environment and quota

Request identity follows `hank9999/kiro.rs` revision
`8fb3dd691d1cedf437bd6a4e9ef9f60b3f217fdd` (2026-11-05 inspection).
Imports accept `machineId`, `kiroVersion`, `systemVersion`, and `nodeVersion`
(or snake_case). New imports persist defaults 0.11.107, darwin#24.6.0,
and Node 22.22.0. Existing records use these stable defaults until saved.
Machine IDs remain account-specific and survive refresh. Generation, catalog,
and quota requests share the account environment; operation-specific SDK tokens
and fresh invocation UUIDs are used. Social and IdC refresh use their respective
header formats. The configured static proxy applies to all these paths.

This is application-level compatibility, not a verified clone of the official
client's TLS/HTTP2 fingerprint. Go handles Host/SNI and supported compression.
The reference's advertised retry maxima and unsupported compression codecs are
not copied: the gateway advertises the actual single SDK attempt.

The account list queries the upstream `getUsageLimits` endpoint through the
existing bounded batch loader. Successful snapshots cache for one minute;
errors cache for 15 seconds and concurrent requests coalesce. Cache identity
includes credentials and proxy. An explicit refresh bypasses the TTL.
Quota sums the primary credit/request allowance, active unexpired trials and
active unexpired bonuses. Precision fields take precedence even when zero.
Unknown/zero limits show no percentage. These are upstream credits, separate
from gateway token billing and the group's response-cache policy.

## Tool chain compatibility

The adapter follows kiro.rs's tool structures with additional validation:

- MCP tool names exceeding 63 bytes use a stable, UTF-8-safe hash alias. Tool
  definitions and historical calls use the same alias. Native, Chat Completions,
  Responses and Gemini outputs restore the original name, including SSE starts.
  Colliding aliases and duplicate names are rejected.
- Nullable MCP schema fields are normalized. Nested schemas, required fields,
  references and `additionalProperties: false` retain their meaning. Invalid
  schemas are rejected instead of being replaced with unconstrained objects.
- Consecutive same-role messages merge, including separately returned parallel
  results. Tool IDs are paired across each turn. Duplicate, missing or orphaned
  results produce an explicit request error; no history is silently deleted.
- Removed historical tools receive historical-only definitions. The response
  allowlist excludes these tools, so the adapter cannot expose an unavailable
  function as a new executable call.
- Tool-result base64 images are moved to the containing Kiro user message while
  text, IDs and error status remain in tool results. Unsupported files and URL
  images are rejected rather than dropped or fetched by the gateway.
- Interleaved upstream tool argument fragments accumulate independently by ID.
  Outbound content blocks remain sequential for existing OpenAI bridges. Partial
  JSON, changed names and continuation after completion produce stream errors.
  EOF alone cannot complete an unfinished tool; terminal metering and valid
  object arguments are required when Kiro omits `stop: true`.
- `tool_choice: auto` and `none` are supported. `none` disables all new tool calls,
  including historical definitions. Forced tool selection and
  `disable_parallel_tool_use: true` are explicitly rejected; the reference's
  converter does not implement these controls either.

The reference also has a separate, narrow Web Search shortcut, triggered by a
single tool named `web_search`, taking its query from the first user message.
It is not a general server-tool execution loop. This adapter does not claim
Anthropic hosted-tool compatibility or copy that shortcut's synthetic search
response/usage behavior. Custom client-side search tools work through the same
function-call path as other tools. Tool execution, retry decisions and side
effects remain owned by the client; this gateway only transports calls/results.
Tools continue to bypass full-response caching. These changes do not introduce
prefix/KV caching or synthetic cache token counts.
