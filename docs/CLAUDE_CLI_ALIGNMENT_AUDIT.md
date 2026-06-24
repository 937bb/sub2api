# Claude CLI Alignment Audit

Date: 2026-06-17
Latest recheck: 2026-06-18

This audit compares the local Claude Code CLI installation, the public
`huangserva/claude-code-cli` source mirror, and the current Sub2API Claude
gateway behavior. The goal is to keep Sub2API compatible with official Claude
Code traffic while avoiding mixed or contradictory client traits.

## Sources Reviewed

| Source | Evidence |
| --- | --- |
| Public source mirror | `C:\Users\Administrator\AppData\Local\Temp\claude-code-cli-src`, `origin/master` at `290fdc9` |
| Local JS package | `claude --version` currently reports `2.1.112`; the readable Volta package image contains bundled JS for that wrapper version |
| Local native package | A separate Volta Node package image contains `@anthropic-ai/claude-code` 2.1.181 with `bin/claude.exe` |
| Local captured behavior | Current local `--print` network traffic sends `claude-cli/2.1.181 (external, sdk-cli)` for `/v1/messages`; earlier 2.1.112 package-path capture also used `sdk-cli` |
| Sub2API implementation | `backend/internal/pkg/claude`, `backend/internal/service`, `backend/internal/handler`, and `docs/SUB_CLAUDE_USAGE.md` |

2026-06-18 recheck: `claude --version` still reports
`2.1.112 (Claude Code)`, but a live mock-endpoint capture of `claude --print`
sent `claude-cli/2.1.181 (external, sdk-cli)` with Stainless package `0.94.0`
and runtime `v24.3.0`. The public `huangserva/claude-code-cli` mirror still
resolves `origin/master` and `HEAD` to `290fdc9`, and the configured Sub2API
upstream still resolves `origin/main` and `HEAD` to `4a5665d`.

Version equality is intentionally not used as a compatibility requirement. Claude
Code can switch package/native versions, so the useful signal is the stable
request shape: user-agent family, entrypoint, auth header type, beta order,
system attribution, session identity, and file-route behavior. Runtime OS/arch
headers are also not a current gap; they should stay internally consistent with
the fingerprint being sent rather than be treated as a global account-pool rule.

## Local Live Capture

The current local `claude --print` command was run against a local mock endpoint
with an isolated `CLAUDE_CONFIG_DIR`. The command completed successfully and
sent one `POST /v1/messages?beta=true` request with these relevant traits:

| Trait | Captured value |
| --- | --- |
| User agent | `claude-cli/2.1.181 (external, sdk-cli)` |
| Auth shape | `Authorization: Bearer ...` when using `ANTHROPIC_AUTH_TOKEN`; `x-api-key: ...` when using `ANTHROPIC_API_KEY` |
| App marker | `x-app: cli` |
| Runtime headers | `x-stainless-lang: js`, package `0.94.0`, runtime `node`, runtime version `v24.3.0`, OS `Windows`, arch `x64` |
| Claude session | `X-Claude-Code-Session-Id` matched `metadata.user_id.session_id` |
| Billing attribution | `x-anthropic-billing-header: cc_version=2.1.181.<fp>; cc_entrypoint=sdk-cli;` |
| Message beta list | `claude-code-20250219,context-1m-2025-08-07,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,mid-conversation-system-2026-04-07,effort-2025-11-24` |
| Date shape | Current date is injected as the first user `<system-reminder>` with `# currentDate`, not inside the system block |

This confirms three important points: version equality is not a reliable target,
`claude --version` is not always the request fingerprint version, and `sdk-cli`
is not a third-party marker. It is the official non-interactive Claude Code
entrypoint for the local CLI path reviewed here.

The auth header shape is driven by how the user configures Claude Code. For
Sub2API usage, `ANTHROPIC_AUTH_TOKEN` still best matches a proxy bearer token,
but `ANTHROPIC_API_KEY` is also an official Claude Code path and results in
`x-api-key` while keeping the same UA, session, metadata, beta, and billing
attribution traits.

