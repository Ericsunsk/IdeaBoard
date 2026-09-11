import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * File size budget (1000 lines) for every production module in `src`.
 *
 * Scope: Vue SFCs, `use*.ts` composables, and — since the policy originally
 * covered only those two shapes and left plain `.ts` domain/infrastructure
 * modules unbounded — all remaining non-test TypeScript modules. Generated
 * Orval output (`generated-query`) stays excluded; specs are exempt because
 * a large fixture table is not the same smell as a large unit of behavior.
 *
 * New or edited files above the budget fail the check. Files allowed to stay
 * above it are recorded in `grantedLargeFileAllowances` with a reason; an
 * allowance whose file no longer exceeds the budget is reported as stale so
 * it can be removed (same ratchet contract as the naming governance check).
 */

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const srcRoot = path.join(webRoot, 'src')

const MAX_FILE_LINES_THRESHOLD = 1000

/**
 * Files permitted to exceed the size budget, with the reason they still do.
 * Remove an entry as soon as its file drops back under the budget — the
 * stale-allowance report below will point at it.
 */
const grantedLargeFileAllowances = new Map<string, string>([])

type FileMetric = {
  relativePath: string
  lines: number
}

function scanDirectory(dir: string): string[] {
  const files: string[] = []
  const entries = fs.readdirSync(dir, { withFileTypes: true })
  for (const entry of entries) {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      if (
        entry.name !== 'node_modules' &&
        entry.name !== 'dist' &&
        entry.name !== 'generated-query'
      ) {
        files.push(...scanDirectory(full))
      }
    } else if (entry.isFile()) {
      files.push(full)
    }
  }
  return files
}

const allFiles = scanDirectory(srcRoot)
const vueComponents: FileMetric[] = []
const composables: FileMetric[] = []
const modules: FileMetric[] = []
const violations: string[] = []
const seenPaths = new Set<string>()
/** Allowances whose file still genuinely exceeds the budget. */
const activeAllowances = new Set<string>()

for (const file of allFiles) {
  const relativePath = path.relative(webRoot, file)
  const lines = fs.readFileSync(file, 'utf8').split('\n').length
  const isSpec = /\.(unit|contract|component|browser)\.spec\.ts$/.test(file)
  if (isSpec) continue

  const allowed = grantedLargeFileAllowances.has(relativePath)
  if (allowed) seenPaths.add(relativePath)
  const exceeds = lines > MAX_FILE_LINES_THRESHOLD
  if (allowed && exceeds) activeAllowances.add(relativePath)

  if (file.endsWith('.vue')) {
    vueComponents.push({ relativePath, lines })
    if (exceeds && !allowed) {
      violations.push(
        `Vue SFC "${relativePath}" exceeds ${MAX_FILE_LINES_THRESHOLD} lines (current: ${lines} lines).`,
      )
    }
    continue
  }

  if (!file.endsWith('.ts')) continue

  const isComposable = path.basename(file).startsWith('use')
  ;(isComposable ? composables : modules).push({ relativePath, lines })

  if (exceeds && !allowed) {
    const kind = isComposable ? 'Composable' : 'Module'
    violations.push(
      `${kind} "${relativePath}" exceeds ${MAX_FILE_LINES_THRESHOLD} lines (current: ${lines} lines).`,
    )
  }
}

for (const report of [vueComponents, composables, modules]) {
  report.sort((a, b) => b.lines - a.lines)
}

const staleAllowances = [...grantedLargeFileAllowances.keys()].filter(
  (relativePath) => !activeAllowances.has(relativePath),
)

console.log('=== Complexity & Component Size Hotspots ===\n')

const sections: [string, FileMetric[]][] = [
  ['Top 5 Largest Vue Components:', vueComponents],
  ['Top 5 Largest Composables:', composables],
  ['Top 5 Largest Modules (non-composable .ts):', modules],
]
for (const [title, report] of sections) {
  console.log(title)
  for (const metric of report.slice(0, 5)) {
    console.log(`  - ${metric.lines.toString().padStart(4, ' ')} lines: ${metric.relativePath}`)
  }
  console.log()
}

if (staleAllowances.length > 0) {
  console.warn('⚠ Stale size allowances (no longer needed; remove the entry):')
  for (const relativePath of staleAllowances.sort()) {
    const reason = seenPaths.has(relativePath)
      ? `now within the ${MAX_FILE_LINES_THRESHOLD}-line budget`
      : 'file no longer exists'
    console.warn(`  - ${relativePath} (${reason})`)
  }
  console.warn()
}

if (violations.length > 0) {
  console.error('✗ Complexity threshold violations detected:')
  for (const violation of violations) {
    console.error(`  - ${violation}`)
  }
  process.exit(1)
}

console.log(
  `✓ All production modules are within the ${MAX_FILE_LINES_THRESHOLD}-line size budget ` +
    `(${grantedLargeFileAllowances.size} granted allowance(s)).`,
)
