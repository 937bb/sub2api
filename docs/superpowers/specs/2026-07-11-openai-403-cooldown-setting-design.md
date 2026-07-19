# OpenAI 403 Cooldown Setting Design

## Goal

Make only the OpenAI 403 temporary-unschedulable cooldown configurable in seconds. Preserve the current 10-minute behavior by default and leave every other temporary-unschedulable duration unchanged.

## Scope

Add one system setting:

```json
{
  "openai_403_cooldown_seconds": 600
}
```

Contract:

- Default: `600` seconds.
- Valid range: `1` through `86400` seconds, inclusive.
- Zero and negative values are invalid; zero does not disable the protection and does not mean “use default.”
- The setting applies only to the OpenAI 403 cooldown path.
- Account-level temporary-unschedulable rules, stream-timeout settings, transport-error cooldowns, model-not-found cooldowns, token-refresh cooldowns, Antigravity penalties, and all other scheduling policies remain unchanged.

## Architecture

Use the existing dynamic system-settings mechanism and its in-memory snapshot. Do not introduce a YAML/environment option, per-account credential field, or request-time database lookup.

The setting belongs to the rate-limit/scheduling policy boundary:

1. The settings view and persisted settings representation expose `openai_403_cooldown_seconds`.
2. Default construction supplies `600` when the key is absent, preserving old installations and cached data.
3. The admin update path validates `1 <= value <= 86400` and rejects invalid input with a specific validation error.
4. `RateLimitService` reads the value from the existing settings snapshot when applying an OpenAI 403 temporary block.
5. The integer value is converted to `time.Duration(value) * time.Second` only at the policy application point.
6. The existing hard-coded 10-minute constant is removed or retained solely as the named default used by settings initialization/fallback; it is no longer the runtime policy source.

## Backend API

The existing settings read/update DTOs gain:

```go
OpenAI403CooldownSeconds int `json:"openai_403_cooldown_seconds"`
```

Validation error:

```text
openai_403_cooldown_seconds must be between 1-86400
```

Backward compatibility:

- Missing persisted key resolves to `600`.
- Existing clients that omit the field must not accidentally write zero. The update flow must follow the repository’s existing full-settings or merge semantics so omission preserves/defaults the value correctly.
- No database migration is needed if the existing settings store is key/value or JSON-backed, as currently expected; implementation must verify this before editing.

## Frontend

Add one numeric field to the existing scheduling/rate-limit settings area:

- Label: OpenAI 403 cooldown
- Unit: seconds
- Minimum: `1`
- Maximum: `86400`
- Default/display fallback: `600`
- Help text should state that an OpenAI account receiving the applicable 403 response is temporarily excluded from scheduling for this duration.

Do not add controls for other cooldowns in this change.

## Error Handling

- Reject values below `1` or above `86400` at the backend boundary even if frontend validation exists.
- Do not silently clamp invalid values.
- If an old settings snapshot lacks the field, use `600`.
- If corrupted persisted data reaches runtime despite validation, fail safely to `600` rather than disabling the protection or creating an extreme cooldown; log according to existing settings-fallback conventions if such logging already exists.

## Performance

- Read from the existing cached settings snapshot.
- Add no request-time database, Redis, or network operation.
- Add only a fixed integer read and duration conversion to the existing 403 path.
- Do not alter unrelated gateway or scheduler hot paths.

## Tests

Backend tests must cover:

1. Default settings return `600`.
2. Missing legacy value resolves to `600`.
3. Values `1`, `600`, and `86400` are accepted.
4. Values `0`, negative values, and `86401` are rejected.
5. The OpenAI 403 path uses seconds precisely, including a non-minute value such as `61` seconds.
6. The configured value affects only OpenAI 403 temporary-unschedulable handling.
7. Other fixed and configurable temporary-unschedulable durations remain unchanged.
8. Settings access uses the existing snapshot and introduces no repository call in the 403 handling path.

Frontend tests should cover field serialization, displayed default, unit/range attributes, and validation behavior according to existing settings-page test conventions.

## Commit Boundary

Implementation should be one focused commit unless backend and frontend conventions make two independently testable commits clearer. It must not include unrelated settings cleanup, general cooldown unification, or changes under `frontend-react`.
