# B usage endpoint capacity (2026-11-10)

`/v1/usage` reads authoritative balance/quota before historical statistics. Optional
statistics share a one-second child context; their timeout cannot cancel the
request context or turn an already-read balance into a user-info error. Missing
statistics are omitted, not substituted with zero values.

API-key dashboard aggregates use a per-instance 15-second cache of actual query
results. Refreshes are merged by key, bounded to two concurrent loads with a
12-second deadline, and limited to 1024 stored entries. Cancellation by a caller
does not cancel another caller's shared refresh. Expired entries and entries from
another local calendar day are not returned. Balance, authentication and billing
never use this cache.

Production B HTTP runtime: `DATABASE_MAX_OPEN_CONNS=32`,
`DATABASE_MAX_IDLE_CONNS=8`; B WS: `DATABASE_MAX_OPEN_CONNS=8`,
`DATABASE_MAX_IDLE_CONNS=2`. These are deployment overrides, not changes to all
installations' defaults. Before a rolling deployment, include C and other
applications in the shared PostgreSQL connection budget, check live connection
headroom, and keep monitoring it through the overlap period. The independent Pro
pool uses a different database server and has its own connection budget.

Roll out HTTP and WS together using the existing 6064 protocol router, preserve
production data mounts, and drain previous instances before stopping them. Verify
real HTTP/SSE and WS multi-turn billing as well as repeated `/v1/usage` requests
on previously affected keys. This change adds no schema migration.
