# ChatGPT IPv6 Relay

This optional TCP relay routes only direct `chatgpt.com:443` connections over
source addresses from a server-routed IPv6 `/64`. New Sub2API instances can
request a fixed source address for an account/model/turn-state route ticket;
legacy clients continue receiving a random address per TCP connection. TLS is
passed through unchanged, so Sub2API still validates the original ChatGPT
certificate and SNI.

## Requirements

- A complete IPv6 `/64` routed to the host.
- Docker host networking.
- `NET_ADMIN` for installing the local `/64` route at container startup.

The relay uses `IPV6_FREEBIND` per socket. It does not enable the global
`net.ipv6.ip_nonlocal_bind` sysctl and does not add every generated address to
the network interface.

## Start

```bash
cd deploy/chatgpt-ipv6-relay
RELAY_IPV6_PREFIX=2a02:ae02:1a:2c00::/64 docker compose up -d --build
curl -fsS http://127.0.0.1:24444/health
```

Then enable the Sub2API route:

```bash
GATEWAY_OPENAI_CHATGPT_IPV6_ONLY=true
GATEWAY_OPENAI_CHATGPT_IPV6_RELAY_ADDR=127.0.0.1:24443
GATEWAY_OPENAI_CHATGPT_IPV6_PREFIX=2a02:ae02:1a:2c00::/64
```

Without a scanner-verified route ticket, accounts with an explicit HTTP,
HTTPS, or SOCKS proxy continue to use that proxy. A verified turn-state with a
bound IPv6 takes precedence over the account proxy so its HTTP and WebSocket
connections keep the same server IPv6 for the state lifetime. `api.openai.com`
and all non-ChatGPT hosts bypass this relay.
