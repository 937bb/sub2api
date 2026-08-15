# OpenAI Transient Retry Compatible Rollback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the group-configured retry-count behavior from `41e56d5d0`, retain its boolean as a model-load retry switch, and return HTTP 503 for classified pre-output HTTP 200 stream load failures only when that switch is enabled.

**Architecture:** Restore gateway same-account retry call sites to parent commit `28cdc5ecc`, keeping model mapping, the existing boolean policy field, and compatibility-only retry-count Ent column. Retain one narrow load classifier in the streaming terminal path, preserve status 503 through `UpstreamFailoverError` only when the group switch is enabled, and let the handler expose 503 only when `RequestScopedTransient` proves the error is a classified load-shed event.

**Tech Stack:** Go 1.26, Gin, Testify, Vue 3, TypeScript, Vitest/Vite.

---

### Task 1: Add failing 503 regression tests

**Files:**
- Modify: `backend/internal/service/openai_gateway_service_test.go`
- Modify: `backend/internal/handler/gateway_responses_stream_terminal_test.go`

- [ ] **Step 1: Change the coded overload service assertion to 503**

In `TestOpenAIStreamingResponseFailedBeforeOutputServerOverloadedCodeReturnsFailover`, add an API-key group with `OpenAITransientErrorRetryEnabled: true` and assert:

```go
require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
```

Add a disabled-switch subtest expecting the original 502 status. Add a
message-only overload case whose terminal payload contains `Our servers are
currently overloaded`, expecting 503 only with the switch enabled.

- [ ] **Step 2: Add a handler distinction test**

Call `handleFailoverExhausted` with a `503` `UpstreamFailoverError` twice: once with `RequestScopedTransient: true`, expecting client 503, and once without it, expecting the existing client 502 mapping.

- [ ] **Step 3: Run tests and verify RED**

Run:

```bash
cd backend
go test ./internal/service -run 'TestOpenAIStreamingResponseFailedBeforeOutput(ServerOverloadedCode|MessageOnlyOverload)' -count=1
go test ./internal/handler -run 'TestHandleFailoverExhausted.*Overload' -count=1
```

Expected: failures showing the classified stream status and handler output are still 502.

### Task 2: Narrow the group policy to a boolean model-load switch

