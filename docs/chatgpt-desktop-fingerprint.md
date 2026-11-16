# ChatGPT desktop outbound identity

Originally inspected on 2026-11-03 and refreshed on 2026-11-16
(Asia/Shanghai), from `/Applications/ChatGPT.app`.
The B-side default now uses the locally installed Codex Desktop profile.
`codex_exec` remains an explicit profile for callers that intentionally need the
noninteractive CLI identity.
This is an application-layer compatibility profile, not proof that the gateway
has an identical TLS stack or device identity.

## Reproducible evidence

Run `python3 scripts/inspect-chatgpt-desktop-identity.py`. It reads shipped bundle
metadata and initializes the bundled CLI in a temporary, credential-free home.
It does not create a conversation or send an inference request. No terminal
variables are passed, so the terminal component is `unknown`; a terminal-launched
process may report a different value. This is a controlled initialization probe,
not a capture of the running desktop's upstream traffic.

| Field | Observed value |
| --- | --- |
| Bundle identifier | `com.openai.codex` |
| App version / build | `26.1103.31037` / `25241` |
| Bundled engine | `0.167.0-alpha.3.2` |
| Originator / initialize client name | `Codex Desktop` |
| Platform from local initialize | `Mac OS 26.6.2; arm64` |
| Chromium metadata | `157.0.8103.39` |
| ASAR SHA-256 | `57602118a9890cb62ddfa750996e41440396d0aac2725b6837b5391305c78acf` |
| Native CLI SHA-256 | `a636331ed87605efc4cf84028ac2884a75363f9d10ead63cffb57ea0016d486e` |

The local initialization returned:

```text
Codex Desktop/0.167.0-alpha.3.2 (Mac OS 26.6.2; arm64) unknown (Codex Desktop; 26.1103.31037)
```

Static extraction of `app.asar` also established:

- `.vite/build/early-bootstrap-824-8c6252e0131a7fe3.js`, near character offset
  150900: exported desktop originator `Codex Desktop`.
- `.vite/build/early-bootstrap-747-42e8ff106714a2dd.js`, near offset
  4487300: app-server initialization uses the desktop client name and app version.
- `.vite/build/early-bootstrap-339-c6ef14271936603a.js`, near offset
  1081700: desktop HTTP helpers fill `originator` and `User-Agent` independently;
  nearby account helpers populate `ChatGPT-Account-Id` and `X-OpenAI-Fedramp`.
- The same module, near offset 508420, sets `OAI-Product-Sku: CODEX` in one
  dedicated request helper. This does **not** establish a universal inference header.
- `.vite/build/early-bootstrap-747-42e8ff106714a2dd.js`, near offset
  1659900: the local Work executor selects `codex_work_desktop` independently
  from the main `Codex Desktop` app-server identity.

Offsets above refer to decoded JavaScript characters, not Mach-O addresses.
REA/Ghidra initialization failed due to the local Swift SDK/compiler mismatch;
the usable evidence is ASAR source extraction, package metadata, native binary
identity, and the isolated executable probe. No successful native decompilation
is claimed.

## Implementation and field ownership

| Field | Gateway behavior |
| --- | --- |
| `User-Agent`, `originator` | Desktop default shared through canonical identity resolution; explicit CLI profiles remain supported |
| `version` | Engine version, never app release version; omitted on standard Desktop inference HTTP/WS and retained only where compact compatibility requires it |
| Final `(Codex Desktop; ...)` group | Preserve independent `clientInfo.version` during engine updates |
| CLI trailers | Update only native CLI trailers that repeat the previous engine version |
| Account/FedRAMP | Continue using selected credential account metadata |
| Installation/window IDs | Preserve per-account scoping in metadata; the standalone `x-codex-installation-id` header is removed from standard Desktop inference |
| `X-Client-Request-ID` | Now allowed on both HTTP paths and scoped by the existing account identity logic, matching WS behavior |
| Turn state, beta features and client metadata | Continue existing transport-specific handling; do not invent static values |

Explicit UA and engine version settings retain priority. The synchronized
version follows standalone stable Codex CLI releases and therefore applies only
to explicit CLI/TUI profiles. It must not rewrite the default Desktop profile:
the Desktop engine and app versions are a measured bundled pair. With no
explicit version override, the Desktop engine remains `0.167.0-alpha.3.2` and
the app version remains `26.1103.31037`. The default platform is the measured
Mac OS 26.6.2/arm64 Desktop environment.
Desktop app versions remain independent when the engine version changes.

See `openai-outbound-identity-audit.md` for the HTTP/WS and auxiliary-path audit.

The default change is source-only until this build is deployed. No production
settings, accounts, credentials or client application files were changed.

## Unverified layers

