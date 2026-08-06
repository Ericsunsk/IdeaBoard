import { spawn, spawnSync, type ChildProcessByStdio } from 'node:child_process'
import { existsSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { createServer as createNetServer } from 'node:net'
import { homedir, tmpdir } from 'node:os'
import path from 'node:path'
import type { Readable } from 'node:stream'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'

export type CommandSpec = {
  command: string
  args: string[]
}

export type RunCommandOptions = {
  allowFailure?: boolean
  timeoutMs?: number
}

type MockSmokeRuntimeOptions = {
  hostEnvName: string
  portEnvName: string
  verboseEnvName: string
  sessionPrefix: string
  defaultPort?: number
}

const WEB_ROOT = fileURLToPath(new URL('..', import.meta.url))
const DEV_SERVER_TIMEOUT_MS = 45_000
const COMMAND_TIMEOUT_MS = 60_000
const COMMAND_BUFFER_SIZE = 1024 * 1024 * 20
const ROUTE_BODY_ARG_LIMIT_BYTES = 64 * 1024
const USE_PROCESS_GROUP = process.platform !== 'win32'
const PLAYWRIGHT_CLI_PACKAGE = '@playwright/cli@0.1.18'
const ROUTE_BODY_SERVER_SCRIPT = String.raw`
const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')

const rootDir = process.argv[1]
const server = http.createServer((request, response) => {
  const requestUrl = new URL(request.url || '/', 'http://127.0.0.1')
  const match = requestUrl.pathname.match(/^\/route-body\/([0-9]+)$/)
  if (!match) {
    response.writeHead(404, { 'content-type': 'text/plain; charset=utf-8' })
    response.end('Route body not found')
    return
  }

  const filePath = path.join(rootDir, match[1] + '.txt')
  if (!fs.existsSync(filePath)) {
    response.writeHead(404, { 'content-type': 'text/plain; charset=utf-8' })
    response.end('Route body not found')
    return
  }

  const stream = fs.createReadStream(filePath)
  response.writeHead(200, {
    'cache-control': 'no-store',
    'content-type': 'text/plain; charset=utf-8',
  })
  stream.pipe(response)
})

server.listen(0, '127.0.0.1', () => {
  const address = server.address()
  if (!address || typeof address === 'string') {
    console.error('Route body server did not expose a TCP address')
    process.exit(1)
  }
  console.log(JSON.stringify({ port: address.port }))
})

process.on('SIGTERM', () => {
  server.close(() => process.exit(0))
})
`

function readPortEnv(rawValue: string | undefined, fallback: number, envName: string): number {
  if (!rawValue) return fallback
  const port = Number(rawValue)
  if (Number.isInteger(port) && port > 0 && port <= 65_535) return port
  throw new Error(`Invalid ${envName}: ${rawValue}`)
}

function readBooleanEnv(rawValue: string | undefined): boolean {
  return /^(1|true|yes)$/i.test(rawValue || '')
}

function isProcessRunning(child: ChildProcessByStdio<null, Readable, Readable>): boolean {
  return child.exitCode == null && child.signalCode == null
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
      args: ['--yes', '--package', PLAYWRIGHT_CLI_PACKAGE, 'playwright-cli'],
    }
  }

  throw new Error('Playwright CLI unavailable: set PWCLI or install Node.js/npm with npx')
}

export function assertNoConsoleErrors(output: string, pagePath: string) {
  if (!output.includes('Returning 0 messages for level "error"')) {
    throw new Error(`Console errors found on ${pagePath}\n${output}`)
  }
}

