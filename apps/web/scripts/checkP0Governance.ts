import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { ESLint } from 'eslint'

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const workspace = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'ideaboard-p0-lint-')))
function write(file: string, source: string) {
  const absolute = path.join(workspace, file)
  fs.mkdirSync(path.dirname(absolute), { recursive: true })
  fs.writeFileSync(absolute, source)
}

try {
  write('package.json', '{"type":"module"}')
  write(
    'tsconfig.json',
    '{"compilerOptions":{"strict":true,"target":"ESNext","module":"ESNext","moduleResolution":"bundler"},"include":["src/**/*.ts"]}',
  )
  write('src/features/board/index.ts', 'export const board = 1\n')
  write('src/features/board/core/index.ts', 'export const cascaded = 1\n')
  write('src/check.ts', 'export const initialized = true\n')
  const baseline = JSON.parse(
    fs.readFileSync(path.join(webRoot, 'scripts/booleanNaming.baseline.json'), 'utf8'),
  ) as { declarations: string[] }
  const legacy = baseline.declarations.find((identity) =>
    /^[^|]+\.ts\|<module>\|variable:[a-zA-Z]+\|1$/.test(identity),
  )
  assert.ok(legacy)
  const [legacyFile, , declaration] = legacy.split('|')
  const legacyName = declaration?.slice('variable:'.length)
  assert.ok(legacyFile && legacyName)
  write(legacyFile, `const ${legacyName}: boolean = true; console.log(${legacyName})`)
  const eslint = new ESLint({
    cwd: workspace,
    overrideConfigFile: path.join(webRoot, 'eslint.config.js'),
    overrideConfig: [
      {
        files: ['src/**/*.ts'],
        languageOptions: {
          parserOptions: {
            project: [path.join(workspace, 'tsconfig.json')],
            tsconfigRootDir: workspace,
          },
        },
      },
    ],
  })
  async function warnings(source: string, file = 'src/check.ts') {
    const [result] = await eslint.lintText(source, { filePath: path.join(workspace, file) })
    assert.ok(result)
    assert.equal(result.messages.filter((message) => message.fatal).length, 0)
    return result.messages.filter(
      (message) =>
        message.ruleId?.startsWith('governance/') &&
        ['governance/boolean-prefix', 'governance/no-barrel-import'].includes(message.ruleId),
    )
  }
  assert.equal((await warnings('const isReady: boolean = true; console.log(isReady)')).length, 0)
  assert.equal((await warnings('const ready: boolean = true; console.log(ready)')).length, 1)
  assert.equal(
    (
      await warnings(
        'function getFlag(): boolean { return true }; const ready = getFlag(); console.log(ready)',
      )
    ).length,
    1,
  )
  assert.equal((await warnings('const ready: string = "yes"; console.log(ready)')).length, 0)
  const bindingCases = [
    ['const { ready } = { ready: true }; console.log(ready)', 'variable:ready'],
    [
      'const { ready: selected = false } = {} as { ready?: boolean }; console.log(selected)',
      'variable:selected',
    ],
    ['const [enabled] = [true]; console.log(enabled)', 'variable:enabled'],
    [
      'const { nested: { selected } } = { nested: { selected: true } }; console.log(selected)',
      'variable:selected',
    ],
    ['function useFlag({ ready }: { ready: boolean }) { return ready }', 'parameter:ready'],
    ['const useFlag = ([ready = false]: boolean[]) => ready', 'parameter:ready'],
    ['function useFlag(...[enabled]: [boolean]) { return enabled }', 'parameter:enabled'],
  ]
  for (const [source, identity] of bindingCases)
    assert.equal(
      (await warnings(source!)).filter((message) => message.message.includes(identity!)).length,
      1,
      identity,
    )
  assert.equal(
    (
      await warnings('const { ready: isReady } = {} as { ready: boolean }; console.log(isReady)')
    ).filter((message) => message.message.includes('variable:ready')).length,
    0,
  )
  assert.equal(
    (
      await warnings(
        'interface State { ready: boolean }; const state: State = { ready: true }; console.log(state)',
      )
    ).filter((message) => message.ruleId === 'governance/boolean-prefix').length,
    2,
  )
  assert.equal(
    (await warnings('const state: { value: boolean } = { value: true }; console.log(state)')).some(
      (message) => message.message.includes('variable:state'),
    ),
    true,
  )
  assert.equal(
    (await warnings('import { board } from "./features/board"; console.log(board)')).length,
    0,
  )
  assert.equal(
    (
      await warnings('import { cascaded } from "./features/board/core"; console.log(cascaded)')
    ).filter((message) => message.ruleId === 'governance/no-barrel-import').length,
    1,
  )
  assert.equal(
    (
      await warnings(
        'const load = () => import("./features/board/core/index.ts"); console.log(load)',
      )
    ).filter((message) => message.ruleId === 'governance/no-barrel-import').length,
    1,
  )
  assert.equal(
    (
      await warnings(
        `const ${legacyName}: boolean = true; const newlyEnabled: boolean = true; console.log(${legacyName}, newlyEnabled)`,
        legacyFile,
      )
    ).filter((message) => message.ruleId === 'governance/boolean-prefix').length,
    1,
  )
  const destructuredLegacy = baseline.declarations.find((identity) =>
    /^[^|]+\.ts\|FunctionDeclaration:[A-Za-z]+\/property:[A-Za-z]+\|variable:[A-Za-z]+\|1$/.test(
      identity,
    ),
  )
  assert.ok(destructuredLegacy)
  const [destructuredFile, owner, binding] = destructuredLegacy.split('|')
  const functionName = owner!.split('/')[0]!.slice('FunctionDeclaration:'.length)
  const bindingName = binding!.slice('variable:'.length)
  write(destructuredFile!, `export function ${functionName}() { return true }`)
  const newBinding = `function ${functionName}() { const { ${bindingName}, newlyEnabled } = {} as { ${bindingName}: boolean; newlyEnabled: boolean }; return [${bindingName}, newlyEnabled] }`
  const bindingWarnings = (await warnings(newBinding, destructuredFile!)).filter((message) =>
    message.message.includes('|variable:'),
  )
  assert.equal(bindingWarnings.length, 1)
  assert.ok(bindingWarnings[0]!.message.includes('variable:newlyEnabled'))
  console.log(
    '✓ P0 ESLint fixtures: typed/inferred booleans, object/array/nested/default/rest/parameter bindings, nonboolean, member, Ref-like state, exact legacy declaration, same-file new declaration, canonical/cascading/static/dynamic imports.',
  )
} finally {
  fs.rmSync(workspace, { recursive: true, force: true })
}
