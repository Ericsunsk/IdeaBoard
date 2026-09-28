/**
 * Accessibility smoke (post-#809 hard-cut replacement).
 *
 * Anonymous-surface only: starts its own Vite dev server (or reuses
 * USER_ROUTE_SMOKE_BASE_URL), drives the public SPA routes with Playwright
 * Chromium, and asserts a baseline accessibility shape on each page plus the
 * auth-guard redirect targets. The web-smoke CI lane runs no backend and no
 * credentials exist in this repo, so this script performs GET-only reads and
 * never mutates state.
 *
 * No-axe approach: `@axe-core/playwright` is not a dependency (removed as
 * unused), no new packages are installed, and the installed Playwright
 * version no longer ships `page.accessibility`. Structural checks run on a
 * single self-contained DOM evaluate per page: document lang/title, at least one named heading, at least one landmark,
 * zero nameless buttons, zero nameless images. This catches regressions in
 * landmarks, heading structure, button names, and image alt text — it is not
 * a WCAG audit and does not replace axe in richer environments.
 *
 * Hermetic: only GET reads, no mutations, no test users, no DB writes.
 *
 * Documented gap: logged-in user/admin journeys (boards, canvas, console)
 * cannot run here because the lane provides no backend, no auth service,
 * and no seeded session. Those flows stay covered by unit/component
 * contract specs; extend this script (never with in-repo credentials) once
 * the lane can mint a session.
 *
 * Env (shared with userRouteSmoke.ts):
 *   USER_ROUTE_SMOKE_HOST (default 127.0.0.1)
 *   USER_ROUTE_SMOKE_PORT (default 5173; auto-bumps while occupied)
 *   USER_ROUTE_SMOKE_BASE_URL (optional: reuse an already-running web server)
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
const SETTLE_TIMEOUT_MS = 10_000

type PageCheck = {
  label: string
  path: string
}

const PUBLIC_PAGES: readonly PageCheck[] = [
  { label: 'home', path: '/' },
  { label: 'terms', path: '/terms' },
  { label: 'privacy', path: '/privacy-policy' },
  { label: 'search', path: '/user/search' },
]

const AUTH_GUARDS: readonly { path: string; loginPath: string }[] = [
  { path: '/user/boards', loginPath: '/user/login' },
  { path: '/admin/users', loginPath: '/admin/login' },
]

type A11yShapeReport = {
  headings: number
  landmarks: number
  namelessButtons: number
  namelessImages: number
}

const COLLECT_A11Y_SHAPE_SOURCE = `
  (() => {
    const isHidden = (element) => {
      if (!(element instanceof HTMLElement)) return false
      if (element.hidden || element.getAttribute('aria-hidden') === 'true') return true
      const style = getComputedStyle(element)
      return style.display === 'none' || style.visibility === 'hidden'
    }
    const accessibleName = (element) => {
      const labelledBy = element.getAttribute('aria-labelledby')
      if (labelledBy) {
        const text = labelledBy
          .split(/\\s+/)
          .map((id) => document.getElementById(id)?.textContent ?? '')
          .join(' ')
          .trim()
        if (text) return text
      }
      const direct =
        element.getAttribute('aria-label') ??
        (element instanceof HTMLInputElement ? element.value : null) ??
        element.getAttribute('title') ??
        element.textContent ??
        ''
      return direct.trim()
    }
    const visible = (element) => !isHidden(element)
    const headings = Array.from(document.querySelectorAll('h1, h2, h3, h4, h5, h6, [role="heading"]'))
      .filter(visible)
      .filter((element) => accessibleName(element).length > 0).length
    const landmarks = Array.from(
      document.querySelectorAll(
        'main, nav, [role="banner"], [role="complementary"], [role="contentinfo"], [role="main"], [role="navigation"], [role="search"]',
      ),
    ).filter(visible).length
    const namelessButtons = Array.from(
      document.querySelectorAll(
        'button, [role="button"], input[type="button"], input[type="submit"]',
      ),
    )
      .filter(visible)
      .filter((element) => !(element instanceof HTMLInputElement && element.type === 'hidden'))
      .filter((element) => accessibleName(element).length === 0).length
    const namelessImages = Array.from(document.querySelectorAll('img, [role="img"]'))
      .filter(visible)
      .filter((element) => accessibleName(element).length === 0)
      .filter(
        (element) => !(element instanceof HTMLImageElement && element.hasAttribute('alt')),
      ).length
    return { headings, landmarks, namelessButtons, namelessImages }
  })()
`

async function checkAccessibilityShape(page: Page, label: string): Promise<void> {
  const violations: string[] = []
  const lang = await page.evaluate(() => document.documentElement.lang.trim())
  if (!lang) violations.push('document is missing html[lang]')
  const title = await page.evaluate(() => document.title.trim())
  if (!title) violations.push('document is missing a non-empty <title>')
  // Pass the function source as a browser expression. The smoke script runs
  // through esbuild/tsx, whose injected `__name` helper is not available in
  // the page context when a compiled callback is serialized by Playwright.
  const raw = await page.evaluate<A11yShapeReport>(COLLECT_A11Y_SHAPE_SOURCE)
  const report: A11yShapeReport = raw
  if (report.headings === 0) violations.push('no named heading found')
  if (report.landmarks === 0) violations.push('no landmark (main/nav/banner/contentinfo/...) found')
  if (report.namelessButtons > 0) {
    violations.push(`${String(report.namelessButtons)} button(s) without an accessible name`)
  }
  if (report.namelessImages > 0) {
    violations.push(`${String(report.namelessImages)} image(s) without alt text`)
  }
  if (violations.length > 0) {
    throw new Error(`${label}: accessibility shape failed:\n- ${violations.join('\n- ')}`)
  }
}

async function checkPublicPage(page: Page, baseUrl: string, check: PageCheck): Promise<void> {
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
  await page
    .locator('h1, h2, [role="heading"]')
    .first()
    .waitFor({ state: 'attached', timeout: SETTLE_TIMEOUT_MS })
  await checkAccessibilityShape(page, check.label)
}

async function checkAuthGuard(
  page: Page,
  baseUrl: string,
  path: string,
  loginPath: string,
): Promise<void> {
  await page.goto(`${baseUrl}${path}`, {
    waitUntil: 'domcontentloaded',
    timeout: NAVIGATION_TIMEOUT_MS,
  })
  await page.waitForURL(`**${loginPath}**`, { timeout: SETTLE_TIMEOUT_MS })
  const url = new URL(page.url())
  if (url.pathname !== loginPath || !url.searchParams.has('redirect')) {
    throw new Error(`${path} should redirect to ${loginPath}?redirect=..., got ${page.url()}`)
  }
  await checkAccessibilityShape(page, `login guard for ${path}`)
}

async function runBrowserChecks(config: SmokeConfig, baseUrl: string): Promise<string[]> {
  const failures: string[] = []
  const consoleErrors: string[] = []
  const pageErrors: string[] = []
  const sessionProbePath = '/api/v1/user'
  const sessionProbeNoise =
    /Failed to load resource.*(401|ERR_CONNECTION_REFUSED|ERR_PROXY_CONNECTION_FAILED)/
  const probePathOf = (url: string): string | null => {
    try {
      return new URL(url).pathname
    } catch {
      return null
    }
  }
  let observedSessionProbeFailures = 0
  const browser = await chromium.launch()
  try {
    const page = await browser.newPage()
    page.on('console', (message) => {
      if (message.type() === 'error') consoleErrors.push(message.text())
    })
    page.on('response', (response) => {
      if (probePathOf(response.url()) === sessionProbePath && response.status() === 401) {
        observedSessionProbeFailures += 1
      }
    })
    page.on('requestfailed', (request) => {
      if (probePathOf(request.url()) === sessionProbePath) observedSessionProbeFailures += 1
    })
    page.on('pageerror', (error) => pageErrors.push(error.message))
    const labels: string[] = [
      ...PUBLIC_PAGES.map((check) => `a11y ${check.path}`),
      ...AUTH_GUARDS.map((guard) => `guard ${guard.path}`),
    ]
    for (let index = 0; index < labels.length; index += 1) {
      const label = labels[index]
      if (!label) continue
      try {
        if (index < PUBLIC_PAGES.length) {
          const check = PUBLIC_PAGES[index]
          if (check) await checkPublicPage(page, baseUrl, check)
        } else {
          const guard = AUTH_GUARDS[index - PUBLIC_PAGES.length]
          if (guard) await checkAuthGuard(page, baseUrl, guard.path, guard.loginPath)
        }
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
  let allowedSessionProbeNoise = 0
  for (const message of consoleErrors) {
    if (sessionProbeNoise.test(message) && allowedSessionProbeNoise < observedSessionProbeFailures) {
      allowedSessionProbeNoise += 1
      continue
    }
    failures.push(`console error: ${message}`)
  }
  return failures
}

async function main(): Promise<void> {
  const config = readSmokeConfig()
  const failures: string[] = []
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
  } catch (error) {
    failures.push(error instanceof Error ? error.message : String(error))
  } finally {
    if (devServer) await stopSmokeDevServer(devServer).catch(() => undefined)
  }
  if (failures.length > 0) {
    console.error(`accessibility smoke failed (${failures.length}):\n${failures.join('\n')}`)
    if (devLogs.length > 0) console.error(devLogs.join('\n'))
    process.exitCode = 1
    return
  }
  console.log(
    `accessibility smoke passed: ${PUBLIC_PAGES.length} public pages, ${AUTH_GUARDS.length} auth guards`,
  )
}

void main()
