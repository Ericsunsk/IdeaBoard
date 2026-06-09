import { spawn, spawnSync, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { existsSync } from 'node:fs'
import { createServer } from 'node:net'
import { homedir } from 'node:os'
import path from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'
import { adminNavigationRouteDefs } from '../src/features/admin/app/adminNavigationMeta'

type CommandSpec = {
  command: string
  args: string[]
}

type SmokePage = {
  label: string
  path: string
}

const WEB_ROOT = fileURLToPath(new URL('..', import.meta.url))
const HOST = process.env.ADMIN_MOCK_SMOKE_HOST || '127.0.0.1'
const PREFERRED_PORT = readPortEnv(process.env.ADMIN_MOCK_SMOKE_PORT, 5173)
const SESSION_NAME = `ams${process.pid}`
const ADMIN_API_RE = /\/api\/admin(?:[/?#]|$)/
const DEV_SERVER_TIMEOUT_MS = 45_000
const COMMAND_BUFFER_SIZE = 1024 * 1024 * 20
const IS_VERBOSE = /^(1|true|yes)$/i.test(process.env.ADMIN_MOCK_SMOKE_VERBOSE || '')

const AUTH_SESSION = {
  session: { id: 'admin-mock-smoke-session' },
  user: { id: 'auth-admin', username: 'admin', role: 'admin' },
}

const USER_INFO = {
  code: 0,
  msg: 'ok',
  data: {
    user: {
      id: 1,
      uid: 'ADMIN1',
      username: 'admin',
      role: 'admin',
      auth_user_id: 'auth-admin',
      avatar: '',
    },
  },
}

const pages: SmokePage[] = adminNavigationRouteDefs.map((route) => ({
  label: route.label,
  path: route.path,
}))

let devServer: ChildProcessWithoutNullStreams | null = null
let smokePort = PREFERRED_PORT
let baseUrl = `http://${HOST}:${smokePort}`
const devServerLogs: string[] = []

function readPortEnv(rawValue: string | undefined, fallback: number): number {
  if (!rawValue) return fallback
  const port = Number(rawValue)
  if (Number.isInteger(port) && port > 0 && port <= 65_535) return port
  throw new Error(`Invalid ADMIN_MOCK_SMOKE_PORT: ${rawValue}`)
}

function recordDevServerOutput(chunk: Buffer, streamName: 'stdout' | 'stderr') {
  const lines = chunk
    .toString('utf8')
    .split(/\r?\n/)
    .filter(Boolean)
    .map((line) => `[vite:${streamName}] ${line}`)
  devServerLogs.push(...lines)
  if (devServerLogs.length > 160) {
    devServerLogs.splice(0, devServerLogs.length - 160)
  }
}

function resolvePlaywrightCli(): CommandSpec {
  const explicitPath = process.env.PWCLI
  const candidatePaths = [
    explicitPath,
    path.join(homedir(), '.cc-switch/skills/playwright/scripts/playwright_cli.sh'),
    path.join(homedir(), '.codex/skills/playwright/scripts/playwright_cli.sh'),
  ].filter((value): value is string => Boolean(value))

  for (const candidate of candidatePaths) {
    if (existsSync(candidate)) {
      return { command: '/bin/bash', args: [candidate] }
    }
  }

  const npxCommand = process.platform === 'win32' ? 'npx.cmd' : 'npx'
  const npxCheck = spawnSync(npxCommand, ['--version'], { encoding: 'utf8' })
  if (npxCheck.status === 0) {
    return {
      command: npxCommand,
      args: ['--yes', '--package', '@playwright/cli', 'playwright-cli'],
    }
  }

  throw new Error('Playwright CLI unavailable: set PWCLI or install Node.js/npm with npx')
}

async function isPortAvailable(port: number): Promise<boolean> {
  return await new Promise((resolve) => {
    const server = createServer()
    server.once('error', () => resolve(false))
    server.once('listening', () => {
      server.close(() => resolve(true))
    })
    server.listen(port, HOST)
  })
}

async function resolveSmokePort(): Promise<number> {
  if (process.env.ADMIN_MOCK_SMOKE_PORT) {
    return PREFERRED_PORT
  }

  for (let candidate = PREFERRED_PORT; candidate < PREFERRED_PORT + 20; candidate++) {
    if (await isPortAvailable(candidate)) {
      return candidate
    }
  }

  throw new Error(`No available port found from ${PREFERRED_PORT} to ${PREFERRED_PORT + 19}`)
}

function runCommand(spec: CommandSpec, args: string[], options: { allowFailure?: boolean } = {}) {
  const fullArgs = [...spec.args, '--session', SESSION_NAME, ...args]
  const result = spawnSync(spec.command, fullArgs, {
    cwd: WEB_ROOT,
    encoding: 'utf8',
    env: process.env,
    maxBuffer: COMMAND_BUFFER_SIZE,
  })

  if (result.error) {
    throw result.error
  }

  const stdout = result.stdout || ''
  const stderr = result.stderr || ''
  if (result.status !== 0 && !options.allowFailure) {
    throw new Error(
      [
        `Command failed: ${spec.command} ${fullArgs.join(' ')}`,
        stdout.trim(),
        stderr.trim(),
      ]
        .filter(Boolean)
        .join('\n'),
    )
  }

  return { stdout, stderr, status: result.status ?? 0 }
}

function startDevServer() {
  devServer = spawn(
    process.platform === 'win32' ? 'npm.cmd' : 'npm',
    ['run', 'dev', '--', '--host', HOST, '--port', String(smokePort), '--strictPort'],
    {
      cwd: WEB_ROOT,
      env: {
        ...process.env,
        VITE_ADMIN_MOCK_DATA: '1',
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    },
  )

  devServer.stdout.on('data', (chunk: Buffer) => recordDevServerOutput(chunk, 'stdout'))
  devServer.stderr.on('data', (chunk: Buffer) => recordDevServerOutput(chunk, 'stderr'))
}

async function waitForDevServer() {
  const deadline = Date.now() + DEV_SERVER_TIMEOUT_MS
  while (Date.now() < deadline) {
    if (devServer?.exitCode != null) {
      throw new Error(`Vite dev server exited early with code ${devServer.exitCode}`)
    }

    try {
      const response = await fetch(baseUrl, { signal: AbortSignal.timeout(1000) })
      if (response.status < 500) return
    } catch {
      await delay(500)
    }
  }

  throw new Error(`Vite dev server did not become ready within ${DEV_SERVER_TIMEOUT_MS}ms`)
}

function setupBrowser(playwrightCli: CommandSpec) {
  runCommand(playwrightCli, ['open', 'about:blank'])
  runCommand(playwrightCli, [
    'route',
    '**/api/auth/get-session',
    '--status',
    '200',
    '--content-type',
    'application/json',
    '--body',
    JSON.stringify(AUTH_SESSION),
  ])
  runCommand(playwrightCli, [
    'route',
    '**/api/user/info',
    '--status',
    '200',
    '--content-type',
    'application/json',
    '--body',
    JSON.stringify(USER_INFO),
  ])
  runCommand(playwrightCli, [
    'route',
    '**/api/admin/**',
    '--status',
    '599',
    '--content-type',
    'application/json',
    '--body',
    JSON.stringify({ code: 599, msg: 'admin mock smoke blocked real admin API request' }),
  ])
}

function assertNoConsoleErrors(output: string, page: SmokePage) {
  if (!output.includes('Returning 0 messages for level "error"')) {
    throw new Error(`Console errors found on ${page.path}\n${output}`)
  }
}

function assertNoAdminApiRequests(output: string, page: SmokePage) {
  const unexpectedRequests = output
    .split(/\r?\n/)
    .filter((line) => ADMIN_API_RE.test(line))
    .join('\n')
  if (unexpectedRequests) {
    throw new Error(`Unexpected admin API requests on ${page.path}\n${unexpectedRequests}`)
  }
}

function assertPageReached(snapshot: string, page: SmokePage) {
  if (!snapshot.includes(`Page URL: ${baseUrl}${page.path}`)) {
    throw new Error(`Admin route did not stay on ${page.path}\n${snapshot}`)
  }
  if (!snapshot.includes(page.label)) {
    throw new Error(`Admin page label not found on ${page.path}\n${snapshot}`)
  }
}

function smokePage(playwrightCli: CommandSpec, page: SmokePage) {
  runCommand(playwrightCli, ['goto', `${baseUrl}${page.path}`])
  const snapshot = runCommand(playwrightCli, ['snapshot']).stdout
  assertPageReached(snapshot, page)

  const consoleOutput = runCommand(playwrightCli, ['console', 'error']).stdout
  assertNoConsoleErrors(consoleOutput, page)

  const requestsOutput = runCommand(playwrightCli, ['requests']).stdout
  assertNoAdminApiRequests(requestsOutput, page)

  if (IS_VERBOSE) {
    console.log(`ok ${page.path}`)
  }
}

async function stopDevServer() {
  if (!devServer || devServer.exitCode != null) return
  devServer.kill('SIGINT')
  await Promise.race([
    new Promise<void>((resolve) => devServer?.once('exit', () => resolve())),
    delay(3000),
  ])
  if (devServer.exitCode == null) {
    devServer.kill('SIGTERM')
    await Promise.race([
      new Promise<void>((resolve) => devServer?.once('exit', () => resolve())),
      delay(1000),
    ])
  }
}

async function main() {
  let playwrightCli: CommandSpec | null = null
  try {
    playwrightCli = resolvePlaywrightCli()
    smokePort = await resolveSmokePort()
    baseUrl = `http://${HOST}:${smokePort}`
    startDevServer()
    await waitForDevServer()
    setupBrowser(playwrightCli)

    for (const page of pages) {
      smokePage(playwrightCli, page)
    }

    console.log(`admin mock smoke passed: ${pages.length} pages`)
  } catch (error) {
    console.error(error instanceof Error ? error.message : error)
    if (devServerLogs.length > 0) {
      console.error(devServerLogs.join('\n'))
    }
    process.exitCode = 1
  } finally {
    if (playwrightCli) {
      runCommand(playwrightCli, ['close'], { allowFailure: true })
    }
    await stopDevServer()
  }
}

void main()
