/**
 * Route visual captures (P7 quota final/routes-browser-visual).
 *
 * Anonymous-surface only: starts its own Vite dev server (or reuses
 * USER_ROUTE_SMOKE_BASE_URL), drives seven navigable SPA routes with Playwright
 * Chromium headless, and writes one desktop + one mobile PNG per route plus a
 * `manifest.json` that records the final URL (auth-guard redirects included),
 * API-fixture warnings, and console/page errors per capture.
 *
 * Surface mapping (requested surface -> captured route):
 *   auth      -> /user/login   (guest route, renders directly)
 *   boards    -> /user/boards  (WorkspaceLayout shell and BoardsPage gallery;
 *                               anonymous visitors are redirected to /user/login?redirect=...,
 *                               which the manifest records instead of faking a session)
 *   canvas    -> /user/canvas  (requiresAuth; anonymous redirect is recorded)
 *   library   -> /user/search  (nearest navigable discovery surface)
 *   account   -> /user/account (requiresAuth; anonymous redirect is recorded)
 *   admin     -> /admin/users  (requiresAuth; anonymous redirect is recorded)
 *
 * Documented gap: the library has no dedicated route — LibraryFloatingIsland
 * and LibraryAssetGallery render as embedded panels inside the workspace and
 * canvas flows, and gated routes cannot render their authenticated state here
 * because the lane provides no backend, no auth service, and no seeded
 * session (same constraint as userRouteSmoke.ts). Those flows stay covered by
 * unit/component contract specs; extend this script (never with in-repo
 * credentials) once the lane can mint a session.
 *
 * Hermetic: only GET reads, no mutations, no test users, no DB writes.
 *
 * Env (server selection shared with userRouteSmoke.ts):
 *   USER_ROUTE_SMOKE_HOST (default 127.0.0.1)
 *   USER_ROUTE_SMOKE_PORT (default 5173; auto-bumps while occupied)
 *   USER_ROUTE_SMOKE_BASE_URL (optional: reuse an already-running web server)
 *   ROUTES_VISUAL_OUT_DIR (default <web>/node_modules/.cache/route-screenshots)
 *   ROUTES_VISUAL_VERBOSE (1/true/yes enables per-capture logging)
 *
 * Wiring (package scripts are defined in apps/web/package.json):
 *   "test:routes-visual": "tsx scripts/captureRouteScreenshots.ts"
 *   "test:routes-visual:check": "tsx scripts/captureRouteScreenshots.ts --check"
 */
