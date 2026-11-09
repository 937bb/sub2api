# Kiro local prefix billing

The group `kiro_response_cache` policy supports two independent modes:

- Missing mode or `response`: retain complete-response reuse and its existing billing.
- `prefix`: run inference on every request and account for locally recorded prompt prefixes. This follows the cache metering approach in ZyphrZero/kiro.rs, reviewed at commit `5c8c3481e3f8d65f670262e4ef65029ffcd4bdbc`.

Prefix mode is local estimated billing. It does not claim Kiro provider cache hits, skip upstream inference, or reduce upstream credits. The operator explicitly selected this accounting model. Normal input, cache creation, cache reads and output use the existing Sub pricing configuration; no prices are embedded in this implementation.

## Group policy

```json
{
  "enabled": true,
  "mode": "prefix",
  "auto_prefix": true,
  "target_percent": 90,
  "ttl_seconds": 3600,
  "max_entries": 128
}
```

In prefix mode, `target_percent` caps the cached-read tokens as a percentage of total input on each request. It does not fabricate matches or promise a fixed hit ratio. Previously recorded prefix tokens beyond the cap become ordinary input, not new cache writes. Newly covered prefix segments are cache creation; the remaining tail is ordinary input. Output is always charged normally. All three input categories sum to the original input total.

The default remains the legacy response mode. The admin group form makes changing the accounting mode explicit. Policies apply to the effective request group, including group overrides, rather than individual account metadata.

## Prefix semantics

Blocks are hashed in tools, system, then message order with SHA-256 and length-framed canonical JSON. Role, tool names and schemas, tool input and results, model, reasoning controls, output configuration and client identity participate. Cache-control annotations do not alter content fingerprints. Equivalent string and single-text-block messages share a fingerprint. Both Opus 4.6 name aliases normalize to the same prefix namespace.

Client-declared ephemeral checkpoints support 5m and 1h TTLs, at most four checkpoints, and a 20-position lookback. Consecutive tool-use or tool-result blocks occupy one lookback position. Longer TTL checkpoints must precede shorter TTL checkpoints. Invalid declarations disable local prefix accounting for that request rather than breaking otherwise valid inference.

Top-level `cache_control` selects an automatic checkpoint. If the client has no checkpoints and the group enables `auto_prefix`, the group TTL is used. Like the reviewed rs implementation, the automatic checkpoint prefers the historical prefix before the newest user question. A first request containing only a user question records that question. Tools and thinking configuration do not bypass prefix accounting. Thinking history is included in identity but is not charged as prompt text because the Kiro converter omits Anthropic signed thinking history.

Prefix proportions use estimated token counts and are scaled to the decoder's input total. They are not exact tokenizer measurements, including for multimodal content. At least one uncached input token is retained when total input is positive, matching rs compatibility with clients that merge nonzero usage values.

## Storage and lifecycle

Redis stores only digests, cumulative estimates and TTL metadata. It does not store prompt text or tool results. Namespaces isolate tenant, API key, group, account, model and account environment. Capacity is shared per group; expired entries are pruned and the earliest-expiring entries are removed when over capacity. Policy changes use a fresh namespace.

Lookup happens before inference. Checkpoint writes and TTL renewal happen only after a successful decoded terminal (`end_turn`, `tool_use`, `max_tokens`, or `stop_sequence`). Cold concurrent requests cannot claim hits from each other's future writes. Refusals, invalid/incomplete streams, canceled requests and Redis failures do not create local cache charges. Redis errors fall back to normal input accounting, with no per-process simulated fallback that could disagree across HTTP/WS runtimes.

Unlike the rs implementation's early recording, this implementation defers writes until inference succeeds. It also uses full SHA-256 digests, bounded shared Redis storage and an explicit group read-token cap.

Complete upstream usage takes priority over local prefix estimates, as in rs. Local responses identify `usage.cache_source=local_prefix`; the decoder source is `local_prefix_estimated`, and the gateway exposes `X-Sub2api-Cache-Mode: local_prefix`. Streaming usage is finalized before the terminal delta. OpenAI/Gemini adapters use the same finalizer and existing protocol-specific usage conversions.

## Verification

Tests cover a growing 12-round tool conversation, string/block equivalence, model aliases, role/content/control isolation, invalid checkpoints, 20-position lookback, parallel tools, mixed TTL splits, token conservation, read caps, Redis sharing/expiry/capacity, concurrent cold lookups, provider usage priority, cancellation/store failure, and Messages/Chat/Responses/Gemini streaming and non-streaming billing.
