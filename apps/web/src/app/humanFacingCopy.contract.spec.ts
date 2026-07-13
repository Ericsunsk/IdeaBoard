import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const legacyCurrencyName = String.fromCodePoint(0x70b9, 0x5b50)
const scannedExtensions = new Set(['.go', '.md', '.ts', '.tsx', '.vue', '.yaml', '.yml'])

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

function listTrackedSourceFiles(repositoryRoot: string) {
  return execFileSync('git', ['ls-files', '-z'], {
    cwd: repositoryRoot,
    encoding: 'utf8',
  })
    .split('\0')
    .filter(Boolean)
    .filter((relativePath) => !relativePath.startsWith('.playwright-cli/'))
    .filter((relativePath) => scannedExtensions.has(path.extname(relativePath)))
}

function isAllowedTechnicalFixture(relativePath: string, line: string) {
  return (
    relativePath === 'apps/api/internal/billing/domain_test.go' &&
    line.includes(`input: "${legacyCurrencyName}:image"`)
  )
}

describe('tracked human-facing currency copy', () => {
  it('does not expose the retired currency name in tracked product source or documentation', () => {
    const repositoryRoot = findRepositoryRoot(process.cwd())
    const violations = listTrackedSourceFiles(repositoryRoot).flatMap((relativePath) => {
      return readFileSync(path.join(repositoryRoot, relativePath), 'utf8')
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
