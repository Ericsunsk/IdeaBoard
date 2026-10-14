import { mkdir, readFile, readdir, stat, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { gzipSync } from 'node:zlib'

type AssetKind = 'js' | 'css' | 'font'

type ManifestChunk = {
  file: string
  imports?: string[]
  isEntry?: boolean
}

type ViteManifest = Record<string, ManifestChunk>

type AssetSummary = {
  kind: AssetKind
  files: number
  bytes: number
  gzipBytes: number
  largest: { file: string; bytes: number; gzipBytes: number } | null
}

// Lazy route, locale, and optional converter chunks should not consume the initial-load budget.
const INITIAL_JS_BUDGET_BYTES = 900_000
const LARGEST_JS_CHUNK_BUDGET_BYTES = 650_000
const OPTIONAL_LAZY_JS_CHUNK_BUDGETS = new Map([
  ['node_modules/heic2any/dist/heic2any.js', 1_400_000],
])
const LOCALE_CHUNK_BUDGET_BYTES = 64_000
// The layered token engine produces a ~213 KB largest CSS asset. Keep a
// small margin for semantic declarations and their physical ingredients.
const CSS_BUDGET = { totalBytes: 350_000, largestBytes: 225_000 }
// Inter Variable's Latin woff2 is ~48 KB; allow a small margin for font updates.
const FONT_BUDGET = { totalBytes: 70_000 }
const TOTAL_GZIP_BUDGET_BYTES = 2_000_000

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url))
const distDirectory = path.resolve(scriptDirectory, '../dist')
const assetsDirectory = path.resolve(scriptDirectory, '../dist/assets')
const manifestPath = path.resolve(distDirectory, '.vite/manifest.json')
// Write the CI budget report on both success and failure.
const reportPath =
  process.env.BUNDLE_REPORT_PATH?.trim() ||
  path.resolve(scriptDirectory, '../node_modules/.cache/bundle-report.md')

async function listAssetFiles(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true })
  const files: string[] = []

  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name)
    if (entry.isDirectory()) {
      files.push(...(await listAssetFiles(entryPath)))
      continue
    }
    if (entry.isFile() && /\.(js|css|woff2?|ttf|otf)$/.test(entry.name)) files.push(entryPath)
  }

  return files
}

async function summarize(kind: AssetKind, files: string[]): Promise<AssetSummary> {
  const suffix = kind === 'font' ? /\.(woff2?|ttf|otf)$/ : new RegExp(`\\.${kind}$`)
  const matchingFiles = files.filter((file) => suffix.test(file))
  const sizes = await Promise.all(
    matchingFiles.map(async (file) => {
      const buffer = await readFile(file)
      return {
        file,
        bytes: buffer.byteLength,
        gzipBytes: gzipSync(buffer).byteLength,
      }
    }),
  )
  const largest = sizes.reduce<AssetSummary['largest']>(
    (current, item) =>
      !current || item.bytes > current.bytes
        ? { file: item.file, bytes: item.bytes, gzipBytes: item.gzipBytes }
        : current,
    null,
  )

  return {
    kind,
    files: sizes.length,
    bytes: sizes.reduce((total, item) => total + item.bytes, 0),
    gzipBytes: sizes.reduce((total, item) => total + item.gzipBytes, 0),
    largest,
  }
}

function formatBytes(bytes: number): string {
  return `${(bytes / 1024).toFixed(1)} KB`
}

async function readManifest(): Promise<ViteManifest> {
  try {
    return JSON.parse(await readFile(manifestPath, 'utf8')) as ViteManifest
  } catch {
    throw new Error(
      `Vite manifest not found at ${path.relative(process.cwd(), manifestPath)}. Run npm run build first.`,
    )
  }
}

