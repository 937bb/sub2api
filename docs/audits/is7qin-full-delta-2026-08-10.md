# is7Qin Full Delta Audit (2026-08-10)

## Scope

- Authoritative upstream baseline: `Wei-Shaw/sub2api` branch `main` at `10a4c6e3ad319587e817109c071259269855ec30` (2026-08-10, version `0.1.173`).
- Released donor source: `is7qin/local` at `54fe05f1fef211c9a7b6361e43d4cb229eb67757` (2026-07-20, version `1.1.0`).
- Integration branch before this correction: `937sub2b` at `d145253ff37a0c5f9b6c3be07a83ae869ef57fab`.
- The official baseline and donor release share ancestor `aa69e3947dac0282c5973bc3a51fadf058bbc9ca`. From that ancestor, the donor branch is 506 commits ahead and official `main` is 1999 commits ahead.
- The donor side has 488 non-merge commits. Patch-equivalence comparison against official `main` classifies 433 as having no equivalent official patch and 55 as already represented upstream.
- The ancestor-to-donor diff changes 950 paths with 96,118 insertions and 11,293 deletions. This is a branch-divergence measurement, not a pure custom patch set, because both repositories continued development after the shared ancestor.
- `is7qin/dev` at `d4acaace4b5886baad2d940e43129b3c232d6806` is only an optional follow-up reference for later corrections to donor-owned features. It is not an upstream baseline and is not used to decide whether official functionality is present or missing.

This was a semantic audit against official `main`. File-level copying was rejected because the donor release is based on an older service, scheduler, migration, and frontend layout.

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

The audit conclusion is not that every donor file was copied. Donor release candidates were measured against official `main`, then only donor-specific behavior that remains valid against the current architecture was retained or reimplemented. Donor `dev` was consulted only where a later fix clarified behavior already owned by the donor feature.
