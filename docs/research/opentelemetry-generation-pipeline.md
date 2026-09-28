# OpenTelemetry Evaluation for the Generation Pipeline

> Scope: whether IdeaBoard should add OpenTelemetry tracing while retaining its existing Prometheus metrics, River/PostgreSQL task facts, and NATS JetStream event layer. Sources are first-party OpenTelemetry documentation or package repositories. Reviewed 2026-09-28.

## Recommendation

Adopt OpenTelemetry **traces only** as an observability enhancement for the Generation Pipeline reset. Keep the existing Prometheus metrics endpoint and metric names as the operational metrics contract. Use OTLP to send traces to an OpenTelemetry Collector, and let the Collector handle batching, filtering, retries, and export to a trace backend.

Do not replace River, PostgreSQL, or NATS with an OpenTelemetry component. OpenTelemetry describes telemetry signals and propagation; it is not a task queue, a database transaction coordinator, or an event bus.

The first implementation should use the stable Go tracing API/SDK, manual domain spans, and standard HTTP instrumentation. It should not add a new workflow or retry dependency.

## Evidence From Official Sources

### Go SDK and instrumentation

- The OpenTelemetry Go documentation marks **Traces** and **Metrics** as stable. The Go SDK is initialized with a `TracerProvider`, resource attributes, a span processor, and an exporter. See [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/) and [manual instrumentation](https://opentelemetry.io/docs/languages/go/instrumentation/).
- The Go instrumentation guide uses `context.Context` as the carrier for the active span and shows nested spans created from the returned context. A span is immutable after it ends. See [creating spans](https://opentelemetry.io/docs/languages/go/instrumentation/#creating-spans).
- The same guide defines W3C Trace Context propagation through `propagation.TraceContext{}` and notes that propagation is what connects spans across process boundaries. See [propagators and context](https://opentelemetry.io/docs/languages/go/instrumentation/#propagators-and-context).
- The maintained `otelhttp` instrumentation package provides HTTP server and client instrumentation, including span creation and propagation hooks: [otelhttp package](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp).

### OTLP and Collector

- OpenTelemetry's Go exporter guide recommends OTLP because it preserves the OpenTelemetry data model and is supported by many backends. It documents both OTLP/HTTP and OTLP/gRPC Go exporters: [Go exporters](https://opentelemetry.io/docs/languages/go/exporters/).
- The OTLP specification defines standard endpoint environment variables, per-signal endpoints, TLS, headers, compression, timeout, and protocol selection. Default OTLP ports are 4317 for gRPC and 4318 for HTTP: [OTLP exporter specification](https://opentelemetry.io/docs/specs/otel/protocol/exporter/).
- The Collector is vendor-neutral and can receive, process, and export telemetry. Its official guidance says direct export is acceptable for development or small deployments, while production generally benefits from a Collector for batching, retries, encryption, and sensitive-data filtering: [Collector introduction](https://opentelemetry.io/docs/collector/).

### Sampling and propagation

- The Go SDK provides `AlwaysOn`, `AlwaysOff`, `TraceIDRatioBased`, and `ParentBased` samplers. The official production guidance is to use `ParentBased` with a ratio-based root sampler, while preserving parent `tracestate`: [Go sampling](https://opentelemetry.io/docs/languages/go/sampling/).
- Sampling decisions should be made at the start of a trace and propagated to downstream services. This is a good fit for the HTTP submit request and the asynchronous task's explicit trace context: [sampling concepts](https://opentelemetry.io/docs/concepts/sampling/).

## Fit With IdeaBoard's Existing Architecture

| Boundary | Trace approach | Keep existing fact/metric system? |
| --- | --- | --- |
| Gin HTTP intake | `otelhttp` middleware or equivalent adapter; start the request span from `r.Context()` | Yes. Keep Prometheus RED metrics and `X-Request-ID`. |
| Quote, readiness, and `RoutingResolver` | Manual child spans with low-cardinality attributes such as `service`, `resolution`, and `pool_version` | Yes. Domain return values remain the source of truth. |
| Transactional submit | Manual span around the PostgreSQL transaction and River enqueue seam | Yes. PostgreSQL transaction remains authoritative; a trace does not prove commit. |
| River worker | Manual worker/task span created from a stored propagation context or linked trace context | Yes. River + PostgreSQL remain task facts and retry state. |
| Model attempt and fallback | Manual spans for `model_attempt`, `gateway_request`, and `model_fallback` | Yes. Existing retry layers and Prometheus counters remain authoritative for policy and alerts. |
| NATS status event | Create a producer span and propagate W3C headers only if the event contract explicitly adopts them | Yes. NATS JetStream remains event transport and audit/realtime fanout, not a queue. |
| PostgreSQL / pgx | Instrument only selected transaction/query boundaries first; avoid exporting SQL text or parameters | Yes. SQL and billing/task state remain PostgreSQL facts. |

OpenTelemetry attributes must remain bounded. Never attach prompt text, image bytes, user identifiers, raw URLs, provider credentials, SQL values, or full error payloads. Use stable IDs only where the existing privacy contract permits them; otherwise use event-local correlation fields in logs and metrics.

## Dependency and Deployment Assessment

The API `go.mod` currently declares `pgx`, River, NATS, and Prometheus directly, but no OpenTelemetry package. `go.sum` contains OTel modules indirectly through the SQLC/gRPC toolchain; that does not make them an application dependency. Adding tracing therefore requires explicit direct modules and version pinning, followed by normal `go mod tidy` and build verification.

Recommended initial modules:

```text
go.opentelemetry.io/otel
go.opentelemetry.io/otel/sdk
go.opentelemetry.io/otel/trace
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
```

Use OTLP/HTTP first if the deployment wants to avoid adding a gRPC transport to the runtime binary; OTLP/gRPC is also supported by the official exporter. Use environment-driven endpoint, header, TLS, compression, and timeout configuration rather than hard-coding a backend.

For production, add an OpenTelemetry Collector as a separately operated process/container only when a trace backend and retention policy are selected. The Collector needs its own health checks, resource limits, queue/backpressure policy, TLS configuration, and data-redaction review. It must not become a hidden dependency of task submission: telemetry export failure cannot fail Quote, Submit, Billing, River enqueue, or Worker terminal convergence.

## Proposed Trace Shape

```text
HTTP generation submit
  -> quote/readiness
  -> routing.resolve
  -> postgres.submit_transaction
  -> river.enqueue
  -> worker.task
  -> execution_attempt
  -> gateway.request
  -> fallback_attempt (optional)
  -> media.persist / terminal_settlement
```

Recommended bounded attributes:

```text
service
requested_resolution
pool_version
selected_model
attempt_number
retry_layer
failure_class
task_queue
```

Do not put the complete candidate list, prompt, provider response, or mutable configuration blob into every span. Store the immutable `RoutingSnapshot` and `ExecutionAttempt` in PostgreSQL; use a trace attribute to point to the domain object only when the existing data-protection policy permits it.

## Sampling Policy

Start with `ParentBased(TraceIDRatioBased(p))` in production, with `p` configurable per environment. Keep development at 100% sampling. The Collector can later perform backend-specific processing, but sampling must not hide the task facts needed for billing or reconciliation.

For incident debugging, prefer a temporary configuration increase or a targeted trace attribute that is sampled by policy. Do not implement a custom sampler that performs network calls or reads mutable Model Pool state; the official guide warns that sampling runs synchronously on span creation.

## Rollout Gates

1. Define the trace attribute allowlist and redaction rules alongside #852/#854.
2. Add one provider-neutral tracer initialization seam with bounded shutdown/flush timeouts.
3. Instrument HTTP intake, `RoutingResolver`, transactional submit, Worker attempt, and Gateway request before adding broad library instrumentation.
4. Verify that Collector/exporter outage leaves all business paths successful and only records telemetry errors locally.
5. Verify that Prometheus dashboards and River/NATS operational runbooks remain valid.
6. Add end-to-end trace propagation tests for HTTP submit -> River worker -> gateway/fallback, without asserting exact trace IDs.

## Decision

OpenTelemetry is a mature, suitable addition for **distributed tracing** in this refactor. It is not a replacement for any current business or task dependency. Adopt it behind a small observability adapter, keep Prometheus as the existing metrics contract, and make Collector deployment an explicit operations decision rather than a hidden application requirement.
