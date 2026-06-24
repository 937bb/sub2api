# Sub Claude Usage

This guide explains how to use Claude Code through Sub2API. It is based on the current Sub2API gateway code and on the Claude Code CLI source mirrors reviewed for request behavior.

## What To Configure

Claude Code should point to Sub2API as an Anthropic-compatible endpoint:

```bash
export ANTHROPIC_BASE_URL="http://localhost:8080"
export ANTHROPIC_AUTH_TOKEN="sk-your-sub2api-key"
claude
```

For a public deployment, replace `http://localhost:8080` with your Sub2API origin, for example:

```bash
export ANTHROPIC_BASE_URL="https://api.example.com"
export ANTHROPIC_AUTH_TOKEN="sk-your-sub2api-key"
claude
```

`ANTHROPIC_AUTH_TOKEN` is the recommended variable for Claude Code proxy use because the Claude Code client sends it as `Authorization: Bearer ...`, which maps cleanly to a Sub2API user key. `ANTHROPIC_API_KEY` is also an official Claude Code path; the local CLI sends it as `x-api-key` while keeping the same Claude Code UA, session, metadata, beta, and billing traits. Sub2API accepts `Authorization: Bearer`, `x-api-key`, and `x-goog-api-key`, but bearer auth is the clearest default for Claude Code users pointing at a proxy.

Claude Code file APIs use a separate path from normal chat. The CLI sends message requests through the Anthropic SDK (`/v1/messages` and `/v1/messages/count_tokens`) with the bearer token above. Startup file resources and bridge attachment uploads use OAuth/session-token endpoints such as `/v1/files`, `/v1/files/{id}/content`, or `/api/oauth/file_upload`; those are not the same as a Sub2API user API key and should not be mixed into normal `/v1/messages` routing. Sub2API now exposes a narrow `/v1/files` proxy for Anthropic OAuth/setup-token accounts. It keeps the official files beta isolated from normal message beta handling and does not implement the separate `/api/oauth/file_upload` bridge endpoint.

## Endpoint Selection

Use the endpoint according to the account group you want to schedule.

| Use case | `ANTHROPIC_BASE_URL` | Main routes |
| --- | --- | --- |
| Anthropic Claude accounts | `https://your-sub2api-host` | `/v1/messages`, `/v1/messages/count_tokens`, `/v1/files`, `/v1/models`, `/v1/usage` |
| Antigravity Claude only | `https://your-sub2api-host/antigravity` | `/antigravity/v1/messages`, `/antigravity/v1/messages/count_tokens`, `/antigravity/v1/models` |
| OpenAI account group with Claude Code compatibility | `https://your-sub2api-host` | `/v1/messages` dispatches to OpenAI when the API key's group platform is `openai` |

For local testing:

```bash
export ANTHROPIC_BASE_URL="http://localhost:8080"
export ANTHROPIC_AUTH_TOKEN="sk-your-sub2api-key"
claude --print "Say hello through Sub2API"
```

For Antigravity Claude-only routing:

```bash
export ANTHROPIC_BASE_URL="http://localhost:8080/antigravity"
export ANTHROPIC_AUTH_TOKEN="sk-your-sub2api-key"
claude
```

## Admin Setup

1. Add at least one upstream account in the admin dashboard.
2. Create or choose a group for the target platform:
   - `anthropic` for normal Claude accounts.
   - `antigravity` for dedicated Antigravity Claude routing.
   - `openai` only when using the Anthropic `/v1/messages` compatibility dispatch into OpenAI models.
3. Bind the upstream account to the group.
4. Create a user API key bound to that group.
5. Use that generated API key as `ANTHROPIC_AUTH_TOKEN`.

If the key is not assigned to a group, gateway requests are rejected before routing.

## Claude Code Compatibility Notes

Sub2API detects real Claude Code requests from the `claude-cli/x.y.z` user agent plus Claude Code request structure. You do not need to manually set `anthropic-beta`, `x-app`, `x-stainless-*`, or Claude Code system-prompt fields. Normal local CLI traffic sends `x-app: cli`; the installed bundled source can also send `x-app: cli-bg` for background session context. That is an official context marker, not a value Sub2API should generate for every request.

For `/v1/messages`, Claude Code identification accepts the CLI user agent, required Anthropic headers, and either a valid `metadata.user_id` or Claude Code's own `x-anthropic-billing-header` system attribution block with `cc_entrypoint=cli` or `cc_entrypoint=sdk-cli`. The attribution block is used to reduce false rejects for official auxiliary requests whose metadata shape may change or be absent. Public source review, installed bundled source review, and local captures confirmed `sdk-cli` as the official non-interactive entrypoint for `--print` / SDK-path traffic. Prompt-similarity-only requests still require valid metadata.

Claude Code can also add optional SDK or workload markers such as `agent-sdk/...`, `client-app/...`, `workload/...`, `cc_workload=...`, `x-client-app`, and remote-session headers. These are official context-specific traits, not baseline local-CLI requirements. Sub2API should tolerate them on real incoming clients where validation allows it, but should not generate them globally for normal Claude account-pool traffic.

