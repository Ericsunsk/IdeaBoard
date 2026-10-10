import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'

/**
 * Regression guard: User-facing surfaces must not contain new Chinese copy.
 *
 * Scope: apps/web/src/**\*.{ts,vue}
 * Excluded by design: features/admin/** (Admin stays Chinese-capable),
 * generated-query/** (generated clients), *.spec.* fixtures, code comments,
 * and reviewed entries in user-copy-locale-allowlist.json (protocol matchers,
 * Admin-only modules, backend-message patterns).
 */

const SRC_ROOT = join(import.meta.dirname, '..', 'src')
const ALLOWLIST_PATH = join(import.meta.dirname, 'user-copy-locale-allowlist.json')
const EXCLUDED_DIR_PARTS = [
  ['features', 'admin'],
  ['api', 'generated-query'],
  ['core', 'i18n', 'locales'],
]
const CJK_PATTERN = /[\u4e00-\u9fff\u3400-\u4dbf]/

type AllowlistEntry = {
  path: string
  reason: string
}

function listFiles(dir: string): string[] {
  const entries = readdirSync(dir)
  return entries.flatMap((entry) => {
    const fullPath = join(dir, entry)
    const stat = statSync(fullPath)
    if (stat.isDirectory()) return listFiles(fullPath)
    return /\.(ts|vue)$/.test(entry) ? [fullPath] : []
  })
}

function isExcluded(relativePath: string): boolean {
  const parts = relativePath.split(sep)
  return EXCLUDED_DIR_PARTS.some((excluded) =>
    excluded.every((part, index) => parts[index] === part),
  )
}

function isSpecFile(relativePath: string): boolean {
  return /\.spec\.(ts|tsx)$/.test(relativePath) || /\.spec\.vue$/.test(relativePath)
}

function stripComments(content: string): string {
  return (
    content
      .replace(/<!--[\s\S]*?-->/g, '')
      .replace(/\/\*[\s\S]*?\*\//g, '')
      .split('\n')
      // Drop line comments while keeping protocol strings like 'https://…' intact.
      .map((line) => line.replace(/(^|[^:'"`])\/\/(?!\/).*/, '$1'))
      .join('\n')
  )
}

function loadAllowlist(): AllowlistEntry[] {
  return JSON.parse(readFileSync(ALLOWLIST_PATH, 'utf8')) as AllowlistEntry[]
}

function main(): void {
  const allowlist = loadAllowlist()
  // Allowlist paths are app-root relative ("src/features/..."); scan paths are src-relative.
  const allowlistedPaths = new Set(allowlist.map((entry) => entry.path.replace(/^src\//, '')))
  const violations: string[] = []

  for (const filePath of listFiles(SRC_ROOT)) {
    const relativePath = relative(SRC_ROOT, filePath)
    if (isExcluded(relativePath) || isSpecFile(relativePath)) continue

    const stripped = stripComments(readFileSync(filePath, 'utf8'))
    if (!CJK_PATTERN.test(stripped)) continue

    if (!allowlistedPaths.has(relativePath.split(sep).join('/'))) {
      stripped.split('\n').forEach((line, index) => {
        if (CJK_PATTERN.test(line)) {
          violations.push(`${relativePath}:${index + 1}: ${line.trim().slice(0, 120)}`)
        }
      })
    }
  }

  if (violations.length > 0) {
    console.error(
      [
        'User-facing Chinese copy detected. Translate it through the i18n boundary',
        '(apps/web/src/core/i18n) or add a reviewed entry to',
        'scripts/user-copy-locale-allowlist.json with a reason.',
        '',
        ...violations,
      ].join('\n'),
    )
    process.exit(1)
  }

  console.log('User copy locale check passed (no unexpected Chinese copy outside Admin).')
}

main()