function collectInitialChunks(manifest: ViteManifest): ManifestChunk[] {
  const initialChunkKeys = new Set<string>()

  function visit(chunkKey: string): void {
    if (initialChunkKeys.has(chunkKey)) return
    const chunk = manifest[chunkKey]
    if (!chunk) {
      throw new Error(`Vite manifest references missing chunk: ${chunkKey}`)
    }
    initialChunkKeys.add(chunkKey)
    for (const importKey of chunk.imports ?? []) visit(importKey)
  }

  for (const [chunkKey, chunk] of Object.entries(manifest)) {
    if (chunk.isEntry) visit(chunkKey)
  }

  if (initialChunkKeys.size === 0) {
    throw new Error('Vite manifest does not contain an entry chunk.')
  }

  return [...initialChunkKeys].map((chunkKey) => {
    const chunk = manifest[chunkKey]
    if (!chunk) {
      throw new Error(`Vite manifest references missing chunk: ${chunkKey}`)
    }
    return chunk
  })
}

async function sumChunkBytes(chunks: ManifestChunk[]): Promise<number> {
  const files = new Set(chunks.map((chunk) => chunk.file).filter((file) => file.endsWith('.js')))
  const sizes = await Promise.all(
    [...files].map(async (file) => (await stat(path.resolve(distDirectory, file))).size),
  )
  return sizes.reduce((total, bytes) => total + bytes, 0)
}

type JsChunkCheck = {
  largest: { file: string; bytes: number } | null
  optionalLazy: { file: string; bytes: number; budgetBytes: number } | null
  failures: string[]
}

type LocaleChunkCheck = {
  largest: { file: string; bytes: number } | null
  failures: string[]
}

async function checkLocaleChunks(manifest: ViteManifest): Promise<LocaleChunkCheck> {
  const failures: string[] = []
  let largest: LocaleChunkCheck['largest'] = null

  for (const [source, chunk] of Object.entries(manifest)) {
    if (!/^src\/shared\/i18n\/locales\/(?!en\/)[^/]+\/index\.ts$/.test(source)) continue
    const bytes = (await stat(path.resolve(distDirectory, chunk.file))).size
    if (!largest || bytes > largest.bytes) largest = { file: chunk.file, bytes }
    if (bytes > LOCALE_CHUNK_BUDGET_BYTES) {
      failures.push(
        `Locale chunk ${source} is ${formatBytes(bytes)}, exceeding ${formatBytes(LOCALE_CHUNK_BUDGET_BYTES)} (${chunk.file})`,
      )
    }
  }

  return { largest, failures }
}

async function checkJavascriptChunks(
  files: string[],
  manifest: ViteManifest,
  initialChunks: ManifestChunk[],
): Promise<JsChunkCheck> {
  const failures: string[] = []
  const manifestSourceByFile = new Map(
    Object.entries(manifest).map(([source, chunk]) => [chunk.file, source]),
  )
  const initialFiles = new Set(initialChunks.map((chunk) => chunk.file))
  let largest: JsChunkCheck['largest'] = null
  let optionalLazy: JsChunkCheck['optionalLazy'] = null

  for (const file of files.filter((candidate) => candidate.endsWith('.js'))) {
    const relativeFile = path.relative(distDirectory, file)
    const source = manifestSourceByFile.get(relativeFile)
    const optionalLazyBudget = source ? OPTIONAL_LAZY_JS_CHUNK_BUDGETS.get(source) : undefined
    if (optionalLazyBudget !== undefined && initialFiles.has(relativeFile)) {
      failures.push(`Optional converter must remain lazy-loaded (${relativeFile})`)
      continue
    }

    const bytes = (await stat(file)).size
    if (optionalLazyBudget !== undefined) {
      if (!optionalLazy || bytes > optionalLazy.bytes) {
        optionalLazy = { file: relativeFile, bytes, budgetBytes: optionalLazyBudget }
      }
    } else if (!largest || bytes > largest.bytes) {
      largest = { file: relativeFile, bytes }
    }

    const budget = optionalLazyBudget ?? LARGEST_JS_CHUNK_BUDGET_BYTES
    if (bytes > budget) {
      failures.push(
        `JS chunk ${formatBytes(bytes)} exceeds ${formatBytes(budget)} (${path.relative(assetsDirectory, file)})`,
      )
    }
  }

  return { largest, optionalLazy, failures }
}

