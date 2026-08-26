import { readFile, readdir, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

type AssetKind = 'js' | 'css'

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
  largest: { file: string; bytes: number } | null
}

// Lazy route and locale chunks should not consume the initial-load budget.
const INITIAL_JS_BUDGET_BYTES = 900_000
const LARGEST_JS_CHUNK_BUDGET_BYTES = 650_000
const LOCALE_CHUNK_BUDGET_BYTES = 64_000
const CSS_BUDGET = { totalBytes: 350_000, largestBytes: 180_000 }

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url))
const distDirectory = path.resolve(scriptDirectory, '../dist')
const assetsDirectory = path.resolve(scriptDirectory, '../dist/assets')
const manifestPath = path.resolve(distDirectory, '.vite/manifest.json')

async function listAssetFiles(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true })
  const files: string[] = []

  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name)
    if (entry.isDirectory()) {
      files.push(...(await listAssetFiles(entryPath)))
      continue
    }
    if (entry.isFile() && /\.(js|css)$/.test(entry.name)) files.push(entryPath)
  }

  return files
}

async function summarize(kind: AssetKind, files: string[]): Promise<AssetSummary> {
  const matchingFiles = files.filter((file) => file.endsWith(`.${kind}`))
  const sizes = await Promise.all(
    matchingFiles.map(async (file) => ({ file, bytes: (await stat(file)).size })),
  )
  const largest = sizes.reduce<AssetSummary['largest']>(
    (current, item) =>
      !current || item.bytes > current.bytes ? { file: item.file, bytes: item.bytes } : current,
    null,
  )

  return {
    kind,
    files: sizes.length,
    bytes: sizes.reduce((total, item) => total + item.bytes, 0),
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

  return [...initialChunkKeys].map((chunkKey) => manifest[chunkKey])
}

async function sumChunkBytes(chunks: ManifestChunk[]): Promise<number> {
  const files = new Set(chunks.map((chunk) => chunk.file).filter((file) => file.endsWith('.js')))
  const sizes = await Promise.all(
    [...files].map(async (file) => (await stat(path.resolve(distDirectory, file))).size),
  )
  return sizes.reduce((total, bytes) => total + bytes, 0)
}

async function findOversizedLocaleChunks(manifest: ViteManifest): Promise<string[]> {
  const failures: string[] = []

  for (const [source, chunk] of Object.entries(manifest)) {
    if (!/^src\/shared\/i18n\/locales\/(?!en\/)[^/]+\/index\.ts$/.test(source)) continue
    const bytes = (await stat(path.resolve(distDirectory, chunk.file))).size
    if (bytes > LOCALE_CHUNK_BUDGET_BYTES) {
      failures.push(
        `Locale chunk ${source} is ${formatBytes(bytes)}, exceeding ${formatBytes(LOCALE_CHUNK_BUDGET_BYTES)} (${chunk.file})`,
      )
    }
  }

  return failures
}

async function main() {
  const [files, manifest] = await Promise.all([listAssetFiles(assetsDirectory), readManifest()])

  const summaries = await Promise.all([summarize('js', files), summarize('css', files)])
  const jsSummary = summaries.find((summary) => summary.kind === 'js')
  const cssSummary = summaries.find((summary) => summary.kind === 'css')
  if (!jsSummary || !cssSummary) throw new Error('Unable to summarize build assets.')

  const initialJsBytes = await sumChunkBytes(collectInitialChunks(manifest))
  const failures: string[] = []

  for (const summary of summaries) {
    const largest = summary.largest
    console.log(
      `${summary.kind.toUpperCase()} assets: ${summary.files} files, total ${formatBytes(summary.bytes)}, largest ${largest ? `${formatBytes(largest.bytes)} (${path.relative(assetsDirectory, largest.file)})` : 'n/a'}`,
    )
  }

  console.log(`Initial JS: ${formatBytes(initialJsBytes)}`)
  if (initialJsBytes > INITIAL_JS_BUDGET_BYTES) {
    failures.push(
      `Initial JS ${formatBytes(initialJsBytes)} exceeds ${formatBytes(INITIAL_JS_BUDGET_BYTES)}`,
    )
  }
  if (jsSummary.largest && jsSummary.largest.bytes > LARGEST_JS_CHUNK_BUDGET_BYTES) {
    failures.push(
      `JS largest ${formatBytes(jsSummary.largest.bytes)} exceeds ${formatBytes(LARGEST_JS_CHUNK_BUDGET_BYTES)} (${path.relative(assetsDirectory, jsSummary.largest.file)})`,
    )
  }
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
  failures.push(...(await findOversizedLocaleChunks(manifest)))

  if (failures.length > 0) {
    throw new Error(
      `Bundle size budget exceeded:\n${failures.map((failure) => `- ${failure}`).join('\n')}`,
    )
  }
}

void main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
})
