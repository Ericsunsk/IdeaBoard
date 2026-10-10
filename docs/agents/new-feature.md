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
   + `ui/` or `presentation/`; or a documented flat exception (`legal`:
   `components/`, `useXxx.ts`, `types.ts`, no empty dirs to fake
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
   rule 10; `checkTestGuardrails` enforces placement). P0 test conventions
   (plan §3.7):
   - Four suffixes: `unit` pins focused deterministic pure logic; `contract`
     covers exactly three categories (API shape without a real backend,
     architecture-policy fixtures with valid/invalid samples, observable
     Seam/singleton identity via `toBe`); `component` pins rendered SFC
     behavior through `src/test/render`; `browser` is reserved for
     real-browser-only geometry. Never add a suffix or a parallel test
     directory for governance.
   - Behavior sentences: `describe` groups the seam, `test` states observable
     behavior (`test('restores selection after locale switch')`). Standardize
     on `test`; grandfathered `it` stays untouched. Never assert private
     implementation details.
   - Factories: grandfathered `makeX` seams stay; new factories use
     `buildX(overrides?)` (`checkTestGuardrails` warns on new exported
     `makeX`, warn-only).
   - Query priority in UI specs: prefer Role queries, TestId is the last
     resort and only for documented visual/layout contracts (same bar as the
     class-assertion exceptions in `checkTestGuardrails`).
   - Query placement: new cached server state must justify Query in the
     feature `CONTEXT.md`; otherwise follow the feature-store pattern
     (`shared/CONTEXT.md` Query Scope Rule, pinned by
     `shared/query/queryPlacement.contract.spec.ts`).
6. **Naming** — kebab-case dirs, `PascalCase.vue` SFCs, `camelCase.ts` modules,
   no parent-prefix stutter (`resource.ts`, not `libraryResource.ts`),
   `index.ts` is barrel-or-documented-boundary only. P0 subset (warn-only in
   ESLint; the canvas Query ban is the sole new error rule): default
   camelCase, typeLike PascalCase, `I-` interface ban, placeholder
   `id-denylist`, abbreviated-identifier replacements, PascalCase at Vue
   registration and in templates. New image/avatar URL fields use `*_url`
   (grandfathered `avatar` shapes stay; `checkNamingGovernance` rule 16
   warns on new ones, warn-only).
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
