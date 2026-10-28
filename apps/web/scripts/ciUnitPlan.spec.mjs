import { test } from 'node:test'
import process from 'node:process'
import assert from 'node:assert/strict'
import { execFileSync, spawnSync } from 'node:child_process'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, symlinkSync, realpathSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { changedUnitInputs, selectUnitTests, unitRunArguments } from './ciUnitPlan.mjs'

function fixture(t) {
  const root = realpathSync(mkdtempSync(resolve(tmpdir(), 'ideaboard-web-impact-')))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const write = (path, content) => {
    mkdirSync(dirname(resolve(root, path)), { recursive: true })
    writeFileSync(resolve(root, path), content)
  }
  const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8' }).trim()
  git('init', '-q')
  // Fixture-local identity only; never changes the user's Git configuration.
  const commit = () => {
    git('add', '.')
    git(
      '-c',
      'user.name=CI Fixture',
      '-c',
      'user.email=ci@example.invalid',
      'commit',
      '-qm',
      'fixture',
    )
    return git('rev-parse', 'HEAD')
  }
  return { root, write, git, commit }
}

test('verified cumulative range only accepts supported local source changes', (t) => {
  const { root, write, git, commit } = fixture(t)
  write('apps/web/src/features/sample/leaf.ts', 'export const value=1')
  const base = commit()
  write('apps/web/src/features/sample/leaf.ts', 'export const value=2')
  commit() // This intermediate commit need not have passed CI.
  write('apps/web/src/features/sample/view.vue', '<template>sample</template>')
  const head = commit()
  assert.deepEqual(changedUnitInputs(root, base, head, 'false').changed, [
    resolve(root, 'apps/web/src/features/sample/leaf.ts'),
    resolve(root, 'apps/web/src/features/sample/view.vue'),
  ])
  assert.match(changedUnitInputs(root, '', head, 'false').reason, /unavailable/)
  assert.match(changedUnitInputs(root, head, base, 'false').reason, /checkout/)
  assert.match(changedUnitInputs(root, base, head, 'true').reason, /complete/)
  write('apps/web/src/features/sample/leaf.ts', 'export const value=3')
  assert.match(changedUnitInputs(root, base, head, 'false').reason, /dirty/)
  git('restore', '.')
  write('apps/web/src/theme/theme.css', ':root{}')
  assert.match(changedUnitInputs(root, head, commit(), 'false').reason, /non-source/)
})

test('deletion, shared routes/contracts, fixture inputs and unavailable ancestors require full tests', (t) => {
  const { root, write, git, commit } = fixture(t)
  write('apps/web/src/features/sample/leaf.ts', 'export const value=1')
  let base = commit()
  git('rm', 'apps/web/src/features/sample/leaf.ts')
  let head = commit()
  assert.match(changedUnitInputs(root, base, head, 'false').reason, /deleted/)
  for (const path of [
    'apps/web/src/features/sample/routes/index.ts',
    'apps/web/src/api/client.ts',
    'apps/web/src/test/setup.ts',
    'apps/web/src/features/sample/data.json',
    'apps/web/vite.config.ts',
  ]) {
    base = head
    write(path, '{}')
    head = commit()
    assert.ok(changedUnitInputs(root, base, head, 'false').reason, path)
  }
  assert.match(changedUnitInputs(root, 'f'.repeat(40), head, 'false').reason, /cannot be verified/)
})

