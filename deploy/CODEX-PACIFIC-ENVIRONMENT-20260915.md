# Codex Pacific request environment

This B-branch patch normalizes optional OAuth/PAT client-environment metadata to
`America/Los_Angeles` and `en-US`. Recognized country fields become `US`.
`Accept-Language` is set to `en-US,en;q=0.9`; the independent quota-query
`oai-language` value is changed from hard-coded `zh-CN` to `en-US`.

Only named environment fields inside `client_metadata`, its known environment
objects, and JSON-encoded `x-codex-turn-metadata` are normalized. Existing
`client_name`, `app_name`, or `sdk_name` values containing Sub2API become the
neutral `api-client`. No new top-level protocol fields are added. Unknown
metadata, opaque routing hints, and malformed embedded metadata are retained.

User instructions, message text, paths, images, tools, tool outputs, and API-key
upstreams are not rewritten. This is not a global substring-removal filter.
Existing account and conversation ID derivation salts remain unchanged; their
internal names are hashed rather than transmitted in plaintext.

HTTP regular, passthrough, and compact builders apply the environment boundary.
Shared identity helpers also normalize the corresponding WS metadata and
headers, but the production rollout continues to force HTTP/SSE.

The IANA timezone supports both PST and PDT. Host, database, billing, and
subscription-reset timezones are not changed. Request metadata cannot change
the geographic origin of an IP, provide a genuine client attestation, or
guarantee the absence of upstream rate limits, errors, or proxy identification.

Validation covers raw payload preservation, metadata numeric precision,
idempotency, OAuth/PAT versus API-key boundaries, compact/passthrough paths,
malformed optional metadata, daylight-saving offsets, and quota-query language.

The subsequent `0.2.4.8` source update also applies the header boundary after
final UA overrides and to model-list, Live, and account-test requests. Alpha
search retains its endpoint-specific contract. A case-insensitive proxy product
token in the UA becomes `api-client`; the remaining UA structure and the
Messages bridge's deliberately absent originator remain intact. Identity
rewrites now preserve large integer and decimal metadata values as well.
