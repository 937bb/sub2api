# OpenAI outbound identity audit

Reviewed on 2026-11-03. The release is based on production commit
`2fa2b14b86b59d4c65737a453129b6fdf6db1ad5`; the original investigation
workspace is retained separately. Deployment results are recorded independently.

## Identity contract

The default is the locally observed `codex_exec` identity:

```text
codex_exec/0.166.0-alpha.5.1 (Mac OS 26.6.2; arm64) unknown (codex_exec; 0.166.0-alpha.5.1)
```

The fallback version and platform come from the installed bundled CLI, not the
latest stable release. Both engine-owned version fields update together.

Runtime settings choose the engine version. An explicit account or global UA
can select another client/platform, including Desktop. Its leading engine version
and inference `version` header share one source. Independent Desktop, extension
and SDK app versions are preserved. Authentication and quota requests share the
UA/originator pair but do not synthesize an inference version header.

With identity enforcement disabled, a valid incoming client/engine is retained.
Missing, invalid or obsolete versions are repaired together with the UA prefix;
a contradictory standalone `version` header cannot survive. Case aliases are
collapsed when final identity headers are written. Missing `originator` remains
a deliberate opt-out for compatibility bridges.

## Audited paths

| Surface | Final identity behavior | Protocol-specific behavior |
| --- | --- | --- |
| Responses HTTP and SSE | Canonical resolver; explicit account UA supported | Legacy `responses=experimental` removed; unrelated beta tokens retained |
| Compact HTTP and passthrough | Same resolver and account scoping | JSON negotiation preserved |
| Chat Completions and Messages bridges | Codex builders; Messages restoration now preserves account UA | Preserve body/session translation; restore identity only on Codex upstream |
| Responses WS v1/v2, pooled and passthrough | Shared WS header builder | Each handshake retains its own Beta value |
| WS retries, reconnects and prewarm | Same builder and pool compatibility | UA, originator, version, protocol Beta, account/FedRAMP, URL and proxy constrain reuse |
| Live setup and sideband | Same resolver with account UA | `quicksilver=v2`; no Responses Beta |
| OAuth token exchange/refresh and PAT | Shared canonical auth pair | No inference version header |
| Quota query/reset | Removed hardcoded Desktop originator; configured UA/account override | Keep endpoint `codex-1` and language; remove synthesized browser fetch hints |
| Codex model manifest | UA and Version share engine version | Preserve independent `client_version` content-negotiation query |
| Account/capability probes | Finalize identity, rather than only fill missing fields | No generic helper overwrites transport Beta |
| Alpha search and image Responses | Existing builders plus final identity cleanup | Image forwarding no longer reintroduces legacy HTTP Beta after the builder |
| Plugin account identity | Canonical resolver with account UA | Plugin chooses transport features |

`X-Client-Request-ID`, `Session-Id`, and `Thread-Id` now pass through both HTTP allowlists and existing account
scoping, as it already did for WS. Installation, window, session, thread and turn
identifiers retain account isolation. The account installation policy below fixes
one device per credential; additional session/thread convergence remains opt-in.

Ordinary API-key/custom-provider forwarding retains explicit header overrides.
The existing synthetic capability probes and Codex manifest adapter continue to
use their Codex profiles. Browser-only Excel/BPS calls and external image
asset downloads retain their own endpoint contracts; a Codex inference identity
is not a universal replacement for every request to an OpenAI-owned domain.

## Transport boundaries

For the observed Mac exec UA, inference HTTPS, auth and quota HTTPS use the
measured native HTTP TLS 1.2 profile, without ALPN or automatic gzip headers.
Responses WSS uses the independently observed Rustls TLS 1.3 profile with
X25519MLKEM768/X25519 key shares and shuffled extensions, also without ALPN.
These use HTTP/1.1, including when the generic HTTP/2 setting is enabled.
Explicit alternate client/platform UAs retain their generic transport; explicit
HTTP TLS profile overrides retain priority. Plain HTTP has no TLS fingerprint
and keeps the existing proxy transport.

