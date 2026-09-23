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
        path: '^src/(app|features/[^/]+/(components|presentation|pages|app))',
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
