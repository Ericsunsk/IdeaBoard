/**
 * User route smoke (post-#809 hard-cut replacement).
 *
 * Anonymous-surface only: starts its own Vite dev server (or reuses
 * USER_ROUTE_SMOKE_BASE_URL), drives the public SPA routes with Playwright
 * Chromium, and asserts the auth guard redirects for gated routes. The
 * web-smoke CI lane runs no backend and no credentials exist in this repo,
 * so authenticated flows are an explicit documented gap (see below).
 *
 * Live backend probes (GET /api/health, /api/livez, /api/readyz,
 * GET /api/settings/branding) run only when USER_ROUTE_SMOKE_API_BASE_URL
 * points at a reachable API; otherwise they are reported as skipped.
 *
 * Hermetic: only GET reads, no mutations, no test users, no DB writes.
 *
 * Documented gap: logged-in user/admin journeys (boards, canvas, console)
 * cannot run here because the lane provides no backend, no auth service,
 * and no seeded session. Those flows stay covered by unit/component
 * contract specs; extend this script (never with in-repo credentials) once
 * the lane can mint a session.
 *
 * Env:
 *   USER_ROUTE_SMOKE_HOST (default 127.0.0.1)
 *   USER_ROUTE_SMOKE_PORT (default 5173; auto-bumps while occupied)
 *   USER_ROUTE_SMOKE_BASE_URL (optional: reuse an already-running web server)
 *   USER_ROUTE_SMOKE_API_BASE_URL (optional: enable live backend probes)
 *   USER_ROUTE_SMOKE_VERBOSE (1/true/yes enables per-check logging)
 */
import { chromium, type Page } from 'playwright'
import {
  logSmokeVerbose,
  readSmokeConfig,
  resolveSmokePort,
  spawnSmokeDevServer,
  stopSmokeDevServer,
  waitForSmokeDevReady,
  type SmokeConfig,
} from './smokeSupport'
const NAVIGATION_TIMEOUT_MS = 15_000
const TEXT_TIMEOUT_MS = 10_000
const API_TIMEOUT_MS = 10_000

type PublicPageCheck = {
  label: string
  path: string
  expectedTexts: readonly string[]
}

type ApiProbeResult = {
  label: string
  status: 'passed' | 'failed'
  detail: string
}

const PUBLIC_PAGES: readonly PublicPageCheck[] = [
  {
    label: 'home',
    path: '/',
    expectedTexts: ['AI CANVAS FOR VISUAL DESIGNERS', 'Get started'],
  },
  { label: 'terms', path: '/terms', expectedTexts: ['Terms of Service'] },
  { label: 'privacy', path: '/privacy-policy', expectedTexts: ['Privacy Policy'] },
]

async function checkPublicPage(page: Page, baseUrl: string, check: PublicPageCheck): Promise<void> {
  const response = await page.goto(`${baseUrl}${check.path}`, {
    waitUntil: 'domcontentloaded',
    timeout: NAVIGATION_TIMEOUT_MS,
  })
  if (!response || !response.ok()) {
    throw new Error(`${check.label} (${check.path}) returned HTTP ${response?.status() ?? 'none'}`)
  }
  const pathname = new URL(page.url()).pathname
  if (pathname !== check.path) {
    throw new Error(`${check.label} redirected unexpectedly to ${page.url()}`)
  }
  for (const text of check.expectedTexts) {
    await page
      .getByText(text)
      .first()
      .waitFor({ state: 'visible', timeout: TEXT_TIMEOUT_MS })
      .catch(() => {
        throw new Error(`${check.label} (${check.path}) missing visible text: ${text}`)
      })
  }
}

async function checkSearchPage(page: Page, baseUrl: string): Promise<void> {
  const response = await page.goto(`${baseUrl}/user/search`, {
    waitUntil: 'domcontentloaded',
    timeout: NAVIGATION_TIMEOUT_MS,
  })
  if (!response || !response.ok()) {
    throw new Error(`search (/user/search) returned HTTP ${response?.status() ?? 'none'}`)
  }
  // /user/search is meta.public: it must render for anonymous visitors
  // instead of bouncing to the login page.
  await page.waitForURL('**/user/search**', { timeout: TEXT_TIMEOUT_MS })
  await page
    .locator('form input[type="text"]')
    .first()
    .waitFor({ state: 'visible', timeout: TEXT_TIMEOUT_MS })
}

