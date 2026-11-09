# Kiro rs compatibility audit — 2026-11-09

Primary reference: ZyphrZero/kiro.rs, fetched master commit
`5c8c3481e3f8d65f670262e4ef65029ffcd4bdbc`.
Sub baseline: `2a789b3e4aa589185fd70804ded67a99d29289c6`, including the
prefix-billing release and the concurrent moderation logging fix.
The reference was inspected as source, including converter.rs, stream.rs,
handlers.rs, cache_metering.rs, websearch_loop.rs and model/events/reasoning.rs.
README statements can lag implementation and are not treated as wire evidence.

## Implemented in this change

- Parse native reasoningContentEvent text, signature and redactedContent.
- Stream thinking_delta and signature_delta separately from answer text, and
  preserve the same blocks in non-streaming Messages responses.
- Close thinking before text, tools or redacted blocks; use the existing block
  serializer when tools interleave. Propagate client cancellation and malformed
  reasoning frames without manufacturing successful terminal events.
- Preserve a provider signature supplied before or after its text. Use an
  explicitly local placeholder only when absent, as rs does for SDK compatibility.
  The placeholder is not a valid Anthropic continuation signature.
- Count reasoning text in estimated output, and use rs's fixed eight-token
  estimate for an opaque redacted block. Neither signature length nor ciphertext
  length is treated as actual token usage. Complete provider usage wins.
- Preserve no-charge refusal behavior and prefix finalization ordering.
- Apply request thinking controls to Messages and the shared Chat/Responses
  bridge. Disabled thinking and explicit display=omitted do not expose reasoning
  as answer text. A signature-only event still cannot become a successful answer.
- Translate manual enabled/budget_tokens to adaptive/effort only when the
  selected account's live schema advertises adaptive but not enabled. Preserve
  explicit effort and other output controls, normalize xhigh against that same
  schema, and leave models advertising manual support unchanged. The rs budget
  thresholds are compatibility approximations, not exact upstream token limits.

Kiro history does not accept Anthropic signatures or encrypted thinking replay.
Round-trip requests containing these blocks remain accepted, but these opaque
fields are omitted upstream. This does not claim cryptographic continuation.

## Evidence and remaining differences

| Surface | rs implementation | Sub state after this change |
| --- | --- | --- |
| Native thinking response | stream.rs process_reasoning_content | Implemented, including stream/nonstream and accounting tests |
| Omitted/disabled thinking | rs can expose reasoning as fallback text if no answer | Intentionally not adopted; client visibility settings are preserved |
| Plain-text thinking tags | stream.rs has a buffered tag state machine and boundary handling | Not implemented; literal assistant text is preserved |
| Thinking history retention | converter.rs keeps selected visible thinking with model/turn-aware rules | Implemented below for visible assistant thinking; opaque signatures/redacted blocks are omitted |
| `-thinking` model aliases | converter.rs normalizes suffixes and handlers derive defaults | Claude terminal suffix supported across Messages/Chat/Responses; explicit thinking controls take precedence; GPT aliases remain unsupported |
| Native reasoning controls | converter.rs selects effort fields with provider/model-specific rules | Catalog-driven manual-to-adaptive budget/effort conversion verified on Opus 4.6 and 5.5; GPT-style upstream reasoning mapping still needs validation |
| Raw tools | converters and stream support naming/schema and partial JSON | Existing Sub support retained; strict pairing rejects malformed history instead of rs repair/filtering |
| Textual invoke fallback | stream.rs can recover known tool invocations from assistant text | Not implemented; text is not promoted to executable tool calls |
| Web search | websearch_loop.rs runs bounded upstream MCP search/tool loops | Server web_search tool types still rejected; no internal search loop |
| Structured output | rs injects schema instructions, buffers and validates JSON | No equivalent guaranteed extraction/validation; catalog fields alone do not establish semantic support |
| Images | rs validates/resizes several image inputs | Sub supports supported base64 formats and lifts tool-result images; remote fetch/resize needs a separate implementation and tests |
| Gemini thinking | rs primarily exposes Anthropic/OpenAI surfaces | Sub Gemini adapter still rejects thinkingConfig; not covered by this change |
| Prefix billing | cache_metering.rs local checkpoint accounting | Existing Redis group policy, cross-instance reads, success-only commit and Sub pricing retained |
| Refusal and truncation | native metadata terminal handling | Existing explicit refusal/no-charge and truncation mapping retained; reasoning must not override them |

## Validation scope

Deterministic tests cover signatures before text, omitted signatures, redacted
ordering, thinking/tool/text interleaving, client cancellation, malformed events,
signature-only empty responses, hidden reasoning estimates, provider-usage
priority, refusals, and tool-result history round trips. Service tests exercise
Messages, Chat and Responses, both streaming and nonstreaming, with cold/warm
prefix billing and output usage conservation.

These are synthetic protocol tests, not evidence that every Kiro model returns
visible reasoning. Real upstream reasoning depends on model, controls and
provider behavior. A successful ordinary answer alone does not prove thinking
display compatibility. Production deployment and live reasoning evidence must
be recorded separately; this audit is not a claim of complete rs parity.

## Live evidence — isolated candidate, 2026-11-09

The probe used the existing account's fixed proxy and the group's existing
90-percent local-prefix policy. Production routing was not changed. Only event
counts, usage and status were recorded; credentials, reasoning text and opaque
signatures were not logged.

- The live model catalog advertises adaptive (not enabled) for both Opus 4.6
  and 5.5. A sanitized schema fixture is included in the package tests.
