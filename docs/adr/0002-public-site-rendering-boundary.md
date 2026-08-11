# ADR-0002: Separate Public Site Rendering From Workspace Rendering

## Context

IdeaBoard needs crawler-readable public content without making the authenticated Canvas Workspace depend on SSR. A single client-rendered shell makes public metadata and first-byte content inconsistent across crawlers, while a separate marketing repository would duplicate deployment, product facts, and brand assets.

## Decision

Keep one frontend repository and domain, but establish a Public Rendering Boundary: Public Site routes are generated as build-time static HTML, while `/user/*`, `/admin/*`, and `/share` remain Vue SPA/CSR routes. Public Site and Workspace may share typed product facts and brand tokens, but each owns its route and rendering responsibility.

## Consequences

Public pages can expose stable HTML, metadata, canonical URLs, and structured data without changing Canvas runtime architecture. The frontend build and deployment must distinguish public static output from the protected SPA fallback, and public content must not depend on authenticated runtime state.

## Status

Accepted