The same local CLI also sends a preliminary `HEAD /` request with `User-Agent:
node` before the message request. A second mock run returned `404` for that
probe and the CLI still completed the `/v1/messages` call successfully, so
Sub2API does not need to add or special-case a root `HEAD /` route for Claude
Code compatibility.

One local-package difference from the public mirror matters for entrypoint
handling. The public `main.tsx` returns early when `CLAUDE_CODE_ENTRYPOINT` is
already set, but the local bundled 2.1.112 package changes an existing
`CLAUDE_CODE_ENTRYPOINT=cli` to `sdk-cli` when the current run is
non-interactive. A live capture with `CLAUDE_CODE_ENTRYPOINT=cli` plus
`--print` still sent `claude-cli/2.1.112 (external, sdk-cli)` and
`cc_entrypoint=sdk-cli`. For Sub2API, this makes `sdk-cli` compatibility a
current packaged-CLI requirement, not just a public-source possibility.

## Local Bundled Source Cross-check

The installed local package was also inspected directly at the bundled
`cli.js` level. These findings are used as request-shape evidence, not as a
version pin:

| Area | Local bundled evidence | Sub2API decision |
| --- | --- | --- |
| User agent | The bundled UA helper formats `claude-cli/<version> (external, <entrypoint>...)` and can append SDK/client-app/workload suffixes | Keep family-based UA recognition; do not chase an exact local version |
| Entrypoint | The bundled entrypoint helper rewrites an existing `CLAUDE_CODE_ENTRYPOINT=cli` to `sdk-cli` for non-interactive runs | Treat `sdk-cli` as official local CLI behavior; add validator support only after approval |
| App marker | The API client sends `x-app: cli` for normal traffic and can send `x-app: cli-bg` for background session context | Current validator requires a non-empty `X-App`, so this is already tolerated; do not generate `cli-bg` for normal mimic traffic |
| Billing attribution | The billing block uses the runtime entrypoint and optional `cc_workload`; source and reverse-engineered evidence keeps the normal OAuth `cch=00000` placeholder before signing/replacement | Keep generated mimic internally consistent; include `cch=00000` by default and sign it after final body rewriting |
| Auth headers | Local and public paths support bearer auth from `ANTHROPIC_AUTH_TOKEN` and API-key auth from `ANTHROPIC_API_KEY` | Sub2API already accepts both inbound user-key shapes; upstream OAuth/setup-token forwarding remains bearer-based |
| Files API | The bundled file helper contains the same OAuth files beta literal as the public source | Files support should be a separate OAuth/setup-token route, not message-beta injection |

## Matching Traits Already Covered