test('native Vitest includes transitive alias/Vue/dynamic imports, self tests and non-import source contracts', async (t) => {
  const { root, write } = fixture(t)
  const installed = resolve(dirname(fileURLToPath(import.meta.url)), '../node_modules')
  symlinkSync(installed, resolve(root, 'node_modules'), 'dir')
  write(
    'vite.config.mjs',
    `import vue from '@vitejs/plugin-vue'; import {fileURLToPath} from 'node:url'; export default { plugins:[vue()], resolve:{alias:{'@':fileURLToPath(new URL('./src',import.meta.url))}}, test:{environment:'node',include:['src/**/*.unit.spec.ts','src/**/*.contract.spec.ts']} }`,
  )
  write('src/features/sample/leaf.ts', 'export const value=1')
  write('src/features/sample/bridge.ts', "export {value} from '@/features/sample/leaf'")
  write(
    'src/features/sample/View.vue',
    "<script setup>import {value} from './bridge'</script><template>{{value}}</template>",
  )
  write(
    'src/features/sample/view.unit.spec.ts',
    "import View from './View.vue';import{test}from'vitest';test('view',()=>View)",
  )
  write('src/features/sample/dynamic.ts', "export const load=()=>import('./leaf')")
  write(
    'src/features/sample/dynamic.unit.spec.ts',
    "import{load}from'./dynamic';import{test}from'vitest';test('dynamic',()=>load())",
  )
  write('src/features/other/other.unit.spec.ts', "import{test}from'vitest';test('other',()=>{})")
  write(
    'scripts/sourceReader.ts',
    "import{readFileSync}from'node:fs';export const read=()=>readFileSync('src/features/sample/leaf.ts','utf8')",
  )
  write(
    'src/features/policy/policy.unit.spec.ts',
    "import{read}from'../../../scripts/sourceReader';import{test,expect}from'vitest';test('source policy',()=>expect(read()).toContain('value=1'))",
  )
  write(
    'src/features/policy/contract.contract.spec.ts',
    "import{test}from'vitest';test('contract',()=>{})",
  )
  write('src/features/sample/untested.ts', 'export const noConsumer=1')
  const plan = await selectUnitTests(root, [resolve(root, 'src/features/sample/leaf.ts')])
  assert.equal(plan.mode, 'related', plan.reason)
  assert.deepEqual(plan.filters, [
    'src/features/policy/contract.contract.spec.ts',
    'src/features/policy/policy.unit.spec.ts',
    'src/features/sample/dynamic.unit.spec.ts',
    'src/features/sample/view.unit.spec.ts',
  ])
  const self = await selectUnitTests(root, [resolve(root, 'src/features/other/other.unit.spec.ts')])
  assert.ok(self.filters.includes('src/features/other/other.unit.spec.ts'))
  const unknown = await selectUnitTests(root, [
    resolve(root, 'src/features/sample/leaf.ts'),
    resolve(root, 'src/features/sample/untested.ts'),
  ])
  assert.equal(unknown.mode, 'full')
  assert.match(unknown.reason, /no reliable/)
  write('src/features/sample/leaf.ts', 'export const value=2')
  const mutated = spawnSync(
    process.execPath,
    [resolve(root, 'node_modules/vitest/vitest.mjs'), ...unitRunArguments(plan.filters, 1, 1)],
    { cwd: root, encoding: 'utf8' },
  )
  assert.equal(mutated.status, 1, mutated.stdout + mutated.stderr)
  assert.match(mutated.stdout + mutated.stderr, /source policy/)
})

test('canonical Vitest runner preserves validated filters, blob reporting and rejects unsafe or empty shards', () => {
  assert.deepEqual(unitRunArguments(['src/features/sample/view.unit.spec.ts'], 1, 1), [
    'run',
    'src/features/sample/view.unit.spec.ts',
    '--shard=1/1',
    '--reporter=blob',
    '--reporter=default',
    '--outputFile.blob=./.vitest-reports/blob-shard-1.json',
  ])
  assert.ok(unitRunArguments([], 4, 4).includes('--shard=4/4'))
  for (const filters of [
    null,
    ['--passWithNoTests'],
    ['src/../sample.unit.spec.ts'],
    ['/tmp/sample.unit.spec.ts'],
  ])
    assert.throws(() => unitRunArguments(filters, 1, 1), /Invalid/)
  for (const [shard, count] of [
    [0, 4],
    [2, 1],
    [1, 0],
    [1, 5],
    [NaN, 4],
  ])
    assert.throws(() => unitRunArguments([], shard, count), /Invalid/)
})

test('native filtered shards write mergeable reports and propagate test failures', (t) => {
  const { root, write } = fixture(t)
  symlinkSync(
    resolve(dirname(fileURLToPath(import.meta.url)), '../node_modules'),
    resolve(root, 'node_modules'),
    'dir',
  )
  write(
    'vite.config.mjs',
    "export default {test:{environment:'node',include:['src/**/*.unit.spec.ts']}}",
  )
  write(
    'src/sample/one.unit.spec.ts',
    "import{test,expect}from'vitest';test('one',()=>expect(1).toBe(1))",
  )
  write(
    'src/sample/two.unit.spec.ts',
    "import{test,expect}from'vitest';test('two',()=>expect(2).toBe(2))",
  )
  const cli = resolve(root, 'node_modules/vitest/vitest.mjs')
  const run = (args) => spawnSync(process.execPath, [cli, ...args], { cwd: root, encoding: 'utf8' })
  const selected = ['src/sample/one.unit.spec.ts', 'src/sample/two.unit.spec.ts']
  for (const shard of [1, 2]) {
    const result = run(unitRunArguments(selected, shard, 2))
    assert.equal(result.status, 0, result.stdout + result.stderr)
  }
  const merged = run(['run', '--mergeReports', './.vitest-reports'])
  assert.equal(merged.status, 0, merged.stdout + merged.stderr)
  assert.match(merged.stdout, /2 passed/)
  write(
    'src/sample/one.unit.spec.ts',
    "import{test,expect}from'vitest';test('broken',()=>expect(1).toBe(2))",
  )
  const failed = run(unitRunArguments(['src/sample/one.unit.spec.ts'], 1, 1))
  assert.equal(failed.status, 1)
  assert.match(failed.stdout + failed.stderr, /broken/)
})
