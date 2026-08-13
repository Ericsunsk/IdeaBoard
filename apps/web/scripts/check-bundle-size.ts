import { readdir, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

type AssetKind = 'js' | 'css'

type AssetSummary = {
  kind: AssetKind
  files: number
  bytes: number
  largest: { file: string; bytes: number } | null
}

// Keep enough headroom for normal feature work while catching accidental chunk growth.
const BUDGETS: Record<AssetKind, { totalBytes: number; largestBytes: number }> = {
  js: { totalBytes: 3_500_000, largestBytes: 650_000 },
  css: { totalBytes: 350_000, largestBytes: 180_000 },
}

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url))
const assetsDirectory = path.resolve(scriptDirectory, '../dist/assets')

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

async function main() {
  let files: string[]
  try {
    files = await listAssetFiles(assetsDirectory)
  } catch {
    throw new Error(
      `Build output not found at ${path.relative(process.cwd(), assetsDirectory)}. Run npm run build first.`,
    )
  }

  const summaries = await Promise.all([summarize('js', files), summarize('css', files)])
  const failures: string[] = []

  for (const summary of summaries) {
    const budget = BUDGETS[summary.kind]
    const largest = summary.largest
    console.log(
      `${summary.kind.toUpperCase()} assets: ${summary.files} files, total ${formatBytes(summary.bytes)}, largest ${largest ? `${formatBytes(largest.bytes)} (${path.relative(assetsDirectory, largest.file)})` : 'n/a'}`,
    )
    if (summary.bytes > budget.totalBytes) {
      failures.push(
        `${summary.kind.toUpperCase()} total ${formatBytes(summary.bytes)} exceeds ${formatBytes(budget.totalBytes)}`,
      )
    }
    if (largest && largest.bytes > budget.largestBytes) {
      failures.push(
        `${summary.kind.toUpperCase()} largest ${formatBytes(largest.bytes)} exceeds ${formatBytes(budget.largestBytes)} (${path.relative(assetsDirectory, largest.file)})`,
      )
    }
  }

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
