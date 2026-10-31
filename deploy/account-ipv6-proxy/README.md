# B OAuth fixed IPv6 egress

Deployed 2026-10-31 for the 1,073 existing, non-deleted OpenAI OAuth accounts.
Each persisted account ID has a dedicated proxy record and source address:
`2a02:ae02:1a:2c00:937b:: + account ID`. Addresses do not depend on tokens,
model, request, or process lifetime. API-key accounts are outside this scope.

The loopback-only CONNECT adapter listens on `127.0.0.1:26443`. Per-account
proxy credentials select an immutable mapping loaded from a private config.
ChatGPT TLS tunnels use the existing relay at `127.0.0.1:25443`, with its
`S2RLYIP6` version 1 source-address preface. Auth/API OpenAI tunnels bind the
same IPv6 directly using Linux IPV6_FREEBIND. Only these three hosts on port
443 are allowed. TLS remains end-to-end. There is no IPv4 or random-IP fallback.

Existing account proxy support carries the binding through HTTP, WS, token
refresh, usage and privacy requests. Proxy-specific connection caches separate
accounts. Established pre-change connections drain normally; new connections
use the binding. Existing relay services and their other clients are unchanged.

Runtime: `/root/candidates/b-account-ipv6-20261031`, container
`sub2api-b-account-ipv6`, image `sub2api-b:account-ipv6-20261031`.
The parent directory is mode 0700. Do not commit generated configs or secrets.
The supplied scripts are records of this deployment, not a general migration:
`prepare.py` refuses to overwrite its backup; `bind.py` checks original proxy
ownership and writes scheduler events in the same transaction; `rollback.py`
restores original bindings only where this release still owns the proxy.
Leave the adapter running when rolling back so active tunnels can finish.

Validation: unit checks for authentication and target restrictions; 12 fresh TLS
trace connections across three hosts and two accounts all matched assigned IPs;
two recently active accounts returned 200 from authenticated read-only usage;
1,073 distinct account/proxy mappings, zero unbound OAuth accounts, refreshed
scheduler snapshots, 115 observed live IPv6 connections, zero adapter dial errors.

Build on Linux: `CGO_ENABLED=0 GO111MODULE=off go build -o account-ipv6-proxy main.go`.
Tests: `GO111MODULE=off go test main.go main_test.go`.

New accounts are NOT automatically provisioned by this deployment. Add a unique
mapping and proxy binding before enabling a newly imported account. Config is
loaded at process start; deploy a second adapter instance and drain the old one
when changing mappings to avoid closing active streams.