| Trait | Official CLI behavior | Current Sub2API status |
| --- | --- | --- |
| User agent family | `claude-cli/<semver> (...)` | Matches family through `claude.DefaultHeaders` and Claude Code UA detection |
| User type | External builds identify as `external` | Default upstream mimic uses `external` |
| Client auth header | CLI can send either bearer auth or `x-api-key` depending on `ANTHROPIC_AUTH_TOKEN` vs `ANTHROPIC_API_KEY` | API key middleware accepts `Authorization: Bearer`, `x-api-key`, and `x-goog-api-key`; upstream OAuth/setup-token forwarding uses bearer auth |
| Stainless headers | JS SDK style headers with runtime-specific package, OS, arch, and Node version | `claude.DefaultHeaders` now follows the latest local network capture; validated real Claude Code clients keep their inbound official fingerprint while missing fields are filled from defaults |
| OAuth beta order | OAuth mimic traffic uses `claude-code`, `oauth`, `interleaved-thinking`, `context-management`, `prompt-caching-scope`, `effort`, `extended-cache-ttl` | `FullClaudeCodeMimicryBetas()` now uses the observed order |
| Message beta variability | Normal CLI message beta lists vary by version and auth path; current local network capture includes `context-1m` and `mid-conversation-system` while older package-path capture did not | Sub2API preserves client beta headers on passthrough; generated OAuth mimic traffic keeps the existing conservative beta list rather than enabling every feature beta by default |
| Current date shape | Current local network capture places `Today's date is YYYY/MM/DD.` inside the first user `<system-reminder>` | Generated OAuth/setup-token mimic now inserts the same current-date reminder before billing calculation |
| Global timezone | CLI body carries its own date; Sub2API should not globally move runtime TZ | Sub2API process timezone is not globally changed |
| Billing fingerprint salt | Public source uses `59cf53e54c78` | Sub2API uses the same salt |
| Billing fingerprint indexing | Public source uses JavaScript string indexes `[4, 7, 20]` | Sub2API now mimics JS UTF-16 string indexing |
| Billing CCH placeholder | Normal Claude Code OAuth billing includes `cch=00000` before runtime signing/replacement; a no-`cch` capture is treated as incomplete evidence | Sub2API generated billing includes `cch=00000` by default and signs the placeholder after final body rewriting |
| Request id forwarding | Public source injects `x-client-request-id` only for first-party Anthropic base URLs | Sub2API preserves valid Claude Code request ids and generates only when needed |
| Ambient browser headers | CLI API traffic does not require browser locale/fetch headers | Sub2API strips ambient browser/locale headers from Anthropic passthrough |
| Session identity | CLI carries a process-level Claude Code session id | Sub2API syncs upstream session identity from rewritten metadata where available |
| Account pool stability | Official clients keep one session identity stable across turns | Sub2API has stable affinity and `max_sessions` handling for Claude account pools |
| Official rate-limit windows | Upstream 5h/7d windows should not be shortened locally | Sub2API honors Anthropic reset headers before local temp-unsched rules |

## Optional Official Traits Reviewed

These traits exist in the public source but are not always present in normal
local CLI traffic. They should be tolerated when official clients send them,
but they should not be globally generated by Sub2API unless Sub2API is also
emulating the matching execution context.

| Trait | Official behavior | Current Sub2API status | Decision |
| --- | --- | --- | --- |
| UA SDK suffix | `getUserAgent()` may append `agent-sdk/<version>` and `client-app/<name>` | Claude Code UA validation is prefix-based, so these suffixes do not block recognition | Tolerate, do not synthesize globally |
| UA workload suffix | `getUserAgent()` may append `workload/<tag>` for scoped background work | Prefix validation tolerates the suffix | Tolerate, do not synthesize globally |
| Billing workload field | Attribution block may add `cc_workload=<tag>;` after `cc_entrypoint` and `cch` | Billing-block fallback checks the prefix and entrypoint marker, so `cc_entrypoint=cli; ... cc_workload=...` is tolerated | Tolerate for real clients; generate only if Sub2API adds a workload concept |
| SDK client app header | `getAnthropicClient()` may add `x-client-app` from `CLAUDE_AGENT_SDK_CLIENT_APP` | Not part of Sub2API default mimic headers | Do not generate for normal CLI mimic; consider only for explicit SDK-mode mimic |
| Remote session/container headers | `x-claude-remote-container-id` and `x-claude-remote-session-id` appear only in remote contexts | Not part of Sub2API default mimic headers | Do not generate unless implementing remote Claude Code context |
| Background app marker | Local bundled API client can send `x-app: cli-bg` for background session context while normal local capture sent `x-app: cli` | Validator only requires a non-empty `X-App` after the Claude Code UA and prompt checks | Tolerate, do not synthesize globally |
| Additional protection header | `x-anthropic-additional-protection: true` is gated by env | Not part of Sub2API default mimic headers | Do not generate globally |
| Custom request headers | `ANTHROPIC_CUSTOM_HEADERS` can add arbitrary headers before requests | Sub2API intentionally uses a narrow forwarding whitelist | Keep narrow; arbitrary custom headers can create mixed fingerprints |
| Extra metadata fields | `CLAUDE_CODE_EXTRA_METADATA` can add fields inside JSON `metadata.user_id` | Parsing tolerates unknown fields, but rewriting emits only `device_id`, `account_uuid`, and `session_id` | Fine for normal CLI; preserve-extra-fields can be evaluated if SDK/remote users rely on it |
| Telemetry / OpenTelemetry | Claude Code can collect local environment and behavior telemetry outside the normal message body | Sub2API does not synthesize telemetry because it cannot truthfully observe the caller's local shell, package manager, process, or behavioral state | Do not generate fake telemetry; add only narrow passthrough if a real Claude Code telemetry route is observed through the configured base URL |

