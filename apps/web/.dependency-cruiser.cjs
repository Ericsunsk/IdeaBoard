/** @type {import('dependency-cruiser').IConfiguration} */
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
        path: '^src/(app|features/[^/]+/(components|pages|app))',
      },
      to: {
        path: '^src/shared/api/generated-query',
      },
    },
    {
      name: 'no-cross-feature-deep-imports',
      severity: 'error',
      comment: 'Features must not import internal implementation details of other features.',
      from: {
        path: '^src/features/([^/]+)',
      },
      to: {
        path: '^src/features/[^/]+/.*internal',
        pathNot: '^src/features/$1',
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