Chromium browser `sec-ch-ua`, native Rust transport behavior, TLS ClientHello,
HTTP/2 settings and header ordering are distinct surfaces. Chromium metadata is
not evidence that Codex inference uses Chromium's TLS fingerprint. This patch
does not add speculative browser headers. The native HTTP and Rustls WS TLS
profiles measured in the subsequent CLI capture are now implemented; see the
outbound audit for exact test coverage and remaining transport differences. Device cookies,
attestation, signed assertions and service-issued turn state are runtime material,
not constants that can be copied into a universal profile. Upstream acceptance,
quota changes and performance have not been measured by this offline inspection.

## Validation

The initial Desktop investigation and tests preceded the CLI default decision.
Current validation and known limits are recorded in
`openai-outbound-identity-audit.md`.

## Follow-up wire compatibility

The native HTTP/1 serializer now reproduces the observed title-case sorted header
shape. The WebSocket serializer reproduces the captured handshake order and
lowercase application headers, retaining required account-specific fields.
The local coder/websocket patch negotiates `client_max_window_bits` and actually
encodes windows 8..15. Tests cover all windows, context takeover, no-context
takeover, malformed server responses, raw upgrade bytes and compressed echo.
See `openai-outbound-identity-audit.md` for evidence and remaining differences.
Custom-CA HTTP/2 negotiation, SETTINGS, connection window and pseudo-header
order are now implemented and covered by actual wire tests. This is not a claim
of 100% equivalence: TLS records/resumption, dynamic identities and compression
bytes still differ or remain unverified.

## November 4 local refresh

The installed bundle is now `26.1029.61003` (build `24614`), with engine
`0.166.0-alpha.6`. The initial observations above remain historical evidence.
The shared fallback constants now use the newly observed engine/app versions;
explicit runtime settings still win. Fresh loopback HTTP/WS and TLS captures,
transport isolation changes and limitations are documented in
`openai-fingerprint-completion-20261104.md`.

## November 15 Desktop protocol and telemetry refresh

The canonical profile is now the installed Desktop identity shown above. A
request proven to originate from an official Desktop client removes fields
absent from the measured native inference request: `version`, underscore
`session_id`, `conversation_id`, underscore `thread_id`, standalone
`x-codex-installation-id`, `accept-language`, and `OAI-Product-Sku`. Internal
compatibility bridges are not native Desktop ingress. They retain the stable
session/conversation identity derived from `prompt_cache_key`, because deleting
that identity rotated the upstream cache namespace for B-to-P traffic. Compact
remains a unary compatibility endpoint and keeps its required version/session
fields.

`OAI-Product-Sku: CODEX` was found in an Electron ChatGPT request helper, while
the bundled native inference engine contains no matching string. It therefore
must not be generalized to `/responses` HTTP or WebSocket traffic.

Telemetry is deliberately separate from inference identity. The gateway does
not fabricate OpenTelemetry, Statsig, Sentry, RUM, or trace events and does not
relay them under an official client identity. The bundled native binary contains
OpenTelemetry/OTLP log, metric and trace exporters plus W3C trace-context
propagation. Real inbound metadata and transport trace context are preserved,
including `analytics_enabled`, `token_provider`,
guardian/MCP fields, `responses_api_metadata`, `traceparent`, `tracestate`, and
`baggage`. The gateway never creates missing trace headers or promotes metadata
values into headers. Gateway metrics remain local observability data.

## November 16 app-server wire capture and cache namespace repair

The bundled app-server was initialized with `clientInfo.name = Codex Desktop`,
`clientInfo.version = 26.1103.31037` and the Desktop originator override. A
credential-free loopback provider captured the actual request serializer. The
sanitized fixtures are:

- `backend/internal/pkg/openai/testdata/codex_desktop_appserver_http_20261116.json`
- `backend/internal/pkg/openai/testdata/codex_desktop_appserver_ws_20261116.json`

HTTP uses the measured Desktop UA/originator and header order. WS uses
`responses_websockets=2026-02-06`, lowercase application header names and offers
`permessage-deflate; client_max_window_bits`. Its lifecycle sends a
`response.create` prewarm with `generate=false`; the formal request then carries
the returned `previous_response_id` and only the input delta. The gateway now
enables this WS v2 lifecycle by default and sends an empty delta when the formal
request is identical to its prewarm. Prewarm handshake and frame metadata are
normalized to `request_kind=prewarm` and omit the formal-turn-only
`root_turn_id` and `turn_started_at_unix_ms` fields; the subsequent formal frame
retains its original `request_kind=turn` metadata. If an internal bridge omits
the handshake `x-codex-turn-metadata` header, the WS v2 path recovers the
original metadata from `client_metadata` before building the upgrade. In both
protocols `prompt_cache_key` equals the Desktop session ID. The standalone
`X-Codex-Installation-Id` header is absent; the installation ID belongs in
`client_metadata`.

