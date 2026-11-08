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


## Native runtime integration — 2026-11-08

This follow-up ports runtime behavior into the existing Go provider and shared
response pipeline; it does not start a separate Rust service or replace Sub's
account pool, static-proxy binding, group policies, persistence or pricing.
The reference remains pinned to `3e20b44af28e1f45ec8a74241236443ede905418`.

| Boundary | Integrated behavior | Sub-specific constraint |
| --- | --- | --- |
| Explicit rejection | `metadataEvent.stopReason=CONTENT_FILTERED` becomes Anthropic `refusal`, Chat `content_filter`, Responses `response.incomplete` with reason `content_filter`, and Gemini `SAFETY`. | A context-only or unexplained empty stream remains an error. Refusal ends immediately without reading a potentially stalled tail. It does not make the upstream accept rejected input. |
| Interrupted tools | A refusal with an unfinished tool produces a permission error without closing that tool, recovering JSON, or publishing a successful terminal. | Valid-looking arguments without the tool's completion marker are still unfinished. Complete tools followed by refusal retain the refusal terminal rather than `tool_calls`. |
| Actual token usage | `metadataEvent.tokenUsage` supplies uncached input, output, cache read and cache write buckets. Repeated snapshots replace rather than add. | Missing/null/empty usage objects do not erase estimates. Negative counters clamp to zero; non-integral/oversized int32 fields fail. A real metadata snapshot takes precedence over legacy metering token fields. |
| Stop reasons | MAX_TOKENS, STOP_SEQUENCE and MODEL_CONTEXT_WINDOW_EXCEEDED retain their native semantics across finalization. | Later tool events cannot overwrite a truncation reason; explicit refusal always wins. |
| Cache and pricing | Actual upstream cache buckets feed existing Sub pricing. Actual local writes/reads retain their existing group policy and cache pricing buckets. | No simulated prompt hits. Refusals never write cache. A request-local refusal marker bypasses billing persistence and charging, including per-request prices and partially generated responses. |
| Streaming usage | Publish final counters at message_delta; message_start carries zero pending counters. | Prevents a stale input estimate surviving an authoritative zero snapshot or refusal through nonzero-only merge code. |
| Account testing | A refused probe reports a failed test instead of a successful account test. | A refusal alone is not evidence of invalid credentials or a suspended account. |

The repository still has no callable RelayKit dependency or package locator.
This integration uses the already active provider decoder and shared response
adapters, with regressions for all affected ingress protocols. No second
translation service or speculative tool repair path is introduced.

Validation covers streamed and buffered requests on all four protocols,
pre-flushed SSE, partial text, complete and incomplete tools, exact/zero/repeated
usage snapshots, upstream/local cache buckets, account-test refusal, cancellation,
and native terminal conversion. Tests use synthetic event streams and fixed
proxy fixtures; they do not establish that arbitrary live prompts will pass the
provider's filters.

The earlier API-key release was rolled back after a real 6064-to-candidate
connection reset. The release follow-up correlated that reset with an immediate
application 403 while Nginx was still uploading the body. An isolated Nginx
reproduction converted 80 of 100 synthetic 4 MiB authentication failures into
502s. Request buffering alone still failed once in a larger upload matrix.
The server now discards bounded HTTP/1 request bodies before early error
responses, preserving existing upload size and read timeout limits. A subsequent
160-request matrix (1/4/16/32 MiB, Content-Length and chunked uploads, three
concurrent writers, unbuffered Nginx) returned the original 403 in every case.
SSE success and WebSocket upgrades do not trigger this rejection drain.
Hosted MCP web search, external IdP login, profile discovery, native reasoning
signatures and measured TLS fingerprint parity remain outside this integration.

A local Claude Code 2.1.319 synthetic replay consumed SSE generated by the Go
Kiro decoder: normal text returned OK with end_turn; refusal was recognized as
stop_reason=refusal and rendered the CLI's model-safeguard message. The CLI exited
with an error for refusal and made two generation calls in that scenario, so
native terminal support does not imply the client will never retry. No real
upstream credentials, user context or production endpoint were used in this
probe, and it is not evidence of live provider acceptance.

Validation outcome: the kiro, apicompat and service default suites passed before
the billing-marker follow-up. Final focused runtime/Kiro/terminal tests passed
with the race detector; shared adapters passed their complete suite. Final unit-tag Kiro, RecordUsage, pricing and isolated memory regressions passed
with stripped test binaries; results are recorded in the local evidence report. Affected-package
go vet and the server build passed (the final build used -s -w after a disk-space
failure). The expanded unit-tag service suite failed twice at the unchanged
TestInflightEstimate_AccountMappingNoDBAndBoundedMemory assertion: retained heap
deltas 8,397,656 and 8,511,840 bytes exceeded its 8,388,608-byte limit. The release
follow-up traced retained allocations to go-cache janitors: fallback-only rate
resolvers created a cache and goroutine on every call despite having no
repository to populate it. They now omit that cache; supplied cache-only
overrides remain supported. The original 8 MiB assertion is unchanged.
The orphan-parser zero-allocation test now runs in a fresh test subprocess,
because AllocsPerRun also counts unrelated background service allocations.
Its zero-allocation assertion is unchanged. Production status and the final
release gates are recorded separately in the release evidence.