async function checkAuthRedirect(
  page: Page,
  baseUrl: string,
  path: string,
  loginPath: string,
): Promise<void> {
  await page.goto(`${baseUrl}${path}`, {
    waitUntil: 'domcontentloaded',
    timeout: NAVIGATION_TIMEOUT_MS,
  })
  await page.waitForURL(`**${loginPath}**`, { timeout: TEXT_TIMEOUT_MS })
  const url = new URL(page.url())
  if (url.pathname !== loginPath || !url.searchParams.has('redirect')) {
    throw new Error(`${path} should redirect to ${loginPath}?redirect=..., got ${page.url()}`)
  }
}

type ApiEnvelope = {
  code?: unknown
  data?: unknown
  msg?: unknown
}

async function fetchJson(url: string): Promise<{ status: number; body: ApiEnvelope | null }> {
  const response = await fetch(url, {
    method: 'GET',
    signal: AbortSignal.timeout(API_TIMEOUT_MS),
  })
  let body: ApiEnvelope | null
  try {
    body = (await response.json()) as ApiEnvelope
  } catch {
    body = null
  }
  return { status: response.status, body }
}

function hasOkDataStatus(body: ApiEnvelope | null): boolean {
  if (!body || typeof body !== 'object') return false
  const data = body.data as { status?: unknown } | null | undefined
  return (
    body.code === 0 && (typeof data === 'object' && data !== null ? data.status === 'ok' : false)
  )
}

async function probeLiveBackend(apiBaseUrl: string): Promise<ApiProbeResult[]> {
  const results: ApiProbeResult[] = []
  for (const path of ['/api/health', '/api/livez']) {
    try {
      const { status, body } = await fetchJson(`${apiBaseUrl}${path}`)
      results.push({
        label: path,
        status: status === 200 && hasOkDataStatus(body) ? 'passed' : 'failed',
        detail:
          status === 200 && hasOkDataStatus(body)
            ? '200 {code:0,data:{status:ok}}'
            : `unexpected: HTTP ${status}`,
      })
    } catch (error) {
      results.push({
        label: path,
        status: 'failed',
        detail: error instanceof Error ? error.message : String(error),
      })
    }
  }
  try {
    const { status, body } = await fetchJson(`${apiBaseUrl}/api/readyz`)
    const ready = status === 200 ? hasOkDataStatus(body) : status === 503 && body !== null
    results.push({
      label: '/api/readyz',
      status: ready ? 'passed' : 'failed',
      detail: ready ? `HTTP ${status} (dependency-gated)` : `unexpected: HTTP ${status}`,
    })
  } catch (error) {
    results.push({
      label: '/api/readyz',
      status: 'failed',
      detail: error instanceof Error ? error.message : String(error),
    })
  }
  try {
    const { status, body } = await fetchJson(`${apiBaseUrl}/api/settings/branding`)
    const data = body?.data as { project_name?: unknown } | null | undefined
    const ok = status === 200 && body?.code === 0 && typeof data?.project_name === 'string'
    results.push({
      label: '/api/settings/branding',
      status: ok ? 'passed' : 'failed',
      detail: ok ? '200 anonymous branding read' : `unexpected: HTTP ${status}`,
    })
  } catch (error) {
    results.push({
      label: '/api/settings/branding',
      status: 'failed',
      detail: error instanceof Error ? error.message : String(error),
    })
  }
  for (const result of results) {
    if (result.status !== 'passed') {
      throw new Error(`live API probe ${result.label} failed: ${result.detail}`)
    }
  }
  return results
}

