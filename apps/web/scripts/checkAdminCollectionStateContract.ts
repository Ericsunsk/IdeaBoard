import { readdir, readFile, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const adminRoot = path.join(webRoot, 'src/features/admin')
const canonicalCollectionState = path.join(
  adminRoot,
  'shared/composables/useCollectionState.ts',
)

async function listAdminSourceFiles(directory: string): Promise<string[]> {
  const entries = await readdir(directory)
  const files = await Promise.all(
    entries.map(async (entry) => {
      const entryPath = path.join(directory, entry)
      if ((await stat(entryPath)).isDirectory()) return listAdminSourceFiles(entryPath)
      return entryPath.endsWith('.ts') || entryPath.endsWith('.vue') ? [entryPath] : []
    }),
  )
  return files.flat()
}

async function main() {
  const duplicateLifecycles: string[] = []

  for (const filePath of await listAdminSourceFiles(adminRoot)) {
    if (filePath === canonicalCollectionState) continue

    const source = await readFile(filePath, 'utf8')
    const ownsManualCollectionLifecycle =
      source.includes('initialError') &&
      source.includes('backgroundError') &&
      source.includes('preserveData')
    const ownsManualRequestCoordination =
      /(?:requestEpoch|pageRequestEpoch|listRequestEpoch|activeRequest)/.test(source) &&
      source.includes('AbortController')
    const ownsQueryRetainedCollectionLifecycle =
      /resource\.ts$/.test(filePath) &&
      source.includes('placeholderData') &&
      source.includes('refetchInterval') &&
      (source.includes('beforeId') || source.includes('before_id') || source.includes('cursor'))

    if (
      (ownsManualCollectionLifecycle && ownsManualRequestCoordination) ||
      ownsQueryRetainedCollectionLifecycle
    ) {
      duplicateLifecycles.push(path.relative(webRoot, filePath))
    }
  }

  if (duplicateLifecycles.length === 0) {
    console.log('Admin collection-state contract passed.')
    return
  }

  console.error('Admin collection-state contract failed.')
  console.error('Use features/admin/shared/composables/useCollectionState for this lifecycle:')
  for (const filePath of duplicateLifecycles) console.error(`- ${filePath}`)
  process.exitCode = 1
}

void main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
})