import { mkdir, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { chromium, type Page } from 'playwright'
import {
  installAnonymousSmokeApi,
  readSmokeConfig,
  resolveSmokePort,
  spawnSmokeDevServer,
  stopSmokeDevServer,
  waitForSmokeDevReady,
  type SmokeConfig,
} from './smokeSupport'

const NAVIGATION_TIMEOUT_MS = 15_000
const SETTLE_DELAY_MS = 1_500

const WEB_ROOT = join(import.meta.dirname, '..')
const DEFAULT_OUT_DIR = join(WEB_ROOT, 'node_modules', '.cache', 'route-screenshots')
type ViewportName = 'desktop' | 'mobile'

type Viewport = {
  name: ViewportName
  width: number
  height: number
}

type RouteCapture = {
  label: string
  path: string
  anonymousGap?: string
}

const ROUTE_CAPTURES: readonly RouteCapture[] = [
  { label: 'home', path: '/' },
  { label: 'auth', path: '/user/login' },
  { label: 'boards', path: '/user/boards' },
  { label: 'canvas', path: '/user/canvas' },
  { label: 'search', path: '/user/search' },
  {
    label: 'account',
    path: '/user/account',
    anonymousGap:
      'Authenticated account state is unavailable in the anonymous smoke lane; the manifest records the login redirect.',
  },
  {
    label: 'admin',
    path: '/admin/users',
    anonymousGap:
      'Authenticated admin state is unavailable in the anonymous smoke lane; the manifest records the admin login redirect.',
  },
]

const VIEWPORTS: readonly Viewport[] = [
  { name: 'desktop', width: 1440, height: 900 },
  { name: 'mobile', width: 390, height: 844 },
]

type CaptureRecord = {
  label: string
  path: string
  viewport: ViewportName
  file: string
  finalUrl: string
  redirected: boolean
  apiWarnings: number
  apiWarningDetails: string[]
  consoleErrors: string[]
  pageErrors: string[]
  status: 'passed' | 'failed'
  detail: string
}

function readOutDir(): string {
  const override = (process.env.ROUTES_VISUAL_OUT_DIR ?? '').trim().replace(/\/+$/, '')
  return override ? override : DEFAULT_OUT_DIR
}

function isVerbose(): boolean {
  return /^(1|true|yes)$/i.test(process.env.ROUTES_VISUAL_VERBOSE ?? '')
}

function logVerbose(message: string): void {
  if (isVerbose()) console.log(message)
}

function selfCheck(outDir: string): string[] {
  const failures: string[] = []
  if (ROUTE_CAPTURES.length !== 7) {
    failures.push(`route table must capture exactly 7 routes, got ${ROUTE_CAPTURES.length}`)
  }
  const labels = new Set<string>()
  for (const route of ROUTE_CAPTURES) {
    if (!route.label) failures.push('route table has an empty label')
    if (labels.has(route.label)) failures.push(`duplicate route label: ${route.label}`)
    labels.add(route.label)
    if (!route.path.startsWith('/')) failures.push(`route ${route.label} path must start with /`)
  }
  const viewportNames = new Set<string>()
  for (const viewport of VIEWPORTS) {
    if (viewport.width <= 0 || viewport.height <= 0) {
      failures.push(`viewport ${viewport.name} must have positive dimensions`)
    }
    if (viewportNames.has(viewport.name)) failures.push(`duplicate viewport: ${viewport.name}`)
    viewportNames.add(viewport.name)
  }
  if (!outDir) failures.push('output directory must not be empty')
  return failures
}

function captureDiagnostics(
  apiWarnings: string[],
  consoleErrors: string[],
  pageErrors: string[],
): string[] {
  return [
    ...apiWarnings.map((message) => `API warning: ${message}`),
    ...consoleErrors.map((message) => `console error: ${message}`),
    ...pageErrors.map((message) => `page error: ${message}`),
  ]
}

async function captureOne(
  page: Page,
  baseUrl: string,
  route: RouteCapture,
  viewport: Viewport,
  outDir: string,
  apiWarnings: string[],
): Promise<CaptureRecord> {
  const fileName = `${route.label}-${viewport.name}.png`
  const filePath = join(outDir, fileName)
  const consoleErrors: string[] = []
  const pageErrors: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error') consoleErrors.push(message.text())
  })
  page.on('pageerror', (error) => pageErrors.push(error.message))
  try {
    const response = await page.goto(`${baseUrl}${route.path}`, {
      waitUntil: 'domcontentloaded',
      timeout: NAVIGATION_TIMEOUT_MS,
    })
    if (!response || !response.ok()) {
      throw new Error(`returned HTTP ${response?.status() ?? 'none'}`)
    }
    await delay(SETTLE_DELAY_MS)
    await page.screenshot({ path: filePath })
    const finalUrl = page.url()
    const finalPath = new URL(finalUrl).pathname
    const expectedPath = new URL(`${baseUrl}${route.path}`).pathname
    const diagnostics = captureDiagnostics(apiWarnings, consoleErrors, pageErrors)
    if (diagnostics.length > 0) {
      return {
        label: route.label,
        path: route.path,
        viewport: viewport.name,
        file: fileName,
        finalUrl,
        redirected: finalPath !== expectedPath,
        apiWarnings: apiWarnings.length,
        apiWarningDetails: [...apiWarnings],
        consoleErrors,
        pageErrors,
        status: 'failed',
        detail: diagnostics.join('; '),
      }
    }
    const detail =
      route.anonymousGap && finalPath !== expectedPath
        ? `saved ${fileName}; accepted anonymous gap: ${route.anonymousGap}`
        : `saved ${fileName}`
    logVerbose(`ok ${route.label} ${viewport.name} -> ${fileName} (final ${finalUrl})`)
    return {
      label: route.label,
      path: route.path,
      viewport: viewport.name,
      file: fileName,
      finalUrl,
      redirected: finalPath !== expectedPath,
      apiWarnings: apiWarnings.length,
      apiWarningDetails: [...apiWarnings],
      consoleErrors,
      pageErrors,
      status: 'passed',
      detail,
    }
  } catch (error) {
    const diagnostics = captureDiagnostics(apiWarnings, consoleErrors, pageErrors)
    const errorDetail = error instanceof Error ? error.message : String(error)
    return {
      label: route.label,
      path: route.path,
      viewport: viewport.name,
      file: fileName,
      finalUrl: page.url(),
      redirected: false,
      apiWarnings: apiWarnings.length,
      apiWarningDetails: [...apiWarnings],
      consoleErrors,
      pageErrors,
      status: 'failed',
      detail: [errorDetail, ...diagnostics].join('; '),
    }
  }
}

