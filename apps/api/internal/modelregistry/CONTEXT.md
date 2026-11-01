# ModelRegistry Context

ModelRegistry is the control-plane registry for the sole AI egress: configured-provider discovery, model catalog, Vision mapping, route preview, and probing. It owns no Wallet, Ledger, prices, task execution, Service Model Pool, or provider SDKs. (Renamed from `internal/aigateway`; AIGateway is the old name.)

> Vision Search is temporarily unavailable: production passes nil for the
> vision gateway seam, so vision extraction fails closed. The `vision`
> mapping key reservation and billing SKU remain.

## Domain Language

| Term                | Meaning                                                                                                                                                                                                                                                                                                |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Service Model Pool  | The Models eligible for an image Service; owned by Generation, outside gateway configuration.                                                                                                                                                                                                          |
| Vision Mapping      | Vision-search-only mapping under the bare key `vision`, stored as `ai_gateway_vision_model`, isolated from Service routing                                                                                                                                                                             |
| Route Preview       | Worker dry-run validation for a model ID plus custom params (ref validity plus live dry-run when Worker credentials are configured; no billing, no storage writes); shares `ValidateWorkerModel` (single ref-validation seam) with execution so preview and execution never diverge on malformed refs |
| Connection Probe    | Worker dry-run at the single `/v1/dry-run` endpoint, single-shot, results logged only; any HTTP status counts as reachable |
| Configured Provider | Gateway-scoped provider_configs (CF_ACCOUNT_ID/CF_GATEWAY_ID + shared CF_AIG_TOKEN Gateway token) annotated with the ai-proxy Worker /v1/providers capability (`available=false` = CF-configured but Worker-unadapted, shown disabled in Admin); plus the workers-ai first-party constant (always available). Either remote failing degrades in place to the catalog fallback (distinct catalog slugs + workers-ai, warn-logged), never failing the listing; model discovery is manual (provider dropdown + hand-filled model name), no community catalog, no models.dev |
| Receipt             | Gateway acknowledgement headers (model, provider, step, cache status, log/event ID); operations observability and attempt facts only, never billing snapshots                                                                                                                                          |
| Cache Bypass        | Gateway-global cache stays off; Gateway transport always sends `cf-aig-skip-cache: true`. Callers cannot opt into caching; cache receipt fields remain observability only.                                                                                                                             |

## Public Seams

- `ports.ConfigRepository` (`ListCatalog/SaveCatalog/ResolveVisionModel`) is the settings persistence seam; `ports.ConfiguredProviderReader.ListConfiguredProviders` is the provider display seam; `ports.ObservedReader.ListModelsObserved` is the observation seam. Handler and wiring program to these; tests inject fakes.
- `infrastructure.ConfigRepository` owns catalog and Vision mapping persistence over platform settings. `infrastructure.ConfiguredProviders` binds remote configuration and its catalog reader at construction, owns CF/Worker reads, merging and catalog fallback, and does not mutate accepted Task routing. `infrastructure.ObservedRepository` reads attempt facts over the runtime DB; `application.ObservedService` composes them. `interfaces/http` is the thin admin adapter (`NewHandler`).
- `PreviewWorkerRoute` is the Worker dry-run validation seam; `ProbeWorkerConnection` is the Worker connectivity diagnostic seam. Both use neutral `platform/workerwire` request/response definitions and endpoint paths, never Runtime DTOs. Probe requests contain route facts and provider options only; no images, upload slots or generation calls.
- Catalog identity is the case-insensitive pair `(provider, id)`: provider is an explicit registry slug; id is the provider-native name and preserves nested vendor/model segments. Same-name models at different providers are distinct. Vision mappings and routing overrides resolve that pair, never a bare id alone.
- `ListCatalog/SaveCatalog/DefaultCatalog` is the model catalog seam; saving cleans up a stale Vision mapping in one transaction under a write lock with a single audit record. The catalog holds routing and display metadata only, never prices; SaveCatalog strictly rejects unknown fields (including the removed per-token price fields).
- `ListConfiguredProviders` is the provider display seam (CF provider_configs ∩ Worker capability with availability flags, workers-ai first-party constant always available; catalog fallback on any remote failure: display name, ID prefix, gateway namespace, available).
- `ListModelsObserved` is the read-only 7-day first-party observation seam (per-model success rate, top failure_stage/category, cache hit rate, on-sale SKU UnitCredits margin rows); gateway actual cost has no receipt/log source so margin cost stays an explicit unavailable marker, never fabricated.
- Billing charges solely on the billing SKU UnitCredits; no catalog field ever feeds settlement (see ADR-0010).

## Invariants

- Provider keys live in Cloudflare (BYOK); this repo holds no provider keys, only the shared CF_AIG_TOKEN for discovery (VPS env) and the Worker internal secret; payloads never persist in gateway logs (`collect-log-payload: false`); metadata carries only `task_id` + `feature` (2 of the 5 allowed entries).
- Unknown provider slugs synthesize a default display (prettified name + `custom-` namespace) instead of silently vanishing.
- All image generation rides the sole Worker egress (single `/v1/generate` endpoint, no provider path branching); the Worker owns shaping and provider routing, the API sends canonical task facts plus presigned output slots.
- Worker dry-runs (`/v1/dry-run`) are single-shot by construction; Preview/Probe never bill and never write storage.
- Provider discovery performs bounded single-shot HTTP reads: CF uses the current outbound allowlist; the Worker uses its operator-configured endpoint. Either remote failure falls back to the catalog; only a catalog read failure surfaces as an error. There is no TTL or token-hash cache.

## Non-goals

- No Guardrails, no cost-driven auto-pricing, no Unified Billing, no direct provider SDK calls (zero-code onboarding applies only where endpoint plus existing dialects cover the supplier).