For Anthropic OAuth or setup-token accounts, Sub2API can send Claude Code-like upstream headers and beta tokens when needed. For Anthropic API-key accounts, the gateway preserves client beta headers; optional automatic beta injection is controlled by `gateway.inject_beta_for_apikey`.

When fingerprint unification is enabled, which is the default, Sub2API uses the service-side Claude Code default fingerprint for generated mimic traffic and for non-Claude-Code passthrough requests. Requests that have already passed Claude Code validation keep their official inbound `User-Agent`, `x-app`, and `x-stainless-*` values, with missing fields filled from the service defaults. This avoids header/body mismatches when the local CLI switches versions while still preventing arbitrary non-CLI clients from choosing upstream fingerprint headers. Disable `enable_fingerprint_unification` only for controlled debugging.

For Anthropic OAuth/setup-token account identity, Sub2API keeps a service-side `extra.cc_device_id` per upstream account and uses it as `metadata.user_id.device_id` when generating Claude Code-style upstream requests. If the field is missing, the gateway backfills it once from the existing legacy Claude user id, then the account fingerprint client id, then a generated id. The value is not taken from the caller's request metadata, so multiple client machines sharing one upstream OAuth account do not cause the upstream account device id to drift.

Sub2API does not synthesize Claude Code telemetry or OpenTelemetry events. The gateway aligns the actual API request identity, headers, billing attribution, and session metadata that it forwards upstream; it does not invent local process, shell, package-manager, or behavioral telemetry that it cannot truthfully observe. If a future Claude Code release sends a documented telemetry API request through the configured Anthropic base URL, handle it as a narrow route-specific passthrough decision rather than a fake event generator.

The `/v1/files` route is a transparent raw-body proxy for `GET /v1/files`, `POST /v1/files`, `GET /v1/files/{file_id}`, `DELETE /v1/files/{file_id}`, and `GET /v1/files/{file_id}/content`. It requires the caller's Sub2API key, selects only Anthropic OAuth/setup-token upstream accounts, obtains the upstream OAuth access token through the existing token path, and forwards with `anthropic-version: 2023-06-01` plus `anthropic-beta: files-api-2025-04-14,oauth-2025-04-20`. The route preserves multipart and binary bodies, filters client auth/cookie/fingerprint headers, and applies the same response-header filtering used by message forwarding. If a selected OAuth/setup-token account is at its concurrency limit but the scheduler allows waiting, the files route waits for the account slot using the same wait-plan capacity rules as message forwarding.

Sub2API also normalizes `x-client-request-id` before forwarding Claude Code requests upstream. For requests that have already passed Claude Code validation, a valid UUID supplied by the official client is preserved; if it is missing or malformed, the gateway generates one before forwarding. Non-Claude-Code passthrough requests still do not get to choose this upstream request id.

Sub2API does not rewrite user prompt content or user-provided timezone-like values inside real Claude Code JSON bodies. For Claude OAuth or setup-token mimic requests generated from non-Claude-Code clients, the gateway inserts a Claude Code-style current-date `<system-reminder>` as the first user message using `America/New_York` and the CLI-observed `YYYY/MM/DD` date shape, then computes the billing fingerprint from that body. Real Claude Code client bodies are left unchanged. Sub2API also does not forward ambient browser or locale headers such as `accept-language` and `sec-fetch-mode` on Anthropic routes, because those are not required by Claude Code's normal API calls and can make upstream requests look like mixed client environments.

For Claude OAuth mimic requests, Sub2API keeps the core Claude Code beta token order conservative. Normal CLI `/v1/messages` beta lists can vary by version and auth path; the latest local mock capture sent `context-1m-2025-08-07` and `mid-conversation-system-2026-04-07`, while older package-path captures did not. Sub2API therefore preserves real client beta headers before policy filtering and avoids globally enabling every feature beta for generated mimic traffic. The `cc_version` billing fingerprint is computed with JavaScript string-index semantics so non-ASCII prompt text follows the same character selection behavior as the CLI.

For non-Claude-Code requests forwarded through Anthropic OAuth or setup-token accounts, Sub2API injects Claude Code-style system blocks by default. Admin settings can disable this (`enable_claude_oauth_system_prompt_injection`), replace the core prompt (`claude_oauth_system_prompt`), or provide JSON block templates (`claude_oauth_system_prompt_blocks`) with `{billing_header}`, `{claude_code_system_prompt}`, and `{claude_code_expansion_prompt}` placeholders. Empty settings keep the built-in three-block shape. Invalid block JSON, or valid JSON that produces no text blocks, falls back to the safe default block shape.

Official Claude Code carries the same top-level session identity in both `metadata.user_id.session_id` and `X-Claude-Code-Session-Id`. Sub2API keeps that relationship after rewriting metadata for an upstream account: the upstream `X-Claude-Code-Session-Id` is synchronized from the final rewritten metadata when metadata is present. If a lightweight request such as `count_tokens` has no metadata, Sub2API derives an account-scoped upstream session from the inbound Claude Code session signal instead of forwarding the user-provided header verbatim. This is important for multi-window use because each Claude Code window has its own process-level session id.

