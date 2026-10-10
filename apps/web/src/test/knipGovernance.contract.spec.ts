// @vitest-environment node
import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { expect, test } from 'vitest'
import { generateMutator } from '@orval/core'
import {
  annotateKnipPublic,
  assertClientCompatibilityAlias,
} from '../../scripts/knipGovernanceConfig'

test('analysis annotation does not change unknown declarations', () => {
  const root = process.cwd()
  expect(
    annotateKnipPublic(
      'export const unknown = 1',
      path.join(root, 'unknown.ts'),
      { version: 1, generated: {}, files: {}, symbols: {} },
      root,
    ),
  ).toBe('export const unknown = 1')
})

test('Orval recognizes the retained ErrorType and BodyType generator protocol', async () => {
  const root = process.cwd()
  const mutator = await generateMutator({
    output: path.join(root, 'src/api/generated-query/public/fixture.ts'),
    mutator: {
      path: path.join(root, 'src/api/orvalMutator.ts'),
      name: 'orvalMutator',
      default: false,
    },
    name: 'fixture',
    workspace: root,
    tsconfig: {},
  })
  expect(mutator?.hasErrorType).toBe(true)
  expect(mutator?.bodyTypeName).toBe('BodyType')
})

test('the retained api/default alias cannot conceal new duplicate aliases', () => {
  expect(() =>
    assertClientCompatibilityAlias(
      'export const api={}; export default api; const first=api; const second=first; export {second as third};',
    ),
  ).toThrow()
  const run = (source: string) =>
    spawnSync(
      process.execPath,
      [
        '--experimental-strip-types',
        '--input-type=module',
        '-e',
        `import { assertClientCompatibilityAlias } from ${JSON.stringify(path.join(process.cwd(), 'scripts/knipGovernanceConfig.ts'))}; assertClientCompatibilityAlias(${JSON.stringify(source)})`,
      ],
      { encoding: 'utf8' },
    )
  expect(run('export const api = {}; export default api').status).toBe(0)
  expect(run('export const api = {}; export default api; export { api as other }').status).toBe(1)
  expect(run('export const api = {}; export default api; export const other = api').status).toBe(1)
})

test('exact analysis-only public symbols preserve Knip dependency, duplicate and orphan checks', () => {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'knip-governance-')))
  const webRoot = process.cwd()
  const factory = path.join(webRoot, 'scripts/knipGovernanceConfig.ts')
  const cli = path.join(webRoot, 'node_modules/knip/bin/knip.js')
  try {
    fs.writeFileSync(path.join(root, 'package.json'), '{"private":true}')
    fs.writeFileSync(path.join(root, 'main.ts'), "import './generated'\n")
    fs.writeFileSync(
      path.join(root, 'knip.jsonc'),
      JSON.stringify({
        entry: ['main.ts!'],
        project: ['main.ts!', 'generated.ts!', 'orphan.ts!', 'Component.vue!'],
        includeEntryExports: true,
      }),
    )
    fs.writeFileSync(
      path.join(root, 'knip.config.ts'),
      `import { createGovernanceConfig } from ${JSON.stringify(factory)}\nexport default createGovernanceConfig('knip.jsonc', ${JSON.stringify(root)}, {version:1,generated:{},files:{},symbols:{'generated.ts':{retained:{kind:'test-contract',evidence:['consumer.spec.ts']}}}})\n`,
    )
    const run = () =>
      spawnSync(
        process.execPath,
        [cli, '--config', 'knip.config.ts', '--production', '--strict', '--reporter', 'json'],
        { cwd: root, encoding: 'utf8', timeout: 20_000 },
      )
    const before = 'export const retained = 1\n'
    fs.writeFileSync(
      path.join(root, 'consumer.spec.ts'),
      "import { retained } from './generated'; console.log(retained)\n",
    )
    fs.writeFileSync(path.join(root, 'generated.ts'), before)
    const positive = run()
    expect(positive.status, positive.stderr + positive.stdout).toBe(0)
    expect(fs.readFileSync(path.join(root, 'generated.ts'), 'utf8')).toBe(before)
    fs.writeFileSync(
      path.join(root, 'generated.ts'),
      "import { secret } from 'unlisted-runtime-fixture'\nexport const retained = 1\nexport const unknown = secret\nexport const duplicate = unknown\n",
    )
    fs.writeFileSync(path.join(root, 'orphan.ts'), 'export const orphan = 1\n')
    const negative = run()
    expect(negative.status, negative.stderr).toBe(1)
    const report = JSON.parse(negative.stdout)
    const issue = report.issues.find((entry: { file: string }) => entry.file === 'generated.ts')
    expect(issue.exports.map((entry: { name: string }) => entry.name)).toContain('unknown')
    expect(issue.exports.map((entry: { name: string }) => entry.name)).not.toContain('retained')
    expect(issue.unlisted.map((entry: { name: string }) => entry.name)).toContain(
      'unlisted-runtime-fixture',
    )
    expect(issue.duplicates.length).toBeGreaterThan(0)
    expect(
      report.issues.find((entry: { file: string }) => entry.file === 'orphan.ts').files,
    ).toHaveLength(1)
    fs.writeFileSync(path.join(root, 'main.ts'), "import './generated'\nimport './Component.vue'\n")
    fs.writeFileSync(
      path.join(root, 'Component.vue'),
      '<script lang="ts">import { missing } from "unlisted-vue-runtime-fixture"; export const unknownVue = missing; export type UnknownVueType = { id: string }</script><template><div /></template>',
    )
    const vue = run()
    expect(vue.status, vue.stderr + vue.stdout).toBe(1)
    const vueIssue = JSON.parse(vue.stdout).issues.find(
      (entry: { file: string }) => entry.file === 'Component.vue',
    )
    expect(vueIssue.unlisted.map((entry: { name: string }) => entry.name)).toContain(
      'unlisted-vue-runtime-fixture',
    )
    expect(vueIssue.exports.map((entry: { name: string }) => entry.name)).toContain('unknownVue')
    expect(vueIssue.types.map((entry: { name: string }) => entry.name)).toContain('UnknownVueType')
    fs.writeFileSync(
      path.join(root, 'consumer.spec.ts'),
      '// retained remains mentioned but its named import has been removed\n',
    )
    const stale = run()
    expect(stale.status).toBe(2)
    expect(stale.stderr).toContain('Missing test Knip evidence')
  } finally {
    fs.rmSync(root, { recursive: true, force: true })
  }
}, 45_000)
