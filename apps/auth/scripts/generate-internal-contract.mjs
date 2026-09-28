import { execFileSync } from 'node:child_process'
import {
  cpSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const authRoot = resolve(scriptDir, '..')
const repoRoot = resolve(authRoot, '../..')
const apiRoot = resolve(repoRoot, 'apps/api')
const specPath = resolve(repoRoot, 'contracts/auth-internal.openapi.json')
const tsTarget = resolve(authRoot, 'src/generated/auth-internal')
const goTarget = resolve(apiRoot, 'internal/identity/auth/internalcontract/client.gen.go')
const isCheck = process.argv.includes('--check')
const tempRoot = mkdtempSync(join(tmpdir(), 'ideaboard-auth-contract-'))
const tempTS = join(tempRoot, 'typescript')
const tempGo = join(tempRoot, 'client.gen.go')

try {
  generateTypeScript(tempTS)
  writeFileSync(join(tempTS, 'routes.gen.ts'), renderRouteContract(), 'utf8')
  generateGo(tempGo)

  if (isCheck) {
    const drift = [...compareDirectories(tsTarget, tempTS), ...compareFiles(goTarget, tempGo)]
    if (drift.length > 0) {
      console.error(`Auth internal contract generated output is stale:\n${drift.join('\n')}`)
      process.exitCode = 1
    }
  } else {
    rmSync(tsTarget, { force: true, recursive: true })
    mkdirSync(dirname(tsTarget), { recursive: true })
    cpSync(tempTS, tsTarget, { recursive: true })
    mkdirSync(dirname(goTarget), { recursive: true })
    cpSync(tempGo, goTarget)
  }
} finally {
  rmSync(tempRoot, { force: true, recursive: true })
}

function generateTypeScript(output) {
  const executable = resolve(authRoot, 'node_modules/.bin/openapi-ts')
  execFileSync(
    executable,
    [
      '--input',
      specPath,
      '--output',
      output,
      '--plugins',
      '@hey-api/typescript',
      'fastify',
      '--no-log-file',
      '--silent',
    ],
    { cwd: authRoot, stdio: 'inherit' },
  )
}

function generateGo(output) {
  execFileSync(
    'go',
    [
      'tool',
      'oapi-codegen',
      '-package',
      'internalcontract',
      '-generate',
      'types,client',
      '-o',
      output,
      specPath,
    ],
    { cwd: apiRoot, stdio: 'inherit' },
  )
}

function renderRouteContract() {
  const document = JSON.parse(readFileSync(specPath, 'utf8'))
  const routes = []
  for (const [openapiPath, pathItem] of Object.entries(document.paths ?? {})) {
    for (const method of ['get', 'post', 'put', 'patch', 'delete']) {
      const operation = pathItem?.[method]
      if (!operation?.operationId) continue
      routes.push({
        operationId: operation.operationId,
        method: method.toUpperCase(),
        openapiPath,
        fastifyPath: openapiPath.replace(/\{([^}]+)\}/g, ':$1'),
        responseStatuses: Object.keys(operation.responses ?? {}).map(Number),
      })
    }
  }

  const properties = routes
    .map((route) =>
      [
        `  ${JSON.stringify(route.operationId)}: {`,
        `    method: ${JSON.stringify(route.method)},`,
        `    openapiPath: ${JSON.stringify(route.openapiPath)},`,
        `    fastifyPath: ${JSON.stringify(route.fastifyPath)},`,
        `    responseStatuses: ${JSON.stringify(route.responseStatuses)},`,
        '  },',
      ].join('\n'),
    )
    .join('\n')

  return [
    '// This file is auto-generated from contracts/auth-internal.openapi.json. Do not edit.',
    '',
    'export const authInternalRoutes = {',
    properties,
    '} as const',
    '',
    'export type AuthInternalOperationID = keyof typeof authInternalRoutes',
    '',
  ].join('\n')
}

function compareDirectories(expectedDir, actualDir) {
  if (!existsSync(expectedDir)) return [`missing ${relative(repoRoot, expectedDir)}`]
  const expectedFiles = listFiles(expectedDir)
  const actualFiles = listFiles(actualDir)
  const names = new Set([...expectedFiles, ...actualFiles])
  return [...names].flatMap((name) => compareFiles(join(expectedDir, name), join(actualDir, name)))
}

function listFiles(root, prefix = '') {
  return readdirSync(join(root, prefix), { withFileTypes: true }).flatMap((entry) => {
    const name = join(prefix, entry.name)
    return entry.isDirectory() ? listFiles(root, name) : [name]
  })
}

function compareFiles(expected, actual) {
  if (!existsSync(expected)) return [`missing ${relative(repoRoot, expected)}`]
  if (!existsSync(actual)) return [`unexpected ${relative(repoRoot, expected)}`]
  return readFileSync(expected).equals(readFileSync(actual))
    ? []
    : [`stale ${relative(repoRoot, expected)}`]
}
