# SearchCatalog Context

SearchCatalog is the image-search context with two subdomains: web image search (Unsplash/Pexels provider key lifecycle) and vision AI search (gateway-backed image understanding plus independent billing). Layout follows the split: `domain` (shared vocabulary and types), `application` (search, vision and settlement services), `infrastructure` (settings repository, supplier HTTP adapters), `interfaces/http` (routes and multipart input), `ports` (gateway and testing contracts). The River reconciliation worker lives in `runtime/interfaces/worker/visionbilling`.

> Vision Search is temporarily unavailable: production wires the vision
> seam as nil (`NewVisionBillingService(..., nil)`), so `Extract` fails
> closed with `vision_search_unavailable`. The code, billing seam, and
> `vision` slug reservation stay; only the docs mark the pause. Re-enable
> by injecting a real `VisionGatewayClient` and removing this note.

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
| Vision Billing   | Per-call vision billing (SKU `ai-search:vision`; callers reuse one idempotency key across retries; hold/capture via credits against the consuming Workspace's stable Billing Account, never a personal wallet) |

## Public Seams

- `SearchProviderAdapter` isolates supplier requests, payloads and result mapping in `infrastructure/unsplash.go` and `infrastructure/pexels.go`; `providers.go` owns shared outbound response handling and connection tests. Application owns aggregation, cursor progression and result URL validation.
- `ConfigRepository.List/Save/RecordTest/APIKey` is the web-search provider config seam; `APIKey` decrypts only when enabled, tested and present, otherwise fails closed.
- `VisionSearchService.Extract` is the vision extraction seam; the model comes only from the AI Gateway vision mapping and is never defaulted in code.
- Vision executes through `VisionGatewayClient` on the central AI Gateway; no endpoint or key exists in vision settings — those belong to AI Gateway.
- `searchcatalog` never imports the runtime transport; composition injects adapters.
- HTTP Search projections append Media-owned inline display variants. Public Search
  receives opaque 24-hour preview capabilities only for provider-returned image
  references; source/original URLs remain provenance and import facts. Display
  reads never save a Library reference or materialize a Media Asset. Runtime Media
  HTTP adapters deliver preview bytes and own capability error/cache projection;
  SearchCatalog issues descriptors without rendering images.

## Invariants

- API keys are encrypted before persistence (`MaterialKindEncrypted`) and decrypted on read; plaintext never reaches memory dumps or logs.
- Storing a new key resets test state to `never`; the enable gate re-checks.
- Vision input/output text is whitespace-normalized; empty or over-`MaxSearchQueryRunes` queries are rejected.
- Oversize vision images (count, single, total, format) fail closed; extra user selections are never silently dropped.
- Vision billing requires an idempotency key; same key with a different body conflicts instead of double-charging.
- Vision billing settles the consuming Workspace's stable Billing Account (ADR-0038 fixed creation scope: the Organization pool for org workspaces, the owner's deterministic Personal account for personal workspaces; fail closed). Every operation persists the settlement triple (submitter `user_id`, consumption `workspace_id`, payer `billing_account_id`); Hold/Capture/Release and background reconciliation converge on the stored payer and never re-resolve from the live Owner or membership. The idempotency hash binds workspace + submitter + input, and replay additionally verifies the stored workspace/payer, so cross-workspace, cross-account, cross-operator, and cross-body key reuse conflicts instead of replaying. There is no personal-wallet fallback on this path. The member quota (#946) joins the reservation transaction at the marked boundary; no limit math lives here.
- Vision mapping is isolated under `ai_gateway_vision_model`; Service Model Pool routing owns image-task model selection, and `vision` is never registered as an image service (see `workflow.ReservedVisionSlug`, locked by test).

## Non-goals

- No generic web-search aggregation beyond the two providers; no automatic vision model selection; no vision result caching.