Profiles are wired through direct, HTTP CONNECT, TLS-wrapped HTTPS CONNECT and
SOCKS dialing. CONNECT and SOCKS operations respect cancellation. Fingerprint
cache keys now include all profile fields. HTTPS-only transports reject redirects to plaintext instead of bypassing
the configured proxy. Browser defaults installed by the quota client's Firefox
preset are removed on the local CLI branch; browser-only calls retain them.

The capture used a temporary CODEX_HOME, a dummy API key and a loopback provider.
No production account was used. Supplying a custom CA changed the CLI HTTP
backend to Rustls with ALPN h2/http1. Both `CODEX_CA_CERTIFICATE` and
`SSL_CERT_FILE` were tested independently and select that backend. The gateway
now selects its separate Rustls profile for either variable, loads the supplied
CA certificates, negotiates H2 or HTTP/1.1, and matches the observed ordered
SETTINGS (`2=0, 4=2097152, 5=16384, 6=16384`), connection WINDOW_UPDATE
(`5177345`) and pseudo-header order (`:method, :scheme, :authority, :path`).
Real TLS/H2 frame tests verify negotiation and serialization; application header
order follows the observed CLI request, with extra required OAuth fields retained.
Native HTTP ClientHello was observed before certificate rejection; successful
native HTTPS application traffic was not captured from the official CLI.
Gateway verified HTTPS and compressed WSS echo tests use a test-local trusted CA.

Remaining differences:

- Native HTTP/1 header casing/order matches the captured loopback request shape
  (Host, User-Agent, Content-Length, then sorted headers). WS upgrade header
  casing/order matches its separate capture, including lowercase application
  headers. Unknown/required OAuth headers are retained deterministically; a
  custom-provider capture is not evidence for removing OAuth-specific headers.
- The opt-in local coder/websocket patch now offers
  `permessage-deflate; client_max_window_bits` and implements negotiated windows
  8..15, including context takeover. Raw upgrade and compressed echo tests pass.
  Compression byte streams, framing and thresholds are not claimed identical
  to the CLI's Rust implementation.
- TLS record/session/resumption behavior is not claimed byte-for-byte identical.
- OAuth-specific inference version requirements remain; an API-key custom-provider
  capture that omits `version` does not prove OAuth should omit it.
- Device attestation, cookies and dynamic session/installation IDs are not copied.

The WS pool now rejects connections whose recorded handshake identity differs
from the next request, even with fingerprint convergence off. Existing tests
that insert synthetic connected sockets were updated to record their fixture
URL, matching the production dial path. This preserves strict matching instead
of treating missing connection metadata as a wildcard.

## Validation

- Cross-protocol matrix: OAuth and setup-token accounts; CLI default and explicit
  Desktop; HTTP, compact, passthrough, passthrough compact, WS v1 and WS v2.
- Regressions: version mismatch/invalid/missing engine; case aliases; auth headers;
  protocol Beta preservation; quota profile; Live account UA.
- Local wire test: actual HTTP serialization and WS upgrade without credentials
  or external upstream calls.
- WS pool tests cover identity/protocol/account/URL changes, unchanged reuse,
  existing queue, retry, prewarm and continuation behavior.
- With `-tags=unit`, the OpenAI and TLS packages pass. The supplementary service
  test binary could not link because disk space was exhausted; no success is
  claimed for that run. The standard full suite, including the new transport
  routing tests, passed.
- Final transport-inclusive `go test -p 1 ./...` passes across the backend.
- `go vet` on the changed packages and the server build pass; locale completeness passes (3 tests).
- `golangci-lint` was attempted but is not installed on this host.

Upstream acceptance, production stability and transport-fingerprint equivalence
are not established by these local tests. The prior investigation saw an
intermittent model-cache concurrency failure; it did not recur in this final
full-suite run, and its earlier cause was not established.

## Transport validation (2026-11-03)