- Before normalization, Opus 4.6 enabled/budget_tokens=2048 returned HTTP 400.
  After normalization, both dotted/streaming and hyphenated/nonstreaming model
  forms returned HTTP 200/end_turn in 3.527 and 3.042 seconds. These calls had
  answer text only: visible reasoning for Opus 4.6 is not established.
- Opus 5.5 enabled/budget_tokens=2048 returned ten thinking deltas, one
  signature delta and answer text in 3.461 seconds. Adaptive/nonstreaming also
  returned thinking and text in 4.143 seconds. Nonempty response signatures
  alone do not prove provider provenance because a compatibility marker is
  supplied when the provider omits a signature.
- A real three-turn Opus 5.5 sequence completed tool_use, tool-result return
  with thinking/text, and a streamed follow-up with thinking/text. The client
  replayed the full assistant blocks. Answers matched the synthetic tool result
  and subsequent arithmetic; no local command/tool execution was involved.
- All seven post-fix calls had exactly one matching usage record, positive
  actual cost and the same output-token count as the response.
- A repeated prompt read 196 of 218 input tokens (89.9 percent). The third
  tool-conversation turn read 435 tokens, created 44, and counted 35 uncached.
  The target is a policy ceiling, not a promise that every growing turn is 90
  percent cached. All these cache quantities are local billing, not proof of
  Kiro-side inference savings.

Probe evidence is stored on the server under
`/root/candidates/kiro-reasoning-20261109/live-manual.json` and `live-tools.json`.
This candidate evidence is not a production deployment record.

The combined candidate passed `go test ./...`, `go test -tags unit ./...`,
`go vet ./...`, targeted Kiro race tests, the static server build and
golangci-lint's changed-code gate. Existing unrelated lint findings were not
represented as a clean repository-wide lint result. Production continues to
use the baseline until the separate release checks and traffic switch occur.

## Next implementation priorities

1. Verify GPT-style upstream reasoning controls and the remaining account models;
   do not extrapolate the verified Opus behavior to every model.
2. Extend real multi-model history and alias validation beyond the verified
   Opus paths, including explicit reasoning controls in OpenAI clients.
3. Add the guarded text-tag parser; test chunk boundaries, ordinary code/text
   and native-event precedence before enabling it.
4. Implement bounded MCP web search with the assigned account proxy, cancellation,
   correct profile headers, per-round accounting and mixed-tool preservation.
5. Implement structured-output validation and image processing as separate,
   testable features with explicit supported surfaces.

## Follow-up: visible history, aliases and prefix identity

This follow-up builds on `ede13f4e191c0eed9d62eac27fb5433178aa68d6`.
The reference remains rs commit `5c8c3481e3f8d65f670262e4ef65029ffcd4bdbc`:
`model_keeps_prior_thinking`, `finished_assistant_turns`,
`convert_assistant_message` and `override_thinking_from_model_name`.

- Opus 4.5+, Sonnet 4.6+ and generation-5+ families retain visible assistant
  thinking. Older models retain it only after the latest nonempty user
  instruction/image, including the current tool-result loop. Thinking text is
  represented as `<thinking>...</thinking>` in assistant history, preserving
  segment order. This is contextual text, not signed native continuation.
- Adjacent assistant text fragments concatenate without extra newlines. Thinking
  and redacted blocks separate text segments as in rs. Opaque signatures and
  redacted payloads are never sent in history or used in prefix digests.
- Prefix estimates include retained visible thinking only. Changing retained
  text changes the digest; changing discarded old thinking, signatures or
  ciphertext does not. Thinking blocks cannot declare their own checkpoints.
- Prefix hashing uses `kiro-prefix-v2-thinking-history`. Old checkpoint hashes
  cannot be reused under the new accounting rules; prefixes warm again after
  deployment. Database and cache policy schemas are unchanged.
- A terminal Claude `-thinking` suffix resolves to the same base model, including
  dotted/hyphenated versions, date suffixes and `[1m]`. Absent explicit thinking,
  it supplies rs's 20,000-token default budget; the account catalog determines
  whether this is translated to adaptive/high. Explicit thinking and output
  effort are preserved, unlike rs's unconditional thinking override. Unknown
  capabilities are not inferred from the suffix. Full-response cache stays
  disabled for thinking aliases. GPT suffixes and arbitrary names merely
  containing `thinking` are not reinterpreted.
- The response retains the requested alias. Kiro accounting records the canonical
  upstream model so alias spelling does not create a separate upstream identity.

Deterministic coverage includes older/newer model boundaries, mixed tool-result
and user content, current tool loops, segment ordering, opaque-value isolation,
prefix estimate/digest changes, alias precedence and stream/nonstream
Messages/Chat/Responses with cold/warm prefix metering.

The isolated live candidate completed three requests using only
`claude-opus-5-5-thinking` (no explicit thinking field): tool invocation,
nonstreaming tool-result continuation with visible thinking, and streamed
follow-up. Responses matched the synthetic tool result and arithmetic; all
three bills matched output usage. Cache reads were 0, 140 and 432 tokens.
Durations were 3.612, 20.571 and 3.332 seconds; this does not establish a latency
improvement. Evidence: `live-history-alias.json` in the same server candidate
directory. Production routing was not changed for these probes.

The follow-up passed default and unit-tagged full backend suites, `go vet`,
targeted Kiro/history/prefix race tests, static server build and changed-code
golangci-lint. The three matching billing records retained the requested alias
and recorded `claude-opus-5.5` as upstream_model. No frontend files or database
migrations are part of this change.
