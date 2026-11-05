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
text tool results are translated to Kiro conversation history. Account-qualified
session IDs prevent reusing an upstream conversation across accounts or tenants.
The existing scheduler supplies account balancing and sticky selection.

GPT/Gemini aliases require explicit account model mappings to actual Kiro models;
joining a group does not grant access to its native provider models. OpenAI WS,
compact, image generation, embeddings and live endpoints exclude Kiro accounts.

The account model list is fetched through its proxy from Kiro's management API.
Unsupported models and unsupported request controls are rejected. Legacy models
without an additional-fields schema use upstream generation defaults; their
`max_tokens` cannot be enforced by this endpoint. Stop sequences, forced tool
selection, assistant prefill, server tools and non-text tool results are currently
unsupported. Signed Anthropic thinking blocks cannot retain an Anthropic
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

Cached responses report zero new input/output usage. Kiro normally reports
credits rather than exact token counts; token usage is explicitly estimated
unless exact upstream counts are present. No fictitious cache token split is
created. Credits are currently exposed on buffered native responses, not stored
as a separate durable billing unit.

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
