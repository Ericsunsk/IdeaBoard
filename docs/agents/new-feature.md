# New Feature Scaffolding Checklist

> Scope: web-only (`apps/web/src/features/*`). Backend (`apps/api`) and Auth (`apps/auth`) changes follow their own `CONTEXT.md` + ADRs, not this checklist.

Use this when creating a feature under `apps/web/src/features/*` or promoting a
flat module into a layered one. It answers one question: *what shape must a new
feature have before it lands?* Contracts live in `DESIGN.md` (Frontend Context
Map §1–§6) and `apps/web/DESIGN.md`; this file is the task checklist, not a
second source of truth.

## Pre-check

Do not create a sibling doc for what is already covered: `domain.md` (read
path), `issue-tracker.md` / `triage-labels.md` (process), `shared/CONTEXT.md`
(shared seams). Extend this checklist instead of forking it.

## Checklist

1. **Barrel `index.ts` at the feature root** — the sole public entrypoint other
   features may import. Cross-feature deep imports (`../library/components/…`)
   are forbidden; `shared` never imports `features/*`.
   - Documented exceptions (also encoded in
     `scripts/checkPublicApiSurface.ts` `INTENTIONAL_FLATS`): `admin` (vertical
     contexts, no single barrel) and `canvas` (intra-feature seams listed in
     `knip.jsonc` `entry` instead of a root barrel).
2. **`CONTEXT.md`** — domain language table, public seams, invariants,
   non-goals. Update it when domain language changes (DESIGN.md Change Rule 7).
3. **Layer split for complex features, flat for light ones** —
   `domain/` (pure: no Vue, router, Query, DTOs) + `application/` (use-case
   orchestration) + `infrastructure/` (transport/storage adapters, DTO mapping)
   + `ui/` or `presentation/`; or a documented flat exception (`legal`,
   `public-site`: `components/`, `useXxx.ts`, `types.ts`, no empty dirs to fake
   the template). Record the choice (layered vs flat + why) in the feature
   `CONTEXT.md` or the PR description.
4. **Admin adapter boundary** — route pages and components never import
   `shared/api/generated-query` directly. Target shape per vertical context:
   `pages/` → `application/` use case → `infrastructure/` or `*Resource.ts` /
   `api.ts` adapter → generated code. New verticals must land with this shape;
   existing flats converge toward it (pilot: `settings`, the widest scatter).
   Building full `application/` anti-corruption adapters for every legacy area
   at once is out of scope — do it area by area.
5. **Specs colocate with suffixes** — every module ships its spec next to it:
   `curves.ts` → `curves.unit.spec.ts`, SFC → `*.component.spec.ts`, contract →
   `*.contract.spec.ts`, browser-only → `*.browser.spec.ts` (naming governance
   rule 10; `checkTestGuardrails` enforces placement).
6. **Naming** — kebab-case dirs, `PascalCase.vue` SFCs, `camelCase.ts` modules,
   no parent-prefix stutter (`resource.ts`, not `libraryResource.ts`),
   `index.ts` is barrel-or-documented-boundary only.
7. **Verification** — `npm run api:surface:check` (barrel presence + surface
   baseline), `vue-tsc -p tsconfig.app.json --noEmit`, `eslint` on touched
   files, `vitest run` on touched areas, `npm run build`.
8. **Composable signatures** — options-object for two or more parameters
   (`useAdminUsersContext({ feedback, confirm })`,
   `useUsageStatisticsQualityTrendCharts(options)`); a single required
   positional arg stays positional (`enabled` flags, source refs like
   `adminStatistics` / `flowNodeRef` / `filters`, `isDataSettled`).
   `create*Feedback(toast, ...wiring)` factories keep `(toast)` first
   positionally by convention (`createBoardFeedback`,
   `createCanvasFeedback`). Never wrap a single arg in an options-object.
