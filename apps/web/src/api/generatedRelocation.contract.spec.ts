import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const webRoot = process.cwd()
const repoRoot = path.resolve(webRoot, '../..')
// Pin the pre-P1 snapshot so committing the relocation cannot erase its
// byte-preservation evidence. Generated schema changes need a reviewed update.
const frozenCheckpoint = 'e43945b9e2e90b1e88c76ee6193309c519e13ddf'
const frozenRoot = 'apps/web/src/shared/api/generated-query/'
const targetRoot = path.join(webRoot, 'src/api/generated-query')

function currentFiles(directory: string): string[] {
  return fs
    .readdirSync(directory, { withFileTypes: true })
    .flatMap((entry) => {
      const absolute = path.join(directory, entry.name)
      return entry.isDirectory() ? currentFiles(absolute) : [path.relative(targetRoot, absolute)]
    })
    .sort()
}

describe('opaque generated-query relocation (api/generated-query-carve-out)', () => {
  it('preserves all 417 frozen generated files byte for byte across the parent-path move', () => {
    const tree = spawnSync(
      'git',
      ['ls-tree', '-r', '--name-only', frozenCheckpoint, '--', frozenRoot],
      {
        cwd: repoRoot,
        encoding: 'utf8',
      },
    )
    if (tree.error) throw tree.error
    expect(tree.status).toBe(0)
    const frozenFiles = tree.stdout.trim().split('\n').filter(Boolean).sort()
    expect(frozenFiles).toHaveLength(417)
    expect(currentFiles(targetRoot)).toEqual(
      frozenFiles.map((file) => file.slice(frozenRoot.length)),
    )
    const changed: string[] = []
    for (const frozenFile of frozenFiles) {
      const baseline = spawnSync('git', ['show', `${frozenCheckpoint}:${frozenFile}`], {
        cwd: repoRoot,
      })
      if (baseline.error) throw baseline.error
      if (baseline.status !== 0)
        throw new Error(`Unable to read frozen generated file ${frozenFile}`)
      const current = fs.readFileSync(path.join(targetRoot, frozenFile.slice(frozenRoot.length)))
      if (
        createHash('sha256').update(current).digest('hex') !==
        createHash('sha256').update(baseline.stdout).digest('hex')
      )
        changed.push(frozenFile)
    }
    expect(changed).toEqual([])
  }, 30_000)

  it('reproduces the current generated output with the actual Orval drift gate', () => {
    const result = spawnSync(
      process.execPath,
      ['--experimental-strip-types', path.join(webRoot, 'scripts/checkApiGeneratedDrift.ts')],
      { cwd: webRoot, encoding: 'utf8', timeout: 120_000 },
    )
    if (result.error) throw result.error
    expect(result.status, result.stderr || result.stdout).toBe(0)
    expect(result.stdout).toContain('Generated API client is up to date.')
  }, 130_000)
})
