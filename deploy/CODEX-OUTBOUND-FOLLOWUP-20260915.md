# Codex outbound follow-up: 0.2.4.10

This release closes verified request-construction gaps in 937sub2b. It does
not assert that metadata changes defeat upstream limits or improve model quality.

## Changes

- Override `Accept-Language` on privacy, account-info, subscription and quota
  requests. req/v3 v3.59.0 `ImpersonateChrome()` installs `zh-CN,zh;q=0.9` by
  default; changing only `oai-language` left that independent header unchanged.
- Give OAuth/PAT image-generation requests the same credential/tenant/session
  identity resolver and staged header/body IDs used by Responses. Stable device
  identity is retained across tenants; conversational identity is isolated.
  Remove the image path's reintroduction of `responses=experimental` after the
  shared request builder. Other endpoint-specific beta behavior is preserved.
- Recognize Unicode-escaped XML root names on the raw JSON path. Support a
  complete trailing environment fragment after the desktop AGENTS instruction
  envelope. Instructions inside the envelope remain opaque. Arbitrary surrounding
  prose, quoted/fenced examples, malformed XML, tool data and images are preserved.
- Preserve Codex delegation headers for PAT as well as OAuth accounts. API-key
  traffic and requests not identified as Codex retain their prior restrictions.
- Remove `tool_namespaces_info` only from the compatibility HTTP/WS metadata
  header, matching [official Codex projection](https://github.com/openai/codex/blob/19286b88190b1b81c2506af97dfca6ab261ff72b/codex-rs/core/src/responses_metadata.rs#L354).
  Body metadata, actual tool definitions, other metadata and integer precision
  remain intact.

The broad XML substring scan in intermediate commit `a5bce8b2` was narrowed
before deployment because it could rewrite quoted examples. That intermediate
commit was pushed to one repository but was never deployed. This release's
tests explicitly protect those examples.

## Verification

The focused tests capture requests with synthetic credentials and a fake
upstream; no accounts or privacy settings are changed by those tests. They cover
the req/v3 defaults, raw/map XML parity, OAuth/PAT image identity, repeated
conversation stability, tenant separation, header/body metadata separation and
API-key exclusions.

Focused regressions, the full service test suite, `go vet ./internal/service`,
and an embedded-frontend server build passed locally. `golangci-lint` is not
installed. Earlier attempts failed because the local disk was full; clearing
rebuildable Go compilation cache allowed the checks to complete.

## Rollout boundaries

The B release retains the live environment, including HTTP-only upstream mode,
IPv6 relay and the 32/8 database pool. No database migrations or frontend source
changes are included relative to deployed 0.2.4.7. Deployment verifies the
candidate, switches new connections, and drains old connections before replacing
the primary listener on port 6064. C and unrelated services are outside this
deployment. Operational results are recorded separately on the server.

Optional user-provided metadata, historical markers in conversation text and
plugin-owned transports are not globally renamed. Plugins may change outbound
requests after the core hands them off; this source patch cannot validate an
independently supplied plugin implementation.
