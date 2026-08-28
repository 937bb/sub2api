# ChatGPT IPv6 Relay

This optional TCP relay routes only direct `chatgpt.com:443` connections over
random source addresses from a server-routed IPv6 `/64`. TLS is passed through
unchanged, so Sub2API still validates the original ChatGPT certificate and SNI.

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
```

Accounts with an explicit HTTP, HTTPS, or SOCKS proxy continue to use that
proxy. `api.openai.com` and all non-ChatGPT hosts bypass this relay.
