# Media Derivatives Context

`internal/media/derivatives` owns display-tier geometry, provider-neutral input
profiles, and on-demand image rendering.
It covers transformed views of managed original media and controlled provider
previews, never ownership or materialization of the source image.
No production implementation lives at the package root;
application startup composes the canonical subpackages directly.

## Pull model (ADR-0007)

Display tiers (`thumb`, `preview`, `large-preview`) render on demand
through imgproxy. The API authenticates workspace media reads and delivers bytes
from the internal renderer through same-origin routes. Display tiers
are not persisted; the asset DTO derives its tier map from `TierSpecs`
and asset readiness.

## Package Map

- `domain`: pure models, tier specs, and input profiles; zero external
  dependencies.
- `infrastructure`: `imgproxy` (source selection, signed internal rendering
  requests, and bounded image reads; geometry owned by `domain.TierSpecs`), and
  `preview` (public Search capabilities, provider preview fetches and temporary
  local staging for the shared renderer).
- `tests`: tier geometry and input-profile suite
  in an isolated test package.

## Domain Language

| Term             | Meaning                                                                                                |
| ---------------- | ------------------------------------------------------------------------------------------------------ |
| Display Tier     | A named display-size class: `thumb`, `preview`, or `large-preview`.                                    |
| Input Profile    | Provider-neutral bounds on an image's dimensions, pixel count, and byte size for external processing.  |
| Processing Input | An image supplied for external model processing under an Input Profile; not a Display Tier.            |
| Rendered Image   | A transformed display view of a managed original or validated external preview; not a new Media Asset. |

Use Display Tier for display sizing and Input Profile for processing limits. Neither is a Service's Output Resolution (`1k`, `2k`, `4k`) or a CSS Token Tier.

## Public Seams

- `TierSpecs`: tier catalogue and format policy.
- `InputProfile`: bounded processing-input contract for external models.
- `imgproxy.Renderer`: pull rendering for workspace display tiers, avatars,
  and branding logos. Workspace sources are Local; public media sources use
  the active Storage Profile. HTTP adapters own authorization and response policy.
- `preview.Service`: server-issued, purpose-bound encrypted Search capabilities
  (24 hours) and authorized Library preview rendering. Sources are exact HTTPS
  Unsplash/Pexels image hosts; shared outbound transport rechecks redirects and
  blocks private addresses without provider-host exceptions. Downloads are
  bounded to 20 MiB and eight concurrent renders; random local staging files
  are removed after rendering, including cancellation/failure.
- `Display`: inline tier URLs with `max_side` bounds. Unknown provider-preview
  dimensions never become fabricated `srcset` width descriptors. Search
  descriptors carry `expires_at`; list/session owners refresh expired results.

## Invariants

- `domain` keeps zero external dependencies.
- No production implementation lives at the package root.
- Display tiers are pull-only: no writer records display rows.
- `InputProfile` is required before assets go to external processing models.

## Source and access boundaries

Public Search issues capabilities only from actual validated provider results;
`GET /api/media/previews/:token/:tier` accepts no caller-selected source URL or
workspace identity. Capabilities expire and cannot read managed private assets.
Runtime Media HTTP adapters register public preview delivery independently of
Search query handling. Public capability expiry/error/cache policy remains
explicit, while public and private image responses share successful bytes
headers and projection.
Library rendition URLs remain workspace-authenticated at byte read and bind the
stored preview version. Library application `SourceResolver` selects direct canonical Media links,
completed remote materializations, or the stored provider preview through
read-only persistence. It also supplies completed-link replay to the
materializer and remote-only Canvas read-through. HTTP adapters keep Workspace
access, source-version, Media readiness and response policy; the resolver
requires neither a materializer nor an executor. Browsing never invokes
materialization, creates Media Assets or fetches the original.
Source/import/download references remain separate from display URLs.
