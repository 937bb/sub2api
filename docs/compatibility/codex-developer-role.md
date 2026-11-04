# Codex outbound developer messages

OpenAI OAuth and setup-token destinations convert text-only `user` messages to
`developer` at the outbound boundary. Clients can continue sending their native
roles through Responses, Chat Completions, Anthropic Messages, or WebSocket.
API-key accounts and other OAuth providers retain the original roles.

The policy applies to HTTP forwarding and passthrough, HTTP-to-WebSocket pooled
requests, WebSocket ctx_pool and passthrough, and the WebSocket-to-HTTP bridge.
Policy evaluation, session identity and replay history retain the original input.
Changed map envelopes are cloned to avoid affecting retries on another account.

Only string content and nonempty arrays of text parts are converted. Messages
containing images, files, audio, unknown content, or legacy tool-result fields
retain their original role as a whole. Function/custom tool outputs, call IDs,
assistant messages, encrypted reasoning and arbitrary text are preserved. There
is no generic 400 retry or silent downgrade of rejected developer messages.

This deliberately changes the instruction priority of converted text. It is a
destination policy, not a transport fingerprint requirement or a guarantee that
all upstream models accept the role.

Regression coverage includes raw/map equivalence, account gating, unchanged
client bytes, precision preservation, Chat/Claude tool chains, real local
WebSocket two-turn sessions in ctx_pool and passthrough, and HTTP bridge replay.