**Files:**
- Modify: `backend/internal/handler/admin/group_handler.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/repository/api_key_repo.go`
- Modify: `backend/internal/repository/group_repo.go`
- Modify: `backend/internal/service/admin_group.go`
- Modify: `backend/internal/service/admin_group_duplicate.go`
- Modify: `backend/internal/service/admin_service.go`
- Modify: `backend/internal/service/admin_service_group_test.go`
- Modify: `backend/internal/service/api_key_auth_cache.go`
- Modify: `backend/internal/service/api_key_auth_cache_impl.go`
- Modify: `backend/internal/service/api_key_auth_cache_profit_test.go`
- Modify: `backend/internal/service/group.go`
- Modify: `frontend/src/i18n/locales/en/admin/overview.ts`
- Modify: `frontend/src/i18n/locales/zh/admin/overview.ts`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/views/admin/GroupsView.vue`
- Preserve: `backend/ent/**`
- Preserve: `backend/migrations/224_group_openai_transient_error_retry.sql`

- [ ] **Step 1: Keep the boolean and remove retry-count behavior**

Keep `OpenAITransientErrorRetryEnabled` through DTO mapping, repository hydration, admin create/update/duplication, and auth snapshots. Remove `OpenAITransientErrorRetryCount`, count normalization, count validation, and count serialization from these runtime/public layers. Keep all `OpenAIModelMapping*` fields and behavior.

- [ ] **Step 2: Rename the frontend switch and remove its count input**

Keep the create/edit switch and boolean request property. Rename its Chinese and English title, label, and hint to model-load retry semantics. Remove the retry count input, count form state, count validation, count request property, and count TypeScript property. Keep `GroupModelMappingEditor` and its request conversion intact.

- [ ] **Step 3: Restore affected tests to compatibility expectations**

Replace tests for configurable retry counts with boolean persistence tests. Update the auth snapshot version comment to describe model mapping and model-load retry while retaining version 22.

- [ ] **Step 4: Verify no runtime/API references remain**

Run:

```bash
rg -n 'OpenAITransientErrorRetryCount|openai_transient_error_retry_count' backend/internal frontend/src
```

Expected: no matches. Retry-enabled matches remain intentionally; count matches under `backend/ent` and migration 224 remain intentionally.

### Task 3: Roll back group retry gateway integrations

**Files:**
- Modify: `backend/internal/handler/openai_responses_failover_cancel_test.go`
- Modify: `backend/internal/service/openai_capacity_shed_test.go`
- Modify: `backend/internal/service/openai_gateway_cc_pipeline.go`
- Modify: `backend/internal/service/openai_gateway_forward.go`
- Modify: `backend/internal/service/openai_gateway_passthrough.go`
- Modify: `backend/internal/service/openai_gateway_response_handling.go`
- Modify: `backend/internal/service/openai_gateway_service_codex_cli_only_test.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`
- Modify: `backend/internal/service/openai_gateway_upstream_errors.go`
- Modify: `backend/internal/service/openai_ws_http_bridge.go`

- [ ] **Step 1: Remove the group policy helpers and calls**

Delete `openAITransientErrorRetryCount` and `configureOpenAITransientErrorRetry`. Keep `openAITransientErrorRetryEnabled` as the request/group switch lookup. Restore parent same-account retry behavior at HTTP, passthrough, chat completions, and WebSocket call sites.

- [ ] **Step 2: Keep only the classifier needed by 503 translation**

Retain `isOpenAITransientCapacityError` for coded and message-only load signals. Retain early stream buffering and classified status conversion only when the switch is enabled, so an opted-in upstream HTTP 200 terminal load failure can escape before downstream output is committed. Do not set `RetryableOnSameAccount` from the group switch.

- [ ] **Step 3: Preserve unrelated retry behavior**

Keep `openAIStreamFailedEventRetryableOnSameAccount` coded capacity behavior from `28cdc5ecc`, Pool Mode retry, and `configureOpenAIQuotaBypass429Retry` unchanged.

### Task 4: Preserve classified stream load failures as HTTP 503

**Files:**
- Modify: `backend/internal/service/openai_gateway_passthrough.go`
- Modify: `backend/internal/handler/openai_gateway_handler.go`

- [ ] **Step 1: Return semantic 503 for load failures**

Add a request-aware stream failure status helper. It returns `http.StatusServiceUnavailable` only when `openAITransientErrorRetryEnabled(c)` and `isOpenAITransientCapacityError(message, payload)` are both true; otherwise it delegates to the parent status behavior. Use it for failover errors and ops recording, while keeping 429 and all non-load statuses unchanged.

- [ ] **Step 2: Preserve 503 only for classified load failures at the handler**

Before the generic `mapUpstreamError` call in `handleFailoverExhausted`, add:

```go
if statusCode == http.StatusServiceUnavailable && failoverErr.RequestScopedTransient {
	h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "upstream_error", "Upstream service overloaded, please retry later", streamStarted)
	return
}
```

This leaves generic upstream 500/502/503/504 mapping unchanged.

- [ ] **Step 3: Run focused tests and verify GREEN**

Run:

```bash
cd backend
go test ./internal/service -run 'Test(OpenAIStreamingResponseFailedBeforeOutput|StreamFailedEventCapacityShed|OpenAIStreamCapacityShed|ConfigureOpenAIQuotaBypass)' -count=1
go test ./internal/handler -run 'Test(HandleFailoverExhausted|OpenAIGatewayHandlerResponses)' -count=1
```

Expected: all selected tests pass.

### Task 5: Verify the compatible rollback

**Files:**
- Verify all modified files.

- [ ] **Step 1: Run backend focused suites**

```bash
cd backend
go test ./internal/service ./internal/handler
```

Expected: both packages pass.

- [ ] **Step 2: Run frontend checks**

Use the package manager declared by the frontend lockfile, then run the repository's type-check and build scripts.

- [ ] **Step 3: Check generated compatibility and diff scope**

```bash
git diff --check
git status --short
git diff --name-only 28cdc5ecc -- backend/ent backend/migrations/224_group_openai_transient_error_retry.sql
```

Expected: no whitespace errors; the Ent compatibility fields and migration remain present; no deployment environment file is changed in the isolated worktree.
