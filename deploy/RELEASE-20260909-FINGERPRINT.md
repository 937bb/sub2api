# 937sub2b Fingerprint Release

## Release Scope

- Version: `0.2.1.7`.
- Image: `sub2api-b:937sub2b-0.2.1.7-fingerprint`.
- Base: the previously deployed `d0ff1b903` (`0.2.1.6`).
- This is an isolated fingerprint backport, not a deployment of all local
  `6d74439e` merge changes. Unfinished group/admin changes were excluded.
- Binary SHA-256: `a5026f93c831802464fd0acd46da309092e7e78ab51bf141b6ac6a2b54c592d1`.
- The frontend is unchanged: all 177 assets matched the previous release.
- OAuth/PAT seeds are backfilled without replacing valid seeds or changing
  explicitly disabled accounts. API-key accounts are excluded.
- The existing global `openai_codex_fingerprint_default_full_enabled=false`
  setting and explicit account modes were preserved. Independent account UA
  selection must not be confused with enabling full session convergence.

## Live Routing

Host: `84.32.220.70`.

Both B domains (`tob.937ai.chat`, `tob.apihelm.com`) and the C application's
upstream configuration continue to use port `6064`. The C application itself
listens on `26063` and was not restarted. CPA and CPAMP were not changed.

During drain, the validated `sub2api-b-fingerprint` container listens on
`26064`. Scoped IPv4/IPv6 NAT rules redirect only new connections addressed to
local port `6064`; established connections retain their original destination.
Source `127.0.0.2` bypasses this temporary IPv4 routing for direct readiness
checks. Do not force-stop an old listener with established connections.

The server previously allocated ephemeral ports from `1024-65535` with no
reserved ports. `/etc/sysctl.d/98-sub2api-b-listen-ports.conf` now reserves
`6064,16068,26064`, preventing listener-port allocation during deployment gaps.

## Automatic Finalization

Deployment directory:
`/www/wwwroot/sub2api-b/releases/0.2.1.7-fingerprint`.

Units:

- `937sub2b-fingerprint-route.service`: restores temporary routing on reboot
  while the old primary is still deployed; retains the original listener if
  the candidate is unavailable.
- `937sub2b-fingerprint-finalize.timer`: checks every 30 seconds.
- `937sub2b-fingerprint-finalize.service`: replaces the primary only when its
  established connection count is zero. It then removes new-connection NAT,
  retains the candidate until its connections also drain, and disables the
  release units after cleanup.

Inspect actual state before any later deployment:

```bash
docker inspect sub2api-b sub2api-b-fingerprint --format '{{.Name}} {{.Config.Image}} {{.State.Status}}'
ss -Htan state established '( sport = :6064 or sport = :26064 )'
systemctl status 937sub2b-fingerprint-finalize.timer
journalctl -u 937sub2b-fingerprint-finalize.service -n 20 --no-pager
```

The finalizer checks exact expected image names and does not mutate a primary
that another release has replaced. Stop its timer before a different rollout;
inspect the remaining NAT rules and live listener state before changing them.

## Verification and Recovery

- Targeted Codex/fingerprint/WS/PAT service tests passed.
- Repository/migration tests, handler/server compile checks and `go vet` passed.
- Migration was first verified against a temporary table in a rolled-back
  transaction, including OAuth/PAT, preserved seeds, explicit off and API keys.
- HTTP and native WS smoke requests both reached `response.completed`.
- The existing C-side Pro upstream key completed an HTTP request through `6064`.
- All 1080 cutover health/login checks returned 200.
- Full golangci-lint was run but did not pass: 17 findings remain, including
  existing branch checks and an unused compatibility wrapper. Its full output
  is in the protected release directory; this is not a clean-lint release.

The release directory contains `docker-compose.before.yml`, the original
fingerprint seed/mode snapshot, verification logs, a release manifest, and a
14 MB source archive. These files are root-only because configuration snapshots
can contain sensitive deployment values. Do not print their contents wholesale.

Before primary replacement, disabling the finalizer timer and running
`route-candidate.sh off` restores new connections to the original `6064`
listener without terminating existing candidate connections. After primary
replacement, recovery must first route new connections to a healthy candidate,
wait for the primary to drain, then restore `docker-compose.before.yml` and
recreate only the `sub2api-b` service. Never use `docker compose down` for this
deployment. Keep the source archive and prior image until rollback is no longer
needed.