export function createMockSmokeRuntime(options: MockSmokeRuntimeOptions) {
  const host = process.env[options.hostEnvName] || '127.0.0.1'
  const preferredPort = readPortEnv(
    process.env[options.portEnvName],
    options.defaultPort ?? 5173,
    options.portEnvName,
  )
  const sessionName = `${options.sessionPrefix}${process.pid}`
  const isVerbose = readBooleanEnv(process.env[options.verboseEnvName])
  const devServerLogs: string[] = []

  let devServer: ChildProcessByStdio<null, Readable, Readable> | null = null
  let routeBodyServer: ChildProcessByStdio<null, Readable, Readable> | null = null
  let routeBodyServerBaseUrl: string | null = null
  let routeBodyDir: string | null = null
  let routeBodyId = 0
  let smokePort = preferredPort

  const baseUrl = () => `http://${host}:${smokePort}`

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

  async function isPortAvailable(port: number): Promise<boolean> {
    return await new Promise((resolve) => {
      const server = createNetServer()
      server.once('error', () => resolve(false))
      server.once('listening', () => {
        server.close(() => resolve(true))
      })
      server.listen(port, host)
    })
  }

  async function resolveSmokePort(): Promise<number> {
    if (process.env[options.portEnvName]) {
      return preferredPort
    }

    for (let candidate = preferredPort; candidate < preferredPort + 20; candidate++) {
      if (await isPortAvailable(candidate)) {
        return candidate
      }
    }

    throw new Error(`No available port found from ${preferredPort} to ${preferredPort + 19}`)
  }

  async function startRouteBodyServer() {
    if (routeBodyServerBaseUrl) return

    routeBodyDir = mkdtempSync(path.join(tmpdir(), 'ideaboard-smoke-route-bodies-'))
    const server = spawn(process.execPath, ['-e', ROUTE_BODY_SERVER_SCRIPT, routeBodyDir], {
      cwd: WEB_ROOT,
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    routeBodyServer = server

    server.stderr.on('data', (chunk: Buffer) => {
      const lines = chunk
        .toString('utf8')
        .split(/\r?\n/)
        .filter(Boolean)
        .map((line) => `[route-body:stderr] ${line}`)
      devServerLogs.push(...lines)
    })

    const port = await new Promise<number>((resolve, reject) => {
      let stdout = ''
      const timeout = setTimeout(() => {
        cleanup()
        reject(new Error('Route body server did not become ready within 5000ms'))
      }, 5000)

      const cleanup = () => {
        clearTimeout(timeout)
        server.stdout.off('data', onData)
        server.off('error', onError)
        server.off('exit', onExit)
      }
      const onData = (chunk: Buffer) => {
        stdout += chunk.toString('utf8')
        const newlineIndex = stdout.indexOf('\n')
        if (newlineIndex === -1) return
        const line = stdout.slice(0, newlineIndex).trim()
        cleanup()
        try {
          const payload = JSON.parse(line) as { port?: unknown }
          const parsedPort = Number(payload.port)
          if (!Number.isInteger(parsedPort) || parsedPort <= 0 || parsedPort > 65_535) {
            reject(new Error(`Invalid route body server port: ${line}`))
            return
          }
          resolve(parsedPort)
        } catch (error) {
          reject(error)
        }
      }
      const onError = (error: Error) => {
        cleanup()
        reject(error)
      }
      const onExit = (code: number | null, signal: NodeJS.Signals | null) => {
        cleanup()
        reject(new Error(`Route body server exited early with code ${code} signal ${signal}`))
      }

      server.once('error', onError)
      server.once('exit', onExit)
      server.stdout.on('data', onData)
    })

    routeBodyServerBaseUrl = `http://127.0.0.1:${port}`
  }

  async function waitForDevServer() {
    const deadline = Date.now() + DEV_SERVER_TIMEOUT_MS
    while (Date.now() < deadline) {
      if (devServer && !isProcessRunning(devServer)) {
        throw new Error(`Vite dev server exited early with code ${devServer.exitCode}`)
      }

      try {
        const response = await fetch(baseUrl(), { signal: AbortSignal.timeout(1000) })
        if (response.status < 500) return
      } catch {
        await delay(500)
      }
    }

    throw new Error(`Vite dev server did not become ready within ${DEV_SERVER_TIMEOUT_MS}ms`)
  }

  async function startDevServer(extraEnv: Record<string, string | undefined> = {}) {
    smokePort = await resolveSmokePort()
    const server = spawn(
      process.platform === 'win32' ? 'npm.cmd' : 'npm',
      ['run', 'dev', '--', '--host', host, '--port', String(smokePort), '--strictPort'],
      {
        cwd: WEB_ROOT,
        detached: USE_PROCESS_GROUP,
        env: {
          ...process.env,
          ...extraEnv,
        },
        stdio: ['ignore', 'pipe', 'pipe'],
      },
    )
    devServer = server

    server.stdout.on('data', (chunk: Buffer) => recordDevServerOutput(chunk, 'stdout'))
    server.stderr.on('data', (chunk: Buffer) => recordDevServerOutput(chunk, 'stderr'))
    await waitForDevServer()
    await startRouteBodyServer()
  }

  function signalDevServer(
    server: ChildProcessByStdio<null, Readable, Readable>,
    signal: NodeJS.Signals,
  ) {
    if (server.pid == null) return
    if (USE_PROCESS_GROUP) {
      try {
        process.kill(-server.pid, signal)
        return
      } catch {
        // Fall back to signaling the direct npm process if the process group is gone.
      }
    }
    server.kill(signal)
  }

  function runCommand(spec: CommandSpec, args: string[], runOptions: RunCommandOptions = {}) {
    const commandArgs =
      args[0] === 'run-code' ? ['run-code', `async (page) => {\n${args[1] ?? ''}\n}`] : args
    const fullArgs = [...spec.args, '--session', sessionName, ...commandArgs]
    const timeoutMs = runOptions.timeoutMs ?? COMMAND_TIMEOUT_MS
    if (isVerbose) {
      console.log(`[mock-smoke] ${args.slice(0, 2).join(' ')}`)
    }
    const result = spawnSync(spec.command, fullArgs, {
      cwd: WEB_ROOT,
      encoding: 'utf8',
      env: process.env,
      maxBuffer: COMMAND_BUFFER_SIZE,
      timeout: timeoutMs,
    })

    const stdout = result.stdout || ''
    const stderr = result.stderr || ''
    if (result.error) {
      const timedOut = result.error && 'code' in result.error && result.error.code === 'ETIMEDOUT'
      if (runOptions.allowFailure) {
        return { stdout, stderr, status: result.status ?? 1 }
      }
      throw new Error(
        [
          timedOut
            ? `Command timed out after ${timeoutMs}ms: ${spec.command} ${fullArgs.join(' ')}`
            : `Command failed: ${spec.command} ${fullArgs.join(' ')}`,
          stdout.trim(),
          stderr.trim(),
        ]
          .filter(Boolean)
          .join('\n'),
      )
    }

    if (result.status !== 0 && !runOptions.allowFailure) {
      throw new Error(
        [`Command failed: ${spec.command} ${fullArgs.join(' ')}`, stdout.trim(), stderr.trim()]
          .filter(Boolean)
          .join('\n'),
      )
    }

    return { stdout, stderr, status: result.status ?? 0 }
  }

  function routeResponse(
    playwrightCli: CommandSpec,
    pattern: string,
    body: string,
    options: { contentType?: string; status?: number } = {},
  ) {
    if (Buffer.byteLength(body, 'utf8') > ROUTE_BODY_ARG_LIMIT_BYTES) {
      routeLargeResponse(playwrightCli, pattern, body, options)
      return
    }

    runCommand(playwrightCli, [
      'route',
      pattern,
      '--status',
      String(options.status ?? 200),
      '--content-type',
      options.contentType ?? 'text/plain',
      '--body',
      body,
    ])
  }

  function routeLargeResponse(
    playwrightCli: CommandSpec,
    pattern: string,
    body: string,
    options: { contentType?: string; status?: number },
  ) {
    if (!routeBodyServerBaseUrl) {
      throw new Error('Large route bodies require startDevServer() before routeResponse()')
    }

    routeBodyId += 1
    const bodyId = String(routeBodyId)
    const bodyUrl = `${routeBodyServerBaseUrl}/route-body/${encodeURIComponent(bodyId)}`
    if (!routeBodyDir) {
      throw new Error('Route body directory is unavailable')
    }
    writeFileSync(path.join(routeBodyDir, `${bodyId}.txt`), body, 'utf8')

    const result = runCommand(playwrightCli, [
      'run-code',
      `await page.route(${JSON.stringify(pattern)}, async (route) => {
    const response = await page.context().request.get(${JSON.stringify(bodyUrl)})
    if (!response.ok()) {
      throw new Error('Route body sidecar failed with status ' + response.status())
    }
    const body = await response.text()
    await route.fulfill({
      status: ${JSON.stringify(options.status ?? 200)},
      contentType: ${JSON.stringify(options.contentType ?? 'text/plain')},
      body,
    })
  })`,
    ])
    if (result.stdout.includes('### Error')) {
      throw new Error(result.stdout)
    }
  }

  function routeJson(playwrightCli: CommandSpec, pattern: string, body: unknown, status = 200) {
    routeResponse(playwrightCli, pattern, JSON.stringify(body), {
      contentType: 'application/json',
      status,
    })
  }

  async function stopDevServer() {
    const bodyServer = routeBodyServer
    const bodyDir = routeBodyDir
    routeBodyServer = null
    routeBodyServerBaseUrl = null
    routeBodyDir = null
    routeBodyId = 0
    if (bodyServer) {
      if (isProcessRunning(bodyServer)) {
        bodyServer.kill('SIGTERM')
        await Promise.race([
          new Promise<void>((resolve) => bodyServer.once('exit', () => resolve())),
          delay(1000),
        ])
      }
      if (isProcessRunning(bodyServer)) {
        bodyServer.kill('SIGKILL')
      }
      bodyServer.stdout.destroy()
      bodyServer.stderr.destroy()
      bodyServer.unref()
    }
    if (bodyDir) {
      rmSync(bodyDir, { recursive: true, force: true })
    }

    const server = devServer
    if (!server || !isProcessRunning(server)) return
    signalDevServer(server, 'SIGINT')
    await Promise.race([
      new Promise<void>((resolve) => server.once('exit', () => resolve())),
      delay(3000),
    ])
    if (isProcessRunning(server)) {
      signalDevServer(server, 'SIGTERM')
      await Promise.race([
        new Promise<void>((resolve) => server.once('exit', () => resolve())),
        delay(1000),
      ])
    }
    if (isProcessRunning(server)) {
      signalDevServer(server, 'SIGKILL')
    }
    server.stdout.destroy()
    server.stderr.destroy()
    server.unref()
  }

  return {
    get baseUrl() {
      return baseUrl()
    },
    get devServerLogs() {
      return devServerLogs
    },
    isVerbose,
    resolvePlaywrightCli,
    routeResponse,
    routeJson,
    runCommand,
    startDevServer,
    stopDevServer,
  }
}
