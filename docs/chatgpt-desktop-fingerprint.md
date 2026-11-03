# ChatGPT desktop outbound identity

Inspected on 2026-11-03 (Asia/Shanghai), from `/Applications/ChatGPT.app`.
The B-side default uses the locally observed CLI (codex_exec) profile. The installed desktop's
Codex app-server identity remains available as an explicit custom UA.
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
| App version / build | `26.1027.51404` / `23880` |
| Bundled CLI engine | `0.166.0-alpha.5.1` |
| Originator / initialize client name | `Codex Desktop` |
| Platform from local initialize | `Mac OS 26.6.2; arm64` |
| Chromium metadata | `156.0.8078.79` |
| ASAR SHA-256 | `52e0a65f9fea51f4287bbd98c63343761f9424517bb793652bb554764bc0bcc8` |
| Native CLI SHA-256 | `58a91f6ea19bea477ae3a2ec48d619f0039afe75d3476260747e11d201fa7cea` |

The local initialization returned:

```text
Codex Desktop/0.166.0-alpha.5.1 (Mac OS 26.6.2; arm64) unknown (Codex Desktop; 26.1027.51404)
```

Static extraction of `app.asar` also established:

- `.vite/build/early-bootstrap-339-9edb60e5d8b0547e.js`, character offset
  136719: exported desktop originator `Codex Desktop`.
- `.vite/build/early-bootstrap-439-a0f25ba32c465de1.js`, near offset
  4153473: app-server initialization uses the desktop client name and app version.
- `.vite/build/early-bootstrap-339-9edb60e5d8b0547e.js`, near offset
  156541: desktop HTTP helpers fill `originator` and `User-Agent` independently;
  nearby account helpers populate `ChatGPT-Account-Id` and `X-OpenAI-Fedramp`.
- The same module, near offset 737797, sets `OAI-Product-Sku: CODEX` in one
  dedicated request helper. This does **not** establish a universal inference header.
- `.vite/build/early-bootstrap-439-a0f25ba32c465de1.js`, near offset
  1418600: browser surface headers and device-cookie/token handling are conditional.

Offsets above refer to decoded JavaScript characters, not Mach-O addresses.
REA/Ghidra initialization failed due to the local Swift SDK/compiler mismatch;
the usable evidence is ASAR source extraction, package metadata, native binary
identity, and the isolated executable probe. No successful native decompilation
is claimed.

## Implementation and field ownership

| Field | Gateway behavior |
| --- | --- |
| `User-Agent`, `originator` | CLI default shared through canonical identity resolution; Desktop is an explicit override |
| `version` | Engine version; not the app release version; omitted on auth requests as before |
| Final `(Codex Desktop; ...)` group | Preserve independent `clientInfo.version` during engine updates |
| CLI trailers | Update only native CLI trailers that repeat the previous engine version |
| Account/FedRAMP | Continue using selected credential account metadata |
| Installation/window IDs | Preserve existing per-account scoping and explicit convergence settings |
| `X-Client-Request-ID` | Now allowed on both HTTP paths and scoped by the existing account identity logic, matching WS behavior |
| Turn state, beta features and client metadata | Continue existing transport-specific handling; do not invent static values |

Explicit UA and engine version settings retain priority. The engine version
setting, followed by the synchronized version, controls the UA prefix. With no
settings or synchronized version, the fallback engine is `0.166.0-alpha.5.1`,
observed in the installed bundle; it is not a claim about the latest stable CLI.
The default platform is the measured Mac OS 26.6.2/arm64 exec environment.
To select the observed Desktop profile, enter the full UA shown above and
explicitly select its engine version.
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