## Official Sub2API Upstream Update Review

The local branch was compared with the configured Sub2API upstream
`origin/main` at `4a5665d` on 2026-06-17. A live `git ls-remote origin` retry
failed with a GitHub SSL connection error, so this review uses the already
fetched local `origin/main` ref.

| Upstream commit | What it changes | Local branch status | Decision |
| --- | --- | --- | --- |
| `8ce7b9a8f` | Adds configurable Claude OAuth system prompt block injection with templates such as `{billing_header}`, `{claude_code_system_prompt}`, and `{claude_code_expansion_prompt}` | Present; this branch now keeps the default three-block shape but exposes admin settings for enable/prompt/block templates | Covered after explicit approval; invalid block JSON falls back to the safe default templates |
| `f6e0ebc6` | Preserves Anthropic 5h/7d window cooldowns from upstream reset headers | Present; current code includes `persistAnthropicExhaustedWindowLimit` and related tests | Covered |
| `b256f9114` | Intercepts `max_tokens=1` Haiku probes for both streaming and non-streaming request shapes | Present; current handler tests confirm the probe does not depend on stream shape | Covered |
| `6baf00d78` | Makes thinking block filtering protocol-aware by mapped upstream model family | Present; Sub2API now skips thinking mutation for passback-required or unknown mapped upstream models and keeps Anthropic-strict filtering/retry behavior | Covered |
| `ab9987b2e` | Fails over on HTTP 2xx responses whose non-streaming body is not valid JSON | Present; non-streaming OAuth/setup-token and Anthropic API-key passthrough paths now return a 502 failover error with the upstream body and headers | Covered |
| `6c7203d83` | Preserves SSE `event:error` data as the failover response body and ops evidence | Present; stream errors now carry the raw SSE `data:` line into `UpstreamFailoverError.ResponseBody` and ops upstream-error context | Covered |
| `b63b41165` | Removes an unused billing attribution helper | Not reviewed as a runtime need | Cleanup only; not needed for the current anti-false-ban scope |

## Important Remaining Differences

