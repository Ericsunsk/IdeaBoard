import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const srcRoot = path.join(webRoot, 'src')

const MAX_SFC_LINES_THRESHOLD = 1000
const MAX_COMPOSABLE_LINES_THRESHOLD = 1000

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
const violations: string[] = []

for (const file of allFiles) {
  const relativePath = path.relative(webRoot, file)
  const content = fs.readFileSync(file, 'utf8')
  const lines = content.split('\n').length

  if (file.endsWith('.vue')) {
    vueComponents.push({ relativePath, lines })
    if (lines > MAX_SFC_LINES_THRESHOLD) {
      violations.push(
        `Vue SFC "${relativePath}" exceeds ${MAX_SFC_LINES_THRESHOLD} lines (current: ${lines} lines).`,
      )
    }
  } else if (
    path.basename(file).startsWith('use') &&
    file.endsWith('.ts') &&
    !file.endsWith('.spec.ts')
  ) {
    composables.push({ relativePath, lines })
    if (lines > MAX_COMPOSABLE_LINES_THRESHOLD) {
      violations.push(
        `Composable "${relativePath}" exceeds ${MAX_COMPOSABLE_LINES_THRESHOLD} lines (current: ${lines} lines).`,
      )
    }
  }
}

vueComponents.sort((a, b) => b.lines - a.lines)
composables.sort((a, b) => b.lines - a.lines)

console.log('=== Complexity & Component Size Hotspots ===\n')

console.log('Top 5 Largest Vue Components:')
for (const comp of vueComponents.slice(0, 5)) {
  console.log(`  - ${comp.lines.toString().padStart(4, ' ')} lines: ${comp.relativePath}`)
}

console.log('\nTop 5 Largest Composables (excluding generated-query):')
for (const comp of composables.slice(0, 5)) {
  console.log(`  - ${comp.lines.toString().padStart(4, ' ')} lines: ${comp.relativePath}`)
}

if (violations.length > 0) {
  console.error('\n✗ Complexity threshold violations detected:')
  for (const v of violations) {
    console.error(`  - ${v}`)
  }
  process.exit(1)
} else {
  console.log('\n✓ All Vue SFCs and composables are within the 1000-line size budget.')
}