Sanitized native captures are stored under
`backend/internal/pkg/tlsfingerprint/testdata/codex_*_20261103.json`.
`TestCodexClientHelloMatchesLocalCapture` checks serialized cipher suites,
extension IDs/order policy, curves, signature algorithms, supported versions,
key-share groups/lengths and the absence of ALPN. Additional tests cover verified
HTTPS, rejection of untrusted certificates, TLS to HTTPS proxies, CONNECT
cancellation, compressed WSS echo, transport cache separation, and stripping inherited browser navigation headers.

The first parallel full-suite attempt exhausted local disk space and two SSE
spool tests failed with `no space left on device`. After removing this task's
stale build directories, the serial full suite passed. The package manager's
script launcher rejected existing lockfile/node_modules state, so the unchanged
locale test was run successfully using the installed Vitest entry point directly.
No lockfile or dependencies were modified.

## Encrypted-content recovery (issue 8301 follow-up)

The sanitizer and rejected-content lineage now cover reasoning/compaction and
assistant `content[]` encrypted fragments (including amsg messages). Plaintext,
tool arguments, call IDs, tool results and JSON integer precision are retained;
arbitrary nested user/tool JSON is not recursively edited. Changed assistant
messages lose their provider-owned message ID so rejected ciphertext cannot be
rehydrated from it. Empty encrypted-only messages are removed.

Request affinity snapshots response/sticky ownership before scheduling rewrites
that binding. Known cross-account attempts strip encrypted fields; unknown
ownership preserves the first attempt and relies on explicit rejection. Parent
and shadow identity are treated as the same owner for first-attempt tracking.
Encrypted requests disable sticky escape and weighted sticky selection, while
normal eligibility and account-failure failover remain available.

Standard HTTP, passthrough and WS-to-HTTP bridge retry once on the same account
after a matching 400, including the reported rs verification text without an
error code. Native WS can retry a self-contained turn before any client output
on a fresh connection to the same account. Already-visible turns and
connection-bound continuations retain their existing delivery/recovery contract;
rejected-fragment lineage still cleans subsequent turns. Unrecovered content
errors and delivered WS failed terminals are excluded from account health/error
rate statistics, and cannot trigger configured HTTP 400 pool failover.

These are locally reproduced fixes. Production incidence/root cause and recovery
rates still require post-deployment observation. Invalid-fragment lineage remains
process-local; shared Redis sticky/response mappings do not make digest lineage
shared between replicas. Purely encrypted context cannot be reconstructed after
removal, so recovery can lose that portion of history. This does not decrypt or
translate another account's ciphertext.

The additional local WebSocket dependency is under
`backend/third_party/coder-websocket` with the original license and patch notes.
Docker build stages copy the replacement module before downloading dependencies.

### Follow-up verification

- Backend full suite: `go test -p 1 ./...` passed (service package 137.166 s).
- Final targeted tests after the last small changes passed (service 3.171 s).
- Changed-package `go vet` and `go build -p 1 ./cmd/server` passed.
- Local websocket module full suite and window-negotiation race tests passed.
- Raw HTTP/1 tests verify captured header order and connection reuse. TLS tests
  exercise the ordered transport with a trusted test CA and reject untrusted CAs.
- Raw WS tests verify header spelling/order, extension offer, compressed echo,
  immutable caller headers, and explicit proxy routing for plain WS.
- Recovery regressions cover exact rs error text, amsg fragments, WS envelopes,
  selective replay lineage, unchanged tool payloads and large integers, owner
  snapshots, shadow-parent identity, weighted/concurrency sticky protection,
  bounded HTTP/WS recovery, and exclusion from account health statistics.

No production traffic, deployment or commit was performed for this validation.

## Production baseline integration

The production branch deliberately removed the old harvest/ticket subsystem.
The release does not reintroduce it. Existing per-request proxy rotation,
connection-route affinity, request tracing, TUI normalization and dedicated-image
billing are preserved. The new pool identity/URL checks extend proxy-route checks.