| Difference | Evidence | Impact | Recommended action |
| --- | --- | --- | --- |
| Entrypoint is not always `cli` | Public `main.tsx` sets `CLAUDE_CODE_ENTRYPOINT` to `sdk-cli` for non-interactive mode; local 2.1.112 package-path and 2.1.181 network captures both recorded `entrypoint=sdk-cli` for `/v1/messages` traffic | Sub2API now recognizes billing-block fallback for `cc_entrypoint=cli` and `cc_entrypoint=sdk-cli` while still requiring the Claude Code UA, required headers, and billing-header prefix | Covered; do not blanket-allow unrelated public-source entrypoints without route-specific evidence |
| Feature beta drift | Current 2.1.181 network capture includes `context-1m-2025-08-07` and `mid-conversation-system-2026-04-07`; existing Sub2API defaults do not add both for generated OAuth mimic | Adding feature betas globally changes upstream feature behavior and may create compatibility failures on accounts or models that do not support them | Preserve real client beta headers; only widen generated mimic betas after account/model policy review |
| Version source mismatch | `claude --version` reports the wrapper package version while the actual network request can use the native package request fingerprint | Relying only on the version command can lead to stale mimic defaults | Use live local mock capture as the source of truth for defaults; preserve real inbound Claude Code fingerprints |
| Extra JSON metadata is not preserved on rewrite | Public `getAPIMetadata()` merges `CLAUDE_CODE_EXTRA_METADATA` into the JSON string stored at `metadata.user_id` | Sub2API parses those requests but rewrites metadata to the stable three-field shape, dropping optional extras | Accept for normal CLI; add preserve-extra-fields only if a real SDK/remote case depends on those fields |
| Files API needs route-specific handling | Public `filesApi.ts` and the local bundled helper use `/v1/files`, `/v1/files/{id}/content`, beta `files-api-2025-04-14,oauth-2025-04-20` | File uploads/downloads should not be handled through the normal messages route | Covered for `/v1/files`; the route is OAuth/setup-token only and keeps file beta isolated from message beta logic |
| File API beta differs by path | Official file downloads use `files-api-2025-04-14,oauth-2025-04-20`; SDK generic file helpers may use only `files-api-2025-04-14` with API keys | Mixing this into normal `/v1/messages` beta logic would be wrong | Keep file beta handling isolated in a files route, not in message beta merge logic |
| Public mirror and local package shape differ | Public mirror is TypeScript source; local 2.1.112 package is bundled JS; local 2.1.181 is native binary | Exact source diff is not one-to-one, so behavior captures are more authoritative for packaged behavior | Use public source for algorithms and local capture for current packaged behavior |

## Files API Implementation Boundary

The route table now registers `/v1/files` alongside `/v1/messages`,
`/v1/messages/count_tokens`, `/v1/models`, `/v1/usage`, and OpenAI-compatible
routes. The official public CLI file helper uses a separate OAuth files path
with these stable traits:

| Operation | Official path | Required upstream shape |
| --- | --- | --- |
| Download content | `GET /v1/files/{file_id}/content` | `Authorization: Bearer <oauth token>`, `anthropic-version: 2023-06-01`, `anthropic-beta: files-api-2025-04-14,oauth-2025-04-20`, binary response |
| Upload | `POST /v1/files` | Multipart body with the raw file and `purpose=user_data`, same OAuth files beta header |
| List recent files | `GET /v1/files?after_created_at=...&after_id=...` | Same OAuth files beta header and pagination passthrough |

The bundled Anthropic SDK inside the local CLI also exposes generic file
helpers for list, retrieve metadata, delete, download, and upload using the
`files-api-2025-04-14` beta. That is not the same as the Claude Code OAuth
helper path above. Sub2API should not merge either files beta into
`/v1/messages`; files handling should stay in a route-specific proxy.

A safe first implementation has been applied:

1. Require the Sub2API user key and an Anthropic group, then schedule only
   Anthropic OAuth/setup-token accounts for the files route.
2. Call `GatewayService.GetAccessToken` for the selected account. That path
   supports setup-token by reading the account credential directly when the
   Claude token provider does not handle setup-token.
3. Reuse the existing account proxy, TLS profile resolution, custom base URL
   validation, and response-header filtering.
4. Forward raw request and response bodies without JSON mutation, token usage
   accounting, message beta merging, or billing-header injection.
5. Keep file scheduling independent from model-based message scheduling because
   the files route has no request-body model field.

The implemented route covers `GET /v1/files`, `POST /v1/files`,
`GET /v1/files/{file_id}`, `DELETE /v1/files/{file_id}`, and
`GET /v1/files/{file_id}/content`. It does not implement the separate
`/api/oauth/file_upload` bridge endpoint.

## Safe Implementation Boundary

The following compatibility fixes were implemented only after explicit
approval because they touch sensitive paths:

