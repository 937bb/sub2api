# Codex state acquisition: 0.2.6.16

State scans now start the configured batch concurrently, including the first
attempt. The maximum is five probes per account/model job and four active jobs
per process. A valid result at the first configured length cancels sibling
requests; fallback results remain usable when no preferred result is returned.
HTTP status, actual returned model, timestamp and remaining lifetime are checked.

The scan-proxy page includes persistent settings for ordered target lengths,
parallel probes and a dynamic proxy source. Defaults are `[332, 292]`, five
probes, and dynamic proxies disabled. Lengths are a local selection policy;
they do not establish upstream quality, quota, or model capabilities.

When enabled, the dynamic source is used only for state acquisition. It never
modifies business proxy records or account bindings. Provider and proxy checks
use independent clients, normal certificate verification, bounded public-only
connections, and no account authorization headers. Failure does not silently
fall back to a direct business connection.

The supplied random provider can return the same rotating gateway repeatedly.
Independent exit checks are retained and deduplicated by observed exit IP,
preferring different observed countries. Each subsequent state probe opens a
fresh CONNECT. A gateway can rotate again at that point, so preflight geography
does not guarantee the geography of the later state request. Up to two provider
batches and ten exit checks are attempted within 25 seconds.

Valid configured states suppress automatic acquisition until the existing
refresh window. Repeated values do not extend their issuance-based expiry.
Account/model leases fence scan status writes; the 75-second acquisition budget
fits inside the 90-second lease. Workers reload the persisted bucket after
claiming a lease, and successful acquisition is persisted before lease release.
State values never move between accounts or models.

## Deployment checks

- Run affected service, repository, handler, routing and migration tests.
- Run the state concurrency/race tests, Go vet and an embedded backend build.
- Set `SUB2API_STATE_TEST_DSN` to an isolated PostgreSQL database and run
  `go test ./internal/repository -run '^TestCodexTurnStateScanPostgresBinding$'`.
  This checks real parameter inference and lease fencing using temporary tables;
  mocks cannot detect PostgreSQL's VARCHAR/TEXT inference conflicts.
- Run frontend i18n, type checking, tests, lint and production build.
- Start the new image on a separate port and verify health, login HTML, actual
  JS/CSS assets, admin settings persistence and a bounded API request.
- Route new B-end connections only after candidate checks pass. Preserve active
  requests while old connections drain; preserve the primary port 6064 used by
  the C-end. Do not restart C-end, databases or other projects.
- Configure the dynamic source through the authenticated admin endpoint and
  verify bounded probe counts and model-specific outcomes without logging tokens.
