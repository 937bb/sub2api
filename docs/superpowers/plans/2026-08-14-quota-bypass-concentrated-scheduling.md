# Quota Bypass Concentrated Scheduling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a group-level switch that independently enables Bypass-first fill-first scheduling while leaving all other Quota Bypass features controlled by the existing switch.

**Architecture:** Persist the boolean through the existing group DTO, repository, API-key, and scheduler-cache paths. Keep `IsQuotaBypassEligible` unchanged and add a current-request-group predicate used only by scheduler concentration branches. Add a dependent Vue checkbox next to the existing Bypass control.

**Tech Stack:** Go, Ent, PostgreSQL, Gin, Vue 3, TypeScript, vue-i18n.

---

### Task 1: Persist And Propagate The Setting

**Files:**
- Create: `backend/migrations/222_group_quota_bypass_concentrated_scheduling.sql`
- Modify: `backend/ent/schema/group.go`
- Regenerate: `backend/ent/**`
- Modify: `backend/internal/service/group.go`
- Modify: `backend/internal/service/admin_service.go`
- Modify: `backend/internal/service/admin_group.go`
- Modify: `backend/internal/handler/admin/group_handler.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Modify: `backend/internal/repository/group_repo.go`
- Modify: `backend/internal/repository/api_key_repo.go`
- Modify: `backend/internal/repository/scheduler_cache.go`
- Test: `backend/internal/repository/scheduler_cache_unit_test.go`
- Test: `backend/internal/service/gateway_quota_bypass_cache_test.go`

- [ ] **Step 1: Write failing round-trip tests**

Add fixtures and assertions for:

```go
QuotaBypassEnabled:                       true,
QuotaBypassConcentratedSchedulingEnabled: true,
```

Run `cd backend && go test ./internal/repository ./internal/service -run 'QuotaBypass.*(Cache|RoundTrip)|SchedulerCache' -count=1`.
Expected: compile failure because the field is missing.

- [ ] **Step 2: Add migration, schema, DTO, and repository fields**

Add a non-null, false-default column and matching Ent field:

```sql
ALTER TABLE groups ADD COLUMN IF NOT EXISTS
quota_bypass_concentrated_scheduling_enabled BOOLEAN NOT NULL DEFAULT FALSE;
```

```go
field.Bool("quota_bypass_concentrated_scheduling_enabled").
    Default(false).
    Comment("是否对该分组启用 Codex 超额绕过集中调度")