Production usage analysis also found that B-to-P bridge requests carried their
Desktop session only in `client_metadata.session_id`,
`client_metadata.thread_id` or `prompt_cache_key`. When the bridge omitted the
`Session-Id` header, session-mode fingerprinting previously collapsed unrelated
conversations into the account-wide fallback namespace. HTTP, passthrough,
native WS and WS v2 now recover the original session from the body, preserve it
separately across account failover and WS-to-HTTP fallback, and derive a distinct
account-scoped namespace without hashing an already rewritten body twice.

An OTLP-enabled repeat capture confirmed the trace relationship rather than
only the static exporter capability. HTTP inference preserves the real
`traceparent` and an explicitly empty `tracestate` header; WS carries no trace
headers on the upgrade and puts the real traceparent in
`client_metadata.ws_request_header_traceparent`, omitting empty tracestate. The
same trace ID appeared in the local OTLP export. Desktop API-key provider mode
also calls `POST /v1/analytics/codex/turn-costs` with only `turn_ids`; sub2api now
serves an authenticated local `{"turns":[]}` response so the client does not
receive a 404 or associate a client turn with an unrelated OAuth account.

## November 15 transport closure

Fresh ClientHello captures from the bundled 0.167 native engine now back all
three implemented transport profiles: native HTTP, native WebSocket and
custom-CA HTTP. The main OAuth and SetupToken HTTP inference path now selects
`CodexHTTPProfile()` directly when neither SmartWS nor the outbound plugin owns
the request. Account connection tests use the same profile. This closes a real
gap where Desktop application headers could previously be sent over the generic
Go TLS/HTTP transport.

WebSocket handshakes do not copy `traceparent`, `tracestate` or `baggage` from
the caller. For native WS `response.create` frames, real inbound
`traceparent`/`tracestate` values are placed in the official
`client_metadata.ws_request_header_traceparent` and
`client_metadata.ws_request_header_tracestate` keys. `baggage` remains an HTTP
header-only propagation value. Trace context is therefore not a WS pool
compatibility key, so a pooled handshake is reusable across request spans.
This applies to pooled HTTP-to-WS forwarding, native WS ingress, and v2
passthrough, including every follow-up `response.create` on a reused socket.

The per-frame `client_metadata.x-codex-ws-stream-request-start-ms` value is
overwritten immediately before each socket write. The same rule applies to the
optional `generate=false` prewarm frame; neither prewarm nor a later turn reuses
the caller's stale timestamp.

Native 0.167 websocket upgrades build their response headers with no turn
state. OAuth/SetupToken Codex paths therefore remove `x-codex-turn-state` from
the upgrade and carry the account-owned value in each
`response.create.client_metadata` object. HTTP requests retain their native
header behavior. Unknown state is preserved only when ownership cannot be
proved; state proven to belong to another credential is removed.

OAuth HTTP and WebSocket requests now retain the official
`x-codex-routing-hint` generated from the final upstream model and effective
service tier. The hint remains a soft WebSocket dial affinity and does not make
an existing continuation socket incompatible. A request explicitly marked as
`x-codex-guardian: reviewer` omits the hint, matching the native client.

Real `x-openai-memgen-request` and
`x-responsesapi-include-timing-metrics` declarations are preserved across HTTP,
passthrough and WebSocket paths. The gateway does not invent either feature.
Codex OAuth requests preserve `prompt_cache_options` unless the selected model
has returned an explicit unsupported-field rejection. The observed
`gpt-6-sol` and `gpt-6.1-sol` models are stripped before dispatch so WebSocket
requests do not fail after an unreplayable frame write; HTTP retains its bounded
error-driven fallback for newly observed models. Generic compatibility providers
keep their existing unsupported-field cleanup. Each native WebSocket frame is
stamped immediately before write with
`client_metadata.x-codex-ws-stream-request-start-ms`, as the Desktop engine does.

The US Desktop locale policy defaults on. OAuth Codex metadata is normalized to
`America/New_York` and `en-US` unless an operator explicitly disables the
setting; the policy still does not synthesize timezone headers or alter user
content outside the structured environment context.

The bundled engine's unmodified device-auth bootstrap ClientHello was captured
on 2026-11-16 through a local CONNECT proxy with an isolated home and no
credentials. It matches the measured native HTTP profile and offers no ALPN.
With `SSL_CERT_FILE` configured, the same executable changes its ClientHello and
offers `h2,http/1.1`; a trusted local MITM then observed HTTP/2
`POST /api/accounts/deviceauth/usercode`, with wire-order application headers
`content-type`, `accept`, `content-length`, no User-Agent, and a JSON body whose
only field is `client_id`. The trusted-MITM evidence is instrumented behavior:
the custom CA changes the transport profile, so it does not prove unmodified
Desktop uses HTTP/2. Token exchange and refresh HTTP version, header order and
serialization remain unmeasured. OpenAI OAuth requests handled by an external
outbound plugin still use the plugin's own transport and require a separate
audit.
