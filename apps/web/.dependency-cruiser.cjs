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
        'Shared modules must not depend on concrete features or app. Only the enumerated P1 compatibility shims have a one-phase exception.',
      from: {
        path: '^src/shared(?:/|$)',
        pathNot:
          '^src/shared/(?:components/(?:UserAccountMenu|UserCreditBalanceButton|userAccountMenu\\.types|SupportChannelQr)|constants/(?:contacts|routePaths|legalLinks)|domain/(?:imageActions|serviceIcons|serviceRegistry)|media/preloadImage)\\.shim\\.ts$',
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
        // `admin/settings/pages/`, `admin/credits/health/`, …) and the workspace
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
        'Features must consume another feature through its public entrypoint; Board domain, application, infrastructure, presentation, and testing modules are private.',
      from: {
        path: '^src/features/[^/]+(?:/|$)',
        pathNot: '^src/features/board(?:/|$)',
      },
      to: {
        path: '^src/features/board/(?:domain|application|infrastructure|presentation|testing)(?:/|$)',
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
        'Features and app layers must consume workspace through its public entrypoint (routes stay public for router chunk splitting; routePaths.ts stays public for cross-feature path constants); shell, infrastructure, layout, and pages modules are private. The single accountWorkspace exception is deliberate: the workspace entrypoint re-exports the shell, which imports account, so routing that file through the entrypoint would close a module cycle (no-circular). (Issue #834)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot:
          '^src/features/workspace(?:/|$)|^src/features/account/application/accountWorkspace\\.ts$',
      },
      to: {
        path: '^src/features/workspace/',
        pathNot: '^src/features/workspace/(?:index\\.ts$|routes/|routePaths\\.ts$)',
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
      name: 'canvas-tool-core-no-zone-imports',
      severity: 'error',
      comment:
        'ADR-0013: tool core/ zones are pure math and types. session/ and ui/ may import core/, never the reverse; core/ also stays clear of the legacy image-tools flat, canvas application/infrastructure/presentation layers, library/search features, and generated-query DTOs. (Cross-scope encapsulation is owned by no-cross-tool-deep-imports; bare framework modules are owned by canvasDomainPurity.)',
      from: {
        path: '^src/features/canvas/tools/[^/]+/core(?:/|$)',
      },
      to: {
        path: '^src/features/canvas/tools/[^/]+/(?:session|ui)(?:/|$)|^src/features/canvas/tools/image-tools(?:/|$)|^src/features/canvas/(?:application|infrastructure|presentation)(?:/|$)|^src/features/(?:library|search)(?:/|$)|^src/api/generated-query(?:/|$)',
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
        'ADR-0013: every consumer outside retouch must go through its index.ts seam; session, core, and ui internals are private. The image-tools consumer exclusion is migration debt, not a pattern: image-tools is the unmigrated legacy flat scope with no index seam yet (no flag-day rewrite), so its pre-existing deep imports are grandfathered until its own migration. Do not extend this exclusion to any other scope. (Issue: frontend SaaS three-zone migration)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot: '^src/features/canvas/tools/(?:retouch|image-tools)(?:/|$)',
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
        'ADR-0013: every consumer outside inpaint must go through its index.ts seam; session, core, and ui internals are private. The image-tools consumer exclusion is migration debt, not a pattern: image-tools is the unmigrated legacy flat scope with no index seam yet (no flag-day rewrite), so its pre-existing deep imports are grandfathered until its own migration. Do not extend this exclusion to any other scope. (Issue: frontend SaaS three-zone migration)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot: '^src/features/canvas/tools/(?:inpaint|image-tools)(?:/|$)',
      },
      to: {
        path: '^src/features/canvas/tools/inpaint/',
        pathNot: '^src/features/canvas/tools/inpaint/index\\.ts$',
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