```

Use this field on response/create models and `*bool` on update models:

```go
QuotaBypassConcentratedSchedulingEnabled bool `json:"quota_bypass_concentrated_scheduling_enabled"`
```

Copy it in every existing `QuotaBypassEnabled` persistence, mapping, API-key,
and scheduler-cache path.

- [ ] **Step 3: Regenerate Ent and pass focused tests**

Run `cd backend && go generate ./ent`, then rerun Step 1. Expected: PASS.

- [ ] **Step 4: Commit persistence changes**

Stage only Task 1 files and commit with `feat(groups): add bypass concentrated scheduling setting`.

### Task 2: Gate Both Concentrated Scheduling Implementations

**Files:**
- Modify: `backend/internal/service/openai_account_scheduler.go`
- Modify: `backend/internal/service/openai_gateway_scheduling.go`
- Test: `backend/internal/service/openai_account_scheduler_test.go`
- Test: `backend/internal/service/gateway_quota_bypass_test.go`

- [ ] **Step 1: Write failing scheduler tests**

Test both legacy and advanced selection with these two requests:

```go
ordinaryReq := OpenAIAccountScheduleRequest{
    GroupQuotaBypassEnabled: true,
    GroupQuotaBypassConcentratedSchedulingEnabled: false,
}
concentratedReq := OpenAIAccountScheduleRequest{
    GroupQuotaBypassEnabled: true,
    GroupQuotaBypassConcentratedSchedulingEnabled: true,
}
```

Assert ordinary load-balanced ordering for the first and existing fill-first
ordering for the second. Add cases proving account-level Bypass and another
attached group's Bypass cannot activate concentration for the current group.
Run `cd backend && go test ./internal/service -run 'OpenAI.*QuotaBypass.*(Concentrat|Ordinary|RequestGroup)' -count=1`.
Expected: compile or behavioral failure.

- [ ] **Step 2: Add the scheduling-only predicate**

Resolve the new field from `req.SchedulingGroup` and add:

```go
func isOpenAIQuotaBypassConcentratedForScheduleRequest(account *Account, req OpenAIAccountScheduleRequest) bool {
    if !req.GroupQuotaBypassEnabled || !req.GroupQuotaBypassConcentratedSchedulingEnabled {
        return false
    }
    return IsQuotaBypassEligible(account, &Group{QuotaBypassEnabled: true})
}
```

Use it for Bypass pool partitioning, sticky yielding, concentrated load reads,
cursor/window order, probe budgets, and full-account cursor movement. Retain
`isOpenAIQuotaBypassEligibleForScheduleRequest` for quota auto-pause exemption.

- [ ] **Step 3: Gate the legacy scheduler**

Make `resolveOpenAIQuotaBypassSchedulingGroup` return a group only when both
current-group fields are true. This disables legacy Bypass ordering/cursor paths
without changing injection, retry, or quota exemption eligibility.

- [ ] **Step 4: Prove non-scheduling behavior remains active**

Use a group with Bypass true and concentration false in existing injection,
auto-pause, and 429 retry tests. Run `cd backend && go test ./internal/service -run 'QuotaBypass|ShouldAutoPauseOpenAIAccountByQuota' -count=1`.
Expected: PASS.

- [ ] **Step 5: Commit scheduler changes**

Stage only Task 2 files and commit with `feat(scheduler): gate bypass concentration by group setting`.

### Task 3: Add The Admin UI Switch

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/views/admin/GroupsView.vue`
- Modify: `frontend/src/i18n/locales/*/admin/overview.ts`

- [ ] **Step 1: Add the typed form property**

Add `quota_bypass_concentrated_scheduling_enabled?: boolean` to group/create/update
types and false defaults to create/edit form state.

- [ ] **Step 2: Add the dependent checkbox**

Place the new checkbox beside the existing control in create and edit forms:

```vue
<input
  v-model="createForm.quota_bypass_concentrated_scheduling_enabled"
  type="checkbox"
  :disabled="!createForm.quota_bypass_enabled"
/>
```

Mirror it for edit. Loading uses `?? false`; reset uses false; turning Bypass off
forces the new value to false before submission.

- [ ] **Step 3: Add translations and verify frontend**

Add `启用集中调度` and `Enable concentrated scheduling` under the existing
`quotaBypass` locale object, keeping every locale structurally consistent. Run
`cd frontend && pnpm build && pnpm lint:check`. Expected: PASS.

- [ ] **Step 4: Commit frontend changes**

Stage only Task 3 files and commit with `feat(admin): configure bypass concentrated scheduling`.

### Task 4: Final Verification

**Files:** Verify all Task 1-3 changes.

- [ ] **Step 1: Format handwritten Go files**

Run `cd backend && gofmt -w internal/service internal/handler internal/repository ent/schema/group.go`.

- [ ] **Step 2: Run focused backend suites**

Run `cd backend && go test ./internal/service ./internal/repository ./internal/handler/admin -count=1`.
Expected: PASS.

- [ ] **Step 3: Compile all backend packages**

Run `cd backend && go test ./... -run '^$'`. Expected: PASS.

- [ ] **Step 4: Re-run frontend verification**

Run `cd frontend && pnpm build && pnpm lint:check`. Expected: PASS.

- [ ] **Step 5: Audit the completed work**

Run `git diff --check`, `git status --short`, and inspect the commits/diff.
Confirm migration, DTO/cache propagation, both scheduling gates, UI behavior,
translations, and tests. Report runtime validation separately and do not claim
it unless a running service was exercised.
