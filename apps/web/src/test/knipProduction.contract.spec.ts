import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import ts from 'typescript'
import { describe, expect, test } from 'vitest'

const webRoot = process.cwd()
const cli = path.join(webRoot, 'node_modules/knip/bin/knip.js')
type Issue = { name: string }
type Report = {
  files?: string[]
  issues: Array<{ file: string; files?: Issue[]; dependencies?: Issue[]; unlisted?: Issue[] }>
}

function runtimeIssues(report: Report) {
  return report.issues.flatMap((row) => [...(row.dependencies ?? []), ...(row.unlisted ?? [])])
}

function readConfig(file: string) {
  const parsed = ts.parseConfigFileTextToJson(
    file,
    fs.readFileSync(path.join(webRoot, file), 'utf8'),
  )
  if (parsed.error) throw new Error(ts.flattenDiagnosticMessageText(parsed.error.messageText, '\n'))
  return parsed.config as { entry: string[]; project: string[]; ignoreDependencies?: string[] }
}

function write(workspace: string, file: string, source: string) {
  const absolute = path.join(workspace, file)
  fs.mkdirSync(path.dirname(absolute), { ['recursive']: true })
  fs.writeFileSync(absolute, source)
}

function run(workspace: string, isProduction: boolean) {
  const result = spawnSync(
    process.execPath,
    [
      cli,
      '--config',
      isProduction ? 'knip.production.jsonc' : 'knip.jsonc',
      '--reporter',
      'json',
      '--no-progress',
      '--include',
      'files,dependencies,unlisted',
      ...(isProduction ? ['--production', '--strict'] : []),
    ],
    { cwd: workspace, encoding: 'utf8', timeout: 20_000 },
  )
  if (result.error) throw result.error
  return { status: result.status, report: JSON.parse(result.stdout) as Report }
}

function withWorkspace(check: (workspace: string) => void) {
  const workspace = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'ideaboard-knip-')))
  try {
    for (const file of ['knip.jsonc', 'knip.production.jsonc'])
      fs.copyFileSync(path.join(webRoot, file), path.join(workspace, file))
    write(
      workspace,
      'package.json',
      JSON.stringify({
        name: 'knip-policy-fixture',
        ['private']: true,
        type: 'module',
        dependencies: { 'runtime-package': '1.0.0' },
        devDependencies: { 'test-package': '1.0.0', vitest: '1.0.0' },
        scripts: { test: 'vitest' },
      }),
    )
    for (const name of ['runtime-package', 'test-package', 'vitest']) {
      write(
        workspace,
        `node_modules/${name}/package.json`,
        JSON.stringify({ name, version: '1.0.0', main: 'index.js' }),
      )
      write(workspace, `node_modules/${name}/index.js`, 'export const value = 1\n')
    }
    write(
      workspace,
      'vitest.config.ts',
      'export default { test: { include: ["src/**/*.spec.ts"] } }\n',
    )
    write(
      workspace,
      'src/app/main.ts',
      'import { value } from "runtime-package"\nconsole.log(value)\n',
    )
    write(workspace, 'src/app/router/index.ts', 'export default []\n')
    write(workspace, 'src/app/App.vue', '<template><main /></template>\n')
    write(
      workspace,
      'src/example.unit.spec.ts',
      'import { value } from "test-package"\nimport { helper } from "./features/demo/testing"\nconsole.log(value, helper)\n',
    )
    write(workspace, 'src/features/demo/testing.ts', 'export const helper = 1\n')
    write(workspace, 'src/test/setup.ts', 'export const setup = 1\n')
    check(workspace)
  } finally {
    fs.rmSync(workspace, { ['recursive']: true, ['force']: true })
  }
}

describe('Knip runtime and test graphs', () => {
  test(
    'actual production config tracks runtime dependency and excludes the test dependency graph',
    () =>
      withWorkspace((workspace) => {
        const result = run(workspace, true)
        expect(runtimeIssues(result.report)).toEqual([])
      }),
    30_000,
  )

  test(
    'normal config retains the test graph and reports a removed test dependency',
    () =>
      withWorkspace((workspace) => {
        const packagePath = path.join(workspace, 'package.json')
        const manifest = JSON.parse(fs.readFileSync(packagePath, 'utf8'))
        delete manifest.devDependencies['test-package']
        fs.writeFileSync(packagePath, JSON.stringify(manifest))
        const result = run(workspace, false)
        expect(result.status).toBe(1)
        expect(
          result.report.issues.flatMap((row) => row.unlisted ?? []).map((issue) => issue.name),
        ).toContain('test-package')
        expect(runtimeIssues(run(workspace, true).report)).toEqual([])
      }),
    30_000,
  )

  test(
    'production fails when runtime import loses its dependency declaration',
    () =>
      withWorkspace((workspace) => {
        const packagePath = path.join(workspace, 'package.json')
        const manifest = JSON.parse(fs.readFileSync(packagePath, 'utf8'))
        delete manifest.dependencies['runtime-package']
        fs.writeFileSync(packagePath, JSON.stringify(manifest))
        const result = run(workspace, true)
        expect(result.status).toBe(1)
        expect(
          result.report.issues.flatMap((row) => row.unlisted ?? []).map((issue) => issue.name),
        ).toContain('runtime-package')
      }),
    30_000,
  )

  test(
    'runtime imports through testing and build helper paths remain visible',
    () =>
      withWorkspace((workspace) => {
        for (const helper of [
          'src/features/demo/testing.ts',
          'src/features/demo/testing/adapter.ts',
          'src/test/helper.ts',
          'src/shared/i18n/localeParity.ts',
          'src/shared/seo/seoStaticHtml.ts',
        ]) {
          write(
            workspace,
            helper,
            'import { value } from "missing-runtime-package"; export const helper = value\n',
          )
          const relative = path
            .relative('src/app', helper)
            .split(path.sep)
            .join('/')
            .replace(/\.ts$/, '')
          write(
            workspace,
            'src/app/main.ts',
            `import { value } from "runtime-package"; import { helper } from "${relative}"; console.log(value, helper)\n`,
          )
          const result = run(workspace, true)
          expect(result.status, helper).toBe(1)
          expect(
            result.report.issues.flatMap((row) => row.unlisted ?? []).map((issue) => issue.name),
            helper,
          ).toContain('missing-runtime-package')
          write(workspace, helper, 'export const helper = 1\n')
        }
      }),
    30_000,
  )

  test(
    'production detects unused declared runtime dependencies',
    () =>
      withWorkspace((workspace) => {
        write(workspace, 'src/app/main.ts', 'console.log("runtime root")\n')
        const result = run(workspace, true)
        expect(result.status).toBe(1)
        expect(
          result.report.issues.flatMap((row) => row.dependencies ?? []).map((issue) => issue.name),
        ).toContain('runtime-package')
      }),
    30_000,
  )

  test('production markers and the narrow CSS exception remain explicit', () => {
    const production = readConfig('knip.production.jsonc')
    expect(
      [...production.entry, ...production.project].every((pattern) => pattern.endsWith('!')),
    ).toBe(true)
    expect(production.ignoreDependencies).toEqual(['tailwindcss'])
    expect(readConfig('knip.jsonc').entry.every((pattern) => !pattern.endsWith('!'))).toBe(true)
  })
})
