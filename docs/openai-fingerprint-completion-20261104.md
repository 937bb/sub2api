# OpenAI outbound fingerprint completion, 2026-11-04

This change starts from rollback commit `157af9664`. It changes outbound identity
and transport reuse only. It does not reinstate persistent conversation binding,
reject old conversation history, add a migration, or change account scheduling.
Production remains on the restored `4c54feab3` runtime pending a separate rollout.

## Current local evidence

The installed application changed since the November 3 capture:

| Artifact | Observed value |
| --- | --- |
| Application/build | 26.1029.61003 / 24614 |
| Bundled engine | 0.166.0-alpha.6 |
| Platform | Mac OS 26.6.2; arm64 |
| ASAR SHA256 | 0c951258b7ab381ec601045ffb02159e53f424548dda18e6a4dcfd126557383b |
| Native CLI SHA256 | 508388df35319042107560d095b271a98a512f59a542848c908b23ed1bad4b44 |

`inspect-chatgpt-desktop-identity.py` initializes the bundled application server in
an isolated home. `capture-codex-loopback-headers.py --mode http|ws --output FILE`
records sanitized request shape from a local fixture with a dummy API key. It
captures WS prewarm, turn, and rejected-upgrade HTTP fallback. It never stores
request bodies, credentials, or raw session IDs. The engine fallback now comes
from one shared constant. Explicit runtime version settings retain precedence.

New TLS fixtures under `backend/internal/pkg/tlsfingerprint/testdata/*20261104.json`
record separate native HTTP, Rustls WS, and custom-CA Rustls HTTP handshakes.
Cipher suites, extensions, groups, signature algorithms and key shares match the
previous profiles. Native HTTP has no ALPN; WS has no ALPN; custom-CA HTTP offers
`h2,http/1.1`. The H2 settings remain `2=0,4=2097152,5=16384,6=16384` with connection
WINDOW_UPDATE 5177345. Existing wire tests now compare against these fresh samples.

## Runtime changes

- Account scoping now covers captured `root_turn_id` and `context_window_id` in
  flat and embedded metadata. A root turn keeps `root_turn_id == turn_id`, including
  explicit session/full convergence. Distinct subagent roots remain distinct.
  Prewarm requests keep empty turn IDs; absent fields are not invented.
- Case aliases of identity headers are collapsed before serialization, including
  plugin-created noncanonical maps. Known installation IDs remain stable.
- Identity metadata is decoded with JSON number preservation. Rewriting IDs does
  not round opaque integers or change tool/user history outside metadata.
- Measured HTTP/1 and custom-CA HTTP/2 transports partition reusable connections by
  client/engine/account/device identity. Turn/session/request IDs do not fragment
  the pool. Known device identities survive bearer refresh; changing the device,
  client identity, account header or FedRAMP context selects a separate transport.
- Each wrapper retains at most 64 cached partitions with LRU eviction. Eviction
  closes idle connections and closes retired idle pools again when active bodies
  finish. It does not cancel active streams. HTTP upgrade bodies keep io.Writer.
- WS handshake HTTP-client caches use the same identity digest in addition to the
  existing proxy and TLS-profile keys. Established WS scheduling is unchanged.
- Token exchange and refresh attach only a credential digest to internal context.
  Shared auth clients therefore cannot reuse one TLS connection across different
  credentials. No internal digest or inference-only installation header is sent
  to authentication endpoints. Without a known device, bearer requests use a
  credential digest as the connection partition; token rotation can open a new
  connection in that fallback case.
- CLI auth/quota requests also remove inherited `sec-ch-ua*` browser hints.
  Endpoint-required account, authorization, beta, and language fields remain.

## Verification and limits

Tests exercise actual HTTP/1 and TLS/H2 socket reuse, credential-isolated OAuth
refresh/exchange, active-stream survival during eviction, concurrent partitions,
metadata case aliases and numeric precision, WS transport partitioning, and the
existing raw HTTP/WS/TLS wire fixtures. Validation passed on the isolated Linux
candidate checkout with Go 1.27.3:

- `go test -p 2 ./...`: entire backend passed.
- `go vet -p 2 ./...`: passed.
- `go build -p 2 -o <candidate>/sub2api-check ./cmd/server`: passed.
- `go test -race -p 2` across the OpenAI, TLS fingerprint, service and repository
  packages, selecting the identity/metadata/WS/auth tests: passed.
- golangci-lint v2.14.0 with `--new-from-rev=157af9664`: no new issues. This is
  an incremental lint gate, not a claim that all legacy code is lint-clean.
- 25 changed backend files matched between local and candidate checkout by
  SHA256; all 21 changed Go files passed gofmt. The loopback capture script
  passed syntax checks and observed one HTTP request and three WS/fallback
  requests in separate isolated runs.

The first full run caught an outdated version assertion; it was updated to the
new observed engine version before the successful rerun. Three unchecked test
assertions were also corrected before lint passed. No production credentials or
live-account inference were used for these tests.

This is not complete native-client equivalence. Successful native macOS HTTPS
application traffic was not captured because the loopback certificate is not
trusted by that backend; the native TLS ClientHello and plaintext HTTP header
shape are separate observations. Custom-CA HTTPS succeeds against a temporary
trusted CA. All captures use a custom provider, not real OAuth inference.
TLS session resumption/record bytes, H2 HPACK decisions, WS compression byte
streams and runtime service-issued assertions remain unverified. Network egress
and endpoint-specific browser surfaces retain their configured behavior.
