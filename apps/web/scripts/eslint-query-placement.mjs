import path from 'node:path'

const QUERY_PACKAGES = ['@tanstack/vue-query', '@tanstack/query-core']
const ALLOWED_ROOTS = [
  'src/features/account/',
  'src/features/admin/',
  'src/features/workspace/',
  'src/shared/',
  'src/core/',
]

function sourcePath(context) {
  return path.relative(context.cwd, context.filename).split(path.sep).join('/')
}

function isHarness(importer) {
  return (
    /\.(spec|test)\.[cm]?[jt]sx?$/.test(importer) ||
    importer.startsWith('src/test/') ||
    /\/testing(?:\/|\.[jt]s$)/.test(importer)
  )
}

function isQuerySource(context, specifier) {
  if (QUERY_PACKAGES.some((name) => specifier === name || specifier.startsWith(`${name}/`)))
    return true
  let target
  if (specifier.startsWith('@/')) target = `src/${specifier.slice(2)}`
  else if (specifier.startsWith('.')) {
    target = path
      .relative(context.cwd, path.resolve(path.dirname(context.filename), specifier))
      .split(path.sep)
      .join('/')
  } else target = specifier
  // These are Query runtime entries; taskResult and credits invalidation are
  // framework-free helpers even while they live under shared/query in P0.
  return /^src\/(shared|core)\/query(?:\/(?:index|queryClient|queryDefaults))?(?:\.[cm]?[jt]s)?$/.test(
    target,
  )
}

function isIdentityCleanup(importer, node) {
  return (
    importer.startsWith('src/features/auth/') &&
    node.type === 'ImportDeclaration' &&
    node.specifiers.length > 0 &&
    node.specifiers.every(
      (specifier) =>
        specifier.type === 'ImportSpecifier' &&
        (specifier.imported.name ?? specifier.imported.value) === 'clearAuthScopedQueryCache',
    )
  )
}

export const queryPlacementRule = {
  meta: {
    type: 'problem',
    schema: [],
    messages: {
      misplaced:
        'Query cache belongs in account/admin/workspace or the shared/core runtime seam; justify new server-state usage in the feature CONTEXT.md before migration.',
    },
  },
  create(context) {
    const importer = sourcePath(context)
    if (
      isHarness(importer) ||
      importer === 'src/app/main.ts' ||
      ALLOWED_ROOTS.some((root) => importer.startsWith(root))
    )
      return {}
    function check(node, source) {
      if (typeof source?.value !== 'string' || !isQuerySource(context, source.value)) return
      if (
        isIdentityCleanup(importer, node) &&
        /(?:shared|core)\/query\/queryClient(?:\.[jt]s)?$/.test(source.value)
      )
        return
      context.report({ node: source, messageId: 'misplaced' })
    }
    return {
      ImportDeclaration(node) {
        check(node, node.source)
      },
      ExportNamedDeclaration(node) {
        check(node, node.source)
      },
      ExportAllDeclaration(node) {
        check(node, node.source)
      },
      ImportExpression(node) {
        check(node, node.source)
      },
      TSImportType(node) {
        check(node, node.source)
      },
      TSImportEqualsDeclaration(node) {
        if (node.moduleReference.type === 'TSExternalModuleReference')
          check(node, node.moduleReference.expression)
      },
    }
  },
}