async function main() {
  const [files, manifest] = await Promise.all([listAssetFiles(assetsDirectory), readManifest()])

  const summaries = await Promise.all([
    summarize('js', files),
    summarize('css', files),
    summarize('font', files),
  ])
  const jsSummary = summaries.find((summary) => summary.kind === 'js')
  const cssSummary = summaries.find((summary) => summary.kind === 'css')
  const fontSummary = summaries.find((summary) => summary.kind === 'font')
  if (!jsSummary || !cssSummary || !fontSummary)
    throw new Error('Unable to summarize build assets.')

  const initialChunks = collectInitialChunks(manifest)
  const initialJsBytes = await sumChunkBytes(initialChunks)
  const failures: string[] = []

  for (const summary of summaries) {
    const largest = summary.largest
    console.log(
      `${summary.kind.toUpperCase()} assets: ${summary.files} files, total ${formatBytes(summary.bytes)} (gzip ${formatBytes(summary.gzipBytes)}), largest ${largest ? `${formatBytes(largest.bytes)} (${path.relative(assetsDirectory, largest.file)})` : 'n/a'}`,
    )
  }

  const totalGzipBytes = summaries.reduce((total, summary) => total + summary.gzipBytes, 0)
  console.log(
    `Total output: ${formatBytes(summaries.reduce((t, s) => t + s.bytes, 0))} (gzip ${formatBytes(totalGzipBytes)})`,
  )
  console.log(`Initial JS: ${formatBytes(initialJsBytes)}`)
  if (initialJsBytes > INITIAL_JS_BUDGET_BYTES) {
    failures.push(
      `Initial JS ${formatBytes(initialJsBytes)} exceeds ${formatBytes(INITIAL_JS_BUDGET_BYTES)}`,
    )
  }
  if (totalGzipBytes > TOTAL_GZIP_BUDGET_BYTES) {
    failures.push(
      `Total gzip output ${formatBytes(totalGzipBytes)} exceeds ${formatBytes(TOTAL_GZIP_BUDGET_BYTES)}`,
    )
  }
  const jsChunkCheck = await checkJavascriptChunks(files, manifest, initialChunks)
  failures.push(...jsChunkCheck.failures)
  if (cssSummary.bytes > CSS_BUDGET.totalBytes) {
    failures.push(
      `CSS total ${formatBytes(cssSummary.bytes)} exceeds ${formatBytes(CSS_BUDGET.totalBytes)}`,
    )
  }
  if (cssSummary.largest && cssSummary.largest.bytes > CSS_BUDGET.largestBytes) {
    failures.push(
      `CSS largest ${formatBytes(cssSummary.largest.bytes)} exceeds ${formatBytes(CSS_BUDGET.largestBytes)} (${path.relative(assetsDirectory, cssSummary.largest.file)})`,
    )
  }
  if (fontSummary.bytes > FONT_BUDGET.totalBytes) {
    failures.push(
      `Font total ${formatBytes(fontSummary.bytes)} exceeds ${formatBytes(FONT_BUDGET.totalBytes)}`,
    )
  }
  const localeChunkCheck = await checkLocaleChunks(manifest)
  failures.push(...localeChunkCheck.failures)

  await writeReport({
    jsChunkCheck,
    cssSummary,
    fontSummary,
    localeChunkCheck,
    totalGzipBytes,
    initialJsBytes,
    failures,
  })

  if (failures.length > 0) {
    throw new Error(
      `Bundle size budget exceeded:\n${failures.map((failure) => `- ${failure}`).join('\n')}`,
    )
  }
}

type ReportInput = {
  jsChunkCheck: JsChunkCheck
  cssSummary: AssetSummary
  fontSummary: AssetSummary
  localeChunkCheck: LocaleChunkCheck
  totalGzipBytes: number
  initialJsBytes: number
  failures: string[]
}

function reportRow(label: string, actual: string, budget: string, ok: boolean): string {
  return `| ${label} | ${actual} | ${budget} | ${ok ? 'pass' : 'BREACH'} |`
}

