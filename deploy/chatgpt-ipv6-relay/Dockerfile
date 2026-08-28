# syntax=docker/dockerfile:1.7

FROM golang:1.27.0-alpine AS builder

WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,id=chatgpt-relay-gomod,target=/go/pkg/mod go mod download
COPY backend/ ./
RUN --mount=type=cache,id=chatgpt-relay-gomod,target=/go/pkg/mod \
    --mount=type=cache,id=chatgpt-relay-gobuild,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/chatgpt-ipv6-relay ./cmd/chatgpt-ipv6-relay

FROM alpine:3.21

RUN apk add --no-cache iproute2 su-exec \
    && addgroup -S relay \
    && adduser -S -G relay relay
COPY --from=builder /out/chatgpt-ipv6-relay /app/chatgpt-ipv6-relay
COPY deploy/chatgpt-ipv6-relay/entrypoint.sh /app/entrypoint.sh
RUN chmod 0755 /app/chatgpt-ipv6-relay /app/entrypoint.sh

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -T 2 -O /dev/null http://127.0.0.1:24444/health || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
