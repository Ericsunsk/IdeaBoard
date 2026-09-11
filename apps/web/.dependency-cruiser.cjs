/**
 * Single owner for module-direction rules: no-circular,
 * shared-not-to-features, route-pages-no-direct-generated-query,
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
