# Local WebSocket patch

Base: github.com/coder/websocket v1.8.14, retained under its original ISC license
in LICENSE.txt. The root Go module uses a local replace directive.

The upstream API and default client/server behavior remain in place, except that
malformed or duplicated compression response parameters are now rejected.
`DialOptions.OfferClientMaxWindowBits` opts into the bare RFC 7692 offer observed
in the installed Codex CLI. A negotiated client window of 8..14 uses
klauspost/compress/flate.NewWriterWindow with per-window pools; absent or 15-bit
windows use the original encoder. Context takeover and no-context takeover both
follow the negotiated extension. No fake extension header is injected.

Patch files: dial.go, compress.go, write.go, go.mod, client_window_test.go.
The parent gateway enables the option only for the measured local CLI UA.

Run `go test ./...` in this directory in addition to the parent backend suite.
The regression test negotiates every allowed client window and exchanges multiple
compressed messages with and without context takeover. Invalid, repeated and
unoffered window parameters are rejected.

When upgrading coder/websocket, rebase these small changes and rerun both suites.
