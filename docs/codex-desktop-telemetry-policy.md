# Codex Desktop telemetry policy

## Decision

Inference compatibility and telemetry are separate surfaces. The gateway must
not invent, replay, or impersonate official OpenAI telemetry. It also must not
rewrite privacy or execution facts merely to match an exit IP.

## Channel ownership

| Channel | Owner | Gateway behavior |
| --- | --- | --- |
| Responses HTTP / WebSocket | Native Codex engine | Apply the verified Desktop inference identity and preserve real request metadata |
| Electron ChatGPT helpers | Desktop shell | Do not copy helper-only headers such as `OAI-Product-Sku` into inference |
| Engine OpenTelemetry | Client process | Preserve real W3C context on inference requests; do not proxy or synthesize official OTLP events |
| Statsig feature gates | Desktop shell | Do not emit fake exposure/log events |
| Sentry / browser RUM | Desktop shell/webview | Do not relay under the gateway identity |
| Gateway metrics/logs | sub2api | Keep in local observability with gateway-specific names |

## Preserved inbound fields

Identity rewriting must preserve caller-supplied values for:

- `analytics_enabled`, `token_provider`, guardian fields;
- `mcp_attribution`, `tool_namespaces_info`, `workspaces`;
- `responses_api_metadata`;
- `traceparent`, `tracestate`, `baggage` when present in the caller's request
  metadata or transport headers. Metadata is not promoted into headers; actual
  inbound HTTP/WS headers are forwarded without creating missing values;
- sandbox, source, trigger, subagent, parent/fork and compaction metadata.

The gateway may namespace account/session/thread/turn identifiers where the
configured fingerprint mode requires isolation. It must not replace execution,
privacy, locale, region, timezone, or analytics values with guessed data.

## Local observability

Gateway metrics should describe gateway facts: queue time, connection reuse,
upstream latency, retries, response status, token usage, cache accounting, and
transport selection. They must not use OpenAI SDK keys, official endpoint names,
or payload schemas that imply the events came from Codex Desktop.

## Static evidence

The installed `codex-exec-server` binary includes OpenTelemetry 0.31 components,
OTLP HTTP/gRPC exporters for logs, metrics and traces, and W3C
`traceparent`/`tracestate` propagation. Static capability does not prove that
every inference request emits telemetry. A later isolated capture enabled local
OTLP JSON exporters and confirmed that HTTP inference carries its real
`traceparent` plus an explicitly empty `tracestate` header, while a WebSocket
upgrade carries neither. The WS `response.create.client_metadata` contains
`ws_request_header_traceparent`; an empty tracestate is omitted. In both cases
the inference trace ID matched the locally exported trace.

The observed OTLP resource identifies `service.name=codex-app-server`,
`service.version=0.167.0-alpha.3.2`, Rust OpenTelemetry 0.31.0, with scopes
`codex_otel.log_only`, `codex-app-server`, and `codex`. These values document the
client process; the gateway does not emit copies of those events.

## Auxiliary turn-cost lookup

Desktop API-key provider mode sends `POST /v1/analytics/codex/turn-costs` with a
`turn_ids` body and no inference trace, session, originator, or account headers.
OAuth mode did not issue this request in the isolated capture. A sub2api API key
can schedule different OAuth accounts, so forwarding the lookup through an
arbitrary account would misattribute the turn. The gateway therefore keeps the
route authenticated and returns the schema-compatible empty envelope
`{"turns":[]}` locally. It performs no scheduling, concurrency acquisition,
audit, or billing.

## Remaining limitation

A client may send telemetry directly from a different network path than the
gateway inference exit. The gateway cannot make those paths identical through
request-header rewriting. Account/exit-region consistency is an infrastructure
decision, not a metadata-forging problem.
