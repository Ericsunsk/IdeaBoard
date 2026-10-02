# ADR-0002: Separate Public Site Rendering From Workspace Rendering

## Context

IdeaBoard needs crawler-readable public content without making the authenticated Canvas Workspace depend on SSR. A single client-rendered shell makes public metadata and first-byte content inconsistent across crawlers, while a separate marketing repository would duplicate deployment, product facts, and brand assets.

## Decision

Keep one frontend repository and domain, but establish a Public Rendering Boundary: Public Site routes are generated as build-time static HTML, while `/user/*`, `/admin/*`, and `/share` remain Vue SPA/CSR routes. Public Site and Workspace may share typed product facts and brand tokens, but each owns its route and rendering responsibility.

## Consequences

Public pages can expose stable HTML, metadata, canonical URLs, and structured data without changing Canvas runtime architecture. The frontend build and deployment must distinguish public static output from the protected SPA fallback, and public content must not depend on authenticated runtime state.

## Status

Deprecated

The Public Rendering Boundary is no longer a boundary. `features/public-site` no
longer exists and no build-time static HTML generation remains in `apps/web`
(no SSR or prerender step in `vite.config.ts`). The implementation was reverted
in `cb5798c9 revert(web): defer public site homepage`, which removed
`PublicHomePage.vue`, `publicSiteStaticHtml.ts`, and `publicSiteVitePlugin.ts`;
the leftover `features/public-site/CONTEXT.md` went with `24ba6ac8 refactor:
complete legacy API hard cut`.

Current contract: `DESIGN.md` "Frontend Context Map & Architecture Contract",
which states that the anonymous public landing page is not a separate context —
`/` renders `apps/web/src/features/canvas/presentation/pages/CanvasLandingPage.vue`
directly and public access plus SEO are expressed through router meta, and that
new public pages belong to the Canvas presentation layer rather than a parallel
`public-site` vertical.

No successor ADR was written, so this record is `deprecated` rather than
`superseded` by an ADR.
