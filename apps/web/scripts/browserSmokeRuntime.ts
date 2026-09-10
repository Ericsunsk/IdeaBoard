import { spawn, type ChildProcessByStdio } from 'node:child_process'
import { createServer as createNetServer } from 'node:net'
import type { Readable } from 'node:stream'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'
import { compileFunction } from 'node:vm'
import { chromium, type Browser, type Page } from 'playwright'

type BrowserSmokeRuntimeOptions = {
  hostEnvName: string
  portEnvName: string
  verboseEnvName: string
  defaultPort?: number
}

type RunCommandOptions = {
  allowFailure?: boolean
}

const WEB_ROOT = fileURLToPath(new URL('..', import.meta.url))
const DEV_SERVER_TIMEOUT_MS = 45_000
const USE_PROCESS_GROUP = process.platform !== 'win32'

function readBooleanEnv(rawValue: string | undefined): boolean {
  return /^(1|true|yes)$/i.test(rawValue || '')
}

function readPortEnv(rawValue: string | undefined, fallback: number, envName: string): number {
  if (!rawValue) return fallback
  const port = Number(rawValue)
  if (Number.isInteger(port) && port > 0 && port <= 65_535) return port
  throw new Error(`Invalid ${envName}: ${rawValue}`)
}

function isProcessRunning(child: ChildProcessByStdio<null, Readable, Readable>): boolean {
  return child.exitCode == null && child.signalCode == null
}

export function createBrowserSmokeRuntime(options: BrowserSmokeRuntimeOptions) {
  const host = process.env[options.hostEnvName] || '127.0.0.1'
  const preferredPort = readPortEnv(
    process.env[options.portEnvName],
    options.defaultPort ?? 5173,
    options.portEnvName,
  )
  const isVerbose = readBooleanEnv(process.env[options.verboseEnvName])
  const devServerLogs: string[] = []
  const consoleErrors: string[] = []
  const requests: string[] = []
  let devServer: ChildProcessByStdio<null, Readable, Readable> | null = null
  let browser: Browser | null = null
  let page: Page | null = null
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
      server.once('listening', () => server.close(() => resolve(true)))
      server.listen(port, host)
    })
  }

  async function resolveSmokePort(): Promise<number> {
    if (process.env[options.portEnvName]) return preferredPort
    for (let candidate = preferredPort; candidate < preferredPort + 20; candidate += 1) {
      if (await isPortAvailable(candidate)) return candidate
    }
    throw new Error(`No available port found from ${preferredPort} to ${preferredPort + 19}`)
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

  async function startDevServer() {
    smokePort = await resolveSmokePort()
    const server = spawn(
      process.platform === 'win32' ? 'npm.cmd' : 'npm',
      ['run', 'dev', '--', '--host', host, '--port', String(smokePort), '--strictPort'],
      {
        cwd: WEB_ROOT,
        detached: USE_PROCESS_GROUP,
        env: process.env,
        stdio: ['ignore', 'pipe', 'pipe'],
      },
    )
    devServer = server
    server.stdout.on('data', (chunk: Buffer) => recordDevServerOutput(chunk, 'stdout'))
    server.stderr.on('data', (chunk: Buffer) => recordDevServerOutput(chunk, 'stderr'))
    await waitForDevServer()
  }

  function requirePage(): Page {
    if (!page) throw new Error('Browser smoke page is not started')
    return page
  }

  async function startBrowser() {
    browser = await chromium.launch({ headless: true })
    const context = await browser.newContext({ viewport: { width: 1280, height: 720 } })
    page = await context.newPage()
    page.on('console', (message) => {
      if (message.type() === 'error') consoleErrors.push(message.text())
    })
    page.on('pageerror', (error) => consoleErrors.push(error.message))
    page.on('request', (request) => requests.push(`${request.method()} ${request.url()}`))
  }

  async function runBrowserCode(source: string) {
    let assertion: (page: Page) => Promise<void>
    try {
      assertion = compileFunction(`return (async () => {\n${source}\n})()`, ['page']) as (
        page: Page,
      ) => Promise<void>
    } catch (error) {
      throw new Error(
        `Invalid browser assertion: ${error instanceof Error ? error.message : String(error)}\n${source}`,
        { cause: error },
      )
    }
    await assertion(requirePage())
  }

  async function runCommand(args: string[], runOptions: RunCommandOptions = {}) {
    const command = args[0]
    if (isVerbose) console.log(`[browser-smoke] ${args.slice(0, 2).join(' ')}`)
    try {
      switch (command) {
        case 'open':
        case 'goto':
          await requirePage().goto(args[1] || 'about:blank')
          break
        case 'run-code':
          await runBrowserCode(args[1] || '')
          break
        case 'snapshot':
          return { stdout: `Page URL: ${requirePage().url()}`, stderr: '', status: 0 }
        case 'console':
          return {
            stdout:
              consoleErrors.length === 0
                ? 'Returning 0 messages for level "error"'
                : consoleErrors.join('\n'),
            stderr: '',
            status: 0,
          }
        case 'requests':
          return { stdout: requests.join('\n'), stderr: '', status: 0 }
        case 'close':
          await closeBrowser()
          break
        default:
          throw new Error(`Unsupported browser smoke command: ${command || '<empty>'}`)
      }
      return { stdout: '', stderr: '', status: 0 }
    } catch (error) {
      if (runOptions.allowFailure) {
        return {
          stdout: '',
          stderr: error instanceof Error ? error.message : String(error),
          status: 1,
        }
      }
      throw error
    }
  }

  async function routeResponse(
    pattern: string,
    body: string,
    routeOptions: { contentType?: string; status?: number } = {},
  ) {
    await requirePage().route(pattern, async (route) => {
      await route.fulfill({
        status: routeOptions.status ?? 200,
        contentType: routeOptions.contentType ?? 'text/plain',
        body,
      })
    })
  }

  async function routeJson(pattern: string, body: unknown, status = 200) {
    await routeResponse(pattern, JSON.stringify(body), { contentType: 'application/json', status })
  }

  async function closeBrowser() {
    const activeBrowser = browser
    browser = null
    page = null
    if (activeBrowser) await activeBrowser.close()
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
        // The process group can disappear before the parent npm process exits.
      }
    }
    server.kill(signal)
  }

  async function stopDevServer() {
    const server = devServer
    devServer = null
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
    if (isProcessRunning(server)) signalDevServer(server, 'SIGKILL')
    server.stdout.destroy()
    server.stderr.destroy()
    server.unref()
  }

  return {
    get baseUrl() {
      return baseUrl()
    },
    devServerLogs,
    isVerbose,
    getPage() {
      return requirePage()
    },
    startBrowser,
    runBrowserCode,
    runCommand,
    routeJson,
    routeResponse,
    closeBrowser,
    startDevServer,
    stopDevServer,
  }
}
