import type { User, UserBoardItem } from '../src/shared/api/generated-query/model'
import { assertNoConsoleErrors, createMockSmokeRuntime, type CommandSpec } from './mock-smoke-utils'

type SmokePage = {
  label: string
  path: string
  expectedText: string[]
}

const smoke = createMockSmokeRuntime({
  hostEnvName: 'USER_ROUTE_SMOKE_HOST',
  portEnvName: 'USER_ROUTE_SMOKE_PORT',
  verboseEnvName: 'USER_ROUTE_SMOKE_VERBOSE',
  sessionPrefix: 'urs',
})

const MOCK_USER = {
  id: 1,
  uid: 'USM001',
  username: 'mock-user',
  role: 'user',
  auth_user_id: 'auth-user-route',
  avatar: '',
  status: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
} satisfies User

const MOCK_BOARD_ID = 'mock-board-1'
const MOCK_BOARD_NAME = 'Mock Canvas Board'
const MOCK_UPDATED_AT = 1_764_000_000_000
const MOCK_CANVAS_UPLOAD_URL = `/uploads/canvas/${MOCK_USER.uid}/${MOCK_BOARD_ID}/content/mock-upload.png`
const MOCK_AVATAR_URL = `/uploads/profile/${MOCK_USER.uid}/avatar/mock-avatar.png`

const AUTH_SESSION = {
  session: { id: 'user-route-smoke-session' },
  user: { id: MOCK_USER.auth_user_id, username: MOCK_USER.username, role: MOCK_USER.role },
}

const USER_INFO = {
  code: 0,
  msg: 'ok',
  data: { user: MOCK_USER },
}

const SITE_BRANDING_SETTING = {
  code: 0,
  msg: 'ok',
  data: {
    project_name: 'IdeaBoard',
    logo_url: '',
  },
}

const MOCK_BOARD_STATE = {
  schemaVersion: 1 as const,
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
  revision: 1,
  state_schema_version: 1,
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
    url: MOCK_CANVAS_UPLOAD_URL,
    width: 512,
    height: 512,
    size: 1024,
    mime: 'image/png',
  },
}

const AVATAR_UPLOAD = {
  code: 0,
  msg: 'ok',
  data: {
    avatar: MOCK_AVATAR_URL,
    width: 64,
    height: 64,
    mime: 'image/png',
    ext: 'png',
  },
}

