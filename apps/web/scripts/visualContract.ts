import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { PNG } from 'pngjs'

const visualSurfaces = [
  'auth',
  'boards',
  'canvas',
  'search',
  'library',
  'account',
  'admin-table',
  'admin-dialog',
  'admin-popover',
] as const

const visualViewports = [
  { name: 'desktop', width: 1440, height: 900 },
  { name: 'mobile', width: 390, height: 844 },
] as const

export const visualCases = visualSurfaces.flatMap((surface) =>
  visualViewports.flatMap((viewport) =>
    (['light', 'dark'] as const).map((mode) => ({
      id: `${surface}-${mode}-${viewport.name}`,
      surface,
      mode,
      viewport: viewport.name,
      width: viewport.width,
      height: viewport.height,
      file: `${surface}-${mode}-${viewport.name}.png`,
    })),
  ),
)

type VisualCase = (typeof visualCases)[number]
type VisualMetrics = Record<string, Record<string, string | number>>
type VisualCapture = VisualCase & {
  status: 'passed'
  errors: string[]
  metrics: VisualMetrics
}
export type VisualManifest = {
  schemaVersion: 1
  sourceRevision: string
  environment: { browser: string; platform: string; locale: 'en'; deviceScaleFactor: 1 }
  captures: VisualCapture[]
}

function readManifest(directory: string): VisualManifest {
  const manifest = JSON.parse(readFileSync(join(directory, 'manifest.json'), 'utf8'))
  if (manifest.schemaVersion !== 1 || !/^[a-f0-9]{40}$/.test(manifest.sourceRevision ?? '')) {
    throw new Error(`${directory}: invalid visual manifest schema/source revision`)
  }
  if (!manifest.environment?.browser || !manifest.environment?.platform) {
    throw new Error(`${directory}: missing browser/platform provenance`)
  }
  if (manifest.environment.locale !== 'en' || manifest.environment.deviceScaleFactor !== 1) {
    throw new Error(`${directory}: unexpected locale/device scale`)
  }
  if (!Array.isArray(manifest.captures) || manifest.captures.length !== visualCases.length) {
    throw new Error(`${directory}: expected ${visualCases.length} successful surface captures`)
  }
  for (const expected of visualCases) validateCapture(directory, manifest.captures, expected)
  const files = readdirSync(directory)
    .filter((file) => file.endsWith('.png'))
    .sort()
  const expectedFiles = visualCases.map((item) => item.file).sort()
  if (JSON.stringify(files) !== JSON.stringify(expectedFiles)) {
    throw new Error(`${directory}: PNG inventory differs from the required surface matrix`)
  }
  return manifest as VisualManifest
}

function validateCapture(directory: string, captures: VisualCapture[], expected: VisualCase) {
  const records = captures.filter((record) => record.id === expected.id)
  const record = records[0]
  if (records.length !== 1 || !record)
    throw new Error(`${directory}: missing/duplicate ${expected.id}`)
  for (const [key, value] of Object.entries(expected)) {
    if (record[key as keyof VisualCase] !== value) {
      throw new Error(`${directory}: incorrect ${expected.id} ${key}`)
    }
  }
  if (record.status !== 'passed' || !Array.isArray(record.errors) || record.errors.length) {
    throw new Error(`${directory}: failed capture ${expected.id}`)
  }
  if (!record.metrics || Object.keys(record.metrics).length < 2 || !record.metrics.surface) {
    throw new Error(`${directory}: missing rendered-control measurements for ${expected.id}`)
  }
  for (const measurements of Object.values(record.metrics)) {
    const geometry = ['x', 'y', 'width', 'height']
    const paint = [
      'font-family',
      'font-size',
      'font-weight',
      'line-height',
      'color',
      'background-color',
      'border-color',
      'border-radius',
      'box-shadow',
      'z-index',
      '::before/width',
      '::after/width',
    ]
    if (
      !measurements ||
      geometry.some(
        (key) => typeof measurements[key] !== 'number' || !Number.isFinite(measurements[key]),
      ) ||
      Number(measurements.width) <= 0 ||
      Number(measurements.height) <= 0 ||
      paint.some((key) => typeof measurements[key] !== 'string')
    ) {
      throw new Error(`${directory}: incomplete rendered measurements for ${expected.id}`)
    }
  }
}

function comparePixels(baseline: PNG, candidate: PNG, expected: VisualCase): string | undefined {
  for (const png of [baseline, candidate]) {
    if (png.width !== expected.width || png.height !== expected.height) {
      return `${expected.id}: expected ${expected.width}x${expected.height} PNG`
    }
  }
  if (baseline.data.equals(candidate.data)) return
  for (let index = 0; index < baseline.data.length; index += 1) {
    const delta = Math.abs(baseline.data[index]! - candidate.data[index]!)
    if (delta > (index % 4 === 3 ? 0 : 1)) {
      return `${expected.id}: pixel channel delta ${delta} exceeds equivalence tolerance`
    }
  }
}

/** Fail closed on missing/failed surfaces before considering pixel equivalence. */
export function compareVisualCaptures(baselineDir: string, candidateDir: string): string[] {
  try {
    const baseline = readManifest(baselineDir)
    const candidate = readManifest(candidateDir)
    if (JSON.stringify(baseline.environment) !== JSON.stringify(candidate.environment)) {
      return ['Visual environment differs between baseline and candidate']
    }
    return visualCases.flatMap((expected) =>
      compareCase(baselineDir, candidateDir, baseline, candidate, expected),
    )
  } catch (error) {
    return [error instanceof Error ? error.message : 'Invalid visual capture input']
  }
}

function compareCase(
  baselineDir: string,
  candidateDir: string,
  baseline: VisualManifest,
  candidate: VisualManifest,
  expected: VisualCase,
): string[] {
  const before = baseline.captures.find((item) => item.id === expected.id)!
  const after = candidate.captures.find((item) => item.id === expected.id)!
  if (JSON.stringify(before.metrics) !== JSON.stringify(after.metrics)) {
    return [
      `${expected.id}: rendered paint, font metrics, geometry, stacking or hit bounds changed`,
    ]
  }
  const issue = comparePixels(
    PNG.sync.read(readFileSync(join(baselineDir, expected.file))),
    PNG.sync.read(readFileSync(join(candidateDir, expected.file))),
    expected,
  )
  return issue ? [issue] : []
}
