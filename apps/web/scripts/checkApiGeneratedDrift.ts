import { Buffer } from 'node:buffer'
import { spawnSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { copyFile, mkdir, mkdtemp, readdir, readFile, rm, stat, symlink } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

type FileDiff = {
  file: string
  kind: 'missing' | 'extra' | 'changed'
}

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const repoRoot = path.resolve(webRoot, '../..')
const generatedDir = path.join(webRoot, 'src/shared/api/generated-query')
const maxDiffsToPrint = 30

async function listFiles(dir: string, baseDir = dir): Promise<string[]> {
  if (!existsSync(dir)) return []

  const entries = await readdir(dir)
  const files = await Promise.all(
    entries.map(async (entry) => {
      const fullPath = path.join(dir, entry)
      const entryStat = await stat(fullPath)
      if (entryStat.isDirectory()) return listFiles(fullPath, baseDir)
      if (entryStat.isFile()) return [path.relative(baseDir, fullPath)]
      return []
    }),
  )

  return files.flat().sort()
}

function resolveOrvalCommand(): { command: string; args: string[] } {
  const binName = process.platform === 'win32' ? 'orval.cmd' : 'orval'
  const localOrval = path.join(webRoot, 'node_modules', '.bin', binName)
  if (existsSync(localOrval)) {
    return { command: localOrval, args: ['--config', 'orval.config.ts'] }
  }

  const npxCommand = process.platform === 'win32' ? 'npx.cmd' : 'npx'
  return { command: npxCommand, args: ['--no-install', 'orval', '--config', 'orval.config.ts'] }
}

async function prepareGenerationWorkspace(tempRoot: string): Promise<string> {
  const tempWebRoot = path.join(tempRoot, 'apps/web')
  const tempApiRoot = path.join(tempRoot, 'apps/api')
  const tempApiSourceRoot = path.join(tempWebRoot, 'src/shared/api')

  await Promise.all([
    mkdir(tempApiRoot, { recursive: true }),
    mkdir(tempApiSourceRoot, { recursive: true }),
  ])

  // Post-#809: the legacy single-file `openapi.yaml` was deleted in the hard
  // cut. The drift baseline is the Public/Control split specs; both are
  // required — a missing spec fails fast instead of passing vacuously.
  const requiredSpecs = ['openapi.public.yaml', 'openapi.control.yaml']
  for (const spec of requiredSpecs) {
    if (!existsSync(path.join(repoRoot, 'apps/api', spec))) {
      throw new Error(
        `Missing required OpenAPI spec: apps/api/${spec}. ` +
          'The drift baseline is openapi.public.yaml + openapi.control.yaml.',
      )
    }
  }

  await Promise.all([
    ...requiredSpecs.map((spec) =>
      copyFile(path.join(repoRoot, 'apps/api', spec), path.join(tempApiRoot, spec)),
    ),
    copyFile(path.join(webRoot, 'package.json'), path.join(tempWebRoot, 'package.json')),
    copyFile(path.join(webRoot, 'orval.config.ts'), path.join(tempWebRoot, 'orval.config.ts')),
    copyFile(
      path.join(webRoot, 'prettier.config.js'),
      path.join(tempWebRoot, 'prettier.config.js'),
    ),
    copyFile(
      path.join(webRoot, 'src/shared/api/orvalMutator.ts'),
      path.join(tempApiSourceRoot, 'orvalMutator.ts'),
    ),
    // Issue #808: the Control client resolves through its own transport
    // mutator (control/v1 prefix); the drift workspace needs it too.
    copyFile(
      path.join(webRoot, 'src/shared/api/controlMutator.ts'),
      path.join(tempApiSourceRoot, 'controlMutator.ts'),
    ),
    // The Public client resolves through its own transport mutator
    // (v1 prefix); the drift workspace needs it too.
    copyFile(
      path.join(webRoot, 'src/shared/api/publicMutator.ts'),
      path.join(tempApiSourceRoot, 'publicMutator.ts'),
    ),
    symlink(path.join(webRoot, 'node_modules'), path.join(tempWebRoot, 'node_modules'), 'dir'),
  ])

  return tempWebRoot
}

function generateIn(workspaceRoot: string) {
  const orval = resolveOrvalCommand()
  const env = { ...process.env }
  delete env.ORVAL_GENERATED_OUTPUT_DIR

  const result = spawnSync(orval.command, orval.args, {
    cwd: workspaceRoot,
    encoding: 'utf8',
    env,
  })

  if (result.error) throw result.error
  if (result.status !== 0) {
    throw new Error(
      [
        `Failed to generate API client with: ${orval.command} ${orval.args.join(' ')}`,
        result.stdout.trim(),
        result.stderr.trim(),
      ]
        .filter(Boolean)
        .join('\n'),
    )
  }
}

async function hasSameContent(leftPath: string, rightPath: string): Promise<boolean> {
  const [left, right] = await Promise.all([readFile(leftPath), readFile(rightPath)])
  return Buffer.compare(left, right) === 0
}

async function compareGenerated(actualDir: string, expectedDir: string): Promise<FileDiff[]> {
  const [actualFiles, expectedFiles] = await Promise.all([
    listFiles(actualDir),
    listFiles(expectedDir),
  ])
  const actualSet = new Set(actualFiles)
  const expectedSet = new Set(expectedFiles)
  const diffs: FileDiff[] = []

  for (const file of expectedFiles) {
    if (!actualSet.has(file)) {
      diffs.push({ file, kind: 'missing' })
      continue
    }

    const actualPath = path.join(actualDir, file)
    const expectedPath = path.join(expectedDir, file)
    if (!(await hasSameContent(actualPath, expectedPath))) {
      diffs.push({ file, kind: 'changed' })
    }
  }

  for (const file of actualFiles) {
    if (!expectedSet.has(file)) {
      diffs.push({ file, kind: 'extra' })
    }
  }

  return diffs.sort((a, b) => a.file.localeCompare(b.file) || a.kind.localeCompare(b.kind))
}

function printDrift(diffs: FileDiff[]) {
  console.error('Generated API client is out of date.')
  console.error('Run `npm run api:generate` in apps/web and commit the generated changes.')
  console.error('')
  console.error('Diff summary:')
  for (const diff of diffs.slice(0, maxDiffsToPrint)) {
    console.error(`- ${diff.kind}: ${diff.file}`)
  }
  if (diffs.length > maxDiffsToPrint) {
    console.error(`- ...and ${diffs.length - maxDiffsToPrint} more`)
  }
}

async function main() {
  const tempRoot = await mkdtemp(path.join(tmpdir(), 'ideaboard-api-generated-'))

  try {
    const tempWebRoot = await prepareGenerationWorkspace(tempRoot)
    const expectedDir = path.join(tempWebRoot, 'src/shared/api/generated-query')
    generateIn(tempWebRoot)

    const diffs = await compareGenerated(generatedDir, expectedDir)
    if (diffs.length > 0) {
      printDrift(diffs)
      process.exitCode = 1
      return
    }

    console.log('Generated API client is up to date.')
  } finally {
    await rm(tempRoot, { recursive: true, force: true })
  }
}

void main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
})
