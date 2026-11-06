# SmartWS capacity diagnostics and tuning

This change makes existing local resource budgets configurable and adds bounded,
process-local account diagnostics. It does not increase provider quotas, change
account concurrency, rotate egress, multiplex generations, or replay sent requests.
The unchanged defaults are the production baseline. There are no migrations.

## Configuration

Set these environment variables before starting the B runtime. Configuration is
read at process initialization, not hot reloaded. Apply the same intended settings
to both HTTP/SSE and native WS runtimes; each process has its own pool and counters.
Native WS uses a separate pool: these options affect only the SmartWS HTTP adapter.

| Variable | Default | Allowed override |
| --- | --- | --- |
| `SMART_WS_IDENTITY_CAPACITY` | 64 | 1–256, at most account capacity |
| `SMART_WS_ACCOUNT_CAPACITY` | 256 | At least identity capacity, at most 512 |
| `SMART_WS_CAPACITY_LIMIT` | 4096 | 128–8192, at least account capacity |
| `SMART_WS_IDLE_SECONDS` | 300 | 30–900 |
| `SMART_WS_COLD_WAIT_MS` | 0 | 0–500 |

Zero or an absent variable selects its existing default. Invalid input rejects the
entire override set, retains production defaults, and
exposes `runtime.tuning_error` in the administrator endpoint. No raw environment
value appears in that error. These are local resource ceilings, not a promise of
simultaneous successful generations. Account scheduling concurrency remains an
independent limit.

The 20-minute maximum connection lifetime, 0.9/s account handshake pacing, burst 2,
concurrent handshakes 2, global queue 256, account queue 32, queue memory 64 MiB,
and maximum 2500ms admission wait remain unchanged.

A positive cold-wait setting permits a new **reusable, isolated** identity to wait
briefly for the normal account dial budget. It does not create extra permits.
A request without a reliable session still falls back immediately. Explicit
upstream HTTP errors keep their existing propagation and cooldown handling.
Queue and byte bounds, caller cancellation, and exact affinity remain in force.
The body is not sent while waiting. After dispatch, failures never trigger an
automatic HTTP replay. Cold waiting trades additional admission latency for a
chance to use WS; it does not guarantee higher throughput.

## Diagnostics

`GET /api/v1/admin/groups/:id/smart-ws` returns the existing group policy plus
`runtime`. Runtime is explicitly scoped to the responding **process**, covering
all groups; it is not filtered by the requested group and is not fleet-wide.
The existing group SmartWS dialog displays the snapshot taken when opened.

`runtime.eligibility` counts HTTP adapter invocations by their first rejection
reason: unsupported request/account, forced HTTP, nonpositive concurrency, global
WS disabled, account mode, missing trusted identity, or disabled group policy.
`pool_attempt` counts invocations reaching the pool, not successful WS requests.
These fixed counters reset on process restart. Native WS does not traverse this
adapter; zero pool counters alone cannot establish which transport served traffic.

`runtime.accounts` exposes upstream account hashes and bounded internal account ID
lists (up to 16 IDs when duplicate database accounts share an upstream identity).
No tokens, proxy credentials, session IDs, prompt text, or raw provider account IDs
are exposed. There are at most 256 account states. States retire after the existing
quiet period and may reset counters; restarts also reset all counters. Monitor
restarts and counter decreases before computing interval deltas.

For each account:

- Gauges: connections, active leases, idle connections, opening connections,
  queued requests and retained queue bytes. Active leases are not proof that the
  provider is currently generating tokens.
- Counters: successful opens, reuse, completed responses, terminal errors,
  stream failures, handshake outcomes and **handshake-only** 429, fallback causes,
  idle expiration/reclamation, reader closure and release closure causes.
- Admission duration: `acquire_count`, `acquire_total_us`, and cumulative buckets
  `acquire_le_10ms`, `50ms`, `100ms`, `250ms`, `500ms`, `2500ms`. These exclude
  handshakes and inference time; they are not TTFT percentiles. Slow observations
  above 2500ms are represented by count minus the last bucket.
- Reuse fraction: reused / (reused + opened). For experiments, prefer interval
  deltas to lifetime ratios. Acquisition and opening counts exclude requests
  rejected before admission.

Monitor the existing upstream error records separately for generated-response
rate limits and quota exhaustion; handshake 429 is not the total 429 count.

## Controlled rollout

1. Deploy instrumentation with no overrides. Capture account-level baseline,
   original upstream errors, successful completions and end-to-end latency.
2. If `fallback_identity_capacity_full` is material, trial a higher identity
   capacity on an isolated candidate, within the existing account and global
   budgets. Do not remove user, billing, credential, session or proxy isolation.
3. If cold fast fallback is material and there is admission-latency headroom,
   trial `SMART_WS_COLD_WAIT_MS=100`, then 250 only if completion throughput
   improves. Compare fallback reduction against added queue time.
4. Increase idle retention only if idle expiry is followed by repeated new dials
   for compatible sessions, accounting for the fixed 20-minute lifetime and
   per-account/global memory limits.
5. Compare fixed model, input/output sizes and session mix at 100, 110, 120, 135,
   and 150 offered concurrency. Record actual active work separately from queue
   occupancy. Provider limits are authoritative; do not repeatedly retry quota
   exhaustion or use changed egress to evade enforcement.

Suggested experiment stop criteria, calibrated to the baseline: success rate
falls by over one percentage point, TTFT p95 rises over 20%, sustained upstream
429 increase, or memory/FD pressure. Roll back the override and allow existing
responses to drain. A release alone is not evidence of a 150-concurrency capacity.
