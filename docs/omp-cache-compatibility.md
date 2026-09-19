# OMP Responses cache compatibility

OMP 18.2.6 sends a stable `prompt_cache_key` when using a custom
`openai-responses` provider. Unlike its built-in `openai` provider, a custom
provider does not also send session or thread headers. This was verified with
the actual CLI, including continued conversations and a tool round trip.

The gateway previously generated a new conversation identity for each such
HTTP request in the account's session/full fingerprint modes. The cache key
remained stable, but session/thread/turn metadata changed. Using the cache key
as a session instead is unsafe: OMP supports independent routing-session and
cache-key overrides, including a cache prefix shared by separate conversations.

For OAuth Responses requests with a cache key and no explicit conversation,
the gateway now retains the account device identity and the credential/tenant
scoped cache key without creating conversation metadata. Header builders and
the final metadata projection also avoid promoting the cache key or a lone
request ID to a thread. HTTP-to-WebSocket adaptation uses a private per-request
execution scope, so shared prompt caching cannot share stored conversation
state. Explicit sessions, native WebSocket connections, compact requests,
Chat/Messages bridges, and API-key forwarding retain their existing behavior.

The OMP missing-key fallback now runs before route splitting and input
transformations. Normal and passthrough OAuth requests therefore use the same
source payload for derivation. Existing client cache keys always take priority.

The captured fixtures and forwarding regressions are under
`backend/internal/service/testdata/omp_responses_18_2_6.*` and
`openai_omp_wire_regression_test.go`. They verify request compatibility and
isolation, including a local relay replacing the user agent. They do not prove
an upstream cache-hit percentage. Production results still depend on prompt
prefixes, account routing, retention, and upstream cache availability.
