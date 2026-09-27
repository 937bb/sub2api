# OMP Responses capture

Captured from the actual `@oh-my-pi/pi-coding-agent@18.2.6` CLI on macOS arm64 using Bun 1.4.2. Installation, configuration, sessions, and working files were isolated in temporary directories. A local HTTP mock served Responses SSE; no real upstream or credentials were used.

The `localcapture` provider used `api: openai-responses`, a loopback `baseUrl`, a fake API key, and model `gpt-6-astra`. CLI options were:

```text
--provider localcapture --model gpt-6-astra
--no-extensions --no-skills --no-rules --no-title --no-tools --no-lsp --no-pty
--thinking off --system-prompt "You are a local mock test assistant." -p
```

`normal_first` started a conversation; `normal_continued` used `--continue`. The two requests retained the same `prompt_cache_key` while appending history. Neither request sent a session or thread header, a body `session_id`, or `client_metadata`.

`explicit_first` and `explicit_second` also used `--provider-session-id routing-session-abc --prompt-cache-key cache-key-independent-xyz`; the second used `--continue`. Only the independently configured cache key appeared on the wire. The routing session was not serialized for this custom provider.

A separate actual CLI tool run used `--tools read`. After a mock `function_call`, OMP read a local fixture and submitted its `function_call_output`; both requests had the same cache key, stable instructions and tool definitions, and no session headers.

These captures establish the client's wire behavior, not an upstream cache-hit guarantee. The mock does not implement prompt caching.

Revalidated on 2026-09-26 with `@oh-my-pi/pi-coding-agent@18.3.2`. A fresh request, `--continue`, and a real `read` tool round trip retained the same cache key and stable request prefix while sending no conversation headers or `client_metadata` through a custom `openai-responses` provider.
