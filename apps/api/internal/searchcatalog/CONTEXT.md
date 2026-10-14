# SearchCatalog Context

SearchCatalog is the image-search context with two subdomains: web image search (Unsplash/Pexels provider key lifecycle) and vision AI search (gateway-backed image understanding plus independent billing). Layout follows the split: `domain` (shared vocabulary and types), `application` (search, vision and settlement services), `infrastructure` (settings repository, supplier HTTP adapters), `interfaces/http` (routes and multipart input), `ports` (gateway and testing contracts). The River reconciliation worker lives in `runtime/interfaces/worker/visionbilling`.

## Domain Language

| Term             | Meaning                                                                                                                      |
| ---------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Search Provider  | An external image-search supplier: Unsplash or Pexels; distinct from an AI model provider.                                   |
| ProviderConfig   | A Search Provider's credentials, enabled intent, and connection-test facts.                                                  |
| TestState        | Connection-test lifecycle `never` / `passed` / `failed`; enabling requires a non-empty key plus `passed`                     |
| FailureCategory  | Test failure class `credentials` / `quota_or_permission` / `network_timeout` / `response_format` / `unknown`                 |
| VisionExtraction | Normalized vision understanding result (query text)                                                                          |
| VisionInput      | Validated vision request: up to 4 images (10MB each and total, format triple-checked) plus free text                         |
| Vision Model     | Vision-only model mapping, isolated from Service Model Pool routing under the bare key `vision`                              |
| Vision Billing   | Per-call vision billing (SKU `ai-search:vision`; callers reuse one idempotency key across retries; hold/capture via credits) |

## Public Seams

- `SearchProviderAdapter` isolates supplier requests, payloads and result mapping in `infrastructure/unsplash.go` and `infrastructure/pexels.go`; `providers.go` owns shared outbound response handling and connection tests. Application owns aggregation, cursor progression and result URL validation.
- `ConfigRepository.List/Save/RecordTest/APIKey` is the web-search provider config seam; `APIKey` decrypts only when enabled, tested and present, otherwise fails closed.
- `VisionSearchService.Extract` is the vision extraction seam; the model comes only from the AI Gateway vision mapping and is never defaulted in code.
- Vision executes through `VisionGatewayClient` on the central AI Gateway; no endpoint or key exists in vision settings — those belong to AI Gateway.
- `searchcatalog` never imports the runtime transport; composition injects adapters.

## Invariants

- API keys are encrypted before persistence (`MaterialKindEncrypted`) and decrypted on read; plaintext never reaches memory dumps or logs.
- Storing a new key resets test state to `never`; the enable gate re-checks.
- Vision input/output text is whitespace-normalized; empty or over-`MaxSearchQueryRunes` queries are rejected.
- Oversize vision images (count, single, total, format) fail closed; extra user selections are never silently dropped.
- Vision billing requires an idempotency key; same key with a different body conflicts instead of double-charging.
- Vision mapping is isolated under `ai_gateway_vision_model`; Service Model Pool routing owns image-task model selection, and `vision` is never registered as an image service (see `workflow.ReservedVisionSlug`, locked by test).

## Non-goals

- No generic web-search aggregation beyond the two providers; no automatic vision model selection; no vision result caching.
