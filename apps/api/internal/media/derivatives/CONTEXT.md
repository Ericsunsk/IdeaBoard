# Media Derivatives Context

`internal/media/derivatives` owns the tier catalogue, provider-neutral input
profiles, and on-demand rendering for
image derivative tiers (thumbnails, previews, processing inputs).
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

| Term           | Meaning                                                                                               |
| -------------- | ----------------------------------------------------------------------------------------------------- |
| Tier           | Named dimension/resolution boundary (`thumb`, `preview`, `large-preview`, `processing-input`)         |
| TierSpec       | Max-side geometry for one tier; display tiers resolve on demand, never pre-generated                  |
| InputProfile   | Bounded, provider-neutral image contract required before sending assets to external processing models |
| Rendered Image | Validated image bytes and MIME returned by pull rendering; independent of HTTP response policy        |

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
