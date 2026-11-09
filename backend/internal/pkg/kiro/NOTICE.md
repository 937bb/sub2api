# Kiro protocol attribution

Protocol reference: hank9999/kiro.rs, MIT License, commit
8fb3dd691d1cedf437bd6a4e9ef9f60b3f217fdd.
https://github.com/hank9999/kiro.rs

This Go adapter follows the credential, conversation and AWS event-stream
structures of that implementation. Account scheduling, persistence, cache
isolation and accounting belong to sub2api.

Primary reference for nullable imports, credential refresh diagnostics and
profile-derived API region selection: ZyphrZero/kiro.rs, MIT License, commit
3e20b44af28e1f45ec8a74241236443ede905418.
https://github.com/ZyphrZero/kiro.rs

The runtime-v3 generation endpoint remains the verified default. Explicit CLI
transport selection follows the same reference endpoint/cli.rs envelope and
headers. Group-selected local prefix metering was subsequently adopted at the
operator's request; see docs/operations/kiro-prefix-billing.md. It is distinct
from upstream-reported cache usage and does not reduce provider inference.

Native refusal and truncation terminals, interrupted-tool handling, and final
metadata token snapshots follow the same ZyphrZero reference's metadata.rs,
anthropic/stream.rs, anthropic/openai.rs and anthropic/responses.rs. The Go
integration preserves Sub's no-charge refusal policy and ends explicit refusals
without waiting for trailing upstream frames. Provider explanations are not
forwarded or logged.

Native reasoning text, signatures, redacted content, sequential block closure,
and output estimates follow ZyphrZero/kiro.rs commit
5c8c3481e3f8d65f670262e4ef65029ffcd4bdbc, specifically
src/kiro/model/events/reasoning.rs and src/anthropic/stream.rs. When the provider
omits a signature, the adapter supplies an explicitly local compatibility
marker, never an Anthropic-verifiable signature. It is not forwarded upstream.
Unlike rs's disabled-thinking fallback, this adapter does not convert hidden
reasoning into answer text. Explicit display=omitted is also respected.

Manual thinking budget-to-effort thresholds follow that reference's
src/anthropic/converter.rs. Translation is gated by the selected account's live
model schema rather than a static model-name table, and preserves explicit
client effort. These tiers are approximate compatibility mappings, not exact
provider reasoning budgets.
