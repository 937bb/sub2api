# B official runtime restoration

Base: Wei-Shaw/sub2api v0.2.19, upstream commit
`d851a38477597157e78450d30af2d8504764d085`.

The backend and Dockerfile are restored to this official revision, except for
the EPUSDT payment integration and its order, quote, fee and webhook support.
USDT orders continue using two decimal places. The shared payment gateway is
not changed by this release.

The B interface, homepage, navigation and loading styles remain. UI controls
for removed custom APIs (SIWC, distillation monitoring, custom response traces
and holiday exclusions) are removed or restored to upstream behavior.

Runtime rollout uses new instances and a graceful nginx reload. New traffic
bypasses the custom fingerprint ingress. Gateway configuration uses official
defaults, including official WebSocket support. The custom account IPv6
binding triggers are disabled; automatically assigned account proxies revert
to their saved original values, while independently edited proxies remain.
Existing instances and IPv6 proxies remain available to drain established
connections. No account, balance, order or historical database columns are
deleted. No new migrations are required by this restoration.

Restoring upstream behavior does not establish the cause of upstream HTTP 429
responses or guarantee their disappearance. Account limits and upstream
responses must still be assessed after the cutover.
