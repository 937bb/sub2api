# SIWC compatibility on the official base

## Cache usage

SIWC uses `POST https://api.openai.com/v1/responses`, with `store=false` and
`stream=true`. Cache reads are measured from `usage.input_tokens_details.cached_tokens`.
Cache creation is measured from `usage.input_tokens_details.cache_write_tokens`
(with compatibility aliases). Missing or explicit zero write usage is zero, not
an estimate of uncached input. Input, cache reads and cache writes are mutually
exclusive billing categories.

Synthetic live controls on October 17, 2026 returned nonzero cache reads but
explicitly zero cache writes. The tested SIWC model rejected
`prompt_cache_options` with `invalid_parameter`. The gateway preserves caller
cache controls but does not inject unsupported controls or manufacture charges.
There is no verified local switch that enables upstream cache-write accounting.

Stable, tenant/grant-scoped `session_id` affinity and append-only tool/history
prefixes remain important. Repeated synthetic requests reached approximately
98% cache reads after warming, but account changes, short prompts and changed
prefixes can still lower real traffic hit rates. Cache state is not shared or
copied between independent accounts by this gateway.

## Credentials and recovery

Terminal refresh errors are recognized from allowlisted OAuth error codes.
An atomic comparison against the failed grant marks only that unchanged account
as needing reauthorization. A concurrent reauthorization or credential rotation
wins; no credential or account is deleted. Manual scheduling flags remain intact.
A successful authorized credential replacement clears only this specific error.

Credential acquisition failures can select another account before inference or
downstream output starts. Temporary network errors do not revoke a still-valid
token. Stream errors preserve SIWC's specific status codes, including temporary
user unavailability. A stream already containing model output is not replayed.

## Stateless history and mixed pools

The selected account's verified authorization mode determines its upstream URL,
token and headers. Full text/tool histories can switch between compatible OAuth
and SIWC accounts. Server-side `previous_response_id` continuation is not supported
by either OAuth HTTP route; clients must send complete history.

SIWC reasoning history retains content, summaries and encrypted content, but
removes `rs_*` lookup IDs because `store=false` does not persist those items.
The gateway requests `reasoning.encrypted_content` for subsequent stateless turns.
Tool call IDs remain unchanged.

Unsupported hosted-tool histories and native compaction triggers exclude SIWC
before account selection. The gateway preserves the complete original request
for a compatible account. It does not silently delete computer/tool interactions.
Local capability mismatches do not penalize account health or create usage charges.

Opaque encrypted reasoning and compacted history must not be assumed portable
across different grants. This release does not promise universal cross-account
decryption or copy upstream cache state.

## Remote compaction boundary

Live SIWC tests on October 17, 2026 returned:

| Operation | Observed result |
| --- | --- |
| `/v1/responses` with `compaction_trigger` | HTTP 400, `subscription_sharing_unsupported_capability` |
| `/v1/responses/compact` | HTTP 401, `hardened_oauth_rule_missing` |
| `/v1/responses` with `context_management` | HTTP 200 on the first attempt, but no verified compaction result; later attempts blocked by account `access_denied` |

Therefore SIWC is excluded from standalone compact routing even if a local
force-on flag is present. Mixed pools can select an eligible ordinary OAuth/API
account for native/standalone compaction. SIWC-only remote compaction has not
been verified and is not advertised as implemented. Existing caller-provided
`context_management` is preserved. Do not synthesize encrypted compaction tokens,
silently discard history, or send SIWC grants to ChatGPT Codex endpoints.

## Reference implementation comparison

The supplied `sub2api-siwc-src-20261017` package forces HTTP streaming and removes
Codex identity headers. Its transport also removes `session_id`; no additional
cache-write accounting or cache-rate measurement was found in the relevant code.
Its design document states real-account end-to-end validation was pending.
Those facts do not establish higher cache performance.

Its unconditional removal of computer tool pairs and account deletion on selected
403 responses were intentionally not adopted: the former loses conversation
context, while the latter is stronger than the available failure evidence.

## Official references

- [SIWC preview limitations](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)
- [SIWC errors and recovery](https://developers.openai.com/siwc/token-sharing-open-source/errors-and-recovery)
- [Prompt caching](https://developers.openai.com/api/docs/guides/prompt-caching)
- [Compaction](https://developers.openai.com/api/docs/guides/compaction)

General Responses documentation does not establish that every feature is enabled
for the SIWC grant. Validate the actual account/model route before rollout.
