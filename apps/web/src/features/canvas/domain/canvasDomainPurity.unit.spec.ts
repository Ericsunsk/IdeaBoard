import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const domainDir = dirname(fileURLToPath(import.meta.url))

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

const BARE_FORBIDDEN = [/^vue$/, /^@vue-flow\//, /^axios$/, /^ofetch$/]

const RESOLVED_FORBIDDEN_PREFIXES = [
  'src/features/canvas/application/',
  'src/features/canvas/infrastructure/',
  'src/features/canvas/presentation/',
  'src/features/canvas/tools/',
  'src/features/library/',
  'src/features/search/',
  'src/shared/i18n',
  'src/shared/api/generated-query/',
]

function resolveWebRelative(sourceFile: string, specifier: string): string | null {
  if (!specifier.startsWith('.')) return null
  const webRoot = resolve(domainDir, '..', '..', '..')
  const resolved = resolve(dirname(sourceFile), specifier)
  const candidates = [resolved, `${resolved}.ts`, join(resolved, 'index.ts')]
  const existing = candidates.find((candidate) => existsSync(candidate))
  return relative(webRoot, existing ?? resolved)
    .split('\\')
    .join('/')
}

describe('canvas domain purity', () => {
  it('keeps application runtimes out of domain/', () => {
    expect(existsSync(join(domainDir, 'graph', 'application'))).toBe(false)
  })

  it('has zero Vue/network/DTO imports in domain sources', () => {
    const violations: string[] = []
    for (const file of collectSourceFiles(domainDir)) {
      const display = relative(domainDir, file).split('\\').join('/')
      const source = readFileSync(file, 'utf8')
      for (const specifier of collectImportSpecifiers(source)) {
        if (BARE_FORBIDDEN.some((pattern) => pattern.test(specifier))) {
          violations.push(`${display} imports forbidden bare module '${specifier}'`)
          continue
        }
        const resolved = resolveWebRelative(file, specifier)
        if (resolved && RESOLVED_FORBIDDEN_PREFIXES.some((prefix) => resolved.startsWith(prefix))) {
          violations.push(`${display} imports forbidden module '${specifier}' (${resolved})`)
        }
      }
      if (/\btoRaw\s*\(/.test(source)) {
        violations.push(`${display} calls Vue toRaw (use cloneJsonValue instead)`)
      }
    }
    expect(violations).toEqual([])
  })
})
