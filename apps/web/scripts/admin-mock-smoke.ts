import { adminNavigationRouteDefs } from '../src/features/admin/app/adminNavigationMeta'
import { assertNoConsoleErrors, createMockSmokeRuntime, type CommandSpec } from './mock-smoke-utils'

type SmokePage = {
  label: string
  path: string
}

const API_REQUEST_RE = /\/api\/(?:admin|auth|settings|user)(?:[/?#]|$)/
const ALLOWED_API_REQUESTS = [
  /\/api\/settings\/branding(?:[?#\s]|$)/,
  /\/api\/auth\/get-session(?:[?#\s]|$)/,
  /\/api\/user\/info(?:[?#\s]|$)/,
  /\/api\/admin\/statistics(?:[?#\s]|$)/,
  /\/api\/admin\/monitor(?:[/?#\s]|$)/,
  /\/api\/admin\/tasks(?:[?#\s]|$)/,
  /\/api\/admin\/channels(?:[?#\s]|$)/,
  /\/api\/admin\/accounts(?:[/?#\s]|$)/,
  /\/api\/admin\/users(?:[?#\s]|$)/,
  /\/api\/admin\/settings\/branding(?:[?#\s]|$)/,
  /\/api\/admin\/settings\/media-upload(?:[?#\s]|$)/,
  /\/api\/admin\/settings\/registration(?:[?#\s]|$)/,
  /\/api\/admin\/settings\/rate-limit(?:[?#\s]|$)/,
  /\/api\/admin\/settings\/task-monitor(?:[?#\s]|$)/,
  /\/api\/admin\/settings\/public-urls(?:[?#\s]|$)/,
  /\/api\/admin\/feature-prompts(?:[?#\s]|$)/,
  /\/api\/admin\/storage\/providers(?:[?#\s]|$)/,
  /\/api\/admin\/storage\/profiles(?:[?#\s]|$)/,
] as const

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

const EMPTY_ADMIN_STATISTICS = {
  code: 0,
  msg: 'ok',
  data: {
    summary: {
      total_calls: 0,
      today_calls: 0,
      today_success: 0,
      today_failed: 0,
      today_cancelled: 0,
      yesterday_calls: 0,
      yesterday_success: 0,
      yesterday_failed: 0,
      yesterday_cancelled: 0,
      this_month_calls: 0,
      this_month_cancelled: 0,
      pending: 0,
      running: 0,
      completed: 0,
      failed: 0,
      cancelled: 0,
    },
    trend: [],
    model_health: [],
    quality_trend: [],
    slow_models: [],
    bottleneck_sources: [],
    failure_categories: [],
    backfill_status: {
      range_start: '2026-01-01',
      range_end: '2026-01-01',
      missing_days: 0,
      backfilled_days: 0,
    },
  },
}

const EMPTY_ADMIN_MONITOR = {
  code: 0,
  msg: 'ok',
  data: {
    monitor: {
      global_queue_len: 0,
      wait_queue_len: 0,
      running_attempts: 0,
      account_loads: {},
      execution_pressure: [],
    },
    stats: {
      pending: 0,
      running: 0,
      completed: 0,
      failed: 0,
      cancelled: 0,
    },
  },
}

const ADMIN_MONITOR_STREAM = [
  'retry: 600000',
  'event: snapshot',
  `data: ${JSON.stringify(EMPTY_ADMIN_MONITOR)}`,
  '',
  '',
].join('\n')

const EMPTY_ADMIN_TASKS = {
  code: 0,
  msg: 'ok',
  data: {
    list: [],
    total: 0,
    pending_count: 0,
    running_count: 0,
    completed_count: 0,
    failed_count: 0,
    cancelled_count: 0,
    other_count: 0,
  },
}

const EMPTY_ADMIN_USERS = {
  code: 0,
  msg: 'ok',
  data: {
    total: 0,
    active_count: 0,
    banned_count: 0,
    admin_count: 0,
    list: [],
  },
}

const EMPTY_ADMIN_CHANNELS = {
  code: 0,
  msg: 'ok',
  data: {
    list: [],
    total: 0,
  },
}

const EMPTY_ADMIN_ACCOUNTS = {
  code: 0,
  msg: 'ok',
  data: {
    list: [],
    total: 0,
  },
}

const ADMIN_ACCOUNT_LIMITS = {
  code: 0,
  msg: 'ok',
  data: {
    limits: {
      max_concurrent: { min: 1, max: 64, default: 3 },
      timeout_seconds: { min: 1, max: 600, default: 120 },
    },
  },
}

const ADMIN_REGISTRATION_SETTING = {
  code: 0,
  msg: 'ok',
  data: {
    enabled: false,
  },
}

const ADMIN_BRANDING_SETTING = {
  code: 0,
  msg: 'ok',
  data: {
    project_name: 'IdeaBoard',
    logo_url: '',
  },
}

const ADMIN_RATE_LIMIT_SETTINGS = {
  code: 0,
  msg: 'ok',
  data: {
    window_seconds: 2,
    max_requests: 120,
  },
}

const ADMIN_TASK_MONITOR_SETTINGS = {
  code: 0,
  msg: 'ok',
  data: {
    list_page_size: 12,
    auto_refresh_enabled: true,
    refresh_interval_seconds: 4,
    result_preview_enabled: true,
  },
}

const ADMIN_PUBLIC_URL_SETTINGS = {
  code: 0,
  msg: 'ok',
  data: {
    public_base_url: 'https://app.example.test',
    public_asset_base_url: 'https://api.example.test',
  },
}

const ADMIN_MEDIA_UPLOAD_SETTINGS = {
  code: 0,
  msg: 'ok',
  data: {
    upload_max_concurrent: 8,
  },
}

const EMPTY_FEATURE_PROMPTS = {
  code: 0,
  msg: 'ok',
  data: {
    prompts: [],
  },
}

const STORAGE_PROVIDERS = {
  code: 0,
  msg: 'ok',
  data: {
    providers: [
      {
        provider: 'local',
        label: 'Local',
        description: '服务器本地存储',
        builtin: true,
        create_allowed: false,
        supports_test: false,
        supports_bucket_info: false,
        fields: [],
      },
      {
        provider: 's3-compatible',
        label: 'S3 Compatible',
        description: 'S3 兼容对象存储',
        builtin: false,
        create_allowed: true,
        supports_test: true,
        supports_bucket_info: true,
        fields: [
          { key: 'bucket_name', label: 'Bucket', type: 'text', required: true },
          { key: 'endpoint', label: 'Endpoint', type: 'text', required: true },
          { key: 'region_id', label: 'Region', type: 'text' },
          { key: 'access_key_id', label: 'Access Key', type: 'text', required: true },
          { key: 'access_key_secret', label: 'Secret Key', type: 'password', required: true, secret: true },
          { key: 'force_path_style', label: 'Path Style', type: 'switch' },
        ],
      },
    ],
  },
}

const STORAGE_PROFILES = {
  code: 0,
  msg: 'ok',
  data: {
    profiles: [
      {
        id: 'local',
        name: 'Local Storage',
        provider: 'local',
        enabled: true,
        is_current: true,
        builtin: true,
        secret_configured: false,
        config: {},
      },
    ],
  },
}

const pages: SmokePage[] = adminNavigationRouteDefs.map((route) => ({
  label: route.label,
  path: route.path,
}))

function setupBrowser(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, ['open', 'about:blank'])
  smoke.routeJson(playwrightCli, '**/api/auth/get-session', AUTH_SESSION)
  smoke.routeJson(playwrightCli, '**/api/settings/branding', ADMIN_BRANDING_SETTING)
  smoke.routeJson(playwrightCli, '**/api/user/info', USER_INFO)
  smoke.routeResponse(playwrightCli, '**/api/admin/monitor/stream', ADMIN_MONITOR_STREAM, {
    contentType: 'text/event-stream',
  })
  smoke.routeJson(playwrightCli, '**/api/admin/statistics**', EMPTY_ADMIN_STATISTICS)
  smoke.routeJson(playwrightCli, '**/api/admin/monitor', EMPTY_ADMIN_MONITOR)
  smoke.routeJson(playwrightCli, '**/api/admin/tasks**', EMPTY_ADMIN_TASKS)
  smoke.routeJson(playwrightCli, '**/api/admin/channels', EMPTY_ADMIN_CHANNELS)
  smoke.routeJson(playwrightCli, '**/api/admin/accounts/limits', ADMIN_ACCOUNT_LIMITS)
  smoke.routeJson(playwrightCli, '**/api/admin/accounts**', EMPTY_ADMIN_ACCOUNTS)
  smoke.routeJson(playwrightCli, '**/api/admin/users**', EMPTY_ADMIN_USERS)
  smoke.routeJson(playwrightCli, '**/api/admin/settings/branding', ADMIN_BRANDING_SETTING)
  smoke.routeJson(playwrightCli, '**/api/admin/settings/media-upload', ADMIN_MEDIA_UPLOAD_SETTINGS)
  smoke.routeJson(playwrightCli, '**/api/admin/settings/registration', ADMIN_REGISTRATION_SETTING)
  smoke.routeJson(playwrightCli, '**/api/admin/settings/rate-limit', ADMIN_RATE_LIMIT_SETTINGS)
  smoke.routeJson(playwrightCli, '**/api/admin/settings/task-monitor', ADMIN_TASK_MONITOR_SETTINGS)
  smoke.routeJson(playwrightCli, '**/api/admin/settings/public-urls', ADMIN_PUBLIC_URL_SETTINGS)
  smoke.routeJson(playwrightCli, '**/api/admin/feature-prompts', EMPTY_FEATURE_PROMPTS)
  smoke.routeJson(playwrightCli, '**/api/admin/storage/providers', STORAGE_PROVIDERS)
  smoke.routeJson(playwrightCli, '**/api/admin/storage/profiles', STORAGE_PROFILES)
}

function assertNoUnexpectedApiRequests(output: string, page: SmokePage) {
  const unexpectedRequests = output
    .split(/\r?\n/)
    .filter((line) => API_REQUEST_RE.test(line))
    .filter((line) => !ALLOWED_API_REQUESTS.some((pattern) => pattern.test(line)))
    .join('\n')
  if (unexpectedRequests) {
    throw new Error(`Unexpected API requests on ${page.path}\n${unexpectedRequests}`)
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
  assertNoUnexpectedApiRequests(requestsOutput, page)

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
    await smoke.startDevServer()
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
    if (playwrightCli && !process.env.GITHUB_ACTIONS) {
      smoke.runCommand(playwrightCli, ['close'], { allowFailure: true, timeoutMs: 10_000 })
    }
    await smoke.stopDevServer()
  }
}

void main()
