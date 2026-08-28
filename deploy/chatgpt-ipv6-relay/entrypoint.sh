#!/bin/sh
set -eu

: "${RELAY_IPV6_PREFIX:?RELAY_IPV6_PREFIX is required}"

case "${RELAY_IPV6_PREFIX}" in
    */64) ;;
    *)
        echo "RELAY_IPV6_PREFIX must be an IPv6 /64" >&2
        exit 1
        ;;
esac

ip -6 route replace local "${RELAY_IPV6_PREFIX}" dev lo table local
exec su-exec relay /app/chatgpt-ipv6-relay