const allowedUserApiRequestPatterns = [
  /\/api\/settings\/branding(?:[?#]|\s|$)/,
  /\/api\/auth\/get-session(?:[?#]|\s|$)/,
  /\/api\/user\/info(?:[?#]|\s|$)/,
  /\/api\/user\/boards(?:[?#]|\s|$)/,
  /\/api\/user\/profile\/avatar(?:[?#]|\s|$)/,
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
    label: 'canvas',
    path: `/user/canvas?boardId=${MOCK_BOARD_ID}`,
    expectedText: [MOCK_BOARD_NAME, '100%'],
  },
]

function setupBrowser(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, ['open', 'about:blank'])
  smoke.routeJson(playwrightCli, '**/api/auth/get-session', AUTH_SESSION)
  smoke.routeJson(playwrightCli, '**/api/settings/branding', SITE_BRANDING_SETTING)
  smoke.routeJson(playwrightCli, '**/api/user/info', USER_INFO)
  smoke.routeJson(playwrightCli, '**/api/user/boards', USER_BOARDS)
  smoke.routeJson(playwrightCli, `**/api/user/boards/${MOCK_BOARD_ID}`, BOARD_UPSERT)
  smoke.routeJson(playwrightCli, `**/api/user/boards/${MOCK_BOARD_ID}/uploads`, BOARD_UPLOAD)
  smoke.routeJson(
    playwrightCli,
    '**/api/admin/**',
    { code: 599, msg: 'user route smoke blocked admin API request' },
    599,
  )
  smoke.routeJson(
    playwrightCli,
    '**/api/user/tasks/**',
    { code: 599, msg: 'user route smoke blocked task API request' },
    599,
  )
  smoke.routeJson(
    playwrightCli,
    '**/api/auth/change-password',
    { code: 599, msg: 'user route smoke blocked auth mutation' },
    599,
  )
  smoke.routeJson(
    playwrightCli,
    '**/api/auth/sign-out',
    { code: 599, msg: 'user route smoke blocked auth mutation' },
    599,
  )
  smoke.routeJson(
    playwrightCli,
    '**/api/user/profile/avatar',
    AVATAR_UPLOAD,
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
}

function waitForExpectedText(playwrightCli: CommandSpec, page: SmokePage) {
  smoke.runCommand(playwrightCli, [
    'run-code',
    page.expectedText
      .map(
        (text) =>
          `await page.getByText(${JSON.stringify(text)}).first().waitFor({ state: "visible", timeout: 15000 })`,
      )
      .join('\n'),
  ])
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

function assertCanvasDom(playwrightCli: CommandSpec, page: SmokePage) {
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

function assertBoardsInteractions(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, [
    'run-code',
    [
      'const nativeDialogs = []',
      'page.on("dialog", async (dialog) => {',
      '  nativeDialogs.push(dialog.message())',
      '  await dialog.dismiss()',
      '})',
      'const moreButton = page.getByRole("button", { name: "More actions" }).first()',
      'await moreButton.waitFor({ state: "visible", timeout: 10000 })',
      'const triggerBox = await moreButton.boundingBox()',
      'if (!triggerBox) throw new Error("Board menu trigger has no bounding box")',
      'await moreButton.click()',
      'const deleteItem = page.getByText("Delete", { exact: true }).last()',
      'await deleteItem.waitFor({ state: "visible", timeout: 10000 })',
      'const deleteBox = await deleteItem.boundingBox()',
      'if (!deleteBox) throw new Error("Board delete menu item has no bounding box")',
      'if (deleteBox.y > triggerBox.y + triggerBox.height + 2) {',
      '  throw new Error(`Board menu should open above the trigger, got triggerY=${triggerBox.y}, itemY=${deleteBox.y}`)',
      '}',
      'await deleteItem.click()',
      'await page.getByRole("heading", { name: "删除画布" }).waitFor({ timeout: 10000 })',
      `await page.getByText(${JSON.stringify(`确认删除「${MOCK_BOARD_NAME}」？此操作不可撤销。`)}).waitFor({ timeout: 10000 })`,
      'if (nativeDialogs.length > 0) {',
      '  throw new Error(`Unexpected native dialog: ${nativeDialogs.join(", ")}`)',
      '}',
      'await page.getByRole("button", { name: "取消" }).click()',
      'await page.getByRole("heading", { name: "删除画布" }).waitFor({ state: "hidden", timeout: 10000 })',
    ].join('\n'),
  ])
}

function assertCenterAvatarUpload(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, [
    'run-code',
    [
      'const avatarButton = page.getByRole("button", { name: "更换头像" })',
      'await avatarButton.waitFor({ state: "visible", timeout: 10000 })',
      'const avatarInput = page.locator(\'input[type="file"][accept="image/*"]\').first()',
      'await avatarInput.setInputFiles({',
      '  name: "mock-avatar.png",',
      '  mimeType: "image/png",',
      '  buffer: Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),',
      '})',
      `await page.locator(${JSON.stringify(`img[src$="${MOCK_AVATAR_URL}"]`)}).first().waitFor({ state: "visible", timeout: 10000 })`,
    ].join('\n'),
  ])
}

function assertCanvasInteractions(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, [
    'run-code',
    [
      'const pane = page.locator(".vue-flow__pane").first()',
      'await pane.waitFor({ state: "visible", timeout: 10000 })',
      'const paneBox = await pane.boundingBox()',
      'if (!paneBox) throw new Error("Canvas pane has no bounding box")',
      'const accountButton = page.getByRole("button", { name: "账户菜单" }).first()',
      'await accountButton.click()',
      'const userMenu = page.getByRole("menu").filter({ hasText: "个人中心" })',
      'await userMenu.waitFor({ state: "visible", timeout: 10000 })',
      'await page.mouse.click(paneBox.x + paneBox.width - 24, paneBox.y + paneBox.height - 24)',
      'await userMenu.waitFor({ state: "hidden", timeout: 10000 })',
      'await pane.click({',
      '  button: "right",',
      '  position: {',
      '    x: Math.min(Math.max(240, paneBox.width / 2), paneBox.width - 80),',
      '    y: Math.min(Math.max(220, paneBox.height / 2), paneBox.height - 80),',
      '  },',
      '})',
      'const contextMenu = page.locator(".canvas-node-context-menu")',
      'await contextMenu.waitFor({ state: "visible", timeout: 10000 })',
      'await page.mouse.click(paneBox.x + paneBox.width - 24, paneBox.y + paneBox.height - 24)',
      'await contextMenu.waitFor({ state: "hidden", timeout: 10000 })',
      'const node = page.locator(".vue-flow__node").first()',
      'await node.hover()',
      'await page.locator(".flow-canvas-node__handle-aggregate.is-visible").first().waitFor({ state: "visible", timeout: 10000 })',
      'await page.mouse.move(paneBox.x + paneBox.width - 24, paneBox.y + paneBox.height - 24)',
      'await page.waitForTimeout(300)',
      'const visibleHandleCount = await page.locator(".flow-canvas-node__handle-aggregate.is-visible").count()',
      'if (visibleHandleCount !== 0) {',
      '  throw new Error(`Expected hover handles to hide after pointer leaves node, got ${visibleHandleCount}`)',
      '}',
    ].join('\n'),
  ])
}

function smokePage(playwrightCli: CommandSpec, page: SmokePage) {
  smoke.runCommand(playwrightCli, ['goto', `${smoke.baseUrl}${page.path}`])
  waitForExpectedText(playwrightCli, page)
  const snapshot = smoke.runCommand(playwrightCli, ['snapshot']).stdout
  assertPageReached(snapshot, page)

  if (page.label === 'boards') {
    assertBoardsInteractions(playwrightCli)
  }

  if (page.label === 'center') {
    assertCenterAvatarUpload(playwrightCli)
  }

  if (page.label === 'canvas') {
    assertCanvasDom(playwrightCli, page)
    assertCanvasInteractions(playwrightCli)
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

    console.log(`user route smoke passed: ${pages.length} pages`)
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
