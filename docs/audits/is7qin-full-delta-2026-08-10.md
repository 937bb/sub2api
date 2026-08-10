# is7Qin Full Delta Audit (2026-08-10)

## Scope

- Released donor branch: `is7qin/local` at `54fe05f1fef211c9a7b6361e43d4cb229eb67757` (2026-07-20).
- Latest donor integration branch: `is7qin/dev` at `d4acaace4b5886baad2d940e43129b3c232d6806` (2026-08-10).
- Integration branch before this follow-up: `937sub2b` at `6e566f9b830b00a35398154635a2ab571315f6a8`.
- The released donor delta from the shared base contains 488 non-merge commits and changes 950 paths. Of 218 added paths, 115 do not exist under the same name in the current tree.
- Donor `dev` and `local` diverged after `3a55ad2b8e03ff2dab09bb1929a9126f3f4e8c10`: `dev` is 402 commits ahead and `local` is 14 commits ahead. Both sides were inspected because `local` is released but old, while `dev` contains later corrections.

This was a semantic audit. File-level copying was rejected because the donor release is based on an older service, scheduler, migration, and frontend layout.

## Ported Or Reimplemented

- OpenAI dynamic 429 scheduling was adapted to the current rate-limit and scheduler services. The default remains disabled. The current implementation includes the later donor corrections for exact `plan_type` policies, optional 5-hour/7-day usage gates, missing-usage fallback age, a 30-day maximum pause, compiled policy lookup, and singleflight-backed settings loading.
- Dynamic 429 success samples now flow through the existing HTTP and WebSocket scheduling result path. Manual rate-limit and temporary-unschedulable resets clear the sampling window.
- OpenAI OAuth-like accounts receive a stable server-owned Codex installation ID. It is generated on account creation, lazily backfilled for existing accounts, reused after a failed backfill, and applied consistently to HTTP headers, WebSocket headers, and request metadata through the current identity pipeline.
- OpenAI authorization callback state can use Redis through the current Redis infrastructure. Redis remains authoritative after a successful write, with memory fallback only for sessions whose Redis write failed. This stores short-lived OAuth login state only; it does not restore request prewarm or retained upstream sessions.
- Admin subscription group switching was adapted to the current transaction and cache model. It locks the subscription row, validates the destination group, preserves the subscription term and usage, and maps conflicts deterministically.
- Previously integrated donor quota-reset, per-key priority, request fingerprint alignment, Personal Access Token, and subscription fixes were retained where they already have current-tree implementations.

## Covered By Newer Current Code

- Account extra redaction is handled by the current DTO mapper and includes newer sensitive fields.
- Model-not-found behavior is handled by the current no-account error and model-availability pipeline.
- Codex instructions and request shaping use the current model-aware prompt and transform pipeline.
- Antigravity project ID fallback exists in the current resolver.
- OpenAI PAT import, refresh, plan synchronization, account testing, and generic batch refresh are already integrated in the current account services.
- OpenAI user-agent, client-version, originator, request metadata, HTTP/WS identity enforcement, and redaction are newer than the donor fingerprint stack. Only the missing stable installation-ID behavior was forward-ported.
- Client-disconnect stream draining and billing preservation exist across the current gateway handlers and use the current concurrency and billing ownership rules.
- The current scheduler uses snapshot, projection, runtime-block, and model-availability components that do not map safely to the donor release's dirty-work files.

## Intentionally Not Copied

- The donor release's dirty-work scheduler, PostgreSQL array helper, and migrations 161-165 were not copied. They target an older repository and cache contract.
- The donor `dev` support-decision publisher/replica, worker-runtime, billing-outbox, and upstream-error architecture was not overlaid. It is a separate experimental architecture with broad schema and lifecycle ownership changes, not an isolated quota extension.
- The old OAuth strict request-body allowlists and startup config rejection were not copied. They would discard fields supported by the current request pipeline and reject current WebSocket/account modes.
- The old full Codex fingerprint object and per-account user-agent snapshot were not copied. The current centralized identity policy is newer; only its missing stable installation ID was added.
- The completed-response snapshot and physical HTTP-attempt admission layer were not copied mechanically. They alter cancellation and resource ownership for every provider and require a dedicated forward-port against current handler semantics.
- Request-session prewarming/retention and the later donor WebSocket prewarm continuation were not restored. The branch explicitly reverted that behavior.
- Old frontend redesign, deployment layouts, generated Ent code, historical migrations, and donor-only test scaffolding were not copied.

## Verification Contract

- Dynamic 429 tests cover thresholds, Setup Token accounts, reset behavior, exact plan policy selection, usage gates, missing-data fallback, duplicate plans, and runtime blocking.
- Codex installation-ID tests cover creation, stability after persistence failure, request-local account cloning, inbound-header override, and body/header agreement.
- OAuth Redis tests cover successful persistence, authoritative misses, expired data, deletion, and memory fallback after Redis write failure.
- Subscription switching tests cover success, idempotence, invalid groups, no active subscription, and write conflicts.

The audit conclusion is not that every donor file was copied. All released and latest donor deltas were classified, and only donor-specific behavior that remains valid against the current architecture was retained or reimplemented.
