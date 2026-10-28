import { appendFileSync, existsSync, readFileSync, readdirSync } from 'node:fs'
import { dirname, isAbsolute, relative, resolve, sep } from 'node:path'
import { execFileSync, spawnSync } from 'node:child_process'
import { fileURLToPath, pathToFileURL } from 'node:url'
import process from 'node:process'
import ts from 'typescript'
import { createVitest } from 'vitest/node'

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const unitFile = /^src\/.+\.(?:unit|component|contract)\.spec\.ts$/
const full = (reason) => ({
  mode: 'full',
  reason,
  filters: [],
  matrix: { shard: [1, 2, 3, 4], shard_count: [4] },
})

function git(root, ...args) {
  return execFileSync('git', args, {
    cwd: root,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  })
}

export function changedUnitInputs(root, base, head, forceFull) {
  if (forceFull !== 'false') return { reason: 'complete verification requested' }
  if (!/^[a-f0-9]{40}$/.test(base) || !/^[a-f0-9]{40}$/.test(head))
    return { reason: 'trusted commit range unavailable' }
  try {
    if (git(root, 'rev-parse', 'HEAD').trim() !== head)
      return { reason: 'checkout does not match planned commit' }
    git(root, 'merge-base', '--is-ancestor', base, head)
    if (git(root, 'status', '--porcelain', '--untracked-files=no').trim())
      return { reason: 'tracked working tree is dirty' }
    const entries = git(root, 'diff', '--name-status', '--no-renames', '-z', base, head).split('\0')
    const changed = []
    for (let i = 0; i < entries.length - 1; i += 2) {
      const [status, path] = entries.slice(i, i + 2)
      if (path.endsWith('.md')) continue
      if (!['A', 'M'].includes(status)) return { reason: `deleted or moved input: ${path}` }
      if (!/^apps\/web\/src\/.+\.(?:ts|vue)$/.test(path))
        return { reason: `shared or non-source input: ${path}` }
      const local = path.slice('apps/web/'.length)
      if (
        /^src\/(?:app|api|core\/(?:contracts|i18n)|theme|test)\//.test(local) ||
        /\/(?:routes|router)\//.test(local)
      ) {
        return { reason: `shared application input: ${path}` }
      }
      changed.push(resolve(root, path))
    }
    return changed.length ? { changed } : { reason: 'no supported local source input' }
  } catch {
    return { reason: 'commit range cannot be verified' }
  }
}

// Filesystem readers inspect inputs outside the import graph. Ask Vitest for
// their consumers too, instead of maintaining a second dependency graph/list.
function filesystemReaders(directory) {
  if (!existsSync(directory)) return []
  const readers = []
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = resolve(directory, entry.name)
    if (entry.isDirectory()) readers.push(...filesystemReaders(path))
    else if (/\.(?:[cm]?js|ts)$/.test(entry.name)) {
      const source = ts.createSourceFile(
        path,
        readFileSync(path, 'utf8'),
        ts.ScriptTarget.Latest,
        true,
      )
      function visit(node) {
        const specifier =
          ts.isImportDeclaration(node) || ts.isExportDeclaration(node)
            ? node.moduleSpecifier
            : ts.isCallExpression(node) &&
                (node.expression.kind === ts.SyntaxKind.ImportKeyword ||
                  node.expression.getText(source) === 'require')
              ? node.arguments[0]
              : undefined
        if (
          ts.isCallExpression(node) &&
          node.expression.kind === ts.SyntaxKind.ImportKeyword &&
          (!node.arguments[0] || !ts.isStringLiteral(node.arguments[0]))
        )
          readers.push(path)
        if (
          specifier &&
          ts.isStringLiteral(specifier) &&
          /^(?:node:)?(?:fs(?:\/promises)?|child_process|module)$/.test(specifier.text)
        )
          readers.push(path)
        ts.forEachChild(node, visit)
      }
      visit(source)
    }
  }
  return readers
}

