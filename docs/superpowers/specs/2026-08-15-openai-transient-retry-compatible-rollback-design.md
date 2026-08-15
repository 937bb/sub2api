# OpenAI Transient Retry Compatible Rollback Design

## Goal

Restore the OpenAI overload handling behavior from before commit `41e56d5d0`
without removing database columns that may already exist in deployed environments,
then translate an upstream HTTP 200 stream that fails from explicit load shedding
before client output into an HTTP 503 response.

## Scope

The rollback removes the configurable same-account retry behavior introduced by
`41e56d5d0`. It keeps the group boolean control, renames its UI meaning to
"model load retry", and uses it only to opt into HTTP 503 translation for
classified pre-output load failures. The retry-count control is removed.

The rollback preserves:

- group-level OpenAI model mapping introduced by the same commit;
- migration `224_group_openai_transient_error_retry.sql` and generated Ent fields
  so databases that already applied the migration remain compatible;
- the pre-existing coded SSE capacity-shed handling for
  `server_is_overloaded` and `slow_down`;
- Pool Mode same-account retry behavior;
- Quota Bypass OAuth 429 same-account retry and guarded rate-limit cleanup;
- client-facing capacity error-code rewriting after output has already started.

The retained retry-count database field becomes compatibility-only storage. The
existing boolean field remains in group administration APIs and auth snapshots,
but no longer changes server-side retry counts or scheduling.

## Runtime Behavior

HTTP overload errors return to the pre-`41e56d5d0` failover path. They may move
to another account when the existing failover classifier accepts them, but the
removed group policy no longer forces a bounded same-account retry.

For streaming Responses, a coded `response.failed` event with
`server_is_overloaded` or `slow_down` keeps the existing request-scoped,
same-account retry behavior. Message-only overload variants added by
`41e56d5d0` may be classified as load failures for status translation, but do
not gain the removed group-configured same-account retry behavior.

When the group switch is enabled and an upstream HTTP 200 stream reports an
explicit capacity/load failure before any semantic output reaches the client,
preserve that failure as status 503 through failover exhaustion. The handler
returns HTTP 503 only for this opt-in classified load-shed condition. When the
switch is disabled, the same stream follows the original status behavior.
Generic upstream 500, 502, 503, and 504 errors continue to use the existing
client-facing 502 mapping.

After semantic output has started, the HTTP 200 response is already committed
and cannot be changed to 503. Keep the existing terminal `response.failed`
event and client-retryable error-code rewrite for that path.

Non-streaming SSE-to-JSON and WebSocket paths return to their previous error and
failover behavior. Quota Bypass 429 and Pool Mode policies remain independent
and unchanged.

## Data Compatibility

Do not delete or rewrite migration `224_group_openai_transient_error_retry.sql`.
Keep the two Ent schema fields and generated Ent accessors so automatic schema
checks do not conflict with databases that already contain the columns.

Keep `openai_transient_error_retry_enabled` through handler requests, public
DTOs, service group models, repositories, and API-key auth snapshots. Remove
`openai_transient_error_retry_count` from those runtime/public layers while
leaving its Ent field and migration column in place. Keep auth snapshot version
22 because it still represents the retained model mapping and boolean policy;
JSON decoding safely ignores the removed retry-count property in cached entries.

## Error Handling

Restore the parent commit's capacity classification at each unrelated call site
instead of adding a new global fallback. Preserve request-scoped marking for
the pre-existing coded streaming capacity-shed events and, when the switch is
enabled, for message-only load failures that are translated to 503. This lets
the handler distinguish the opt-in load response from a generic upstream 503.

## Verification

Tests must prove that:

1. the group flag no longer enables configurable same-account retries;
2. with the flag enabled, a pre-output HTTP 200 stream load failure produces a
   failover status of 503 and a final client HTTP status of 503;
3. with the flag disabled, the same stream retains the original status behavior;
4. generic upstream 500, 502, 503, and 504 failures still map to client 502;
5. coded streaming `server_is_overloaded` and `slow_down` still retry on the
   same account before normal failover;
6. Pool Mode and Quota Bypass 429 tests continue to pass;
7. model mapping tests continue to pass;
8. backend focused tests and frontend type checking/build checks pass;
9. only intended source files are staged, excluding deployment environment
   changes.
