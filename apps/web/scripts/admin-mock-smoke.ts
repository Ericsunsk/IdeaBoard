import { adminNavigationRouteDefs } from '../src/features/admin/app/adminNavigationMeta'
import { assertNoConsoleErrors, createMockSmokeRuntime, type CommandSpec } from './mock-smoke-utils'

type SmokePage = {
  label: string
  path: string
}

const ADMIN_API_RE = /\/api\/admin(?:[/?#]|$)/

const smoke = createMockSmokeRuntime({
  hostEnvName: 'ADMIN_MOCK_SMOKE_HOST',
  portEnvName: 'ADMIN_MOCK_SMOKE_PORT',
  verboseEnvName: 'ADMIN_MOCK_SMOKE_VERBOSE',
  sessionPrefix: 'ams',
  defaultPort: 5174,
})

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
      status: 1,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
  },
}

const pages: SmokePage[] = adminNavigationRouteDefs.map((route) => ({
  label: route.label,
  path: route.path,
}))

function setupBrowser(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, ['open', 'about:blank'])
  smoke.routeJson(playwrightCli, '**/api/auth/get-session', AUTH_SESSION)
  smoke.routeJson(playwrightCli, '**/api/user/info', USER_INFO)
  smoke.routeJson(
    playwrightCli,
    '**/api/admin/**',
    { code: 599, msg: 'admin mock smoke blocked real admin API request' },
    599,
  )
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
  if (!snapshot.includes(`Page URL: ${smoke.baseUrl}${page.path}`)) {
    throw new Error(`Admin route did not stay on ${page.path}\n${snapshot}`)
  }
  if (!snapshot.includes(page.label)) {
    throw new Error(`Admin page label not found on ${page.path}\n${snapshot}`)
  }
}

function smokePage(playwrightCli: CommandSpec, page: SmokePage) {
  smoke.runCommand(playwrightCli, ['goto', `${smoke.baseUrl}${page.path}`])
  const snapshot = smoke.runCommand(playwrightCli, ['snapshot']).stdout
  assertPageReached(snapshot, page)

  const consoleOutput = smoke.runCommand(playwrightCli, ['console', 'error']).stdout
  assertNoConsoleErrors(consoleOutput, page.path)

  const requestsOutput = smoke.runCommand(playwrightCli, ['requests']).stdout
  assertNoAdminApiRequests(requestsOutput, page)

  if (smoke.isVerbose) {
    console.log(`ok ${page.path}`)
  }
}

function assertAdminUserMenu(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, [
    'run-code',
    [
      'const accountButton = page.getByRole("button", { name: "账户菜单" }).first()',
      'await accountButton.waitFor({ state: "visible", timeout: 10000 })',
      'await accountButton.click()',
      'const profileItem = page.getByText("个人中心", { exact: true }).last()',
      'const logoutItem = page.getByText("退出登录", { exact: true }).last()',
      'await profileItem.waitFor({ state: "visible", timeout: 10000 })',
      'await logoutItem.waitFor({ state: "visible", timeout: 10000 })',
      'await page.mouse.click(24, 24)',
      'await logoutItem.waitFor({ state: "hidden", timeout: 10000 })',
    ].join('\n'),
  ])
}

async function main() {
  let playwrightCli: CommandSpec | null = null
  try {
    playwrightCli = smoke.resolvePlaywrightCli()
    await smoke.startDevServer({ VITE_ADMIN_MOCK_DATA: '1' })
    setupBrowser(playwrightCli)

    for (const page of pages) {
      smokePage(playwrightCli, page)
    }
    assertAdminUserMenu(playwrightCli)

    console.log(`admin mock smoke passed: ${pages.length} pages`)
  } catch (error) {
    console.error(error instanceof Error ? error.message : error)
    if (smoke.devServerLogs.length > 0) {
      console.error(smoke.devServerLogs.join('\n'))
    }
    process.exitCode = 1
  } finally {
    if (playwrightCli) {
      smoke.runCommand(playwrightCli, ['close'], { allowFailure: true })
    }
    await smoke.stopDevServer()
  }
}

void main()