The upgraded runtime-only vue-i18n test configuration requires compiled message
functions. The SubscriptionPlanCard test fixture now supplies those functions;
its validity-period and layout assertions are unchanged.

Custom-CA transport tests cover each CA variable independently, H2 negotiation,
HTTP/1 fallback, connection reuse, actual SETTINGS/window/pseudo-header bytes,
certificate trust and serialized ClientHello. H2 compression/framing/record and
session resumption behavior remain unverified beyond these assertions. Changing
CA files or environment requires restarting the affected runtime connections.

### Release validation on the production baseline

- Full backend suite and full `go vet ./...` passed on Linux with Go 1.27.3.
- Frontend: 384 files / 3099 tests passed; typecheck and production Vite build passed.
- Local websocket fork full suite and window-negotiation race tests passed.
- No database migration files changed relative to production `2fa2b14b8`.
- Production build, rollout and live smoke results are recorded in the deployment report.

## Follow-up: custom-CA wire casing and receive windows

The local req v3.59.0 patch makes HTTP/1 field-name lowercasing opt-in for the
Codex custom-CA backend, including serializer-owned host and content-length.
Native HTTP and WS retain their independently measured casing. A sanitized
header-name fixture from the local CLI now verifies the entire HTTP/1 order,
framing, values, caller immutability and two requests on one verified TLS
connection. The H2 test checks application headers as well as pseudo-headers.

The req HTTP/2 receive counter previously initialized streams at 4 MiB even
when its custom SETTINGS advertised 2 MiB. It now uses the advertised window
(and the RFC default when a custom SETTINGS frame omits the window). A 1 KiB
window streaming test reproduces the old stall after exactly 1024 response
bytes; with the patch, both complete responses pass. Removing either patch
makes its corresponding wire regression fail.

`backend/third_party/req/SUB2API_PATCH.md` documents the two changes. The
verified upstream module and license are retained; dependency versions remain
unchanged. Full TLS record/session resumption and WS compression-byte equality
remain unverified, so this is not a claim of complete client equivalence.

Follow-up release checks passed: full backend tests and vet, the local req full
suite and targeted race checks, the websocket full suite, and frontend 384 test
files / 3099 tests, typecheck and production build. The first frontend run
reported animation callbacks after environment teardown; KeyUsageView's fixture
now owns and clears fake timers before restoring globals. Its assertions and
production UI code are unchanged. Production rollout is recorded separately.


## Account installation identity (2026-11-04)

Each usable OpenAI OAuth credential now resolves a single installation identity
independent of downstream API keys, client installation headers, conversation
IDs and refreshable access tokens. An explicit account device ID takes priority;
existing persistent fingerprint seeds retain the previous installation UUID.
Otherwise the durable ChatGPT account/user namespace deterministically derives
one installation UUID across processes and restarts. No identity is fabricated
from a deployment-local row ID when stable account information is absent.

Both installation aliases in existing client metadata and embedded turn metadata
use this identity. HTTP and WS builders add the installation header even for SDK
requests without one, collapsing header case aliases. Payloads without client
metadata keep their existing shape, including compact requests. The credential
owner supplies the device for shadow accounts even when their local row opts into
session convergence. Existing explicit session/full convergence remains opt-in;
ordinary sessions, threads, turns and request IDs retain separate scopes.

The shared scheduler projection preserves device, account/user and UA inputs.
OpenAI HTTP/TLS pools enforce account isolation even when the global pool setting
is proxy-only. WS pool compatibility includes the resolved account installation
when session convergence is off, so an edited device cannot reuse an old
handshake. The TLS/HTTP client protocol profile stays coherent with the selected
client family; TLS randoms, cryptographic keys and request IDs are not frozen.

This requires no schema migration or bulk account update. The derived fallback
is stable while its upstream account namespace is unchanged; changing explicit
device/seed/account ownership deliberately changes the device. Accounts missing
all stable identity inputs must be repaired rather than sharing a fabricated
constant. Auth endpoints keep their own header contract instead of receiving
inference-only installation metadata.
