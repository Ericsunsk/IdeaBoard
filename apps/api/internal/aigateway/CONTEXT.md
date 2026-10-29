# AIGateway Context

AIGateway is the operating context for the sole AI egress: gateway connection, model catalog, Vision mapping, route preview, and probing. It owns no Wallet, Ledger, prices, task execution, Service Model Pool, or provider SDKs.

> Vision Search is temporarily unavailable: production passes nil for the
> vision gateway seam, so vision extraction fails closed. The `vision`
> mapping key reservation and billing SKU remain.

## Domain Language

| Term                | Meaning                                                                                                                                                                                                                                                                                                |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Gateway             | Cloudflare AI Gateway connection quadruple (base URL, account ID, gateway ID, token); least-privilege token with budget cap. Any http(s) Base URL host is accepted (no allowlist); URL changes are audit-logged, never blocked. Custom domains omit `/v1/{account}/{gateway}` from the path (the hostname identifies the gateway), so they always need explicit account/gateway IDs |
| ResolvedGateway     | Effective resolved connection; management surfaces only a `token_configured` boolean, never the secret                                                                                                                                                                                                 |
| Service Model Pool  | The Models eligible for an image Service; owned by Generation, outside gateway configuration.                                                                                                                                                                                                          |
| Vision Mapping      | Vision-search-only mapping under the bare key `vision`, stored as `ai_gateway_vision_model`, isolated from Service routing                                                                                                                                                                             |
| Route Preview       | Dry-run routing for a model ID plus custom params (namespace, path, source rule, chat judgment); shares `dialects.Resolve` (single resolve-then-shape seam) with execution so preview and execution never diverge                                                                                      |
| Connection Probe    | Direct probe at the primary gateway (bypasses fallback topology), single-shot (`cf-aig-max-attempts: 1`), cache-skipped, results logged only                                                                                                                                                           |
| Model Detection     | Union of control-plane configured providers with the LiteLLM community catalog and the models.dev catalog over one shared attribution; same keys dedupe with models.dev winning metadata, relay partitions never match; LiteLLM failure stays fail-closed, models.dev failure degrades to LiteLLM-only |
| Configured Provider | Control-plane-attested provider slug; Workers AI carries no key so it never appears there — availability probing admits it when alive                                                                                                                                                                  |
| Receipt             | Gateway acknowledgement headers (model, provider, step, cache status, log/event ID); operations observability and attempt facts only, never billing snapshots                                                                                                                                          |
| Cache Bypass        | Gateway-global cache stays off; Gateway transport always sends `cf-aig-skip-cache: true`. Callers cannot opt into caching; cache receipt fields remain observability only.                                                                                                                             |

## Public Seams

- `ConfigRepository.Resolve/Update` is the gateway connection seam; `ResolveVisionModel` reads only the Vision mapping. Task execution does not read a live Service mapping here.
- `PreviewGatewayRoute` is the routing dry-run seam; `ProbeGatewayConnection` is the connectivity diagnostic seam.
- `ListCatalog/SaveCatalog/DefaultCatalog` is the model catalog seam; saving cleans up a stale Vision mapping in one transaction under a write lock with a single audit record. The catalog holds routing and display metadata only, never prices; SaveCatalog strictly rejects unknown fields (including the removed per-token price fields).
- `DetectModels` is the model discovery seam; `ListProviders` is the provider display seam (display name, ID prefix, gateway namespace).
- `GetModelReference` is the read-only models.dev reference seam (USD/MTok input/output, context limit, last_updated); results are display-only and degrade to an explicit error code on allowlist gaps or upstream failures.
- `ListModelsObserved` is the read-only 7-day first-party observation seam (per-model success rate, top failure_stage/category, cache hit rate, on-sale SKU UnitCredits margin rows); gateway actual cost has no receipt/log source so margin cost stays an explicit unavailable marker, never fabricated.
- Billing charges solely on the billing SKU UnitCredits; no catalog or detection field ever feeds settlement (see ADR-0010).

## Invariants

- Provider keys live in Cloudflare (BYOK); this repo holds only the gateway token; payloads never persist in gateway logs (`collect-log-payload: false`); metadata carries only `task_id` + `feature` (2 of the 5 allowed entries).
- Community catalog IDs and display names stay verbatim; deprecated and malformed entries are skipped.
- Unknown provider slugs synthesize a default display (community guess + `custom-` namespace) instead of silently vanishing.
- All gateway calls use the provider-native surface with an explicit path; there is no OpenAI-compatible rewrite (chat-shaped payloads ride provider-native too, `IsChat` only drives dialect shaping).
- Probes bypass the fallback topology; single-shot semantics verified empirically at implementation time.
- Control-plane lists cache per token hash with short TTL and a bound; the catalog caches 24h; every failure surfaces.

## Non-goals

- No Guardrails, no cost-driven auto-pricing, no Unified Billing, no direct provider SDK calls (zero-code onboarding applies only where endpoint plus existing dialects cover the supplier).
