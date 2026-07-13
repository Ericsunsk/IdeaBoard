import { existsSync, readdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const legacyCurrencyName = String.fromCodePoint(0x70b9, 0x5b50)
const scannedExtensions = new Set(['.go', '.md', '.ts', '.vue', '.yaml', '.yml'])
const ignoredDirectories = new Set([
  '.git',
  '.codegraph',
  '.playwright-cli',
  'coverage',
  'dist',
  'node_modules',
])

function findRepositoryRoot(start: string) {
  let candidate = path.resolve(start)
  while (true) {
    if (
      existsSync(path.join(candidate, 'DESIGN.md')) &&
      existsSync(path.join(candidate, 'apps/web/package.json'))
    ) {
      return candidate
    }

    const parent = path.dirname(candidate)
    if (parent === candidate) throw new Error(`IdeaBoard repository root not found from ${start}`)
    candidate = parent
  }
}

function collectFiles(target: string): string[] {
  const entries = readdirSync(target, { withFileTypes: true })
  return entries.flatMap((entry) => {
    if (entry.isDirectory() && ignoredDirectories.has(entry.name)) return []

    const entryPath = path.join(target, entry.name)
    if (entry.isDirectory()) return collectFiles(entryPath)
    return scannedExtensions.has(path.extname(entry.name)) ? [entryPath] : []
  })
}

function isAllowedTechnicalFixture(relativePath: string, line: string) {
  return (
    relativePath === 'apps/api/internal/billing/domain_test.go' &&
    line.includes(`input: "${legacyCurrencyName}:image"`)
  )
}

describe('human-facing currency copy', () => {
  it('does not expose the retired currency name in product surfaces or documentation', () => {
    const repositoryRoot = findRepositoryRoot(process.cwd())
    const files = collectFiles(repositoryRoot)
    const violations = files.flatMap((file) => {
      const relativePath = path.relative(repositoryRoot, file)
      return readFileSync(file, 'utf8')
        .split(/\r?\n/)
        .flatMap((line, index) =>
          line.includes(legacyCurrencyName) && !isAllowedTechnicalFixture(relativePath, line)
            ? [`${relativePath}:${index + 1}`]
            : [],
        )
    })

    expect(violations).toEqual([])
  })
})