async function runCaptures(
  config: SmokeConfig,
  baseUrl: string,
  outDir: string,
): Promise<CaptureRecord[]> {
  const records: CaptureRecord[] = []
  const browser = await chromium.launch()
  try {
    for (const route of ROUTE_CAPTURES) {
      for (const viewport of VIEWPORTS) {
        const context = await browser.newContext({
          viewport: { width: viewport.width, height: viewport.height },
        })
        try {
          const page = await context.newPage()
          const apiWarnings: string[] = []
          await installAnonymousSmokeApi(page, config, baseUrl, apiWarnings)
          records.push(await captureOne(page, baseUrl, route, viewport, outDir, apiWarnings))
          await page.close().catch(() => undefined)
        } finally {
          await context.close().catch(() => undefined)
        }
      }
    }
  } finally {
    await browser.close().catch(() => undefined)
  }
  return records
}

async function main(): Promise<void> {
  const outDir = readOutDir()
  const checkFailures = selfCheck(outDir)
  if (process.argv.includes('--check')) {
    if (checkFailures.length > 0) {
      console.error(`routes-visual self-check failed:\n${checkFailures.join('\n')}`)
      process.exitCode = 1
      return
    }
    console.log(
      `routes-visual self-check passed: ${ROUTE_CAPTURES.length} routes x ${VIEWPORTS.length} viewports -> ${outDir}`,
    )
    return
  }
  if (checkFailures.length > 0) {
    console.error(`routes-visual self-check failed:\n${checkFailures.join('\n')}`)
    process.exitCode = 1
    return
  }
  await mkdir(outDir, { recursive: true })
  const config = readSmokeConfig()
  let baseUrl = config.baseUrlOverride
  let selfStarted = false
  let devServer: ReturnType<typeof spawnSmokeDevServer>['child'] | null = null
  let devLogs: string[] = []
  const failures: string[] = []
  let records: CaptureRecord[] = []
  try {
    if (!baseUrl) {
      const port = await resolveSmokePort(config.host, config.port)
      const server = spawnSmokeDevServer(config.host, port)
      devServer = server.child
      devLogs = server.logs
      baseUrl = `http://${config.host}:${port}`
      await waitForSmokeDevReady(baseUrl, devServer)
      selfStarted = true
      logVerbose(`dev server ready at ${baseUrl}`)
    } else {
      logVerbose(`reusing web server at ${baseUrl}`)
    }
    records = await runCaptures(config, baseUrl, outDir)
    for (const record of records) {
      if (record.status !== 'passed') {
        failures.push(`${record.label} ${record.viewport}: ${record.detail}`)
      }
    }
    const manifest = {
      generatedAt: new Date().toISOString(),
      baseUrl,
      selfStarted,
      outDir,
      viewports: VIEWPORTS.map((viewport) => ({
        name: viewport.name,
        width: viewport.width,
        height: viewport.height,
      })),
      captures: records,
    }
    await writeFile(join(outDir, 'manifest.json'), `${JSON.stringify(manifest, null, 2)}\n`)
  } catch (error) {
    failures.push(error instanceof Error ? error.message : String(error))
  } finally {
    if (devServer) await stopSmokeDevServer(devServer).catch(() => undefined)
  }
  if (failures.length > 0) {
    console.error(`routes-visual failed (${failures.length}):\n${failures.join('\n')}`)
    if (devLogs.length > 0) console.error(devLogs.join('\n'))
    process.exitCode = 1
    return
  }
  console.log(
    `routes-visual passed: ${records.length} captures (${ROUTE_CAPTURES.length} routes x ${VIEWPORTS.length} viewports) -> ${outDir}`,
  )
}

void main()