export async function selectUnitTests(root, changed) {
  const context = await createVitest('test', { root, watch: false })
  try {
    const inventory = await context.globTestSpecifications()
    const available = new Set(inventory.map((spec) => spec.moduleId))
    // Check each changed input, not just the union: an untested new module must
    // not be hidden by another changed file that happens to have consumers.
    for (const path of changed) {
      context.config.related = [path]
      if (!(await context.getRelevantTestSpecifications()).length)
        return full(`no reliable related tests for ${relative(root, path)}`)
    }
    context.config.related = [
      ...changed,
      ...filesystemReaders(resolve(root, 'src')),
      ...filesystemReaders(resolve(root, 'scripts')),
      ...inventory
        .filter((spec) => spec.moduleId.endsWith('.contract.spec.ts'))
        .map((spec) => spec.moduleId),
    ]
    const selected = await context.getRelevantTestSpecifications()
    const filters = [...new Set(selected.map((spec) => spec.moduleId))].sort().map((file) => {
      if (!available.has(file)) throw new Error(`Unknown Vitest specification: ${file}`)
      return relative(root, file).split(sep).join('/')
    })
    if (!filters.length || filters.some((file) => !unitFile.test(file)))
      return full('related specification set is not valid')
    const count = Math.min(4, filters.length)
    return {
      mode: 'related',
      reason: 'native Vitest dependencies plus source contracts',
      filters,
      matrix: { shard: Array.from({ length: count }, (_, i) => i + 1), shard_count: [count] },
    }
  } finally {
    await context.close()
  }
}

export function unitRunArguments(filters, shard, count) {
  if (
    !Array.isArray(filters) ||
    filters.some(
      (file) =>
        typeof file !== 'string' ||
        !unitFile.test(file) ||
        file.split('/').includes('..') ||
        isAbsolute(file),
    )
  )
    throw new Error('Invalid unit test filters')
  if (
    !Number.isInteger(shard) ||
    !Number.isInteger(count) ||
    count < 1 ||
    count > 4 ||
    shard < 1 ||
    shard > count
  )
    throw new Error('Invalid unit test shard')
  return [
    'run',
    ...filters,
    `--shard=${shard}/${count}`,
    '--reporter=blob',
    '--reporter=default',
    `--outputFile.blob=./.vitest-reports/blob-shard-${shard}.json`,
  ]
}

async function main() {
  if (process.argv[2] === 'run') {
    const args = unitRunArguments(
      JSON.parse(process.env.TEST_FILTERS ?? 'null'),
      Number(process.env.TEST_SHARD),
      Number(process.env.TEST_SHARD_COUNT),
    )
    const result = spawnSync(
      process.execPath,
      [resolve(webRoot, 'node_modules/vitest/vitest.mjs'), ...args],
      { cwd: webRoot, stdio: 'inherit', env: process.env },
    )
    if (result.error) throw result.error
    process.exitCode = result.status ?? 1
    return
  }
  const repo = git(webRoot, 'rev-parse', '--show-toplevel').trim()
  const inputs = changedUnitInputs(
    repo,
    process.env.CI_BASE_SHA ?? '',
    process.env.CI_HEAD_SHA ?? '',
    process.env.CI_FORCE_FULL ?? 'true',
  )
  let plan = full(inputs.reason)
  if (inputs.changed) {
    try {
      plan = await selectUnitTests(webRoot, inputs.changed)
    } catch (error) {
      plan = full(`related planning failed: ${error.message}`)
    }
  }
  console.log(
    `Web unit plan: ${plan.mode}; ${plan.reason}; ${plan.filters.length || 'all'} test files`,
  )
  if (process.env.GITHUB_STEP_SUMMARY)
    appendFileSync(
      process.env.GITHUB_STEP_SUMMARY,
      `### Web unit selection\n\nMode: ${plan.mode}. ${plan.reason.replace(/[\r\n`]/g, ' ')}.\n\nSelected ${plan.filters.length || 'all'} test files across ${plan.matrix.shard.length} shards. Browser, build and static checks retain their complete coverage.\n`,
    )
  if (process.env.GITHUB_OUTPUT) {
    appendFileSync(
      process.env.GITHUB_OUTPUT,
      `matrix=${JSON.stringify(plan.matrix)}\nfilters=${JSON.stringify(plan.filters)}\nmode=${plan.mode}\n`,
    )
  } else console.log(JSON.stringify(plan))
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href)
  await main()
