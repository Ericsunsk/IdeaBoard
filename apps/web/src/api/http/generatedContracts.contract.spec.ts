import { spawnSync } from 'node:child_process'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const webRoot = process.cwd()

describe('generated API contracts', () => {
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
