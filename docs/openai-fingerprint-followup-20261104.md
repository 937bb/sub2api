# OpenAI outbound fingerprint follow-up, 2026-11-04

This candidate builds on `8371cc2d6`. It closes auxiliary HTTP transport gaps and
matches newly captured WebSocket framing. It does not change account scheduling,
conversation binding, message roles, database migrations, or production routing.

## Auxiliary HTTP paths

PAT `whoami` and the OAuth Codex models-manifest fallback previously used the
shared generic Go transport even when their headers selected the measured local
CLI identity. They now use `httpclient.GetCodexClient`:

- The exact measured CLI UA selects the existing native or custom-CA TLS/HTTP
  profile and identity-partitioned connection pool.
- Other identities, including explicitly configured Desktop/Linux/browser UAs,
  retain their existing transport. A transport measured for the CLI is not
  assumed to match those clients.
- Native and custom-CA clients occupy separate cache entries.
- Plain HTTP retains explicit proxy routing and the applicable ordered HTTP
  serializer. HTTPS uses the existing measured CONNECT/SOCKS dialer.
- Endpoint headers, authentication, plugins, response handling, and existing
  timeout settings are preserved. No inference-only identity headers are added.
- `CloseIdleConnections` now passes through metrics and host-validation wrappers.
  This lets callers actually retire idle identity partitions without canceling
  active streams.

## Successful local WebSocket capture

`scripts/capture-codex-ws-frames.py` runs the installed CLI against an isolated
loopback provider, using a fake API key and temporary CODEX_HOME. It serves a
minimal successful Responses exchange, once without compression and once with
`permessage-deflate`. Both CLI runs exited successfully. The sanitized fixture is
`backend/internal/service/testdata/codex_ws_frames_20261104.json`.

The CLI hash remains
`508388df35319042107560d095b271a98a512f59a542848c908b23ed1bad4b44`.
Both runs contained a prewarm request and a subsequent inference request.

| Observation | Uncompressed | permessage-deflate |
| --- | --- | --- |
| Text frame opcode | 1 | 1 |
| FIN | true per message | true per message |
| Client masking | enabled | enabled |
| RSV1 | false | true |
| Decoded message sizes | 36022 / 22177 bytes | 36022 / 22177 bytes |
| JSON trailing newline | absent | absent |
| HTML escaping of observed angle brackets | absent | absent |
| Second message needs previous DEFLATE context | n/a | yes |

Compressed byte counts vary with generated IDs. The fixture does not retain
request bodies, real credentials, masking keys, raw IDs, or process output.

## WebSocket changes

The local coder/websocket patch adds opt-in `DialOptions.SingleFrameMessages`.
The gateway enables it only for the measured CLI UA. This buffers compressed
messages for one final frame instead of emitting fragments followed by an empty
final continuation. Compression windows and context takeover remain negotiated.
Small first writes do not begin compression midway through a buffered message.
The default library behavior is unchanged. See `third_party/coder-websocket/
LOCAL_CHANGES.md` for maintenance instructions.

Gateway-generated JSON for that identity now disables HTML escaping and removes
the encoder's trailing newline. Quotes/control characters remain JSON-escaped;
other client identities retain their prior serializer. Raw passthrough frame
payloads are preserved. No JSON property-order or universal Unicode-byte
identity claim is made.

Single-frame compression adds a temporary buffer proportional to encoded message
size; it is released after the message is sent. This is a memory tradeoff of
matching the observed framing.

## Verification

Targeted tests passed for auxiliary endpoint ClientHello selection and untrusted
certificate rejection, actual custom-CA HTTP/2 reuse and identity isolation,
explicit HTTP and HTTPS CONNECT proxy routing, idle-pool cleanup, JSON serialization, and the
WebSocket frame/window matrix. Validation completed on the isolated Linux
candidate checkout with Go 1.27.3:

- Whole backend: `go test -p 2 ./...` passed (53 packages).
- Patched WebSocket library: `go test -p 2 ./...` passed.
- Backend and patched-library `go vet` passed; backend production binary compiled.
- Targeted backend and WebSocket window/frame tests passed with `-race`, including
  the final test cleanup changes.
- golangci-lint v2.14.0, `--new-from-rev=157af9664`, passed for both modules
  with zero new issues. This is an incremental lint gate.
- All 14 changed Go files passed gofmt; the reproducible capture script passed
  syntax validation and a second successful two-mode loopback run.

Test corrections during validation included selecting the CLI UA in the fixture
instead of the deliberately unsupported Desktop UA, and handling test cleanup
errors. The production implementation did not expand the measured-profile gate.
No live-account inference or production deployment was performed.

## Remaining limits

This improves measured compatibility; it does not establish full native-client
identity. Successful native macOS HTTPS application traffic and real OAuth WS
inference were not captured. TLS record/resumption equivalence, HPACK decisions,
small-message compression thresholds, compressed byte-for-byte equality, and
service-issued assertions remain unverified. The local successful WS exchange
uses a custom provider and dummy credential. Production remains on the rollback
runtime until a separately validated rollout.
