import { spawn, spawnSync, type ChildProcessByStdio } from 'node:child_process'
import { existsSync } from 'node:fs'
import { createServer } from 'node:net'
import { homedir } from 'node:os'
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
const COMMAND_BUFFER_SIZE = 1024 * 1024 * 20

function readPortEnv(rawValue: string | undefined, fallback: number, envName: string): number {
  if (!rawValue) return fallback
  const port = Number(rawValue)
  if (Number.isInteger(port) && port > 0 && port <= 65_535) return port
  throw new Error(`Invalid ${envName}: ${rawValue}`)
}

function readBooleanEnv(rawValue: string | undefined): boolean {
  return /^(1|true|yes)$/i.test(rawValue || '')
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
      const server = createServer()
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

  async function waitForDevServer() {
    const deadline = Date.now() + DEV_SERVER_TIMEOUT_MS
    while (Date.now() < deadline) {
      if (devServer?.exitCode != null) {
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
  }

  function runCommand(spec: CommandSpec, args: string[], runOptions: RunCommandOptions = {}) {
    const fullArgs = [...spec.args, '--session', sessionName, ...args]
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

  function routeJson(playwrightCli: CommandSpec, pattern: string, body: unknown, status = 200) {
    routeResponse(playwrightCli, pattern, JSON.stringify(body), {
      contentType: 'application/json',
      status,
    })
  }

  async function stopDevServer() {
    const server = devServer
    if (!server || server.exitCode != null) return
    server.kill('SIGINT')
    await Promise.race([
      new Promise<void>((resolve) => server.once('exit', () => resolve())),
      delay(3000),
    ])
    if (server.exitCode == null) {
      server.kill('SIGTERM')
      await Promise.race([
        new Promise<void>((resolve) => server.once('exit', () => resolve())),
        delay(1000),
      ])
    }
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
