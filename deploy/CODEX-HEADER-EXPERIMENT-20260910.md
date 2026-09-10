# Codex header comparison, 2026-09-10

Controlled comparisons did not establish that changing the environment UA,
originator, version, or legacy beta header improves completion rate. Keep the
0.2.4.3 production identity rules and settings until a candidate demonstrates a
repeatable benefit. The opt-in diagnostic is separate from the capacity recovery
fix described below, released as 0.2.4.4.

## Method

The diagnostic uses the production HTTP/WS header builders, account identity
projection, configured fingerprint mode, and WS dialer. It operates on authorized
account snapshots supplied through stdin. It never accesses a repository or
updates account credentials, cooldowns, quotas, scheduling, or settings. Requests
consume a small amount of real upstream usage.

The model was `gpt-5.6-sol`, with the same instructions and short input throughout.
Variants were interleaved within each account and their order reversed in the
second round. A successful observation requires `response.completed`; HTTP 200,
WS 101, or a text delta alone is insufficient. A 35-second observation limit
applies only to the diagnostic, not production requests.

The first comparison used the production IPv6 relay. Separate WS handshakes can
use different source addresses, so those results cannot isolate the UA effect.
Follow-up comparisons bound only the diagnostic sockets to one IPv6 address
already assigned to the server. Production network configuration was unchanged.
HTTP requests reuse an account-specific HTTP/2 client. Only completed WS turns
may reuse their diagnostic connection; error frames can precede a trailing
`response.failed`, so failed sockets are closed to prevent misattribution.

## Completed comparisons

Counts are completed requests / attempted requests, not production success-rate
estimates. Small samples and changing upstream conditions limit inference.

| Header variant | Relay WS | Relay HTTP |
| --- | ---: | ---: |
| Existing account UA, version 0.154.0 | 3/4 | 2/4 |
| Supplied Mac environment, version 0.154.0 | 4/4 | 2/4 |
| Supplied Mac environment, exact version 0.153.3 | 2/4 | 2/4 |
| `codex_cli_rs` identity, version 0.154.0 | 2/4 | 2/4 |
| Existing UA, no synthesized beta-features header | 2/4 | 2/4 |

The relay HTTP results split by account: one completed all ten observations and
the other completed none, regardless of variant. Failures included HTTP 429 and
HTTP 200 containing `server_is_overloaded`.

The original failing account was subsequently removed by another live operation.
The runner rejected that deleted account before sending another request. A
currently active account was selected for the follow-up; results from these
different account cohorts must not be treated as a before/after comparison.

| Fixed-source WS, three rounds | Existing account UA | Mac UA, 0.154.0 |
| --- | ---: | ---: |
| Normally completing account | 3/3 | 3/3 |
| Account returning overload errors | 0/3 | 0/3 |

The Mac variant on the failing account produced an overload error, a server error,
and a request that emitted text but did not complete within the observation limit.
Changing the UA did not recover successful completion on that account.

An initial fixed-source run was excluded from these counts because its diagnostic
retained sockets after error events, allowing a trailing failure frame to enter
the next observation. The diagnostic was corrected and the entire fixed-source
WS comparison repeated. The production pooled forwarder already retires sockets
after error events; this was a diagnostic issue, not a newly discovered gateway
bug. That excluded run also observed intermittent plain-text 403 handshakes
served through Cloudflare. The server header alone does not establish which
upstream component rejected the connection or why.

## Operational implications

The final fixed-source HTTP comparison also tested the legacy
`OpenAI-Beta: responses=experimental`: the normally completing account was 2/2
with and without it, while the failing account was 0/2 with and without it.
Adding that header did not recover completion. The comparisons above total 60
valid observations, excluding the superseded diagnostic run.

## Capacity recovery fix (0.2.4.4)

Inspection of the HTTP-to-WS pooled forwarder found that
`error.code=server_is_overloaded` did not enter the gateway's typed failover path.
It flushed a raw failure instead; a standalone overloaded `response.failed`
could return a nil forwarding error. HTTP/SSE already used typed recovery for
these events. Regression tests reproduced both failures on the unchanged base.

For OAuth/PAT, a capacity error received before any downstream output now enters
that existing bounded HTTP/SSE recovery path. The failed socket is retired so a
trailing failure event cannot leak into a later request. This adds no nested WS
retries, increases no retry budget, and introduces no account cooldown or UA
rotation. After any text/tool output has been delivered, the turn is not replayed.
API-key and native downstream WS behavior are unchanged.

This repairs a concrete early-failure recovery gap. It does not create upstream
capacity or guarantee an increase in production success rate when every eligible
account is unavailable. No database or frontend changes are required.

## Header rules retained

- Keep UA, originator, and version paired from one effective version. An explicit
  account UA already takes precedence over the panel UA and generated profile.
- Preserve `responses_websockets=2026-02-06` for WS negotiation. Do not replace it
  with the legacy HTTP beta token.
- Preserve account seeds, installation IDs, and healthy WS connections. Do not
  rotate identity on each error or infer account revocation from handshake 403.
- Track handshake rejection, stream overload, rate limiting, and incomplete
  streams separately. These observations do not demonstrate a header remedy for
  upstream capacity or account limits.
- Do not invent attestation or enable Responses Lite solely as a header trick.
  Lite has payload/tool requirements and can reintroduce protocol errors.

Current official Codex [client source](https://github.com/openai/codex/blob/main/codex-rs/core/src/client.rs)
builds shared response headers, separate WS beta negotiation, and explicit Lite
options. It does not promise that a particular OS/terminal UA avoids overload.

## Running the diagnostic

Build from `backend` with an explicit build tag:

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -ldflags='-s -w' \
  -tags codexdiagnostic -o codex-header-diagnostic ./internal/service
CODEX_HEADER_DIAGNOSTIC=1 GIN_MODE=release ./codex-header-diagnostic \
  -test.run='^TestCodexHeaderLiveDiagnostic$' -test.timeout=15m \
  < /secure/authorized-account-snapshots.json
```

The stdin object accepts `Accounts` (up to three direct OAuth/PAT service account
snapshots), `Models` (up to two), `Transports` (`http`, `ws`), `Variants`,
`Version`, `Rounds` (one to three), `DefaultFull`, `Relay`, and optional
`SourceIPv6`. Tokens belong only in the protected stdin data. Do not paste real
snapshots into issues or logs. With no explicit enablement the test skips, and
without the build tag it is not compiled into tests or the server.

Output is bounded JSON with status, completion, latency, safe header identity,
and error classification. A test-process `PASS` means the experiment finished;
individual `success` fields determine upstream outcomes. Account repositories,
scheduler retries, live connection pools, and production load distribution are
intentionally outside this experiment.
