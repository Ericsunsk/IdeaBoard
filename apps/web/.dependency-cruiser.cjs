/**
 * Single owner for module-direction rules: no-circular,
 * shared-not-to-features, route-pages-no-direct-generated-query,
 * features-admin-no-public-client, non-admin-no-control-client,
 * no-cross-feature-deep-imports, canvas-tool-core-no-zone-imports,
 * no-cross-tool-deep-imports.
 *
 * File-local Overlay adapter conventions live in eslint.config.js
 * (no-restricted-imports) and are smoke-tested by
 * scripts/checkEslintImportBoundaries.ts. Do not mirror direction
 * rules in both systems.
 *
 * Bare framework modules (`vue`, `vue-router`, `pinia`, `@tanstack/*`,
 * network clients) inside tool `core/` zones are owned by the colocated
 * purity spec (`features/canvas/domain/canvasDomainPurity.unit.spec.ts`),
 * which can see specifiers dependency-cruiser only resolves to node_modules.
 * Do not mirror them here either.
 *
 * @type {import('dependency-cruiser').IConfiguration}
 */
module.exports = {
  forbidden: [
    {
      name: 'no-circular',
      severity: 'error',
      comment:
        'This dependency is part of a cycle. Move shared types or abstractions into independent modules.',
      from: {},
      to: {
        circular: true,
      },
    },
    {
      name: 'shared-not-to-features',
      severity: 'error',
      comment:
        'Shared modules must not depend on concrete features or app. The P1 compatibility shim exception is retired (no .shim.ts remains in src/shared); every shared-to-feature edge is a violation.',
      from: {
        path: '^src/shared(?:/|$)',
      },
      to: {
        path: '^src/(?:features|app)(?:/|$)',
      },
    },
    {
      name: 'horizontal-not-to-features',
      severity: 'error',
      comment:
        'Horizontal modules must not depend on concrete features or app, including type-only and re-export edges.',
      from: {
        path: '^src/(?:core|api|theme|lib|components)(?:/|$)',
      },
      to: {
        path: '^src/(?:features|app)(?:/|$)',
      },
    },
    {
      name: 'route-pages-no-direct-generated-query',
      severity: 'error',
      comment:
        'Route pages and app layers must use feature/resource/shared API adapters instead of generated-query directly.',
      from: {
        // Matched by FILENAME, not by directory depth.
        //
        // The previous pattern was `features/[^/]+/(components|presentation|pages|app)`,
        // which assumes the second path segment is a layer name. `[^/]+` cannot
        // cross a `/`, so only `features/<x>/<layer>/…` matched — 13 of the 31
        // route pages. Every real admin vertical (`admin/users/`,
        // `admin/settings/ui/pages/`, `admin/credits/ui/orders/`, …) and the workspace
        // layout have a *vertical* name in that second segment, so all of them
        // fell through the rule unnoticed.
        //
        // `*Page.vue` / `*Layout.vue` is the definition that router wiring
        // actually uses: every `component:` / route-table entry in
        // `features/*/app/routes.ts` and `features/workspace/routes/` points at
        // one of those two suffixes. The negative fixtures in
        // `scripts/checkDependencyBoundaries.ts` assert this pattern against
        // three separate admin directories so the rule cannot silently shrink
        // back to one directory again.
        path: '^src/app/|src/.*(?:Page|Layout)\\.vue$',
      },
      to: {
        path: '^src/api/generated-query',
      },
    },
    {
      name: 'features-admin-no-public-client',
      severity: 'error',
      comment:
        'Admin surface must use the control client (or legacy generated-query) instead of the public client. (Issue #800)',
      from: {
        path: '^src/features/admin',
      },
      to: {
        path: '^src/api/generated-query/public',
      },
    },
    {
      name: 'non-admin-no-control-client',
      severity: 'error',
      comment:
        'Non-admin features must use the public client (or legacy generated-query) instead of the admin-only control client. (Issue #800)',
      from: {
        path: '^src/features',
        pathNot: '^src/features/admin',
      },
      to: {
        path: '^src/api/generated-query/control',
      },
    },
    {
      name: 'no-cross-feature-deep-imports',
      severity: 'error',
      comment:
        'Features must consume another feature through its public entrypoint or the narrow Board canvas port; Board core, session, ui (including gallery), and testing modules are private.',
      from: {
        path: '^src/features/[^/]+(?:/|$)',
        pathNot: '^src/features/board(?:/|$)',
      },
      to: {
        path: '^src/features/board/(?:core|session|ui|gallery|testing)(?:/|$)',
      },
    },
    {
      name: 'no-cross-feature-deep-imports',
      severity: 'error',
      comment:
        'Features and app layers must consume auth through its public entrypoint; session, components, pages, and root auth modules are private. (Issue #834)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot: '^src/features/auth(?:/|$)',
      },
      to: {
        path: '^src/features/auth/',
        pathNot: '^src/features/auth/index\\.ts$',
      },
    },
    {
      name: 'no-cross-feature-deep-imports',
      severity: 'error',
      comment:
        'Features and app layers must consume workspace through its public entrypoint (routes stay public for router chunk splitting; routePaths.ts stays public for cross-feature path constants; only the ui/dialogs and ui/settings narrow seams stay public for the account avatar menu); shell, infrastructure, layout, pages, and every other ui module are private. The single accountWorkspace exception is deliberate: the workspace entrypoint re-exports the shell, which imports account, so routing that file through the entrypoint would close a module cycle (no-circular). (Issue #834)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot:
          '^src/features/workspace(?:/|$)|^src/features/account/session/accountWorkspace\\.ts$',
      },
      to: {
        path: '^src/features/workspace/',
        pathNot:
          '^src/features/workspace/(?:index\\.ts$|routes/|routePaths\\.ts$|ui/dialogs/index\\.ts$|ui/settings/index\\.ts$)',
      },
    },
    {
      name: 'no-cross-feature-deep-imports',
      severity: 'error',
      comment:
        'Features and app layers must consume library through its public entrypoint; every module behind the barrel is private. (Issue #834)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot: '^src/features/library(?:/|$)',
      },
      to: {
        path: '^src/features/library/',
        pathNot: '^src/features/library/index\\.ts$',
      },
    },
    {
      name: 'no-cross-feature-deep-imports',
      severity: 'error',
      comment:
        'Features and app layers must consume search through its public entrypoint; every module behind the barrel is private. (Issue #834)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot: '^src/features/search(?:/|$)',
      },
      to: {
        path: '^src/features/search/',
        pathNot: '^src/features/search/index\\.ts$',
      },
    },
    {
      name: 'no-cross-feature-deep-imports',
      severity: 'error',
      comment:
        'Features and app layers must consume notification through its public entrypoint; every module behind the barrel is private. (Issue #834)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot: '^src/features/notification(?:/|$)',
      },
      to: {
        path: '^src/features/notification/',
        pathNot: '^src/features/notification/index\\.ts$',
      },
    },
    {
      name: 'core-pure-domain-no-ui',
      severity: 'error',
      comment:
        'Core zones are pure domain models, math, and algorithms. Core must never import UI frameworks (Vue/Reka UI), styling, or infrastructure adapters.',
      from: {
        path: '^src/(?:features/[^/]+/core|core)(?:/|$)',
        pathNot:
          '\\.(?:spec|test)\\.[jt]sx?$|^src/core/(?:components|composables|media|query|router|i18n|lib|auth)/',
      },
      to: {
        path: '(?:^(?:vue|reka-ui|@vueuse/core|tailwindcss)(?:/|$)|(?:^|/)node_modules/(?:vue|reka-ui|@vueuse/core|tailwindcss)(?:/|$)|(?:^src/features/[^/]+/(?:ui|adapters)|^@/features/[^/]+/(?:ui|adapters)))',
      },
    },
    {
      name: 'canvas-tool-core-no-zone-imports',
      severity: 'error',
      comment:
        'ADR-0013: tool core/ zones are pure math and types. session/ and ui/ may import core/, never the reverse; core/ also stays clear of the canvas application/infrastructure/presentation layers, library/search features, and generated-query DTOs. (Cross-scope encapsulation is owned by no-cross-tool-deep-imports; bare framework modules are owned by canvasDomainPurity.)',
      from: {
        path: '^src/features/canvas/tools/[^/]+/core(?:/|$)',
      },
      to: {
        path: '^src/features/canvas/tools/[^/]+/(?:session|ui)(?:/|$)|^src/features/canvas/(?:application|infrastructure|presentation)(?:/|$)|^src/features/(?:library|search)(?:/|$)|^src/api/generated-query(?:/|$)',
      },
    },
    {
      name: 'canvas-tool-core-no-i18n-runtime',
      severity: 'error',
      comment:
        'ADR-0013: core sources carry locale-free label keys; only the pure localeCore constants/types may be imported from core/i18n. Colocated specs may verify display-copy parity. The retired shared/i18n TTL shims are covered too.',
      from: {
        path: '^src/features/canvas/tools/[^/]+/core(?:/|$)',
        pathNot: '\\.spec\\.ts$',
      },
      to: {
        path: '^src/(?:shared|core)/i18n(?:$|/(?!localeCore\\.ts$))',
      },
    },
    {
      name: 'no-cross-tool-deep-imports',
      severity: 'error',
      comment:
        'ADR-0013: consumers outside retouch must use retouch/index.ts; retouch session, core, and ui internals are private. image-tools consumers use image-tools/index.ts or its cycle-safe ui/core narrow sub-barrels where routing through the root barrel would create a mutual-barrel cycle. Spec files are exempt (mocks do not ship; same carve-out as canvas-tool-core-no-i18n-runtime). (Issue: frontend SaaS three-zone migration)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot: '^src/features/canvas/tools/retouch(?:/|$)|\\.spec\\.ts$',
      },
      to: {
        path: '^src/features/canvas/tools/retouch/',
        pathNot: '^src/features/canvas/tools/retouch/index\\.ts$',
      },
    },
    {
      name: 'no-cross-tool-deep-imports',
      severity: 'error',
      comment:
        'ADR-0013: consumers outside inpaint must use inpaint/index.ts; inpaint session, core, and ui internals are private. image-tools consumers use image-tools/index.ts or its cycle-safe ui/core narrow sub-barrels where routing through the root barrel would create a mutual-barrel cycle. Spec files are exempt (mocks do not ship; same carve-out as canvas-tool-core-no-i18n-runtime). (Issue: frontend SaaS three-zone migration)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot: '^src/features/canvas/tools/inpaint(?:/|$)|\\.spec\\.ts$',
      },
      to: {
        path: '^src/features/canvas/tools/inpaint/',
        pathNot: '^src/features/canvas/tools/inpaint/index\\.ts$',
      },
    },
    {
      name: 'no-cross-tool-deep-imports',
      severity: 'error',
      comment:
        'ADR-0013: consumers outside outpaint must use outpaint/index.ts; outpaint session, core, and ui internals are private. image-tools consumers use image-tools/index.ts or its cycle-safe ui/core narrow sub-barrels where routing through the root barrel would create a mutual-barrel cycle. Spec files are exempt (mocks do not ship; same carve-out as canvas-tool-core-no-i18n-runtime). (Issue: frontend SaaS three-zone migration)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot: '^src/features/canvas/tools/outpaint(?:/|$)|\\.spec\\.ts$',
      },
      to: {
        path: '^src/features/canvas/tools/outpaint/',
        pathNot: '^src/features/canvas/tools/outpaint/(?:index\\.ts|core/index\\.ts)$',
      },
    },
    {
      name: 'no-cross-screens-controllers-deep-imports',
      severity: 'error',
      comment:
        'Consumers outside screens/controllers must use controllers/index.ts; controller modules are private. Screens-internal modules may import controllers directly.',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot: '^src/features/canvas/ui/screens(?:/|$)|\\.spec\\.ts$',
      },
      to: {
        path: '^src/features/canvas/ui/screens/controllers/',
        pathNot: '^src/features/canvas/ui/screens/controllers/index\\.ts$',
      },
    },
    {
      name: 'no-cross-vertical-deep-imports',
      severity: 'error',
      comment:
        'Board-style convergence: the credits vertical consumes sibling verticals through their index.ts seams only; models, monitor, prompts, settings, statistics, and users internals are private. Routes in admin/app keep lazy page imports for chunk splitting and are out of scope.',
      from: {
        path: '^src/features/admin/credits(?:/|$)',
      },
      to: {
        path: '^src/features/admin/(?:models|monitor|prompts|settings|statistics|users)/',
        pathNot:
          '^src/features/admin/(?:models|monitor|prompts|settings|statistics|users)/index\\.ts$',
      },
    },
    {
      name: 'no-cross-vertical-deep-imports',
      severity: 'error',
      comment:
        'Board-style convergence: the models vertical consumes sibling verticals through their index.ts seams only; credits, monitor, prompts, settings, statistics, and users internals are private. Routes in admin/app keep lazy page imports for chunk splitting and are out of scope.',
      from: {
        path: '^src/features/admin/models(?:/|$)',
      },
      to: {
        path: '^src/features/admin/(?:credits|monitor|prompts|settings|statistics|users)/',
        pathNot:
          '^src/features/admin/(?:credits|monitor|prompts|settings|statistics|users)/index\\.ts$',
      },
    },
    {
      name: 'no-cross-vertical-deep-imports',
      severity: 'error',
      comment:
        'Board-style convergence: the monitor vertical consumes sibling verticals through their index.ts seams only; credits, models, prompts, settings, statistics, and users internals are private. Routes in admin/app keep lazy page imports for chunk splitting and are out of scope.',
      from: {
        path: '^src/features/admin/monitor(?:/|$)',
      },
      to: {
        path: '^src/features/admin/(?:credits|models|prompts|settings|statistics|users)/',
        pathNot:
          '^src/features/admin/(?:credits|models|prompts|settings|statistics|users)/index\\.ts$',
      },
    },
    {
      name: 'no-cross-vertical-deep-imports',
      severity: 'error',
      comment:
        'Board-style convergence: the prompts vertical consumes sibling verticals through their index.ts seams only; credits, models, monitor, settings, statistics, and users internals are private. Routes in admin/app keep lazy page imports for chunk splitting and are out of scope.',
      from: {
        path: '^src/features/admin/prompts(?:/|$)',
      },
      to: {
        path: '^src/features/admin/(?:credits|models|monitor|settings|statistics|users)/',
        pathNot:
          '^src/features/admin/(?:credits|models|monitor|settings|statistics|users)/index\\.ts$',
      },
    },
    {
      name: 'no-cross-vertical-deep-imports',
      severity: 'error',
      comment:
        'Board-style convergence: the settings vertical consumes sibling verticals through their index.ts seams only; credits, models, monitor, prompts, statistics, and users internals are private. Routes in admin/app keep lazy page imports for chunk splitting and are out of scope.',
      from: {
        path: '^src/features/admin/settings(?:/|$)',
      },
      to: {
        path: '^src/features/admin/(?:credits|models|monitor|prompts|statistics|users)/',
        pathNot:
          '^src/features/admin/(?:credits|models|monitor|prompts|statistics|users)/index\\.ts$',
      },
    },
    {
      name: 'no-cross-vertical-deep-imports',
      severity: 'error',
      comment:
        'Board-style convergence: the statistics vertical consumes sibling verticals through their index.ts seams only; credits, models, monitor, prompts, settings, and users internals are private. Routes in admin/app keep lazy page imports for chunk splitting and are out of scope.',
      from: {
        path: '^src/features/admin/statistics(?:/|$)',
      },
      to: {
        path: '^src/features/admin/(?:credits|models|monitor|prompts|settings|users)/',
        pathNot:
          '^src/features/admin/(?:credits|models|monitor|prompts|settings|users)/index\\.ts$',
      },
    },
    {
      name: 'no-cross-vertical-deep-imports',
      severity: 'error',
      comment:
        'Board-style convergence: the users vertical consumes sibling verticals through their index.ts seams only; credits, models, monitor, prompts, settings, and statistics internals are private. Routes in admin/app keep lazy page imports for chunk splitting and are out of scope.',
      from: {
        path: '^src/features/admin/users(?:/|$)',
      },
      to: {
        path: '^src/features/admin/(?:credits|models|monitor|prompts|settings|statistics)/',
        pathNot:
          '^src/features/admin/(?:credits|models|monitor|prompts|settings|statistics)/index\\.ts$',
      },
    },
  ],
  options: {
    doNotFollow: {
      path: 'node_modules',
    },
    tsConfig: {
      fileName: './tsconfig.app.json',
    },
    tsPreCompilationDeps: true,
    reporterOptions: {
      text: {
        highlightFocused: true,
      },
    },
  },
}
