# Codex conversation isolation repair

## Scope

Local branch `codex/937sub2b-neutral-markers-20260914`, based on
`eeff23e50f9fab38dd769746a1a59e969e134274`. This extends the generated-marker
cleanup and outbound audit. No production restart, deployment, account-setting
change, database migration, commit, or push was performed.

## Production changes

- Route ordinary Responses, raw HTTP passthrough, Chat Completions conversion,
  Messages conversion, native WS, and WS passthrough through one request-level
  conversation identity resolver.
- Preserve the existing account installation ID/seed. For `session` and `full`,
  use credential/API-key-scoped session and thread IDs instead of the legacy
  account-wide session. Explicit body metadata is authoritative over conflicting
  headers; both flat and embedded metadata are recognized.
- Preserve stable IDs across retries for identified conversations. Without a
  conversation ID, keep a random fallback within the inbound request/WS context,
  not across unrelated HTTP requests. A generic Responses prompt-cache key is
  not treated as proof that conversational state can be shared.
- Preserve the Messages bridge's existing conversation binding (source metadata
  or digest lineage) when no explicit session exists. The first implementation
  broke its continuation tests; this compatibility path was restored and tested.
  Digest-based inference remains ambiguous when clients provide identical
  history/cache anchors without an explicit session; this patch does not claim
  to solve that ambiguity. Clients should supply distinct conversation IDs.
- Namespace locally cached Messages turn-state by the effective conversation
  and thread, in addition to account/API key/cache key. Separate explicit chats
  sharing a cache prefix cannot read each other's cached state.
- Restrict task-guard/bridge recognition to text in developer messages. A user
  quotation, assistant quotation, image URL, or tool argument is not a bridge
  signal. Continue accepting legacy markers and JSON Unicode escapes.
- Inspect raw developer text fields without decoding the complete input array.
  This avoids allocating decoded image/tool payloads just to find a guard.
- Update English/Chinese settings descriptions without changing enum values or
  automatically changing stored account settings. The legacy pure fingerprint
  helper remains for deterministic account-test probes; production forwarding
  goes through the isolated resolver.

## Preserved behavior and deliberate exclusions

The patch does not change account concurrency limits, scheduler admission,
IPv6 relay, TLS implementation, UA profiles, quota injection, quota query
headers, language headers, or arbitrary user metadata/text. Existing connection
pool compatibility/idle-replacement logic remains in place. These are not all
interchangeable endpoint contracts, so no unverified desktop-header bundle was
applied globally.

Explicit cache keys retain the existing credential/tenant namespace behavior;
keys tied to a supplied body session follow that isolated session. User images,
tools, arguments, instructions, and input are not scrubbed for branding. API-key
transport and explicit fingerprint `off`/`device` modes retain their behavior.

## Verification

- Regression tests cover OAuth/PAT; API-key exclusions; off/device/session/full;
  tenant, account, and conversation separation; retry stability; header/body and
  map/raw parity; anonymous contexts; embedded metadata; image preservation;
  cache-key behavior; and turn-state separation.
- Real service forwarding is tested with local fake HTTP/WS upstreams in all
  four fingerprint modes. Existing Messages continuation tests remain enabled.
- Existing assertions that expected the old account-wide session were updated
  to expect credential/API-key-scoped body-session/thread values while retaining
  all header/body/cache/turn-state equality checks.
- Two existing Messages test fixtures mutated global Gin mode from parallel
  tests. They now run serially so targeted race detection can check the request
  logic without that fixture race.
- Go vet and backend build passed locally. `golangci-lint` is unavailable.
- Four changed locale modules were imported successfully and their key sets
  compared against HEAD: no keys were added or removed. Full frontend lint/build
  could not run: the local Volta pnpm launcher fails; bundled pnpm rejects the
  frozen lockfile/overrides combination. The lockfile was not changed.

Final command results:

- `go test ./internal/service -count=1 -timeout=180s`: passed, 102.150s.
- Targeted `go test -race ./internal/service` covering conversation isolation,
  transport parity, neutral markers, and Messages continuation: passed, 2.844s.
