# Codex protocol comparison and Lite bridge repair, 2026-09-10

## Confirmed compatibility defect

An HTTP request that explicitly sets
`X-OpenAI-Internal-Codex-Responses-Lite: true` is normalized for Lite in
`forwardOnce`. The HTTP-to-WS forwarder previously omitted that mode when
creating the upstream frame. Codex expects the per-request flag
`client_metadata.ws_request_header_x_openai_internal_codex_responses_lite = "true"`.
The reverse WS-to-HTTP conversion already preserved it.

Version 0.2.4.5 copies the explicit HTTP mode onto each WS `response.create`.
Metadata is copied before adding per-request values, so the HTTP fallback body
and subsequent requests cannot inherit the flag through a shared nested map.
The mode does not participate in handshake identity or create another connection
pool. Ordinary requests are not automatically converted to Lite.

The regression failed on 0.2.4.4 for OAuth, PAT (`setup-token`), and API-key
HTTP-to-WS paths. It passes with the repair, including four sequential requests
that alternate Lite and non-Lite modes on one connection. HTTP passthrough,
function definitions, existing client metadata, and per-account Lite
normalization are checked. Separate cases verify that canonical request maps
remain unchanged for absent, string-valued, and interface-valued metadata maps.

This repairs an explicit protocol contract. It is not evidence that Lite
increases quota or fixes upstream capacity/rate limiting.

## Additional controlled comparisons

The opt-in diagnostic uses the production request builders and authorized
in-memory account snapshots. Account seeds/device IDs, settings, concurrency,
network configuration, and production connection pools are unchanged. Model:
`gpt-5.6-sol`; two active accounts; two rounds with reversed variant order.
The source IPv6 is fixed to an address already assigned to the server.
Only `response.completed` counts as completion. The 35-second observation
deadline belongs to the diagnostic, not the production gateway.

The latest WS run opens a fresh socket for each observation to remove idle-age
differences between variants. The device-mode comparison changes only the
in-memory identity projection and retains the existing seed/device ID. It does
not update account configuration. The Lite comparison uses the official empty
`additional_tools` prefix, developer base instructions, disabled parallel tool
calls, `reasoning.context=all_turns`, and the transport-specific Lite marker.

| Latest comparison | Account 60415 completed | Account 60476 completed |
| --- | ---: | ---: |
| WS, existing configuration | 1/2 | 1/2 |
| WS, explicit compatible Lite request | 0/2 | 1/2 |
| WS, device mode with existing account seed | 0/2 | 1/2 |
| HTTP/2, existing configuration | 0/2 | 2/2 |
| HTTP/2, explicit compatible Lite request | 0/2 | 2/2 |
| HTTP/1.1, existing request | 0/2* | 2/2 |

Account 60415 returned overload errors, timeouts, and HTTP 429. Account 60476
completed all three WS variants in the first round and rejected all three
handshakes with empty HTTP 403 responses in the second. These small samples do
not establish a stable benefit from Lite, device mode, or HTTP/1.1.

`*` The first HTTP/1.1 attempt on 60415 immediately followed a 429 with
`Retry-After: 2`; exclude it from comparative inference. The diagnostic now
skips remaining variants for an account while a received Retry-After is active.

Earlier protocol trials also tested removal of the model routing hint, a reduced
official-compatible header set, zstd HTTP request compression, omission of
optional WS frame fields, and disabled WS compression. None established a
repeatable completion improvement. The preceding UA comparison is documented
in [CODEX-HEADER-EXPERIMENT-20260910.md](CODEX-HEADER-EXPERIMENT-20260910.md).

## API and Codex endpoint differences

The public API WebSocket guide documents named `stream_id` lanes. The Codex
subscription endpoint returned `Unsupported parameter: stream_id` in three
accepted-connection observations. Do not enable that multiplexing scheme for
Codex OAuth/PAT based only on the public API guide.

Both tested accounts' official model manifests advertise
`use_responses_lite=true` and `prefer_websockets=true` for the tested model.
The manifest advertises Lite=false for `gpt-5.5`. Full Responses nevertheless
completed on the newer model, so this is not proof that non-Lite requests are
invalid. Existing API-key custom-provider manifest compatibility remains intact;
globally forcing Lite would change hosted-tool behavior.

## Excluded diagnostic failures

- An early successful WS socket was left idle while other variants timed out.
  Its later `keepalive ping timeout` is excluded; it is not evidence of a
  production pool defect. The final comparison uses fresh sockets.
- Four initial HTTP/1.1 attempts advertised h2 through cloned TLS ALPN settings
  while using the HTTP/1 parser. Their malformed-response errors are diagnostic
  defects, not upstream incompatibility. The helper now advertises only
  `http/1.1`; a local TLS server confirms H2/H1 negotiation and preservation of
  the original H2 transport. The corrected live run confirms both protocols.
- A previously selected account was revoked/deleted before a run started. The
  runner refused it without sending a request. Account-cohort changes must not
  be treated as an improvement attributable to a header change.

## Sources and evidence

Official source inspected on 2026-09-10:

- [Codex client](https://github.com/openai/codex/blob/main/codex-rs/core/src/client.rs)
- [Responses metadata](https://github.com/openai/codex/blob/main/codex-rs/core/src/responses_metadata.rs)
- [Base instructions fragment](https://github.com/openai/codex/blob/main/codex-rs/core/src/context/base_instructions.rs)
- [WS request types](https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/common.rs)
- [Public API WS guide](https://developers.openai.com/api/docs/guides/websocket-mode)

Sanitized raw observations are preserved in the operator's local experiment
directory `sub2api-protocol-experiment-20260910.nskMsp`:

- `protocol-ws-1789049546971814854.jsonl`
- `protocol-http-1789050087921606386.jsonl`
- `protocol-ws-1789050800669169052.jsonl`
- `protocol-http-1789050926236745152.jsonl`

No credentials or account snapshots are included in these reports. The
diagnostic is guarded by the `codexdiagnostic` build tag and an explicit runtime
opt-in; it is not part of the production binary.
