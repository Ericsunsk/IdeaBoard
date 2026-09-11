import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * Public API surface ratchet for declared feature entrypoints.
 *
 * Each barrel listed in `knip.jsonc`'s `entry` array is a documented seam that
 * other features consume. Those barrels use a mix of explicit `export { ... }`
 * and `export *`, and an `export *` chain silently widens the public surface
 * whenever an internal module grows a new export — a leak that is invisible in
 * review and expensive to retract later (consumers start depending on it).
 *
 * This check expands every entrypoint to its full transitive symbol set and
 * compares it against `publicApiSurface.baseline.json`. Adding or removing a
 * public symbol must therefore be a deliberate edit to the baseline, which is
 * what makes the surface reviewable in a diff.
 *
 * Run `npm run api:surface:update` to regenerate the baseline after an
 * intentional change.
 */

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const baselinePath = path.join(webRoot, 'publicApiSurface.baseline.json')
const knipConfigPath = path.join(webRoot, 'knip.jsonc')

const EXPORT_DECL =
  /^export\s+(?:declare\s+)?(?:async\s+)?(?:type|interface|class|function|const|let|var|enum)\s+([A-Za-z_$][\w$]*)/gm
const EXPORT_DEFAULT = /^export\s+default\b/m
const EXPORT_NAMED = /^export\s+(?:type\s+)?\{([\s\S]*?)\}\s*(?:from\s*['"]([^'"]+)['"])?/gm
const EXPORT_STAR = /^export\s*\*\s*(?:as\s+([\w$]+)\s*)?from\s*['"]([^'"]+)['"]/gm

/** Strip `//` line comments without touching string contents. */
function stripJsonComments(source: string): string {
  let result = ''
  let inString = false
  for (let index = 0; index < source.length; index += 1) {
    const char = source[index]
    if (char === '"') {
      inString = !inString
      result += char
      continue
    }
    if (!inString && char === '/' && source[index + 1] === '/') {
      while (index < source.length && source[index] !== '\n') index += 1
      continue
    }
    result += char
  }
  return result.replace(/,(\s*[\]}])/g, '$1')
}

function readEntrypoints(): string[] {
  const raw = fs.readFileSync(knipConfigPath, 'utf8')
  const parsed = JSON.parse(stripJsonComments(raw)) as { entry?: string[] }
  return [...(parsed.entry ?? [])].sort()
}

function resolveModule(fromFile: string, specifier: string): string | null {
  if (!specifier.startsWith('.')) return null
  const base = path.resolve(path.dirname(fromFile), specifier)
  const candidates = [base, `${base}.ts`, `${base}.vue`, path.join(base, 'index.ts')]
  return candidates.find((file) => fs.existsSync(file) && fs.statSync(file).isFile()) ?? null
}

type SurfaceScan = { symbols: Set<string>; files: Set<string> }

/** Expand an entrypoint to every symbol it exposes, following `export *`. */
function scanEntrypoint(file: string, seen = new Set<string>()): SurfaceScan {
  const symbols = new Set<string>()
  const files = new Set<string>()
  if (seen.has(file)) return { symbols, files }
  seen.add(file)
  files.add(file)

  const source = fs.readFileSync(file, 'utf8')

  for (const match of source.matchAll(EXPORT_DECL)) symbols.add(match[1] ?? '')
  if (EXPORT_DEFAULT.test(source)) symbols.add('default')

  for (const match of source.matchAll(EXPORT_NAMED)) {
    for (const raw of (match[1] ?? '').split(',')) {
      const specifier = raw
        .trim()
        .replace(/^type\s+/, '')
        .trim()
      const exportedName =
        specifier
          .split(/\s+as\s+/)
          .at(-1)
          ?.trim() ?? ''
      if (/^[A-Za-z_$][\w$]*$/.test(exportedName)) symbols.add(exportedName)
    }
  }

  for (const match of source.matchAll(EXPORT_STAR)) {
    if (match[1]) {
      symbols.add(match[1])
      continue
    }
    const target = resolveModule(file, match[2] ?? '')
    if (!target) continue
    const nested = scanEntrypoint(target, seen)
    nested.symbols.forEach((name) => symbols.add(name))
    nested.files.forEach((entry) => files.add(entry))
  }

  symbols.delete('')
  return { symbols, files }
}

function collectSurface(): Record<string, string[]> {
  const surface: Record<string, string[]> = {}
  for (const entrypoint of readEntrypoints()) {
    const absolute = path.resolve(webRoot, entrypoint)
    if (!fs.existsSync(absolute)) continue
    const { symbols } = scanEntrypoint(absolute)
    surface[entrypoint] = [...symbols].sort()
  }
  return surface
}

const current = collectSurface()

if (process.argv.includes('--update')) {
  fs.writeFileSync(baselinePath, `${JSON.stringify(current, null, 2)}\n`)
  const total = Object.values(current).reduce((sum, names) => sum + names.length, 0)
  console.log(
    `✓ Wrote public API baseline: ${Object.keys(current).length} entrypoint(s), ${total} symbol(s).`,
  )
  process.exit(0)
}

if (!fs.existsSync(baselinePath)) {
  console.error(`✗ Missing public API baseline at ${path.relative(webRoot, baselinePath)}.`)
  console.error('  Run: npm run api:surface:update')
  process.exit(1)
}

const baseline = JSON.parse(fs.readFileSync(baselinePath, 'utf8')) as Record<string, string[]>
const problems: string[] = []

for (const entrypoint of Object.keys(current)) {
  if (!(entrypoint in baseline)) {
    problems.push(`Entrypoint "${entrypoint}" is declared but absent from the baseline.`)
  }
}
for (const entrypoint of Object.keys(baseline)) {
  if (!(entrypoint in current)) {
    problems.push(`Entrypoint "${entrypoint}" is in the baseline but no longer declared.`)
  }
}

for (const [entrypoint, names] of Object.entries(current)) {
  const expected = baseline[entrypoint]
  if (!expected) continue
  const currentSet = new Set(names)
  const baselineSet = new Set(expected)
  const added = names.filter((name) => !baselineSet.has(name))
  const removed = expected.filter((name) => !currentSet.has(name))
  if (added.length > 0) {
    problems.push(
      `"${entrypoint}" exposes ${added.length} new public symbol(s): ${added.join(', ')}`,
    )
  }
  if (removed.length > 0) {
    problems.push(
      `"${entrypoint}" no longer exposes ${removed.length} symbol(s): ${removed.join(', ')}`,
    )
  }
}

if (problems.length > 0) {
  console.error('✗ Public API surface drift detected:')
  for (const problem of problems) console.error(`  - ${problem}`)
  console.error('\nIf this change is intentional, run: npm run api:surface:update')
  process.exit(1)
}

const total = Object.values(current).reduce((sum, names) => sum + names.length, 0)
console.log(
  `✓ Public API surface matches the baseline (${Object.keys(current).length} entrypoint(s), ${total} symbol(s)).`,
)