For Claude OAuth or setup-token accounts, one upstream Claude account can be configured with concurrency greater than one. Multi-concurrency should be paired with independent CLI sessions, not a shared masked session. The legacy `session_id_masking_enabled` account option is ignored by Claude Code identity rewriting so that multiple windows on the same upstream account are not collapsed into one upstream session/cache identity. Use `max_sessions` when you need to cap how many distinct Claude Code sessions may stay active on one upstream account.

When multiple upstream Claude accounts are equally eligible, Sub2API uses the validated Claude Code metadata session plus the Sub2API user id as a stable affinity seed for tie-breaking. This keeps one CLI window sticky after assignment while reducing the chance that several fresh windows from the same user all land on the same upstream account. If all tied candidates are Claude OAuth/setup-token accounts with `max_sessions` configured, the scheduler also prefers the account with fewer active Claude Code sessions before applying the affinity tie-break.

When Anthropic returns official 5h or 7d rate-limit window headers, Sub2API honors that upstream reset before local temporary-unschedulable rules. This keeps an exhausted Claude account out of rotation for the correct upstream window instead of shortening the cooldown with a local generic 429 rule.

For Anthropic-compatible third-party upstreams, thinking-block cleanup is keyed by the mapped upstream model, not only the client-requested Claude model. Models that require thinking history to be passed back verbatim, such as DeepSeek/Kimi/Moonshot/GLM/MiniMax M and Qwen thinking variants, are preserved instead of being rewritten by the Anthropic-strict signature rectifier.

Useful admin controls:

| Control | When to use |
| --- | --- |
| Group `claude_code_only` | Restrict a group to Claude Code clients. Non-Claude-Code clients are rejected or routed to the configured fallback group. |
| Group `fallback_group_id` | Provide a fallback group when `claude_code_only` blocks a request. |
| Group `allow_messages_dispatch` | For `openai` groups, allow Claude-style `/v1/messages` requests to dispatch into OpenAI-compatible models. |
| Group `messages_dispatch_model_config` | Customize Claude model family mapping for OpenAI dispatch. Defaults exist for Opus, Sonnet, and Haiku families. |
| Account `mixed_scheduling` | Let an Antigravity account participate in general `/v1/messages` or `/v1beta/` scheduling. Dedicated `/antigravity/...` routes are still isolated. |
| Account `anthropic_passthrough` | For Anthropic API-key accounts that should forward more directly to an Anthropic-compatible upstream. |
| Account TLS fingerprint | For Anthropic OAuth/setup-token accounts, simulate Claude Code-like TLS behavior when enabled. |
| Account `max_sessions` | Limit the number of distinct Claude Code sessions that may be active on one Anthropic OAuth/setup-token account. Leave unset for no session-count cap. |

## Common Issues

`401 API_KEY_REQUIRED`  
Claude Code did not send a Sub2API key. Set `ANTHROPIC_AUTH_TOKEN`, or use a client that sends `Authorization: Bearer <key>` or `x-api-key: <key>`.

`403 GROUP_*` or group assignment errors  
The API key is not bound to an active group, or the user is no longer allowed to use the group.

`this group only allows Claude Code clients`  
The group has `claude_code_only` enabled. Use official Claude Code, or configure a fallback group.

Plan Mode cannot exit automatically on Antigravity Claude  
Exit Plan Mode manually with `Shift + Tab`, then type the approval or rejection response.

Anthropic Claude and Antigravity Claude should not share the same long conversation  
Keep them separated by group or by separate Claude Code sessions. Their upstream behavior and model families are different enough that mixed context can produce confusing results.

## Source References

Current Sub2API behavior is implemented in:

- `backend/internal/server/routes/gateway.go`: registers `/v1`, `/antigravity/v1`, and `/antigravity/v1beta` routes.
- `backend/internal/server/middleware/api_key_auth.go`: accepts `Authorization: Bearer`, `x-api-key`, and `x-goog-api-key`.
- `backend/internal/server/middleware/middleware.go`: applies forced Antigravity platform routing and group assignment checks.
- `backend/internal/handler/gateway_handler.go`: handles `/v1/messages` and `/v1/messages/count_tokens`, including Claude Code detection.
- `backend/internal/handler/gateway_helper.go`: sets Claude Code client context from request user agent and body shape.
- `backend/internal/service/claude_code_validator.go`: validates Claude Code user agent, headers, metadata, and system prompt markers.
- `backend/internal/service/gateway_service.go`: selects accounts, handles mixed scheduling, builds upstream Claude requests, merges `anthropic-beta`, and applies Claude Code mimic headers.
- `backend/internal/pkg/claude/constants.go`: defines Claude Code header defaults, beta tokens, and model aliases used by the gateway.