function actualBytes(largest: { file: string; bytes: number } | null): string {
  return largest ? `${formatBytes(largest.bytes)} (${largest.file})` : 'n/a'
}

async function writeReport(input: ReportInput): Promise<void> {
  const {
    jsChunkCheck,
    cssSummary,
    fontSummary,
    localeChunkCheck,
    totalGzipBytes,
    initialJsBytes,
    failures,
  } = input
  const lines = [
    '## Bundle budget',
    '',
    `Status: ${failures.length === 0 ? 'pass' : 'BREACH'} (${failures.length} breach(es))`,
    '',
    '| Check | Actual | Budget | Result |',
    '| --- | --- | --- | --- |',
    reportRow(
      'Initial JS',
      formatBytes(initialJsBytes),
      formatBytes(INITIAL_JS_BUDGET_BYTES),
      initialJsBytes <= INITIAL_JS_BUDGET_BYTES,
    ),
    reportRow(
      'Total gzip',
      formatBytes(totalGzipBytes),
      formatBytes(TOTAL_GZIP_BUDGET_BYTES),
      totalGzipBytes <= TOTAL_GZIP_BUDGET_BYTES,
    ),
    reportRow(
      'CSS total',
      formatBytes(cssSummary.bytes),
      formatBytes(CSS_BUDGET.totalBytes),
      cssSummary.bytes <= CSS_BUDGET.totalBytes,
    ),
    reportRow(
      'CSS largest',
      cssSummary.largest ? formatBytes(cssSummary.largest.bytes) : 'n/a',
      formatBytes(CSS_BUDGET.largestBytes),
      !cssSummary.largest || cssSummary.largest.bytes <= CSS_BUDGET.largestBytes,
    ),
    reportRow(
      'Font total',
      formatBytes(fontSummary.bytes),
      formatBytes(FONT_BUDGET.totalBytes),
      fontSummary.bytes <= FONT_BUDGET.totalBytes,
    ),
    reportRow(
      'JS chunks (largest)',
      actualBytes(jsChunkCheck.largest),
      `largest ${formatBytes(LARGEST_JS_CHUNK_BUDGET_BYTES)}`,
      !jsChunkCheck.largest || jsChunkCheck.largest.bytes <= LARGEST_JS_CHUNK_BUDGET_BYTES,
    ),
    // Optional lazy converters are exempt from the per-chunk budget above and
    // carry their own cap (vite.config.ts chunkSizeWarningLimit parity).
    reportRow(
      'Optional lazy chunks (largest)',
      jsChunkCheck.optionalLazy
        ? `${formatBytes(jsChunkCheck.optionalLazy.bytes)} (${jsChunkCheck.optionalLazy.file})`
        : 'n/a',
      jsChunkCheck.optionalLazy
        ? `largest ${formatBytes(jsChunkCheck.optionalLazy.budgetBytes)}`
        : 'n/a',
      !jsChunkCheck.optionalLazy ||
        jsChunkCheck.optionalLazy.bytes <= jsChunkCheck.optionalLazy.budgetBytes,
    ),
    reportRow(
      'Locale chunks (largest)',
      actualBytes(localeChunkCheck.largest),
      `largest ${formatBytes(LOCALE_CHUNK_BUDGET_BYTES)}`,
      !localeChunkCheck.largest || localeChunkCheck.largest.bytes <= LOCALE_CHUNK_BUDGET_BYTES,
    ),
    '',
  ]
  if (failures.length > 0) {
    lines.push('### Breaches', '', ...failures.map((failure) => `- ${failure}`), '')
  }
  try {
    await mkdir(path.dirname(reportPath), { recursive: true })
    await writeFile(reportPath, `${lines.join('\n')}\n`)
    console.log(`Bundle report: ${path.relative(process.cwd(), reportPath)}`)
  } catch (error) {
    // Never mask the gate: a report write failure is diagnostic only.
    console.error(
      `Unable to write bundle report to ${reportPath}: ${error instanceof Error ? error.message : error}`,
    )
  }
}

void main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
})
