# Kiro protocol attribution

Protocol reference: hank9999/kiro.rs, MIT License, commit
8fb3dd691d1cedf437bd6a4e9ef9f60b3f217fdd.
https://github.com/hank9999/kiro.rs

This Go adapter follows the credential, conversation and AWS event-stream
structures of that implementation. Account scheduling, persistence, cache
isolation and accounting belong to sub2api. No simulated cache usage is used.

Primary reference for nullable imports, credential refresh diagnostics and
profile-derived API region selection: ZyphrZero/kiro.rs, MIT License, commit
3e20b44af28e1f45ec8a74241236443ede905418.
https://github.com/ZyphrZero/kiro.rs

The runtime-v3 generation endpoint remains the verified default. Explicit CLI
transport selection follows the same reference endpoint/cli.rs envelope and
headers. Simulated prefix-cache accounting is not adopted.

Native refusal and truncation terminals, interrupted-tool handling, and final
metadata token snapshots follow the same ZyphrZero reference's metadata.rs,
anthropic/stream.rs, anthropic/openai.rs and anthropic/responses.rs. The Go
integration preserves Sub's no-charge refusal policy and ends explicit refusals
without waiting for trailing upstream frames. Provider explanations are not
forwarded or logged.
