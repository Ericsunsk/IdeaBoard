import type { User, UserBoardItem } from '../src/shared/api/generated-query/model'
import { assertNoConsoleErrors, createMockSmokeRuntime, type CommandSpec } from './mock-smoke-utils'

type SmokePage = {
  label: string
  path: string
  expectedText: string[]
}

const smoke = createMockSmokeRuntime({
  hostEnvName: 'USER_MOCK_SMOKE_HOST',
  portEnvName: 'USER_MOCK_SMOKE_PORT',
  verboseEnvName: 'USER_MOCK_SMOKE_VERBOSE',
  sessionPrefix: 'ums',
})

const MOCK_USER = {
  id: 1,
  uid: 'USM001',
  username: 'mock-user',
  role: 'user',
  auth_user_id: 'auth-user-mock',
  avatar: '',
  status: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
} satisfies User

const MOCK_BOARD_ID = 'mock-board-1'
const MOCK_BOARD_NAME = 'Mock Workspace Board'
const MOCK_UPDATED_AT = 1_764_000_000_000
const MOCK_WORKSPACE_UPLOAD_URL = `/uploads/workspace/${MOCK_USER.uid}/${MOCK_BOARD_ID}/content/mock-upload.png`

const AUTH_SESSION = {
  session: { id: 'user-mock-smoke-session' },
  user: { id: MOCK_USER.auth_user_id, username: MOCK_USER.username, role: MOCK_USER.role },
}

const USER_INFO = {
  code: 0,
  msg: 'ok',
  data: { user: MOCK_USER },
}

const MOCK_BOARD_STATE = {
  imageNodes: [],
  promptNodes: [
    {
      id: 1,
      type: 'prompt',
      x: 160,
      y: 140,
      width: 320,
      title: 'Smoke Prompt',
      text: 'A compact browser smoke board',
      outputCount: 1,
      aspectRatio: '1:1',
      inputImages: [],
    },
  ],
  fusionNodes: [],
  connections: [],
  viewport: { canvasX: 0, canvasY: 0, zoomLevel: 100 },
  updatedAt: MOCK_UPDATED_AT,
}

const MOCK_BOARD = {
  board_id: MOCK_BOARD_ID,
  name: MOCK_BOARD_NAME,
  state: MOCK_BOARD_STATE,
  access_role: 'owner',
  created_at: MOCK_UPDATED_AT - 86_400_000,
  updated_at: MOCK_UPDATED_AT,
} satisfies UserBoardItem

const USER_BOARDS = {
  code: 0,
  msg: 'ok',
  data: {
    list: [MOCK_BOARD],
  },
}

const BOARD_UPSERT = {
  code: 0,
  msg: 'ok',
  data: {
    board: MOCK_BOARD,
  },
}

const BOARD_UPLOAD = {
  code: 0,
  msg: 'ok',
  data: {
    url: MOCK_WORKSPACE_UPLOAD_URL,
    width: 512,
    height: 512,
    size: 1024,
    mime: 'image/png',
  },
}