async function runBrowserChecks(config: SmokeConfig, baseUrl: string): Promise<string[]> {
  const failures: string[] = []
  const consoleErrors: string[] = []
  const pageErrors: string[] = []
  // Anonymous-lane benign noise: every visited page fires the session probe
  // GET /api/v1/user, which correctly answers 401 for anonymous visitors
  // (live backend) or fails to connect when the lane runs backend-less (CI).
  // Chromium logs both at browser level regardless of app handling, so the
  // script correlates them with the observed probe requests (count-matched)
  // instead of failing the lane. Any 401/console error WITHOUT a matching
  // observed probe request still fails.
  const SESSION_PROBE_PATH = '/api/v1/user'
  const SESSION_PROBE_NOISE =
    /Failed to load resource.*(401|ERR_CONNECTION_REFUSED|ERR_PROXY_CONNECTION_FAILED)/
  const probePathOf = (url: string): string | null => {
    try {
      return new URL(url).pathname
    } catch {
      return null
    }
  }
  let observedProbeFailures = 0
  const browser = await chromium.launch()
  try {
    const page = await browser.newPage()
    page.on('console', (message) => {
      if (message.type() === 'error') consoleErrors.push(message.text())
    })
    page.on('response', (response) => {
      if (
        probePathOf(response.url()) === SESSION_PROBE_PATH &&
        response.status() === 401
      ) {
        observedProbeFailures += 1
      }
    })
    page.on('requestfailed', (request) => {
      if (probePathOf(request.url()) === SESSION_PROBE_PATH) {
        observedProbeFailures += 1
      }
    })
    page.on('pageerror', (error) => pageErrors.push(error.message))
    const checks: Array<() => Promise<void>> = [
      ...PUBLIC_PAGES.map(
        (check) => () => checkPublicPage(page, baseUrl, check).then(() => undefined),
      ),
      () => checkSearchPage(page, baseUrl),
      () => checkAuthRedirect(page, baseUrl, '/user/boards', '/user/login'),
      () => checkAuthRedirect(page, baseUrl, '/admin/users', '/admin/login'),
    ]
    const labels = [
      ...PUBLIC_PAGES.map((check) => `page ${check.path}`),
      'page /user/search',
      'redirect /user/boards',
      'redirect /admin/users',
    ]
    for (let index = 0; index < checks.length; index += 1) {
      const run = checks[index]
      const label = labels[index]
      if (!run || !label) continue
      try {
        await run()
        logSmokeVerbose(config, `ok ${label}`)
      } catch (error) {
        failures.push(`${label}: ${error instanceof Error ? error.message : String(error)}`)
      }
    }
    await page.close().catch(() => undefined)
  } finally {
    await browser.close().catch(() => undefined)
  }
  for (const message of pageErrors) failures.push(`uncaught page error: ${message}`)
  let allowedProbeNoise = 0
  for (const message of consoleErrors) {
    if (
      SESSION_PROBE_NOISE.test(message) &&
      allowedProbeNoise < observedProbeFailures
    ) {
      allowedProbeNoise += 1
      continue
    }
    failures.push(`console error: ${message}`)
  }
  return failures
}

async function main(): Promise<void> {
  const config = readSmokeConfig()
  const failures: string[] = []
  let probeResults: ApiProbeResult[] = []
  let baseUrl = config.baseUrlOverride
  let devServer: ReturnType<typeof spawnSmokeDevServer>['child'] | null = null
  let devLogs: string[] = []
  try {
    if (!baseUrl) {
      const port = await resolveSmokePort(config.host, config.port)
      const server = spawnSmokeDevServer(config.host, port)
      devServer = server.child
      devLogs = server.logs
      baseUrl = `http://${config.host}:${port}`
      await waitForSmokeDevReady(baseUrl, devServer)
      logSmokeVerbose(config, `dev server ready at ${baseUrl}`)
    } else {
      logSmokeVerbose(config, `reusing web server at ${baseUrl}`)
    }
    failures.push(...(await runBrowserChecks(config, baseUrl)))
    if (config.apiBaseUrl) {
      probeResults = await probeLiveBackend(config.apiBaseUrl)
      logSmokeVerbose(config, `live API probes passed against ${config.apiBaseUrl}`)
    }
  } catch (error) {
    failures.push(error instanceof Error ? error.message : String(error))
  } finally {
    if (devServer) await stopSmokeDevServer(devServer).catch(() => undefined)
  }
  if (failures.length > 0) {
    console.error(`user route smoke failed (${failures.length}):\n${failures.join('\n')}`)
    if (devLogs.length > 0) console.error(devLogs.join('\n'))
    process.exitCode = 1
    return
  }
  const probeSummary =
    probeResults.length > 0
      ? `, ${probeResults.length} live API probes`
      : ', live API probes skipped (USER_ROUTE_SMOKE_API_BASE_URL unset)'
  console.log(
    `user route smoke passed: ${PUBLIC_PAGES.length + 1} public pages, 2 auth redirects${probeSummary}`,
  )
}

void main()
