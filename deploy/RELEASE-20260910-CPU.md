# 937sub2b 0.2.4.2: reduce large-request CPU and allocation costs

Repeated JSON processing was the main application CPU hotspot during the
September 10 incident. A 20-second sample attributed about 60.6% of leaf CPU to
gjson/sjson and encoding/json. Frequent upstream 429 responses and failover
repeated the request processing; the sample does not identify the provider's
underlying reason for those 429 responses.

Ordinary Responses requests now skip full bootstrap decoding when the necessary
automation/delegation envelope is absent. Requests with matching tool names still
receive the original duplicate-key and context validation, including Unicode
escapes. Native Responses ingress avoids a full object tree when legacy fields
are absent, retaining JSON validation and existing decoder errors.

Quota classification and injection share one immutable input inspection within
each forwarding attempt. Array iteration avoids full input copies and temporary
item slices. The synthetic suffix is built separately and inserted with one body
copy. Original image content, numeric spellings, whitespace and tool history are
preserved. Native tool outputs and compaction keep their existing behavior.
Compaction and regex checks also use necessary-marker prefilters.

This release does not change inference concurrency, account selection, 429 retry
policy, quota eligibility, fingerprints, WS configuration, or database schema.
No mutable body or account-dependent decision is cached across requests.

## Measured local costs

Representative 4 MiB requests, Apple M4, Go 1.27; rounded benchmark results:

| Operation | Before | After | Allocated bytes before / after |
| --- | ---: | ---: | ---: |
| Ordinary bootstrap checks | 20.4 ms | 0.20 ms | 83.9 MB / 0 |
| Bootstrap checks with Unicode output | 27.6 ms | 1.45 ms | 100.7 MB / 64 B |
| Quota injection | 8.25 ms | 1.51 ms | 16.8 MB / 4.21 MB |
| Quota classification | 6.35 ms | 4.37 ms | 8.4 MB / 0 |
| Compaction detection | 1.48 ms | 0.082 ms | 4.2 MB / 0 |
| Native legacy-ingress check | 5.84 ms | 4.52 ms | 21 MB / no per-request allocation |

These are individual operation benchmarks, not end-to-end throughput guarantees
or measured production CPU reductions. Actual schema repair still incurs its
existing parsing cost. Upstream 429 responses can still occur.

## Validation and deployment

The default full Go test suite passes, including new regression cases for
escaped bootstrap and legacy keys, escaped regex markers, exact numbers,
unchanged input buffers, compaction and genuine tool continuations.
`go vet` passes, and golangci-lint reports no new issues relative to `33005dae`.
The previously documented unrelated B lint findings remain outside this change.

Deploy only B using a candidate listener on 36064. Keep C's upstream at
`http://127.0.0.1:6064` and retain all existing embedded frontend assets.
Validate pages, asset hashes, administrator reads, HTTP streaming, native WS,
and tool continuation before routing new connections. Keep existing connections
on their listener until they drain; do not stop an active listener or impose a
forced drain deadline. A release-specific finalizer may complete this after the
interactive deployment session.

Operational artifacts and rollback records belong under
`/var/tmp/sub2api-cpu0242-20260910` and
`/www/wwwroot/sub2api-b/releases/0.2.4.2`. While the old primary remains active,
rollback by disabling this release's finalizer and removing its new-connection
redirect. After primary replacement, route through a healthy candidate, drain
the primary, and restore its protected Compose snapshot. Never restore the
database as part of this code-only rollback.
