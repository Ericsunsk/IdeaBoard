import { spawn, type ChildProcess } from 'node:child_process'
import { createServer } from 'node:net'
import { join } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import type { Page } from 'playwright'

const WEB_ROOT = join(import.meta.dirname, '..')
const DEFAULT_HOST = '127.0.0.1'
const DEFAULT_PORT = 5173
const MAX_PORT_PROBES = 20
const DEV_SERVER_READY_TIMEOUT_MS = 45_000
const DEV_SERVER_POLL_INTERVAL_MS = 500
const DEV_SERVER_STOP_TIMEOUT_MS = 5_000

export type SmokeConfig = {
  host: string
  port: number
  baseUrlOverride: string | null
  apiBaseUrl: string | null
  verbose: boolean
}

/** Backend-less local lanes use explicit anonymous reads; reused servers stay live. */
export async function installAnonymousSmokeApi(
  page: Page,
  config: SmokeConfig,
  baseUrl: string,
  failures: string[],
): Promise<void> {
  if (config.baseUrlOverride) return
  const origin = new URL(baseUrl).origin
  await page.route(
    (url) => url.pathname.startsWith('/api/'),
    async (route) => {
      const request = route.request()
      const url = new URL(request.url())
      const fixtures: Record<string, { status: number; body: unknown }> = {
        '/api/auth/get-session': { status: 200, body: null },
        '/api/v1/user': {
          status: 401,
          body: { error: { code: 'unauthorized', message: 'Authentication required' } },
        },
        '/api/settings/branding': {
          status: 200,
          body: { data: { project_name: 'IdeaBoard', logo_url: '' } },
        },
        '/api/auth/capabilities': {
          status: 200,
          body: { data: { social: {}, providers: [] } },
        },
        '/api/v1/support-channels': { status: 200, body: { data: { channels: [] } } },
        '/api/search': {
          status: 200,
          body: { data: { query: url.searchParams.get('q') ?? '', results: [], has_more: false } },
        },
      }
      const fixture = Object.hasOwn(fixtures, url.pathname) ? fixtures[url.pathname] : undefined
      if (url.origin !== origin || request.method() !== 'GET' || !fixture) {
        // Never let a new API dependency or accidental write reach a real backend.
        failures.push(`unexpected anonymous smoke API request: ${request.method()} ${url.pathname}`)
        await route.abort('blockedbyclient')
        return
      }
      await route.fulfill({
        status: fixture.status,
        contentType: 'application/json',
        body: JSON.stringify(fixture.body),
      })
    },
  )
}

export function readSmokeConfig(): SmokeConfig {
  const rawPort = process.env.USER_ROUTE_SMOKE_PORT
  const port = rawPort ? Number(rawPort) : DEFAULT_PORT
  if (!Number.isInteger(port) || port <= 0 || port > 65_535) {
    throw new Error(`Invalid USER_ROUTE_SMOKE_PORT: ${rawPort}`)
  }
  return {
    host: (process.env.USER_ROUTE_SMOKE_HOST ?? '').trim() || DEFAULT_HOST,
    port,
    baseUrlOverride: normalizeBaseUrl(process.env.USER_ROUTE_SMOKE_BASE_URL),
    apiBaseUrl: normalizeBaseUrl(process.env.USER_ROUTE_SMOKE_API_BASE_URL),
    verbose: /^(1|true|yes)$/i.test(process.env.USER_ROUTE_SMOKE_VERBOSE ?? ''),
  }
}

function normalizeBaseUrl(value: string | undefined): string | null {
  const trimmed = (value ?? '').trim().replace(/\/+$/, '')
  return trimmed ? trimmed : null
}

async function isPortFree(host: string, port: number): Promise<boolean> {
  return await new Promise((resolve) => {
    const server = createServer()
    server.once('error', () => resolve(false))
    server.once('listening', () => server.close(() => resolve(true)))
    server.listen(port, host)
  })
}

export async function resolveSmokePort(host: string, preferred: number): Promise<number> {
  for (let candidate = preferred; candidate < preferred + MAX_PORT_PROBES; candidate += 1) {
    if (await isPortFree(host, candidate)) return candidate
  }
  throw new Error(`No free port for smoke near ${preferred} on ${host}`)
}

export function spawnSmokeDevServer(
  host: string,
  port: number,
): { child: ChildProcess; logs: string[] } {
  const logs: string[] = []
  const child = spawn(
    process.execPath,
    [
      join(WEB_ROOT, 'node_modules/vite/bin/vite.js'),
      'dev',
      '--host',
      host,
      '--port',
      String(port),
      '--strictPort',
    ],
    { cwd: WEB_ROOT, stdio: ['ignore', 'pipe', 'pipe'] },
  )
  const recordChunk = (chunk: Buffer, streamName: string) => {
    for (const line of chunk.toString('utf8').split(/\r?\n/)) {
      if (!line) continue
      logs.push(`[vite:${streamName}] ${line}`)
      if (logs.length > 80) logs.splice(0, logs.length - 80)
    }
  }
  child.stdout?.on('data', (chunk: Buffer) => recordChunk(chunk, 'stdout'))
  child.stderr?.on('data', (chunk: Buffer) => recordChunk(chunk, 'stderr'))
  return { child, logs }
}

export async function waitForSmokeDevReady(baseUrl: string, child: ChildProcess): Promise<void> {
  const deadline = Date.now() + DEV_SERVER_READY_TIMEOUT_MS
  for (;;) {
    if (child.exitCode != null || child.signalCode != null) {
      throw new Error(`Vite dev server exited before becoming ready (code=${child.exitCode})`)
    }
    try {
      const response = await fetch(`${baseUrl}/`, { signal: AbortSignal.timeout(2_000) })
      if (response.status < 500) {
        await response.arrayBuffer().catch(() => undefined)
        return
      }
    } catch {
      // Server is still starting; keep polling until the deadline.
    }
    if (Date.now() >= deadline) {
      throw new Error(`Vite dev server did not become ready at ${baseUrl} within 45s`)
    }
    await delay(DEV_SERVER_POLL_INTERVAL_MS)
  }
}

export async function stopSmokeDevServer(child: ChildProcess): Promise<void> {
  if (child.exitCode != null || child.signalCode != null) {
    child.stdout?.destroy()
    child.stderr?.destroy()
    return
  }
  // Spawn Vite directly, then wait for both its exit and log pipes to close.
  // Killing an npm wrapper leaves Vite alive with inherited pipes on Linux.
  await new Promise<void>((resolve) => {
    const timer = setTimeout(() => child.kill('SIGKILL'), DEV_SERVER_STOP_TIMEOUT_MS)
    child.once('close', () => {
      clearTimeout(timer)
      child.stdout?.destroy()
      child.stderr?.destroy()
      resolve()
    })
    child.kill('SIGTERM')
  })
}

export function logSmokeVerbose(config: SmokeConfig, message: string): void {
  if (config.verbose) console.log(message)
}
