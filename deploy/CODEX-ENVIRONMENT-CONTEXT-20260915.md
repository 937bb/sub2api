# Codex XML environment and client identity follow-up

Source release: `0.2.4.9`. This follows the user's explicit request to normalize
the model-visible `<environment_context><timezone>` and compare proxy labels
against official Codex source. It supersedes the `0.2.4.8` `api-client` fallback
for recognized proxy-branded client identity fields only.

## Official source evidence

The GitHub tree was resolved and relevant source files were fetched at commit
`19286b88190b1b81c2506af97dfca6ab261ff72b` of `openai/codex`:

| Source | Confirmed behavior |
| --- | --- |
| [Environment renderer](https://github.com/openai/codex/blob/19286b88190b1b81c2506af97dfca6ab261ff72b/codex-rs/core/src/context/world_state/environment.rs) | Renders environment context as a `user` fragment with kind `environments.environment_context`; emits `current_date` and `timezone` separately. |
| [Environment tests](https://github.com/openai/codex/blob/19286b88190b1b81c2506af97dfca6ab261ff72b/codex-rs/core/src/context/world_state/environment_render_tests.rs) | Explicitly tests `<timezone>America/Los_Angeles</timezone>` inside `<environment_context>`, including multiple execution environments. |
| [Default HTTP client](https://github.com/openai/codex/blob/19286b88190b1b81c2506af97dfca6ab261ff72b/codex-rs/login/src/auth/default_client.rs) | Default originator is `codex_cli_rs`; recognizes `codex-tui`, `codex_vscode`, and certain other first-party names. UA combines originator, build version, OS, architecture, terminal, and optional suffix. |
| [TUI client](https://github.com/openai/codex/blob/19286b88190b1b81c2506af97dfca6ab261ff72b/codex-rs/tui/src/lib.rs) | Initializes local and remote app-server clients with `client_name: "codex-tui"`. |
| [Responses metadata](https://github.com/openai/codex/blob/19286b88190b1b81c2506af97dfca6ab261ff72b/codex-rs/core/src/responses_metadata.rs) | Treats JSON-encoded `client_metadata["x-codex-turn-metadata"]` as canonical metadata; flat fields and HTTP/WS headers are compatibility projections. |

The official configuration webpage returned HTTP 403. Claims here rely on the
fetched, pinned source files, not that unavailable page or assumed behavior.
These sources establish formats, not that changing them avoids rate limits.

## Implementation

The earlier policy normalized JSON metadata but intentionally left message
strings untouched. This patch adds the specifically requested exception:
complete, parseable environment-context fragments in eligible text fields now
have their direct `<timezone>` value set to `America/Los_Angeles`. It works
without a `client_metadata` object. The XML renderer preserves the rest of each
fragment byte-for-byte, including workspace paths, permissions, shell and date.

Responses `input` strings, user/developer/system message text parts, and a
standalone environment fragment in `instructions` are supported. Converted
Chat Completions/Messages use the same map boundary; ordinary HTTP/compact and
WS preparation use the raw boundary. Only OpenAI OAuth/PAT accounts are changed.

Quoted prose, fenced examples, malformed XML/JSON, nested unrelated timezone
tags, assistant messages, tool calls/outputs, images and API-key upstreams are
preserved. A missing timezone is not invented. An environment block embedded
in arbitrary surrounding prose is deliberately not rewritten: the gateway
cannot prove that such text is harness metadata rather than a quoted example.
Multiple complete environment blocks in one text field are supported.

The raw path traverses JSON without decoding image/tool payloads and splices
only changed string tokens in wire order. It makes one final output copy,
preserving opaque fields and exact numeric values. The map and raw paths are
tested for equivalent results and idempotency.

Proxy-branded `client_name`, `app_name`, and `sdk_name` metadata now use the
existing Codex identity resolver's originator (normally `codex-tui`). These
optional metadata keys are not claimed to be mandatory official fields. A
proxy-branded UA uses the resolver's complete UA and effective Codex version;
the Sub2API server version is not reused as the Codex client version. Existing
originator headers are paired; a deliberately absent Messages-bridge
originator remains absent for compatibility. Normal non-proxy client names
and UA values retain their existing policy.

## Boundaries

The fetched official sources do not establish equivalents for our synthetic
image/task compatibility delimiters or the reserved Python tool alias. Their
neutral names and historical recognition are retained; they are not renamed
into invented official Codex features. Hash salts and repository attribution
also remain. No authentication, tool contract, quota logic, IPv6 connection,
concurrency limit, or production WS setting is changed.

`current_date` is retained because historical date-only values contain no time
of day from which an exact Pacific date can be derived. Host/database/billing
timezones are unchanged. Request environment metadata does not relocate the
network egress or authenticate a genuine desktop device.

This source update does not perform a production rollout.

## Validation

- Focused XML, metadata, account-boundary and header tests passed (1.228s).
- Full `go test -ldflags='-s -w' ./internal/service -count=1 -timeout=300s`
  passed (139.234s).
- The final added Codex identity-pair regression passed separately (1.177s).
- `go vet ./internal/service` passed.
- Local build initially ran out of disk space. Only previously generated,
  rebuildable task binaries in `/tmp` were removed. Production was untouched.
- Final `go build -ldflags='-s -w' -o /tmp/sub2api-codex0249-server ./cmd/server`
  passed when run separately after the tests.
- Formatting and `git diff --check` passed. Local `golangci-lint` remains
  unavailable and is not reported as passed.
