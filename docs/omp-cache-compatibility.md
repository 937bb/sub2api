# OMP Responses cache compatibility

OMP 18.2.6 sends a stable `prompt_cache_key` when using a custom
`openai-responses` provider. Unlike its built-in `openai` provider, a custom
provider does not also send session or thread headers. This was verified with
the actual CLI, including continued conversations and a tool round trip.

The gateway previously generated a new conversation identity for each such
HTTP request in the account's session/full fingerprint modes. The cache key
remained stable, but session/thread/turn metadata changed. Removing those
random fields alone was insufficient: the ChatGPT Responses transport also
uses the `session-id` header for cache affinity. Cache affinity must remain
separate from conversation execution: OMP supports independent routing-session
and cache-key overrides, including a cache prefix shared by separate conversations.

Codex's own `ModelClient::responses_session_id()` documents and implements
this distinction in [the official client source](https://github.com/openai/codex/blob/main/codex-rs/core/src/client.rs):
root agents use their prompt-cache key as the Responses session header while
actual session identity remains in turn metadata. This behavior was checked
against the official source on 2026-09-19.

For OAuth Responses requests with a cache key and no explicit conversation,
the gateway retains the account device identity and credential/tenant-scoped
cache key without creating conversation metadata. At the final HTTP send
boundary it uses that scoped key as `session-id` for cache routing only. It
does not derive a thread, client request ID, or conversation metadata from it.
HTTP-to-WebSocket adaptation continues to use a private per-request execution
scope; the HTTP-only routing header does not become a WebSocket connection key
or turn-state session key. Explicit sessions, native WebSocket connections,
compact requests, Chat/Messages bridges, and API-key forwarding retain their
existing behavior. Existing account/model turn-state policy is unchanged.

The OMP missing-key fallback now runs before route splitting and input
transformations. Normal and passthrough OAuth requests therefore use the same
source payload for derivation. Existing client cache keys always take priority.

The captured fixtures and forwarding regressions are under
`backend/internal/service/testdata/omp_responses_18_2_6.*` and
`openai_omp_wire_regression_test.go`. They verify request compatibility and
isolation, including a local relay replacing the user agent.

A live diagnostic used OMP 18.2.6, synthetic reference text, one continued
session, one prompt-cache key, the same OAuth account, and the same returned
Terra model. With session headers removed/preserved in the sequence
off/off/on/on/off/off, cached input tokens were
0/0/11008/11008/0/11008 (about 12000 total input tokens per request). Both
preserved-header rounds achieved approximately 91%; an absent-header request
also hit after warming. These observations support stable cache routing,
not a guaranteed hit rate or a claim that headers explain every cache miss.
Quota injection remained enabled throughout; a separate local forwarding
regression verified that upstream cached-token counts reach both the response
and usage accounting unchanged.

These small diagnostics do not estimate a customer's overall cache percentage.
Production results still depend on prompt prefixes, account routing, retention,
and upstream cache availability. Aggregate OpenAI cache percentage must use
`sum(cache_read_tokens) / sum(input_tokens + cache_read_tokens +
cache_creation_tokens)` because stored `input_tokens` excludes cached input.
