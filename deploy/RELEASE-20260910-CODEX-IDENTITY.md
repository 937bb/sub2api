# Codex outbound identity consistency (937sub2b 0.2.4.3)

An explicitly configured panel User-Agent was shadowed by generated account
environments. Some OAuth/PAT forwarding paths also hashed the same session or
installation value twice, or used different values in HTTP/WS headers and body
metadata. This release fixes those inconsistencies across Responses, passthrough,
WebSocket and the Chat Completions/Messages adapters.

The environment precedence is explicit account UA, explicit panel UA, then the
existing stable account environment. All versions still follow the existing
manual/synchronized Codex version setting. Supplying a UA containing an older
version does not silently downgrade the active version. Blank panel configuration
keeps the existing per-account fallback.

Account seeds, saved installation IDs and body prompt-cache keys are retained.
OAuth/PAT session headers use the same UUID derivation as body metadata; a value
already normalized in the body is projected without another hash. Explicit
conversation IDs remain independent. The current fingerprint convergence setting
is honored, including off mode. API-key traffic remains outside these projections.
Session headers necessarily change once on paths that previously used hex or a
second hash; existing upstream sockets are drained during deployment.

The administrator account test accepts an optional `codex_user_agent`. It applies
to a request-local credential copy, validates the identity, and uses the existing
read-only diagnostic path without persisting the override or changing cooldowns.
This allows a same-account comparison without editing production credentials.

## Evidence and limits

Official Codex source, inspected on 2026-09-10:

- [UA construction](https://github.com/openai/codex/blob/main/codex-rs/login/src/auth/default_client.rs)
  uses `os_info` and terminal detection, with an optional parenthesized suffix.
- [Session headers](https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/requests/headers.rs)
  project session and thread identities.
- [Responses metadata](https://github.com/openai/codex/blob/main/codex-rs/core/src/responses_metadata.rs)
  describes body metadata and direct headers as projections of one snapshot.

These sources support consistent formatting and identity projection. They do not
establish that a particular OS/terminal UA avoids upstream overload or rate
limits. Internal hash namespace strings are not sent upstream. This release does
not change account quotas, billing, request concurrency, scheduling or quota retry
payloads, and does not claim to eliminate upstream 429/403/502 responses.

## Validation and deployment

Regression coverage includes panel/account precedence, runtime version pairing,
OAuth/PAT UUID isolation, HTTP/WS/body parity with convergence disabled, saved
installation IDs, cache-only requests, API-key exclusion and diagnostic override
isolation. Existing tool, stream, reconnect and compatibility tests are required.

The default `go test ./...` suite and `go vet ./...` pass. The separate `unit`
build tag currently fails to compile in the unchanged base as well, due to six
stale image-cooldown test references; this unrelated baseline issue is not
represented as fixed. New regression tests run in the passing default suite.

Deploy B only with the unchanged frontend embedded. Retain local port 6064 and
C's existing upstream URL. Validate the candidate, pages/assets, HTTP/SSE, WS and
tool continuations before switching new connections. Never stop a listener with
live connections. Check any previous release finalizer before starting another.

Rollback restores the previous image and explicit UA setting, while retaining
account seeds and installation IDs. As with deployment, route new connections to
the healthy listener first and let existing streams drain.
