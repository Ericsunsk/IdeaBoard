# Media Derivatives Context

`internal/media/derivatives` owns the asynchronous lifecycle, image rendering,
consistency verification, and bounded repair of image derivative assets
(thumbnails, previews, processing inputs).
It covers only transformed derivatives of original media assets,
never the originals themselves.
No production implementation lives at the package root;
application startup composes the canonical subpackages directly.

## Package Map

- `domain`: pure models, policies, tier specs, input profiles, events,
  and error sentinels; zero external dependencies.
- `ports`: contracts for storage access (`Storage`), integrity checking
  (`Verifier`), image rendering (`Renderer`), and event publishing
  (`EventPublisher`).
- `application`: `processing` (claim, render, persist, complete, retry
  scheduling); `reconciliation` (storage audit, orphan detection,
  invalidation notification); `repair` (transactional recovery
  under attempt caps).
- `infrastructure`: `rendering` (`GoRenderer` native Catmull-Rom scaling,
  optional `VipsRenderer` libvips CLI); `events` (`UserEventPublisher`
  adapting domain events to NATS JetStream via `UserEventHub`);
  `worker` (River `Worker`, `OutboxDispatcher`, Prometheus metrics).
- `tests`: integration, edge-case, and scenario suite
  in an isolated test package.

## Domain Language

| Term | Meaning |
| ---- | ------- |
| Derivative | Transformed version of an original media asset rendered to a specific tier and format (e.g. WebP, PNG, JPEG) |
| Tier | Named dimension/resolution boundary (`thumb`, `preview`, `large-preview`, `processing`, `processing-input`) |
| InputProfile | Bounded, provider-neutral image contract required before sending assets to external processing models |
| Reconciler | Periodic background auditor verifying physical storage objects match database records; marks missing or damaged rows |
| Repair | Bounded transactional retry resetting missing or invalid derivatives to pending, up to a maximum attempt cap |
| OutboxDispatcher | River periodic job scheduler coordinating reconciliation, queue hydration, and River job insertion |

## Public Seams

- `Renderer`: renders a derivative for a tier/format.
  `GoRenderer` is the native implementation;
  `VipsRenderer` is the optional libvips CLI.
- `Storage`: derivative object persistence access.
- `Verifier`: integrity checking of stored derivatives.
- `EventPublisher` / `UserEventPublisher`: publishes domain events
  to NATS JetStream via `UserEventHub`.
- `processing`: coordinates claim, render, persist, complete,
  and retry scheduling.
- `reconciliation`: background audit of storage against records,
  orphan object detection, and invalidation notification.
- `repair`: transactional recovery of missing/invalid records
  up to bounded attempt caps.
- `Worker` / `OutboxDispatcher`: River worker implementations and periodic
  scheduling of reconciliation, queue hydration, and job insertion.

## Invariants

- `domain` keeps zero external dependencies.
- No production implementation lives at the package root.
- `InputProfile` is required before assets go to external processing models.
- Repair retries are bounded by a maximum attempt cap.