1. Configurable Claude OAuth system prompt blocks: affects generated Claude
   OAuth mimic body construction.
2. `/v1/files` proxying: affects network request forwarding, OAuth token use,
   account selection, and raw response passthrough.

Default generated entrypoint and user-agent selection now follows the latest
local network capture. Real Claude Code clients are still not forced to that
single version; Sub2API preserves their inbound official fingerprint where
validation has identified the caller as Claude Code.

## Current Alignment Status

| Area | Status | Evidence / next action |
| --- | --- | --- |
| Local/public CLI source comparison | Covered | Public mirror at `290fdc9`; local 2.1.112 wrapper package and 2.1.181 native package inspected |
| Local live request traits | Covered | Local mock captures confirm `2.1.181 sdk-cli`, auth variants, metadata/session, beta list, current-date reminder placement, and `HEAD /` tolerance; later source/reverse-engineered evidence restored `cch=00000` as the generated default |
| Version pinning | Covered as non-requirement | CLI can switch versions; Sub2API preserves validated inbound Claude Code fingerprints and keeps generated defaults internally consistent |
| OS/arch | Covered as non-issue | Runtime OS/arch are fingerprint traits; current issue is mixed fingerprints, not forcing a global OS/arch |
| `x-app` marker | Covered as non-issue | Normal local capture sent `cli`; bundled source can send `cli-bg` in background context; current validation already tolerates non-empty official app markers |
| Date behavior | Covered in code | Generated mimic requests insert a current-date user reminder with `YYYY/MM/DD`; real client bodies are not rewritten |
| Billing fingerprint | Covered in code | Salt and JS string-index behavior are implemented and tested |
| CCH | Covered for current default | Generated billing includes `cch=00000` and CCH signing is enabled by default; signing still runs after body normalization/sanitization |
| Request/session identity | Covered in code | Request id preservation/generation and session header/metadata synchronization are implemented |
| Account device identity | Covered in code | Anthropic OAuth/setup-token accounts persist `extra.cc_device_id`; generated metadata uses that value instead of caller-provided metadata so one upstream account keeps a stable device id |
| Telemetry | Intentionally not synthesized | The gateway aligns forwarded API requests and documents the boundary; fake environment/process telemetry is not generated |
| Account scheduling | Covered in code | Stable affinity, active-session preference, `max_sessions`, and Anthropic reset-window handling are implemented |
| Official Sub2API updates already useful here | Covered | Anthropic reset-window cooldowns, streaming Haiku probe interception, mapped-model thinking filters, non-JSON 2xx failover, and SSE `event:error` body preservation are present |
| Official Sub2API updates still missing here | None in the reviewed anti-false-ban set | The useful reviewed items are now either covered or intentionally left as design-only |
| `sdk-cli` billing fallback | Covered in code | Billing-block fallback now accepts `cli` and the locally captured `sdk-cli`, while unrelated entrypoints still fall through |
| Generated SDK-mode mimic | Covered for default path | Default generated UA, billing entrypoint, signed CCH placeholder, system identity/cache controls, and current-date reminder are internally aligned |
| `/v1/files` | Covered in code | Route-level OAuth/setup-token transparent proxy is implemented with official files beta and raw body/response passthrough |
| Root `HEAD /` probe | No change needed | CLI tolerates 404 and continues to `/v1/messages` |

The public source also names other entrypoints, including `mcp`,
`claude-code-github-action`, `claude-vscode`, `local-agent`, `claude-desktop`,
and `remote`. Those should not be blanket-allowed for billing-block fallback
without a matching local capture or route-specific reason. The immediate
compatibility target is `sdk-cli`, because it is both in the public source and
present in local `/v1/messages` capture data.

## Remaining Design Item

Generated feature-beta expansion remains intentionally conservative. Do not add
new functional beta tokens globally unless the target account/model policy has
been reviewed; real Claude Code client beta headers are preserved before policy
filtering.
