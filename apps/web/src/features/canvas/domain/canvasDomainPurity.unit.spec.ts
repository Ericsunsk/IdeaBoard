import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const domainDir = dirname(fileURLToPath(import.meta.url))
const toolsDir = resolve(domainDir, '..', 'tools')

/**
 * ADR-0013: purity is enforced per tool-scope `core/`, not `domain/`-wide.
 * `session/` legitimately owns reactivity and IO, and `ui/` owns controls,
 * so a `domain/`-wide assertion would false-fail them. Each migrated tool
 * scope (`retouch`, `inpaint`, …) contributes its `core/` directory here;
 * unmigrated scopes without a `core/` directory stay out until migrated.
 */
function collectCoreDirs(): string[] {
  return readdirSync(toolsDir)
    .map((entry) => join(toolsDir, entry, 'core'))
    .filter((coreDir) => existsSync(coreDir) && statSync(coreDir).isDirectory())
    .sort()
}

function collectSourceFiles(dir: string): string[] {
  return readdirSync(dir)
    .map((entry) => join(dir, entry))
    .flatMap((path) => {
      if (statSync(path).isDirectory()) return collectSourceFiles(path)
      if (!path.endsWith('.ts') || path.endsWith('.spec.ts')) return []
      return [path]
    })
    .sort()
}

function collectImportSpecifiers(source: string): string[] {
  const specifiers: string[] = []
  const patterns = [
    /(?:^|[;}\s])import(?:\s+[^'"]*?\s+from)?\s*['"]([^'"]+)['"]/g,
    /export\s+[^'"]*?\s+from\s*['"]([^'"]+)['"]/g,
    /import\(\s*['"]([^'"]+)['"]\s*\)/g,
  ]
  for (const pattern of patterns) {
    for (const match of source.matchAll(pattern)) {
      if (match[1]) specifiers.push(match[1])
    }
  }
  return specifiers
}

const BARE_FORBIDDEN = [
  /^vue$/,
  /^vue-router$/,
  /^vue-i18n$/,
  /^pinia$/,
  /^@tanstack\//,
  /^@vue-flow\//,
  /^axios$/,
  /^ofetch$/,
]

const RESOLVED_FORBIDDEN_PREFIXES = [
  'src/features/canvas/application/',
  'src/features/canvas/infrastructure/',
  'src/features/canvas/presentation/',
  'src/features/canvas/tools/image-tools/',
  'src/features/library/',
  'src/features/search/',
  'src/shared/api/generated-query/',
]

/**
 * Documented carve-outs (not forbidden above):
 *
 * - `src/shared/i18n/localeCore.ts`: pure locale constants and types only;
 *   the i18n runtime and barrels initialize Vue reactivity and are forbidden.
 * - `src/features/board/ports/`: `retouch/core/recipe.ts` type-imports the
 *   recipe contract (`import type`, erased at compile) shared with board.
 * - other tool `core/` directories: pure-to-pure sharing stays
 *   framework-free; cross-scope encapsulation is owned by the
 *   `no-cross-tool-deep-imports` dependency-cruiser rules, not mirrored here.
 */
function isForbiddenToolTarget(resolved: string): boolean {
  if (!resolved.startsWith('src/features/canvas/tools/')) return false
  return !/\/core\//.test(resolved)
}

function isForbiddenI18nTarget(resolved: string): boolean {
  return (
    (resolved === 'src/shared/i18n' || resolved.startsWith('src/shared/i18n/')) &&
    resolved !== 'src/shared/i18n/localeCore.ts'
  )
}

function resolveWebRelative(sourceFile: string, specifier: string): string | null {
  const webRoot = resolve(domainDir, '..', '..', '..', '..')
  if (!specifier.startsWith('.') && !specifier.startsWith('@/')) return null
  const resolved = specifier.startsWith('@/')
    ? resolve(webRoot, 'src', specifier.slice(2))
    : resolve(dirname(sourceFile), specifier)
  const candidates = [resolved, `${resolved}.ts`, join(resolved, 'index.ts')]
  const existing = candidates.find((candidate) => existsSync(candidate))
  return relative(webRoot, existing ?? resolved)
    .split('\\')
    .join('/')
}

describe('canvas tool core purity', () => {
  it('rejects relative and aliased i18n runtime/barrel imports while allowing localeCore', () => {
    const sourceFile = join(toolsDir, 'retouch', 'core', 'layers', 'layerStack.ts')
    for (const specifier of [
      '../../../../../../shared/i18n',
      '@/shared/i18n',
      '@/shared/i18n/index.ts',
      '@/shared/i18n/i18nRuntime',
      '@/shared/i18n/userLocale',
    ]) {
      expect(isForbiddenI18nTarget(resolveWebRelative(sourceFile, specifier)!)).toBe(true)
    }
    expect(isForbiddenI18nTarget(resolveWebRelative(sourceFile, '@/shared/i18n/localeCore')!)).toBe(
      false,
    )
  })

  it('keeps application runtimes out of domain/', () => {
    expect(existsSync(join(domainDir, 'graph', 'application'))).toBe(false)
  })

  it('has zero Vue/network/DTO/zone imports in tool core sources', () => {
    const coreDirs = collectCoreDirs()
    expect(coreDirs.length).toBeGreaterThan(0)
    const violations: string[] = []
    for (const coreDir of coreDirs) {
      const scope = relative(toolsDir, coreDir).split('\\').join('/')
      for (const file of collectSourceFiles(coreDir)) {
        const display = `${scope}/${relative(coreDir, file).split('\\').join('/')}`
        const source = readFileSync(file, 'utf8')
        for (const specifier of collectImportSpecifiers(source)) {
          if (BARE_FORBIDDEN.some((pattern) => pattern.test(specifier))) {
            violations.push(`${display} imports forbidden bare module '${specifier}'`)
            continue
          }
          const resolved = resolveWebRelative(file, specifier)
          if (
            resolved &&
            (RESOLVED_FORBIDDEN_PREFIXES.some((prefix) => resolved.startsWith(prefix)) ||
              isForbiddenToolTarget(resolved) ||
              isForbiddenI18nTarget(resolved))
          ) {
            violations.push(`${display} imports forbidden module '${specifier}' (${resolved})`)
          }
        }
        if (/\btoRaw\s*\(/.test(source)) {
          violations.push(`${display} calls Vue toRaw (use cloneJsonValue instead)`)
        }
      }
    }
    expect(violations).toEqual([])
  })
})
