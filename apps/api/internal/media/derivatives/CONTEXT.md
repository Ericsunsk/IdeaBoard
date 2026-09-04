# Media Derivatives Bounded Context

## Responsibilities

`internal/media/derivatives` manages the asynchronous lifecycle, image rendering, consistency verification, and bounded repair of image derivative assets (thumbnails, previews, processing inputs).

## Ubiquitous Language

- **Derivative**: A transformed version of an original media asset rendered to a specific tier and format (e.g. WebP, PNG, JPEG).
- **Tier**: A named dimension/resolution boundary (`thumb`, `preview`, `large-preview`, `processing`, `processing-input`).
- **InputProfile**: A bounded, provider-neutral image contract required before sending assets to external processing models.
- **Reconciler**: A periodic background auditor that verifies physical storage objects match database records and marks missing or damaged rows.
- **Repair**: A bounded, transactional retry mechanism that resets missing or invalid derivatives back to pending until reaching the maximum attempt cap.
- **OutboxDispatcher**: A River periodic job scheduler that coordinates reconciliation, queue hydration, and River job insertion.

## Architectural Layers

- `domain`: Pure domain models, policies, tier specs, input profiles, events, and error sentinels. Has zero external dependencies.
- `ports`: Contracts defining storage access (`Storage`), integrity checking (`Verifier`), image rendering (`Renderer`), and event publishing (`EventPublisher`).
- `application`:
  - `processing`: Rendering workflow coordination, claim, render, persist, complete, and retry scheduling.
  - `reconciliation`: Background audit against storage, orphan object detection, and invalidation notification.
  - `repair`: Transactional recovery of missing/invalid records up to bounded attempt caps.
- `infrastructure`:
  - `rendering`: `GoRenderer` (native Go Catmull-Rom scaling) and optional `VipsRenderer` (libvips CLI).
  - `events`: `UserEventPublisher` adapting domain events to NATS JetStream via `UserEventHub`.
  - `worker`: River worker implementations (`Worker`, `OutboxDispatcher`) and Prometheus metrics.
- `tests`: Integration, edge-case, and scenario test suite in an isolated test package.
- Root: No production implementation is kept at this level. Application startup composes the canonical domain, application, ports, and infrastructure packages directly.
