import { spawn, type ChildProcess } from 'node:child_process'
import { createServer } from 'node:net'
import { join } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'

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
    'npx',
    ['vite', 'dev', '--host', host, '--port', String(port), '--strictPort'],
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
  if (child.exitCode != null || child.signalCode != null) return
  child.kill('SIGTERM')
  const exited = await Promise.race([
    new Promise<boolean>((resolve) => {
      child.once('exit', () => resolve(true))
    }),
    delay(DEV_SERVER_STOP_TIMEOUT_MS).then(() => false),
  ])
  if (!exited && (child.exitCode == null || child.signalCode == null)) child.kill('SIGKILL')
}

export function logSmokeVerbose(config: SmokeConfig, message: string): void {
  if (config.verbose) console.log(message)
}
