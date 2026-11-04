# Persistent OpenAI OAuth conversation binding

The default `gateway.openai_conversation_binding_enabled` policy pins an OAuth
conversation to a durable account/credential/device identity. The authenticated
API key and group namespace the session hash; HTTP, Messages, Chat Completions
and WS use the same policy. API-key provider accounts and other platforms retain
their existing behavior. Disable the policy only as an explicit operator rollback.

Migration 250 adds `openai_conversation_bindings`. Atomic insert-on-conflict elects
one owner across concurrent processes. Binding rows do not expire with Redis and
have no cascading foreign keys: account deletion must not silently reassign them.
Only hashed keys, internal account IDs, a credential/device hash and creation time
are stored. Response aliases allow response-only continuation after cache expiry.
No conversation content or access token is stored.

Bound requests bypass weighted sticky and sticky escape. They still undergo group,
capability, transport, privacy, runtime and profit eligibility checks. An excluded,
disabled, rate-limited, incompatible or changed identity returns an actionable
503 instead of trying another credential. A final outbound check detects identity
edits while waiting. Temporary failures retain the binding. Failed claims and
storage errors fail closed. New first-turn attempts may bind before admission;
therefore even a failed initial attempt retains its account for retries.

Client session-id/conversation identifiers or a consistent prompt_cache_key are
required for reliable identification across protocols. Existing content-derived
fallback is supported but can change with model/tools/history; an unknown history
continuation is rejected with 409 rather than assigned a new OAuth account. Without
a stable identifier or response chain, two stateless requests cannot reliably be
recognized as the same conversation. New and migrated clients should send a unique,
stable session-id, not a shared generic cache key. Changing API key/group starts a
separate binding namespace; continuing old history without verifiable ownership
fails closed.

Existing conversations without durable bindings may import a previous response
owner only if its authenticated API-key ownership is verified. Ordinary legacy
sticky records alone are not sufficient. Unknown old history must start a new
conversation explicitly. Encrypted reasoning/compaction history is never evidence
that another credential is eligible. Strict binding prevents scheduler-driven
cross-account sanitization; existing same-account invalid-cipher recovery remains.

Validation includes concurrent first claims, expired cache, disabled credentials,
changed device identity, shadow ownership, tenant isolation, response aliases,
storage failure, runtime profile diagnostics, UI rendering and production HTTP/WS
roundtrips. Full production/native wire equivalence is not claimed.
