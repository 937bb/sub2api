# Codex outbound compatibility audit

Historical pre-repair findings: see `CODEX-CONVERSATION-ISOLATION-20260915.md`
for the subsequent forwarding/session and marker-recognition fixes.

## Boundary

Inspected the local `codex/937sub2b-neutral-markers-20260914` worktree after the
marker cleanup. This is not a capture of production traffic. No production
configuration, credentials, proxy settings, or running containers were changed.
This audit adds diagnostic tests, not new identity-rewriting behavior.

## Confirmed findings

1. **Environment data remains in several channels.** Both HTTP builders and the
   WS handshake copy `Accept-Language`. The account identity and fingerprint
   helpers preserve unknown `client_metadata` values and unknown fields inside
   `x-codex-turn-metadata`. A local fixture retained `Asia/Shanghai`, a custom
   client name, and thread source through both identity helpers. User input can
   also contain paths, timezone, instructions, and tool descriptions; these are
   not safely removable by a global string replacement. Preserving language is
   not itself a bug or evidence of upstream discrimination.

2. **Quota queries have separate identity settings.**
   `openai_quota_service.go:31` defines `codex-1`, `Codex Desktop`, and `zh-CN`,
   and `buildCodexCommonHeaders` sends them. These values do not come from the
   inference identity resolver. Different endpoints can legitimately need
   different headers; their difference alone does not establish an error.

3. **Full convergence collapses distinct threads.**
   `openai_codex_fingerprint.go:362` sets `threadID = sessionID`, independently
   of the original client session. A test with two distinct client sessions
   confirmed equal session/thread IDs and different turn IDs. This is a real
   change in session semantics, not cosmetic branding. Actual upstream state
   mixing or answer-quality impact has not been demonstrated.

4. **The environment UA is a selected profile, not an observed device.**
   `openai_codex_identity.go:32` lists nine fixed environment suffixes. Account
   seeds select among them unless an override exists. Thus per-account IDs do
   not mean every account has a unique UA or an authentic physical environment.

5. **Synthetic quota history is visible request content.**
   `gateway_quota_bypass.go:257` and `:575` insert synthetic tool history when
   enabled and applicable. Renaming delimiters does not make invented history
   originate from a real tool invocation. This audit does not modify that path
   or attempt to disguise it. Whether it affects errors needs separate evidence.

6. **Main inference and WS transports are not the desktop implementation.**
   `openai_plugin_transport.go` normally calls `httpUpstream.Do` after optional
   plugin handling. The separate account-test path can call `DoWithTLS`.
   `openai_ws_client.go` uses `coder/websocket`, Go HTTP transports, and configured
   compression. The installed Codex binary contains `reqwest`, `rustls`, and
   `tokio-tungstenite` strings. Static strings are not a wire-level TLS/HTTP/WS
   capture; no exact fingerprint equality or upstream classification is proven.
   A TLS setting on account tests must not be assumed to cover inference/WS.

7. **The Messages compatibility route is intentionally different.**
   `openai_gateway_forward.go:1500` removes `originator` for recognized Messages
   bridge requests. The identity enforcement helper requires that header and
   therefore skips that branch. It cannot be claimed that every OAuth request
   has exactly the same terminal identity processing. Do not remove the branch
   without validating the historical compatibility requirement.

## Confirmed negative checks

- With dummy OAuth and PAT accounts, ordinary HTTP, passthrough HTTP, and WS
  builders all rejected sentinel `X-Sub2API-Quota-Bypass`, `X-Forwarded-For`,
  `Forwarded`, `Via`, `X-Real-IP`, `X-Stainless-Lang`,
  `X-Stainless-Package-Version`, `Referer`, and `Cookie` headers.
- The `sub2api:*` identity derivation salts are inputs to hashing/UUID generation,
  not plaintext upstream labels. Changing them would unnecessarily rotate IDs.
- Branded model descriptions are returned to clients; they are not directly
  inserted into inference payloads by that code. Clients could later echo them.
- Legacy markers and user content remain intentionally untouched by the previous
  cleanup. This is not a promise that every request contains no branding.

## Desktop evidence and limitations

The earlier resource scan found no old gateway marker literals in the installed
desktop resources. This follow-up scan found standard identity field names and
network-library strings in the bundled `codex` executable, including
`session-id`, `thread-id`, `x-client-request-id`, `x-codex-window-id`, and
`x-codex-turn-metadata`. Their presence does not establish exact values or runtime
behavior. An attempted official configuration-reference fetch failed with a
connection reset; no documentation-based claim about anti-abuse behavior is made.

## Validation and priorities

`go test ./internal/service -run '^TestOutboundAudit_' -count=1 -v -timeout=90s`
passed locally. All fixtures use dummy credentials and make no upstream calls.
Tests document current behavior rather than endorse a future identity policy.

For reliability, prioritize correct per-conversation state isolation, consistent
effective request settings, minimal request mutation, and accurate error tracing.
Use matched model/reasoning/input/load comparisons before attributing failures to
identity. Do not manufacture platform attestations, erase required attribution,
or present a proxy as an indistinguishable first-party human client. Header
rewriting cannot establish that guarantee or explain 429/5xx by itself.
