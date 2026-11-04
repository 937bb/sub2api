# Account fingerprint diagnostics

Open **Accounts → row actions → View fingerprint** for OpenAI OAuth and setup-token accounts, including credential shadows.

`GET /api/v1/admin/accounts/:id/fingerprint` is read-only, uses the existing administrator authentication and compliance middleware, and returns `Cache-Control: no-store`. Scoped administrator API keys are not granted this new route. Unsupported account types return 400; missing accounts return 404.

The service loads the current account and resolves shadow credential ownership using the same resolver as the gateway. The displayed installation ID comes directly from `codexAccountInstallationID`, which also supplies outbound HTTP headers and WebSocket metadata. Source precedence is explicit device override, persistent seed, upstream account metadata, then setup-token identity. Missing stable identity is reported without generating new state.

Only the diagnostic allowlist is serialized. Credentials, fingerprint seeds and upstream account identifiers are not returned. The selected row supplies convergence policy and account User-Agent/WS configuration; the credential owner supplies the device. The view flags a configured convergence mode that lacks its required seed.

HTTP and WS process settings can differ. Account User-Agent overrides and WS preferences are labeled as configuration, not packet measurements. TLS handshakes, final transport headers, and request-dependent session/thread IDs cannot be inferred from this view. The account WS preference uses the existing resolver with two different defaults to distinguish an explicit override from inherited runtime configuration.

The modal fetches on opening and refresh, clears stale content, cancels on account changes/close/unmount, and ignores late results from aborted requests. The accounts page pauses automatic refresh while it is open.

Validation covers identity equality with outbound headers and metadata, source precedence, credential redaction, shadow inheritance and invalid parents, unsupported/missing accounts, ID validation and cache control, menu availability, late responses, close/reopen and retry.

## Runtime and transport details (2026-11-04)

The diagnostic allowlist now includes the serving admin process's canonical
User-Agent, originator, engine version, identity enforcement, custom-CA branch,
force-HTTP, WS router configuration and strict conversation policy. It does not
claim to inspect a separate WS process. When identity enforcement is disabled,
incoming request headers can change the effective identity.

Expandable native HTTP, custom-CA HTTP/H2 and WS templates expose TLS versions,
cipher suites, extensions, groups, key shares, signature algorithms, ALPN,
header ordering, H2 SETTINGS and connection window, and WS compression policy.
The hash is SHA256 of configuration, not JA3/JA4 or a live capture. Baseline
samples are dated 2026-11-03; local bundle identity was rechecked 2026-11-04 and
remains engine 0.166.0-alpha.5.1. Dynamic credentials and session values are not
returned. Capture status is explicitly `not_captured`.
