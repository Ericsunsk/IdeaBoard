import { afterEach, describe, expect, it } from 'vitest'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { PNG } from 'pngjs'
import {
  compareVisualCaptures,
  visualCases,
  type VisualManifest,
} from '../../scripts/visualContract'

const temporaryDirectories: string[] = []

function directory() {
  const result = mkdtempSync(join(tmpdir(), 'ideaboard-visual-'))
  temporaryDirectories.push(result)
  return result
}

afterEach(() => {
  for (const path of temporaryDirectories.splice(0)) rmSync(path, { recursive: true, force: true })
})

function metrics() {
  return {
    x: 0,
    y: 0,
    width: 32,
    height: 32,
    'font-family': 'sans-serif',
    'font-size': '14px',
    'font-weight': '500',
    'line-height': '20px',
    color: 'rgb(0, 0, 0)',
    'background-color': 'rgb(255, 255, 255)',
    'border-color': 'rgb(0, 0, 0)',
    'border-radius': '8px',
    'box-shadow': 'none',
    'z-index': 'auto',
    '::before/width': 'auto',
    '::after/width': 'auto',
  }
}

function manifest(): VisualManifest {
  return {
    schemaVersion: 1,
    sourceRevision: 'a'.repeat(40),
    environment: { browser: 'chromium-test', platform: 'test', locale: 'en', deviceScaleFactor: 1 },
    captures: visualCases.map((item) => ({
      ...item,
      status: 'passed',
      errors: [],
      metrics: { surface: metrics(), control: metrics() },
    })),
  }
}

function writeManifest(path: string, value: unknown) {
  writeFileSync(join(path, 'manifest.json'), JSON.stringify(value))
}

function fullCapture(path: string) {
  writeManifest(path, manifest())
  const images = new Map<string, Buffer>()
  for (const item of visualCases) {
    const size = `${item.width}x${item.height}`
    if (!images.has(size))
      images.set(size, PNG.sync.write(new PNG({ width: item.width, height: item.height })))
    writeFileSync(join(path, item.file), images.get(size)!)
  }
}

describe('visual acceptance rejects invalid evidence', { timeout: 30000 }, () => {
  it('rejects one identical PNG without a manifest', () => {
    const before = directory(),
      after = directory()
    const image = PNG.sync.write(new PNG({ width: 1, height: 1 }))
    for (const path of [before, after]) writeFileSync(join(path, 'single.png'), image)
    expect(compareVisualCaptures(before, after).length).toBeGreaterThan(0)
  })

  it.each(['failed', 'missing', 'duplicate', 'redirect', 'measurements', 'partial-measurements'])(
    'rejects %s capture metadata before comparing pixels',
    (invalid) => {
      const before = directory(),
        after = directory()
      const value = manifest()
      if (invalid === 'failed') value.captures[0]!.errors.push('401 console error')
      if (invalid === 'missing') value.captures.pop()
      if (invalid === 'duplicate') value.captures[1] = value.captures[0]!
      if (invalid === 'redirect') Object.assign(value.captures[0]!, { surface: 'login-redirect' })
      if (invalid === 'measurements') value.captures[0]!.metrics = {}
      if (invalid === 'partial-measurements')
        value.captures[0]!.metrics = { surface: { width: 32 }, control: { width: 32 } }
      for (const path of [before, after]) writeManifest(path, value)
      const expectedIssue: Record<string, string> = {
        failed: 'failed capture',
        missing: 'expected 36 successful',
        duplicate: 'missing/duplicate',
        redirect: 'incorrect',
        measurements: 'missing rendered-control',
        'partial-measurements': 'incomplete rendered',
      }
      expect(compareVisualCaptures(before, after)[0]).toContain(expectedIssue[invalid])
    },
  )

  it('accepts the full equivalent matrix and rejects a real channel or geometry regression', () => {
    const before = directory(),
      after = directory()
    fullCapture(before)
    fullCapture(after)
    expect(compareVisualCaptures(before, after)).toEqual([])
    const item = visualCases[0]!
    const png = new PNG({ width: item.width, height: item.height })
    png.data[0] = 2
    writeFileSync(join(after, item.file), PNG.sync.write(png))
    expect(compareVisualCaptures(before, after)[0]).toContain('pixel channel delta 2')
    const changed = manifest()
    changed.captures[0]!.metrics.control!.width = 31
    writeManifest(after, changed)
    expect(compareVisualCaptures(before, after)[0]).toContain('rendered paint')
  })

  it('rejects incorrect image dimensions even when the manifest is complete', () => {
    const before = directory(),
      after = directory()
    fullCapture(before)
    fullCapture(after)
    writeFileSync(
      join(after, visualCases[0]!.file),
      PNG.sync.write(new PNG({ width: 1, height: 1 })),
    )
    expect(compareVisualCaptures(before, after)[0]).toContain('expected 1440x900 PNG')
  })
})