const allowedUserApiRequestPatterns = [
  /\/api\/auth\/get-session(?:[?#]|\s|$)/,
  /\/api\/user\/info(?:[?#]|\s|$)/,
  /\/api\/user\/boards(?:[?#]|\s|$)/,
  new RegExp(`/api/user/boards/${MOCK_BOARD_ID}(?:[?#]|\\s|$)`),
  new RegExp(`/api/user/boards/${MOCK_BOARD_ID}/uploads(?:[?#]|\\s|$)`),
]

const pages: SmokePage[] = [
  {
    label: 'home',
    path: '/user/home',
    expectedText: ['IdeaBoard', 'EXPLORE, EXPAND, AND REFINE YOUR IDEAS', 'Get started'],
  },
  {
    label: 'boards',
    path: '/user/boards',
    expectedText: ['Welcome to IdeaBoard!', 'New Canvas', MOCK_BOARD_NAME],
  },
  {
    label: 'center',
    path: '/user/center',
    expectedText: [MOCK_USER.username, MOCK_USER.uid, '个人基本资料'],
  },
  {
    label: 'workspace',
    path: `/user/workspace?boardId=${MOCK_BOARD_ID}`,
    expectedText: [MOCK_BOARD_NAME, '100%'],
  },
]

function setupBrowser(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, ['open', 'about:blank'])
  smoke.routeJson(playwrightCli, '**/api/auth/get-session', AUTH_SESSION)
  smoke.routeJson(playwrightCli, '**/api/user/info', USER_INFO)
  smoke.routeJson(playwrightCli, '**/api/user/boards', USER_BOARDS)
  smoke.routeJson(playwrightCli, `**/api/user/boards/${MOCK_BOARD_ID}`, BOARD_UPSERT)
  smoke.routeJson(playwrightCli, `**/api/user/boards/${MOCK_BOARD_ID}/uploads`, BOARD_UPLOAD)
  smoke.routeJson(
    playwrightCli,
    '**/api/admin/**',
    { code: 599, msg: 'user mock smoke blocked admin API request' },
    599,
  )
  smoke.routeJson(
    playwrightCli,
    '**/api/user/tasks/**',
    { code: 599, msg: 'user mock smoke blocked task API request' },
    599,
  )
  smoke.routeJson(
    playwrightCli,
    '**/api/auth/change-password',
    { code: 599, msg: 'user mock smoke blocked auth mutation' },
    599,
  )
  smoke.routeJson(
    playwrightCli,
    '**/api/auth/sign-out',
    { code: 599, msg: 'user mock smoke blocked auth mutation' },
    599,
  )
  smoke.routeJson(
    playwrightCli,
    '**/api/user/profile/avatar',
    { code: 599, msg: 'user mock smoke blocked avatar mutation' },
    599,
  )
}

function prepareOriginStorage(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, ['goto', `${smoke.baseUrl}/user/home`])
  smoke.runCommand(playwrightCli, [
    'run-code',
    [
      'await page.evaluate(async () => {',
      '  localStorage.clear()',
      '  sessionStorage.clear()',
      "  if (!('indexedDB' in window)) return",
      "  const databases = typeof indexedDB.databases === 'function' ? await indexedDB.databases() : []",
      '  await Promise.all(databases.map((db) => new Promise((resolve) => {',
      '    if (!db.name) return resolve(undefined)',
      '    const request = indexedDB.deleteDatabase(db.name)',
      '    request.onsuccess = request.onerror = request.onblocked = () => resolve(undefined)',
      '  })))',
      '})',
    ].join('\n'),
  ])
}

function assertPageReached(snapshot: string, page: SmokePage) {
  if (!snapshot.includes(`Page URL: ${smoke.baseUrl}${page.path}`)) {
    throw new Error(`User route did not stay on ${page.path}\n${snapshot}`)
  }

  const missingText = page.expectedText.filter((text) => !snapshot.includes(text))
  if (missingText.length > 0) {
    throw new Error(`Missing expected text on ${page.path}: ${missingText.join(', ')}\n${snapshot}`)
  }
}

function assertExpectedUserApiRequests(output: string, page: SmokePage) {
  const unexpectedRequests = output
    .split(/\r?\n/)
    .filter((line) => line.includes('/api/user/') || line.includes('/api/auth/'))
    .filter((line) => !allowedUserApiRequestPatterns.some((pattern) => pattern.test(line)))
    .join('\n')

  if (unexpectedRequests) {
    throw new Error(`Unexpected user/auth API requests on ${page.path}\n${unexpectedRequests}`)
  }
}

function assertWorkspaceDom(playwrightCli: CommandSpec, page: SmokePage) {
  smoke.runCommand(playwrightCli, [
    'run-code',
    [
      "await page.waitForSelector('.vue-flow', { timeout: 10000 })",
      `await page.getByText(${JSON.stringify(MOCK_BOARD_NAME)}).waitFor({ timeout: 10000 })`,
      "await page.getByText('100%').waitFor({ timeout: 10000 })",
      "const flowCount = await page.locator('.vue-flow').count()",
      'if (flowCount !== 1) throw new Error(`Expected exactly one .vue-flow, got ${flowCount}`)',
    ].join('\n'),
  ])

  const consoleOutput = smoke.runCommand(playwrightCli, ['console', 'error']).stdout
  assertNoConsoleErrors(consoleOutput, page.path)
}

function smokePage(playwrightCli: CommandSpec, page: SmokePage) {
  smoke.runCommand(playwrightCli, ['goto', `${smoke.baseUrl}${page.path}`])
  const snapshot = smoke.runCommand(playwrightCli, ['snapshot']).stdout
  assertPageReached(snapshot, page)

  if (page.label === 'workspace') {
    assertWorkspaceDom(playwrightCli, page)
  }

  const consoleOutput = smoke.runCommand(playwrightCli, ['console', 'error']).stdout
  assertNoConsoleErrors(consoleOutput, page.path)

  const requestsOutput = smoke.runCommand(playwrightCli, ['requests']).stdout
  assertExpectedUserApiRequests(requestsOutput, page)

  if (smoke.isVerbose) {
    console.log(`ok ${page.path}`)
  }
}

async function main() {
  let playwrightCli: CommandSpec | null = null
  try {
    playwrightCli = smoke.resolvePlaywrightCli()
    await smoke.startDevServer()
    setupBrowser(playwrightCli)
    prepareOriginStorage(playwrightCli)

    for (const page of pages) {
      smokePage(playwrightCli, page)
    }

    console.log(`user mock smoke passed: ${pages.length} pages`)
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
