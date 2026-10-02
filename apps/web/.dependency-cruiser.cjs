/**
 * Single owner for module-direction rules: no-circular,
 * shared-not-to-features, route-pages-no-direct-generated-query,
 * features-admin-no-public-client, non-admin-no-control-client,
 * no-cross-feature-deep-imports.
 *
 * File-local Overlay adapter conventions live in eslint.config.js
 * (no-restricted-imports) and are smoke-tested by
 * scripts/checkEslintImportBoundaries.ts. Do not mirror direction
 * rules in both systems.
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
      comment: 'Shared modules must not depend on concrete feature modules.',
      from: {
        path: '^src/shared',
      },
      to: {
        path: '^src/(features|app)',
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
        path: '^src/shared/api/generated-query',
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
        path: '^src/shared/api/generated-query/public',
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
        path: '^src/shared/api/generated-query/control',
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
        'Features and app layers must consume workspace through its public entrypoint (routes stay public for router chunk splitting); shell, infrastructure, layout, and pages modules are private. The single accountWorkspace exception is deliberate: the workspace entrypoint re-exports the shell, which imports account, so routing that file through the entrypoint would close a module cycle (no-circular). (Issue #834)',
      from: {
        path: '^src/(?:app|features/[^/]+)(?:/|$)',
        pathNot:
          '^src/features/workspace(?:/|$)|^src/features/account/application/accountWorkspace\\.ts$',
      },
      to: {
        path: '^src/features/workspace/',
        pathNot: '^src/features/workspace/(?:index\\.ts$|routes/)',
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
