# OpenAI Transient Retry Compatible Rollback Design

## Goal

Restore the OpenAI overload handling behavior from before commit `41e56d5d0`
without removing database columns that may already exist in deployed environments.

## Scope

The rollback removes the group-level transient overload retry control introduced
by `41e56d5d0` from the admin UI, API DTOs, auth snapshots, service policy, and
OpenAI HTTP, SSE, passthrough, and WebSocket request paths.

The rollback preserves:

- group-level OpenAI model mapping introduced by the same commit;
- migration `224_group_openai_transient_error_retry.sql` and generated Ent fields
  so databases that already applied the migration remain compatible;
- the pre-existing coded SSE capacity-shed handling for
  `server_is_overloaded` and `slow_down`;
- Pool Mode same-account retry behavior;
- Quota Bypass OAuth 429 same-account retry and guarded rate-limit cleanup;
- client-facing capacity error-code rewriting after output has already started.

The retained database fields become compatibility-only storage and no longer
affect runtime scheduling or appear in group administration APIs.

## Runtime Behavior

HTTP overload errors return to the pre-`41e56d5d0` failover path. They may move
to another account when the existing failover classifier accepts them, but the
removed group policy no longer forces a bounded same-account retry.

For streaming Responses, a coded `response.failed` event with
`server_is_overloaded` or `slow_down` keeps the existing request-scoped,
same-account retry behavior. Message-only overload variants added by
`41e56d5d0` no longer receive special treatment.

Non-streaming SSE-to-JSON and WebSocket paths return to their previous error and
failover behavior. Quota Bypass 429 and Pool Mode policies remain independent
and unchanged.

## Data Compatibility

Do not delete or rewrite migration `224_group_openai_transient_error_retry.sql`.
Keep the two Ent schema fields and generated Ent accessors so automatic schema
checks do not conflict with databases that already contain the columns.

Remove the fields from handler requests, public DTOs, service group models, and
API-key auth snapshots. Existing column values are ignored by runtime behavior.

Because the auth snapshot shape changes, increment its schema version so Redis
does not hydrate stale snapshots containing the removed runtime policy.

## Error Handling

Restore the parent commit's capacity classification at each touched call site
instead of adding a new global fallback. Preserve request-scoped marking only
for the pre-existing coded streaming capacity-shed events, preventing those
events from temporarily quarantining an otherwise healthy account.

## Verification

Tests must prove that:

1. a group flag can no longer enable generic same-account overload retries;
2. coded streaming `server_is_overloaded` and `slow_down` still retry on the
   same account before normal failover;
3. Pool Mode and Quota Bypass 429 tests continue to pass;
4. model mapping tests continue to pass;
5. backend focused tests and frontend type checking/build checks pass;
6. only intended source files are staged, excluding deployment environment
   changes.
