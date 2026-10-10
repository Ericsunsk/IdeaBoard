# ADR-0013: Frontend structure: feature-first SaaS layout over tiered DDD

Status: Accepted
Owner: Frontend Architecture
Decision date: 2026-10-10
Affected contexts: `canvas` (tools scope: `retouch`, `inpaint`), prospectively `board`, `search`, `account`, `admin`
Supersedes: the Tiered DDD four-layer frontend mandate from ADR-0005 and
`DESIGN.md` Frontend Contract §2/§5/§6 (bounded-context map from ADR-0005 is unchanged)

## Context

`DESIGN.md` Frontend Contract §2 requires heavy core subdomains (`board`,
`canvas`, `search`, `account`, `admin`) to land a standard four-layer split
(`domain/` / `application/` / `infrastructure/` / `presentation` or `ui/`),
and `apps/web/DESIGN.md` repeats the Tiered DDD rule. Purity is enforced by
`domain/canvasDomainPurity.unit.spec.ts`, `dependency-cruiser` direction
rules, `checkNamingGovernance.ts`, and `checkTestGuardrails.ts`.

At tool granularity the ceremony tax is visible. Retouch
(`features/canvas/tools/retouch/`) carries passthrough wrappers that only
rebind pure predicates to props — `Chrome.vue` holds `chromeSelection`,
`stackForCapabilities`, and `canCopy/canMerge/canRemoveSelectedLayer`
computeds (~15 lines) over the framework-free `processing/layerStack.ts`
predicates, plus emit-forwarding (`forwardUpdateParam`). Forcing this scope
into four layers would scatter one control's model, session, and view across
four directories. Meanwhile inpaint (`features/canvas/tools/inpaint/`) ships
the same class of feature flat today (`model.ts`, two `useInpaintChrome*`
composables, `components/`, `fabric/`, `lib/`, `config/`) with no layers at
all — a mandated four-layer split would diverge the two tools instead of
converging them.

The durable value is already leaf-local: `retouch/processing/` (`curves`,
`levels`, `hsl`, `sharpen`, `histogram`, `layerStack`, `engine`) is
framework-free pure math with colocated unit specs. What needs enforcing is
that leaf's purity, not the depth of the tree above it. See
`DESIGN.md` Frontend Contract, `apps/web/DESIGN.md` Directory Structure
section, and `features/canvas/CONTEXT.md`.

## Decision

Frontend adopts a feature-first SaaS layout. The Tiered DDD four-layer
requirement no longer applies to frontend code; feature cohesion, a pure
core, and control-colocated placement replace it. ADR-0005's bounded-context
map is unchanged — only the layering mandate is lifted.

Inside a tool/feature scope (reference shape:
`features/canvas/tools/retouch/`), three zones apply:

1. **`core/` — pure math and types only.** Framework-free: no `vue`
   reactivity, no router/Pinia/TanStack Query, no `generated-query` DTOs.
   This is the only zone where domain purity is enforced (the purity spec
   scope narrows from `domain/` to each scope's `core/`).
2. **`session/` — stateful orchestration and adapters.** `useXSession`,
   `useXMode`, coordinate/histogram/stage adapters, chrome sources, apply
   pipelines, runtime render/WebGL handoffs. May import `vue` reactivity and
   `core/`; owns lifecycle, reactivity, and IO boundaries.
3. **`ui/` — controls colocated by control, not by technical kind.** One
   directory per control (e.g. `curves/`, `levels/`, `histogram/`); each
   control colocates its SFC, local helpers, and specs
   (`.(unit|contract|component|browser).spec.ts`). Cross-control contracts
   stay at the `ui/` root or in `core/`.

Each tool scope keeps one `index.ts` seam as its only legal cross-scope
exit (named re-exports of model types, session entry, apply/chrome
factories, chrome component). Scope internals (today's `processing/*`
equivalents) stay private behind it; no deep imports across tool scopes.

Scopes outside `canvas/tools` keep their current layout until migrated
per-scope. No flag-day rewrite.

## Consequences

What gets easier: a new control lands in one directory; pure math stays
deterministic and unit-tested without passthrough ceremony; `retouch` and
`inpaint` converge on one shape instead of two.

What gets harder / is excluded: four-layer directory uniformity is gone;
reviewers judge `core/` purity and seam discipline, not directory depth.
`domain/`-wide purity assertions must narrow or they will false-fail
legitimate `session/` reactivity.

The following toolchain changes enforce this migration:

- `dependency-cruiser`: add `core/` direction rules (`session/`/`ui/` may
  import `core/`; `core/` imports nothing feature-local; cross-tool deep
  imports forbidden except via the tool `index.ts`).
- `checkNamingGovernance.ts`: permit control-colocated directories; review
  anti-stutter scope for control siblings.
- `checkTestGuardrails.ts` + spec suffix convention: keep
  `.(unit|contract|component|browser).spec.ts`; colocated specs live next to
  their control.
- Narrow `canvasDomainPurity.unit.spec.ts` from `domain/` to per-scope
  `core/` directories.

## Status

Accepted
