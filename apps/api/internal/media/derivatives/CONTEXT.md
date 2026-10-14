# Media Derivatives Context

`internal/media/derivatives` owns display-tier geometry, provider-neutral input
profiles, and on-demand image rendering.
It covers only transformed views of original media assets,
never the originals themselves.
No production implementation lives at the package root;
application startup composes the canonical subpackages directly.

## Pull model (ADR-0007)

Display tiers (`thumb`, `preview`, `large-preview`) render on demand
through imgproxy. The API authenticates media reads and delivers bytes
from the internal renderer through same-origin routes. Display tiers
are not persisted; the asset DTO derives its tier map from `TierSpecs`
and asset readiness.

## Package Map

- `domain`: pure models, tier specs, and input profiles; zero external
  dependencies.
- `infrastructure`: `imgproxy` (source selection, signed internal rendering
  requests, and bounded image reads; geometry owned by `domain.TierSpecs`).
- `tests`: tier geometry and input-profile suite
  in an isolated test package.

## Domain Language

| Term             | Meaning                                                                                               |
| ---------------- | ----------------------------------------------------------------------------------------------------- |
| Display Tier     | A named display-size class: `thumb`, `preview`, or `large-preview`.                                   |
| Input Profile    | Provider-neutral bounds on an image's dimensions, pixel count, and byte size for external processing. |
| Processing Input | An image supplied for external model processing under an Input Profile; not a Display Tier.           |
| Rendered Image   | A transformed view of an Original Image; not a new Media Asset.                                       |

Use Display Tier for display sizing and Input Profile for processing limits. Neither is a Service's Output Resolution (`1k`, `2k`, `4k`) or a CSS Token Tier.

## Public Seams

- `TierSpecs`: tier catalogue and format policy.
- `InputProfile`: bounded processing-input contract for external models.
- `imgproxy.Renderer`: pull rendering for workspace display tiers, avatars,
  and branding logos. Workspace sources are Local; public media sources use
  the active Storage Profile. HTTP adapters own authorization and response policy.

## Invariants

- `domain` keeps zero external dependencies.
- No production implementation lives at the package root.
- Display tiers are pull-only: no writer records display rows.
- `InputProfile` is required before assets go to external processing models.
