# Kiro reference review — 2026-11-07

Primary reference: `ZyphrZero/kiro.rs` at
`3e20b44af28e1f45ec8a74241236443ede905418`.
The earlier reference remains `hank9999/kiro.rs` at
`8fb3dd691d1cedf437bd6a4e9ef9f60b3f217fdd` (MIT).
The sub2api implementation baseline is `1783c07f8942d3a23ff03d2119049f019735db8b`.

## Changes implemented

| Boundary | Finding | Result |
| --- | --- | --- |
| Credential import | Reference optional credential fields accept JSON null; our Go validator rejected it, including Google exports with null idToken/clientId/expiresAt. | Frontend and backend treat known nullable credential fields as absent. Real type errors and conflicting non-null aliases remain errors. |
| Credential replacement | Null identity/auth fields could suppress preservation and reset the account's machine or authentication selection. | Remove null placeholders before preserving environment and merging redacted secrets. Retain stored tokens, machine identity, profile and regions when omitted/null. |
| Admin refresh | Kiro was not handled by refreshSingleAccount and fell through to Claude OAuth. | Single/batch admin refresh uses the same Kiro executor, fixed proxy, distributed lock and conditional durable credential update as inference. A stale concurrent admin snapshot does not force a second rotation. |
| Refresh diagnostics | Upstream HTTP failures became a generic admin 502; invalid_grant was lost before shared race recovery. | Preserve the operation, HTTP status and allowlisted reason; never expose the upstream body, token, proxy address/password or arbitrary error text. Refresh 400 invalid_grant remains recognizable to shared race recovery. |
| API region | The reference resolves a real profile ARN's region separately from the OAuth region. | Explicit api_region wins, followed by validated real profile ARN region, then region/default. Authentication region stays separate; the known Builder ID placeholder does not select a region. |

## Verified production observation

Account 65444 remained active and schedulable, bound to proxy 1234. Its model-list
request failed during credential acquisition: the request to
`prod.us-east-1.auth.desktop.kiro.dev` returned HTTP 401 in roughly 183–213 ms.
This establishes credential-refresh rejection, not account suspension and not a
model-catalog timeout. Fresh credentials from a successful Kiro sign-in are needed
to establish this account's current usable models. No account status or proxy was
changed during this comparison.

A diagnostic invocation of the existing admin refresh action also reproduced its
wrong-provider branch. The new regression verifies that Kiro can never reach the
Claude OAuth service through that action.

## Comparison boundaries and remaining gaps

- The reference's Social refresh endpoint/body and stable machine User-Agent
  match our existing flow. Its additional advertised compression formats are not
  copied into a client that cannot decode them.
- The reference uses legacy GET `q.*.amazonaws.com/ListAvailableModels` and IDE/CLI
  generation paths. Our verified runtime-v3 POST endpoint and management catalog
  are retained. Changing a transport requires live evidence for the affected
  account/model; this review does not establish that legacy paths are superior.
- Tool-name aliases, schema normalization, paired tool results and event-stream
  parsing already exist locally. Reference features such as hosted MCP web search,
  external IdP OAuth, real-profile discovery and additional tool repair behavior
  are not claimed as implemented by this change. Protocol extensions need to use
  the repository's RelayKit path and preserve actual tool results and accounting.
- Reference prompt-cache metering is explicitly simulated. Our group policy keeps
  charging only the implemented local response cache's actual creation/read events;
  no synthetic prefix-hit counters are introduced.
- Account-specific HTTP identity is preserved. Go TLS remains in use; JA3/JA4
  equivalence with the official desktop client has not been measured. Kiro uses
  HTTP event streams here, not WebSocket.

## Validation

Focused regression coverage includes nullable Social exports, camel/snake aliases,
nested imports, preserved IdC secrets and identity, real/profile-placeholder region
selection, sanitized HTTP/transport errors, fixed-proxy manual refresh, durable
rotation, lock requirements and concurrent refresh suppression. Existing Kiro
cross-protocol, tool and cache tests run with the affected packages.

Validation completed on an isolated server source tree: all tests in
`internal/pkg/kiro`, `internal/service`, and `internal/handler/admin` with the
`unit` tag passed; focused credential/refresh tests passed with `-race`.
Frontend import tests (15 cases) and `vue-tsc --noEmit` passed.
The production server entrypoint compiled successfully.
These changes have not been switched into the production runtime.


## API key and explicit CLI endpoint follow-up — 2026-11-08

A new API-key account uses its own static proxy. Live preflight established that
both management POST and legacy REST model discovery returned 22 models, and
both runtime-v3 and reference CLI envelopes completed a minimal Opus 5.5 turn.
The previously inferred requirement that every API key needs CLI was therefore
not supported: runtime-v3 remains the default; `endpoint: "cli"` selects the
reference AWS JSON endpoint explicitly.

Implemented the CLI transport envelope and headers without expanding client
protocol translation: the already-converted Kiro message objects retain text,
tools, results, images and model identity. Only the origin, historical modelId,
and agentContinuationId envelope fields differ. Unlike a whole-body string
replacement, a user's literal `AI_EDITOR` text remains unchanged. API-key calls
omit OAuth profileArn across generation, model discovery and usage. Endpoint
selection changes the environment/cache key and the admin fingerprint display.

The account form accepts a bare `ksk_` key as well as credential JSON. Existing
OAuth accounts retain their deployed runtime-v3 path and refresh lifecycle.

No RelayKit dependency, integration or package locator exists in the checked
repository; the only references are in resource-safety documentation. This patch
does not add new client protocol conversions or reconstruct tools. Hosted MCP
search and other translation extensions remain outside this change.

Release validation passed: default and unit-tag backend suites, focused Kiro
race tests, full go vet, 394 frontend test files / 3185 tests, type checking and
production frontend compilation. Live and rollout results are recorded separately.
