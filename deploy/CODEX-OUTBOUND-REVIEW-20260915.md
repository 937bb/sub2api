# Codex outbound review and source synchronization

## Scope and release state

Source version: `0.2.4.8`, on `937sub2b`. This update incorporates the previously
validated `0.2.4.7` Pacific environment and conversation-isolation changes, and
preserves all six additional commits through `175cdb50` from
`937bb/937sub2api`. Both repositories can advance without a force push.

This review does not perform a production rollout, restart containers, change
databases, enable WS, or change C-branch code. The previously deployed B binary
was `0.2.4.7`; source publication must not be presented as a new deployment.

## Changes and evidence

- Recognized optional OAuth/PAT environment metadata uses
  `America/Los_Angeles`, `en-US`, and `US`; quota queries use `oai-language:
  en-US`. The IANA timezone handles PST/PDT without a hard-coded UTC offset.
- Newly generated image/task compatibility delimiters and the reserved Python
  tool alias no longer contain the proxy product name. Old delimiters remain
  accepted for historical conversations. Model-list descriptions now use
  neutral wording while retaining actual provider names and capabilities.
- The header boundary runs after final overrides on regular HTTP, passthrough,
  and WS builders, as well as applicable model-list, Live, Alpha search, and
  account-test paths. A case-insensitive `Sub2API` product token in User-Agent
  becomes `api-client`. No first-party attestation is synthesized.
- Messages compatibility intentionally omits `originator` and `OpenAI-Beta`.
  The cleanup retains this behavior, including when an inbound or configured
  mixed-case proxy UA is present. API-key and other-provider requests retain
  their header policy.
- A regression fixture reproduced numeric corruption in both raw metadata and
  JSON-encoded turn headers: `9007199254740993` became `9007199254740992`.
  Account scoping and fingerprint merging now use the existing strict
  `UseNumber` decoder. Large integers and high-precision decimals retain their
  exact JSON values across the combined environment/identity rewrite.
- Conversations retain account/API-key separation and distinct session/thread
  identity; shared prompt-cache prefixes do not justify merging unrelated
  conversational state. Remote heartbeat/retry changes preserve pre-output
  failover and prevent replay after observed output or usage.
- The imported WS recovery/reuse regression now supplies an explicit session
  ID for both requests. Its old anonymous fixture depended on account-wide
  conversation convergence. The recovered same-conversation connection still
  reuses successfully; the isolation policy was not relaxed to pass the test.

## Preserved contracts and limits

User messages, instructions, images, tools, tool outputs, and paths may contain
project names or timezone text. They are not subjected to a global replacement.
Unknown metadata and opaque upstream routing/state fields remain functional.
Required credentials, tool identifiers, and session relationships remain.

Internal `sub2api:*` hash salts, Go import paths, repository attribution,
application logs, and billing timezones are not outbound plaintext branding.
Renaming hash salts would unnecessarily invalidate stable IDs and caches.
The local Live error explaining that no DeviceCheck provider exists remains
truthful; this review does not fabricate a provider or an attestation.

Quota-extension behavior, concurrency, and IPv6 transport are unchanged by
this review. Existing synthetic quota tool history is still request content;
neutral marker names do not change its provenance or prove its effectiveness.
Go transports are not the Codex desktop network stack, and account UA profiles
are configured metadata rather than evidence of a physical client. No claim is
made that these changes eliminate identification, 429/5xx, or quality problems.

## Validation

All regression fixtures use dummy credentials and local mock upstreams. The
numeric preservation fixture failed on all four pre-fix paths and passed after
the decoder change. Environment/metadata/header and model-manifest focused tests
passed, as did `go vet ./internal/service ./internal/handler` and a server build.
Final validation:

- `go test ./internal/service -count=1 -timeout=300s`: passed, 139.071s.
- `go test ./internal/handler ./internal/pkg/apicompat -count=1 -timeout=300s`:
  both packages passed in the combined initial run, 38.903s and 1.038s.
- Focused WS recovery/reuse and metadata precision regression: passed, 1.366s.
- `go vet ./internal/service ./internal/handler`: passed on the final source.
- `go build -ldflags='-s -w' -o /tmp/sub2api-codex0248-server ./cmd/server`:
  passed on the final source.
- Formatting and `git diff --check`: passed.

The first combined test run exhausted local disk space while linking the
service test binary. Removing only this task's rebuildable server binary and
running the service suite separately resolved that build failure. The next
run identified the imported anonymous WS reuse fixture described above; the
corrected full suite then passed. No production files were removed.

Local `golangci-lint` is unavailable; it is not reported as passed. Previously
validated locale changes preserve all translation keys; no new frontend
behavior is introduced by this review. Full frontend build/typecheck remains
unverified due to the previously recorded local dependency tooling issue.
