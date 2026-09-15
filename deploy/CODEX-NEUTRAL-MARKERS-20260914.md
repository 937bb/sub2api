# Codex generated-marker cleanup

## Scope and evidence

Branch: `codex/937sub2b-neutral-markers-20260914`, based on
`eeff23e50f9fab38dd769746a1a59e969e134274` (`origin/937sub2b`).

The locally installed Codex desktop bundle is version `26.908.40834`, with
`codex-cli 0.154.0-alpha.6.2`. A case-insensitive literal scan of its `app.asar`,
`codex`, and `codex-code-mode-host` resources found none of the old markers
listed below, nor `X-Sub2API-Quota-Bypass`. This is bounded static evidence, not
a network capture or proof about server-provided/runtime strings.

The desktop executable contains protocol identity strings including
`originator`, `session_id`, `x-codex-installation-id`, and `x-codex-turn-state`.
These are not Sub2API branding and are not removed by this patch.

## Changes

| Newly generated old value | New value |
| --- | --- |
| `python__sub2api` | `python__client` |
| `<sub2api-codex-image-generation>` | `<image-generation-compat>` |
| `<sub2api-codex-spark-image-unsupported>` | `<spark-image-unsupported>` |
| `<sub2api-claude-code-todo-guard>` | `<task-tracking-compat>` |

Closing delimiters change consistently. The contents and activation conditions
of compatibility instructions are unchanged. Reserved Python tool names retain
collision validation and reverse mapping for HTTP, SSE, and WebSocket responses.

Regression tests also reproduced a pre-existing marker-detection bug: Go JSON
encoding escapes `<` as `\u003c`, which raw substring detection misses. This
could insert duplicate task instructions and miss bridge classification when
there is no recognizable prompt-cache key. Detection now examines decoded
string values. Both current and legacy task markers suppress duplicate insertion
and are recognized as bridge requests.

Existing historical text, including legacy markers and user-defined tools
literally called `python__sub2api`, is not globally rewritten. Image/Spark legacy
markers likewise suppress duplicate insertion. This patch is not a guarantee
that all requests contain zero occurrences of the word Sub2API.

## Preserved behavior

- Account/session identity, UUID/hash salts, request headers, UA, and TLS settings.
- Overdraft scheduling and downstream quota-bypass response headers.
- User text, image inputs, tool arguments, and API-key transport behavior.
- Cache identity derivation. Changed instruction prefixes/tool schemas can still
  cause a cold cache for affected requests after deployment.

## Verification

Run from `backend/`:

```sh
go test ./internal/service -run 'Test(NeutralMarkers|AliasOpenAIOAuthReservedToolNames|RestoreCodexToolNames|CodexToolNameReverse|ApplyCodexOAuthTransform|.*Compat.*|.*ImageGeneration.*|Codex.*Identity.*|.*MessagesBridge.*)' -count=1 -timeout=180s
go vet ./internal/service
go build -o /tmp/sub2api-neutral-markers-20260914-server ./cmd/server
```

All commands above passed locally. Edited Go files were formatted and
`git diff --check` passed. `golangci-lint` is not installed locally; that check
could not run. No production requests or answer-quality A/B tests were performed.

The complete service package suite also passed:
`go test ./internal/service -count=1 -timeout=180s` (101.919 seconds).
The final additional neutral-marker cases passed separately after that run.

## Release boundary

Local changes only: no push, container restart, or live deployment. The branch
base contains Lite changes beyond the observed live revision
`5e79215ade773a412d1ea66f5d4307b24761120c`; do not blindly deploy the entire base
as a marker-only change. Review the deployment revision separately.

There is no demonstrated causal link between the branding strings and reduced
model capability, 429, or 5xx errors. This removes unnecessary generated names
and fixes a reproduced compatibility bug; error-rate improvement needs a
controlled comparison with matching model, reasoning settings, input, and load.
