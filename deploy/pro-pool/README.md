# P deployment: isolated source based on B

P source: /www/wwwroot/937pro-pool-b-code (branch codex/pool-b-code).
B source remains /www/wwwroot/sub2api-b-v290-merge. Do not fast-forward B for P-only fixes.

Runtime data, PostgreSQL (15433), Redis (26379 DB 0), credentials and simple mode remain independent.
Current image: 937pro-pool:load-2d965fc15.
HTTP: 30482, WS: 30484; stable internal entry: 6066.
Public pool.apihelm.com remains an administration site; inference uses the private entry.

HTTP uses GOMEMLIMIT=7GiB, GOGC=80, a 10 GiB container limit.
WS uses GOMEMLIMIT=1400MiB, GOGC=80, a 2 GiB container limit.
The Go limit is a soft GC target, not a hard cap or proof of absence of memory leaks.

CONCURRENCY_PROCESS_OWNER_FILE must be persisted and unique per listener.
Never share this file between HTTP and WS or rolling candidates. Its flock remains
held for the process lifetime. On a restart only that listener's predecessor
prefix is reaped. Unknown/other process prefixes and aggregate wait counters are
preserved. A locked marker or failed Redis cleanup fails startup without erasing
the predecessor marker. Keep GATEWAY_SCHEDULING_SKIP_STARTUP_SLOT_CLEANUP=true.

The first deployment cannot infer old processes which had no marker. Never clear
all concurrency keys. Verify the old process died and save its exact prefix and
slots before an explicit one-time cleanup.

The live entry currently binds the historical deployment template under
/www/wwwroot/937pro-pool/deploy/pro-pool; its identical configuration is tracked
here for P releases. Update a bind-mounted template in place and verify what the
container sees. Do not run compose up without reviewing container changes.
Runtime environment file: /www/wwwroot/937pro-pool-runtime/.env (never commit it).

Recovery release evidence: /root/candidates/pool-concurrency-audit-20261111.
Backup: /www/backups/937pro-pool/concurrency-recovery-20261111/pool-before.dump.
Backend tests/vet/build and frontend 395 files / 3187 tests/typecheck/build passed.
A candidate-only SIGKILL restart test verified cleanup of its own slots while
preserving another owner. HTTP tool roundtrip and WS two-turn usage passed.

2026-11-12 load repair: app commit 2d965fc15 changes text-role replacement to a
linear pass. P moderation remains enabled. Custom request-body evidence is disabled.
B moderation and body evidence are disabled; normal usage/billing remain enabled.
Evidence: /root/candidates/load-repair-20261112. Separate B app commit b091060fa.
Retain the pre-repair containers for rollback, and drain before stopping them.
No schema migrations or pricing/concurrency-policy changes in this release.

2026-11-12: User requested removal of custom request-body logging. Both P listeners
omit OPENAI_REQUEST_AUDIT_DIR and OPENAI_REQUEST_AUDIT_KEY_FILE. This prevents
initializing the audit store and serializing/redacting/compressing request bodies.
Keep these environment variables absent on future deployments. Content moderation,
usage, billing and normal operational logs remain enabled. Historical encrypted
files were not deleted by this configuration change.