- `go vet ./internal/service`: passed.
- `go build -o /tmp/sub2api-conversation-isolation-20260915-server ./cmd/server`:
  passed.
- `git diff --check`: passed.
- Four changed locale modules: syntax and unchanged key sets verified; full
  frontend build remains unverified for the tooling reasons above.

## Follow-up deep repair

A second local review reproduced additional boundary failures before applying
fixes. The review was limited to compatibility and state correctness; it did not
change upstream rate-limit policy or attempt to impersonate a first-party user.

### Findings and fixes

1. **Identity aliases disagreed.** Account scoping accepted hyphenated fields,
   but conversation resolution and subsequent metadata rewriting recognized only
   some of them. Resolution now accepts canonical and legacy aliases in body and
   embedded header metadata, with explicit flat canonical fields taking priority.
   Rewriting synchronizes recognized aliases already present; it does not add
   extra alias fields or remove unrelated metadata. Session-derived cache keys
   also recognize alias/embedded source sessions without a second hash.
2. **WS continuation could revert to handshake identity.** When the first frame
   supplied its own session/thread and a later frame omitted them, the later frame
   used handshake IDs or a new fallback. A connection-local binding now preserves
   the accepted conversation. Bindings are checked against account, credential
   namespace, API key, and mode; explicit conversation changes remain explicit.
   Failed normalization and rejected policy frames do not advance this binding.
3. **Native WS overwrote frame metadata with handshake metadata.** Codex header
   metadata is now a fallback merged after frame identity resolution. Explicit
   frame metadata is retained. Other header metadata fields are retained when
   the fallback is used. API-key native WS also preserves explicitly supplied
   embedded frame metadata rather than overwriting it with the handshake value.
4. **Guard insertion lost opaque input fields.** The typed input round trip could
   discard custom tool input/extension fields and reasoning summary/status, and
   convert structured tool output into a string. Guard insertion now retains raw
   JSON items, including integer precision and multimodal output. Developer
   messages with an omitted `type` remain ahead of the inserted guard. This is a
   guarantee about the guard helper, not every transformation in the gateway.

### Additional verification

- Pre-fix fixtures failed for alias resolution, WS omitted-field continuation,
  and preservation of an existing developer prefix.
- New regressions exercise canonical/alias precedence, device-mode exclusions,
  account/credential/tenant changes, malformed frames, custom tools, structured
  image outputs, and large integer extension fields.
- A real local downstream WS exercises both native pooled and passthrough service
  entrypoints with fake upstreams. Two accepted turns retain the first frame's
  session/thread; native mode dials exactly one upstream connection. A subsequent
  policy-denied passthrough frame emits an error, closes under the existing policy,
  never reaches upstream, and does not replace the accepted conversation binding.
- No real account tokens or upstream requests are used by these fixtures.

Follow-up validation:

- Targeted race suite including both WS entrypoints and policy denial: passed,
  2.473s. No race was reported.
- `go vet ./internal/service ./internal/pkg/apicompat`: passed.
- Backend server build: passed, output
  `/tmp/sub2api-deep-repair-20260915-server`.
- Targeted coverage: request resolver 94.3%; WS inheritance 86.7%; metadata
  alias synchronization 100%; raw guard insertion 84.8%; map guard insertion
  95.2%. These are per-function results, not whole-package coverage.
- `git diff --check`: passed. No frontend files or lockfiles were changed in
  this follow-up; the earlier frontend build limitation remains.
- `go test ./internal/service ./internal/pkg/apicompat -count=1 -timeout=240s`:
  passed; service 112.924s, protocol package 0.744s.

## Deployment considerations

This is not deployed. The base contains other Lite changes beyond the observed
live revision, so a release must review the full selected diff. Scope the rollout
to new connections and drain existing streams; do not stop the serving container.
Session/full IDs intentionally change from the unsafe account-wide scheme, so
expect new WS connections and potentially temporary cold caches for affected
conversations. Keep the prior image available for rollback; no data migration
is needed. Avoid mixed-version routing within a continuing conversation.

No successful upstream error-rate or answer-quality A/B evaluation is claimed.
Correct state isolation is not evidence that the service is indistinguishable
from a first-party human client, nor a guarantee against 429/5xx errors.
