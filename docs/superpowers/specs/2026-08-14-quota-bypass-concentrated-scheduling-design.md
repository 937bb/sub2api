# Quota Bypass Concentrated Scheduling Design

## Goal

Add a group-level switch named `quota_bypass_concentrated_scheduling_enabled`
next to the existing Codex Quota Bypass switch.

The two settings have separate responsibilities:

- `quota_bypass_enabled` controls Quota Bypass behavior, including request
  injection, quota auto-pause exemption, same-account retry after eligible 429
  responses, and usage-log metadata.
- `quota_bypass_concentrated_scheduling_enabled` controls only whether eligible
  accounts use the Bypass-first, fill-one-account-before-moving scheduling
  strategy.

## Configuration Semantics

The new setting belongs to a group and defaults to `false`.

Concentrated scheduling is active for a request only when both fields on the
current request group are `true`:

```text
quota_bypass_enabled
AND quota_bypass_concentrated_scheduling_enabled
```

An account-level Quota Bypass flag or another attached group's settings may
make the account eligible for Quota Bypass features, but they do not activate
concentrated scheduling for the current request. This keeps scheduling behavior
request-group scoped and prevents unrelated group membership from changing the
traffic distribution.

The admin UI places the new checkbox next to the existing Quota Bypass checkbox.
It is disabled when Quota Bypass is off. Turning Quota Bypass off also sends the
concentrated scheduling field as `false`, so hidden or stale UI state cannot
reactivate the behavior later without an explicit choice.

## Backend Data Flow

Add a non-null boolean database column with a `false` default and expose it
through the Ent group schema, service group model, admin create/update request
types, response DTOs, repository persistence, API-key group loading, and
scheduler cache snapshots.

The request scheduling context receives the resolved current-group setting.
Quota Bypass eligibility remains unchanged and continues to support account
configuration and attached-group inheritance. A separate concentrated
scheduling predicate requires both current-group switches and is used only by
the scheduler-specific branches.

## Scheduling Behavior

Both the legacy OpenAI scheduler and the advanced account scheduler must follow
the same rule.

When concentrated scheduling is enabled:

- eligible Bypass candidates are prioritized ahead of regular candidates at
  the same scheduling priority;
- the Bypass cursor/window and fill-first ordering remain active;
- soft-sticky requests may yield to the concentrated Bypass pool;
- Bypass-specific acquisition probing and full-account cursor movement remain
  active.

When concentrated scheduling is disabled:

- Bypass accounts participate in the ordinary candidate pool;
- normal load reads, score ordering, sticky behavior, Top-K selection, and
  weighted/random selection apply;
- no Bypass cursor/window state is read or updated for candidate selection.

Quota auto-pause exemption is not a candidate-ordering concern and remains tied
to Quota Bypass eligibility, regardless of the concentrated scheduling switch.
Request injection and 429 retry behavior are also unchanged.

## Compatibility And Error Handling

Existing rows and older clients receive the database/API default `false`, so
deploying this change preserves ordinary scheduling until an administrator
explicitly enables concentration for a group.

Create and update APIs accept the new field using the same optional/boolean
patterns as the existing Quota Bypass field. Cache rebuild and API-key refresh
paths must copy the field so a successful admin update is visible to subsequent
scheduling decisions without requiring a process restart.

If the current group is unavailable, concentrated scheduling stays off. This is
the conservative behavior and matches the current request-group requirement.

## Testing

Implementation follows test-driven development. Focused tests must prove:

- group create, update, DTO mapping, API-key loading, and scheduler cache
  snapshots preserve the new field;
- Quota Bypass enabled with concentration disabled uses ordinary scheduling;
- enabling concentration preserves the existing Bypass-first fill-first order;
- legacy and advanced schedulers enforce the same semantics;
- account-level Bypass and Bypass inherited from another attached group do not
  activate concentration for the current request;
- request injection, quota auto-pause exemption, and eligible 429 retry remain
  active when concentration is disabled;
- the admin form displays, loads, submits, and resets the new field correctly.

Verification includes focused Go tests, relevant frontend type/test checks,
formatting, compilation, and `git diff --check`. Runtime behavior is reported
separately and is not claimed unless exercised against a running service.
