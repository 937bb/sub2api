# Kiro context-only responses: reference comparison

## Scope and evidence

On 2026-11-06, a Claude Code greeting failed when the client automatically
attached its global instructions and Skills/MCP context. The request was
reconstructed from local CLI request records without authentication headers. The native Kiro
response was HTTP 200 followed by one `contextUsageEvent` and EOF. There were
no assistant, tool, reasoning, metering, or explicit error events.

The comparison uses `hank9999/kiro.rs` commit
`8fb3dd691d1cedf437bd6a4e9ef9f60b3f217fdd`. GitHub's HEAD endpoint was checked
on the same date. Its original converter and request types were compiled in a
small offline Rust harness. Credentials were passed only to a separate native
sender, through stdin. The account and its fixed proxy were held constant.

| Request through the reference converter | Converted request bytes | Upstream events |
| --- | ---: | --- |
| Original failing request | 141176 | context only |
| Original request, preserving trailing system instructions | 192990 | context only |
| Baseline with the original top-level system and tools, without attached local context | 43610 | 5 assistant, 1 context, 1 metering |
| Baseline plus a 92-character fragment of the attached Skills description | 43724 | context only |
| Small request with that fragment and no tools | 925 | context only |
| Repeat of baseline plus the same fragment | 43724 | context only |

The reference converter drops messages after the last user message. Preserving
the trailing system instructions before conversion was therefore a separate
test, not an assumption that the first comparison retained all information.

The fragment was isolated by bisecting the 46,947-character context block and
falls in the introductory Skills list and an `ad-pentest` description. Private
instructions, credential material, and complete client requests are excluded
from this repository. A larger neutral Chinese input had already succeeded on
the same route, so byte count and Chinese encoding alone do not explain this
failure.

## Conclusions and limits

- The empty response is reproducible before B-side protocol conversion, using
  the reference converter as well as our converter.
- The same route successfully generates text and metering for the baseline.
- The upstream does not identify why it ends the response. Content filtering
  is a hypothesis, not a confirmed error classification.
- Reference HTTP 200 is insufficient evidence of success: its handlers can
  finalize an empty answer. Copying that behavior would mask the failure.
- No header adjustment, prompt deletion, account switch, or invented output is
  part of the fix below. The original full-context conversation remains an
  unresolved upstream failure.

## B-side correctness fix

Delay `message_start` until a real text or tool block exists. The compatibility
pipe waits for that first event or a decoding error before returning a success
response to the existing adapters. This prevents buffered adapters from turning
`message_start` followed by an empty-stream error into a blank success.

Report `kiro_empty_response` with an explicit description and retain only event
counts in diagnostics. Mark the error response as committed so outer handlers
do not append a generic replacement error. Preserve protocol-specific failure
framing when a keepalive has already committed the HTTP stream.

An empty response produces no billable result and never reaches the cache
finalizer. Waiting for first content is cancellable and closes the upstream
body on cancellation. No new upstream read deadline is introduced.

Regression coverage includes Messages, Chat Completions, Responses, and Gemini,
streaming and buffered modes, pre-flushed keepalives, no cache insertion or
billable result, and client cancellation while waiting for first content.

This fixes empty-response handling. It does not make the Kiro upstream answer
the original full-context request.

## Verification evidence

The new buffered Chat Completions regression was also run with the original
stream decoder and compatibility bridge from `8faa94263`. It failed because
the old path returned no error for the empty upstream response. With the fix,
all targeted Kiro/NoContent tests passed, including the cancellation test.

Candidate verification on the server completed successfully:

- `go test -p 4 ./...`
- `go vet -p 4 ./...`
- `go build -p 4 -o <candidate>/server-check ./cmd/server`
- `go test -p 4 -race ./internal/pkg/kiro ./internal/service -run 'Test(Kiro|NoContent)' -count=1`

SHA256 comparison confirmed that all nine modified Go source/test files in the
candidate match the local working tree. No production container, routing,
account setting, or credential was changed by this patch verification. The
deployment status is recorded separately in the release evidence.
