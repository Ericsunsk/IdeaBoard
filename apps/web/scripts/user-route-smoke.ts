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

const MOCK_ADMIN = {
  ...MOCK_USER,
  id: 2,
  uid: 'ADM001',
  username: 'mock-admin',
  role: 'admin',
  auth_user_id: 'auth-admin-route',
} satisfies User

const MOCK_BOARD_ID = 'mock-board-1'
const MOCK_BOARD_NAME = 'Mock Canvas Board'
const MOCK_UPDATED_AT = 1_764_000_000_000
const MOCK_CANVAS_UPLOAD_URL = `/uploads/canvas/${MOCK_USER.uid}/${MOCK_BOARD_ID}/content/mock-upload.png`
const MOCK_AVATAR_URL = `/uploads/profile/${MOCK_USER.uid}/avatar/mock-avatar.png`

function createAuthSession(user: User) {
  return {
    session: { id: `user-route-smoke-session-${user.uid}` },
    user: { id: user.auth_user_id, username: user.username, role: user.role },
  }
}

function createUserInfo(user: User) {
  return {
    code: 0,
    msg: 'ok',
    data: { user },
  }
}

const AUTH_SESSION = createAuthSession(MOCK_USER)
const USER_INFO = createUserInfo(MOCK_USER)

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

function setSessionUserRoute(playwrightCli: CommandSpec, user: User) {
  const userInfoBody = JSON.stringify(createUserInfo(user))
  const authSessionBody = JSON.stringify(createAuthSession(user))

  smoke.runCommand(playwrightCli, [
    'run-code',
    [
      'await page.unroute("**/api/user/info").catch(() => undefined)',
      'await page.unroute("**/api/auth/get-session").catch(() => undefined)',
      'await page.route("**/api/user/info", async (route) => {',
      '  await route.fulfill({',
      '    status: 200,',
      '    contentType: "application/json",',
      `    body: ${JSON.stringify(userInfoBody)},`,
      '  })',
      '})',
      'await page.route("**/api/auth/get-session", async (route) => {',
      '  await route.fulfill({',
      '    status: 200,',
      '    contentType: "application/json",',
      `    body: ${JSON.stringify(authSessionBody)},`,
      '  })',
      '})',
    ].join('\n'),
  ])
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

function assertHomeMobileHeroLayout(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, [
    'run-code',
    [
      'for (const viewportSize of [{ width: 375, height: 812 }, { width: 390, height: 844 }]) {',
      '  await page.setViewportSize(viewportSize)',
      '  await page.waitForTimeout(150)',
      '  const viewport = page.viewportSize()',
      '  if (!viewport) throw new Error("Mobile viewport is not available")',
      '  const safeElements = page.locator("[data-home-hero-safe]")',
      '  const safeCount = await safeElements.count()',
      '  if (safeCount < 4) throw new Error(`Expected at least 4 mobile-safe hero elements, got ${safeCount}`)',
      '  for (let index = 0; index < safeCount; index += 1) {',
      '    const element = safeElements.nth(index)',
      '    const label = (await element.textContent())?.trim().replace(/\\s+/g, " ").slice(0, 80) || `hero element ${index}`',
      '    const box = await element.boundingBox()',
      '    if (!box) throw new Error(`Home mobile hero element has no bounding box: ${label}`)',
      '    const left = Math.round(box.x)',
      '    const right = Math.round(box.x + box.width)',
      '    const top = Math.round(box.y)',
      '    const bottom = Math.round(box.y + box.height)',
      '    if (left < 0 || top < 0 || right > viewport.width || bottom > viewport.height) {',
      '      throw new Error(`Home mobile hero element is clipped: ${label} at ${left},${top},${right},${bottom} within ${viewport.width}x${viewport.height}`)',
      '    }',
      '  }',
      '}',
      'await page.setViewportSize({ width: 1280, height: 720 })',
    ].join('\n'),
  ])
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
      'const assertPageChromeRoundControl = (name, box) => {',
      '  if (!box) throw new Error(`${name} has no bounding box`)',
      '  if (Math.abs(box.width - 40) > 1 || Math.abs(box.height - 40) > 1) {',
      '    throw new Error(`${name} should use the 40px page chrome round-control contract, got ${box.width}x${box.height}`)',
      '  }',
      '}',
      'const accountButton = page.getByRole("button", { name: "账户菜单" }).first()',
      'await accountButton.waitFor({ state: "visible", timeout: 10000 })',
      'const accountButtonBox = await accountButton.boundingBox()',
      'assertPageChromeRoundControl("Boards account button", accountButtonBox)',
      'const viewportSize = page.viewportSize()',
      'if (!viewportSize || !accountButtonBox) throw new Error("Boards account chrome metrics are unavailable")',
      'const chromeInset = await page.evaluate(() => Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--spacing-ib-page-chrome-inset")))',
      'if (!Number.isFinite(chromeInset)) throw new Error("Page chrome inset token is not readable on boards page")',
      'const accountRight = viewportSize.width - (accountButtonBox.x + accountButtonBox.width)',
      'if (Math.abs(accountButtonBox.y - chromeInset) > 1 || Math.abs(accountRight - chromeInset) > 1) {',
      '  throw new Error(`Boards account button should align to ${chromeInset}px page chrome inset, got top=${accountButtonBox.y}, right=${accountRight}`)',
      '}',
      'await accountButton.click()',
      'const adminMenuItem = page.getByText("管理后台", { exact: true }).first()',
      'if (await adminMenuItem.isVisible().catch(() => false)) {',
      '  throw new Error("Regular user should not see admin menu item on boards page")',
      '}',
      'await page.keyboard.press("Escape")',
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
      'const layout = page.locator(".canvas-layout").first()',
      'const flowWrap = page.locator(".canvas-flow-wrap").first()',
      'const controlGroup = page.locator(".canvas-control-group").first()',
      'const layoutBefore = await layout.boundingBox()',
      'const flowWrapBefore = await flowWrap.boundingBox()',
      'const controlGroupBefore = await controlGroup.boundingBox()',
      'if (!layoutBefore || !flowWrapBefore || !controlGroupBefore) throw new Error("Canvas layout metrics are unavailable before account menu opens")',
      'const flowWrapBackground = await flowWrap.evaluate((element) => getComputedStyle(element).backgroundImage)',
      'if (!flowWrapBackground.includes("radial-gradient")) throw new Error(`Canvas flow wrapper should own dot background, got ${flowWrapBackground}`)',
      'const vueFlowRoot = page.locator(".vue-flow").first()',
      'const vueFlowRootBackground = await vueFlowRoot.evaluate((element) => {',
      '  const style = getComputedStyle(element)',
      '  return { color: style.backgroundColor, image: style.backgroundImage }',
      '})',
      'if (vueFlowRootBackground.color !== "rgba(0, 0, 0, 0)" || vueFlowRootBackground.image !== "none") {',
      '  throw new Error(`Vue Flow root should stay transparent, got color=${vueFlowRootBackground.color}, image=${vueFlowRootBackground.image}`)',
      '}',
      'const vueFlowViewport = page.locator(".vue-flow__viewport").first()',
      'const viewportBackground = await vueFlowViewport.evaluate((element) => getComputedStyle(element).backgroundImage)',
      'if (viewportBackground !== "none") throw new Error(`Vue Flow viewport should not own dot background, got ${viewportBackground}`)',
      'const header = page.locator(".canvas-page-chrome--header").first()',
      'const toolPalette = page.locator(".canvas-page-chrome--left").first()',
      'const zoomControls = page.locator(".canvas-page-chrome--status").first()',
      'const headerBox = await header.boundingBox()',
      'const toolPaletteBox = await toolPalette.boundingBox()',
      'const zoomControlsBox = await zoomControls.boundingBox()',
      'const viewportSize = page.viewportSize()',
      'if (!headerBox || !toolPaletteBox || !zoomControlsBox || !viewportSize) throw new Error("Canvas chrome metrics are unavailable")',
      'const chromeInset = await layout.evaluate((element) => Number.parseFloat(getComputedStyle(element).getPropertyValue("--spacing-ib-page-chrome-inset")))',
      'if (!Number.isFinite(chromeInset)) throw new Error("Page chrome inset token is not readable")',
      'const assertChromeInset = (name, actual) => {',
      '  if (Math.abs(actual - chromeInset) > 1) {',
      '    throw new Error(`${name} should use canvas chrome inset ${chromeInset}px, got ${actual}`)',
      '  }',
      '}',
      'const assertPageChromeRoundControl = (name, box) => {',
      '  if (!box) throw new Error(`${name} has no bounding box`)',
      '  if (Math.abs(box.width - 40) > 1 || Math.abs(box.height - 40) > 1) {',
      '    throw new Error(`${name} should use the 40px page chrome round-control contract, got ${box.width}x${box.height}`)',
      '  }',
      '}',
      'assertChromeInset("Canvas header top", headerBox.y)',
      'assertChromeInset("Canvas header left", headerBox.x)',
      'assertChromeInset("Canvas header right", viewportSize.width - (headerBox.x + headerBox.width))',
      'assertChromeInset("Canvas left toolbar left", toolPaletteBox.x)',
      'assertChromeInset("Canvas zoom right", viewportSize.width - (zoomControlsBox.x + zoomControlsBox.width))',
      'assertChromeInset("Canvas zoom bottom", viewportSize.height - (zoomControlsBox.y + zoomControlsBox.height))',
      'const accountButton = page.getByRole("button", { name: "账户菜单" }).first()',
      'const shareButton = page.getByRole("button", { name: "分享功能稍后接入" }).first()',
      'const exportButton = page.getByRole("button", { name: "导出" }).first()',
      'const selectToolButton = page.getByRole("button", { name: "选择 (V)" }).first()',
      'const zoomOutButton = page.getByRole("button", { name: "缩小" }).first()',
      'await accountButton.waitFor({ state: "visible", timeout: 10000 })',
      'const accountButtonBox = await accountButton.boundingBox()',
      'const shareButtonBox = await shareButton.boundingBox()',
      'const exportButtonBox = await exportButton.boundingBox()',
      'const selectToolButtonBox = await selectToolButton.boundingBox()',
      'const zoomOutButtonBox = await zoomOutButton.boundingBox()',
      'assertPageChromeRoundControl("Canvas account button", accountButtonBox)',
      'assertPageChromeRoundControl("Canvas share button", shareButtonBox)',
      'assertPageChromeRoundControl("Canvas export button", exportButtonBox)',
      'assertPageChromeRoundControl("Canvas left toolbar select button", selectToolButtonBox)',
      'if (!accountButtonBox || !zoomOutButtonBox) throw new Error("Canvas round-control metrics are unavailable")',
      'assertChromeInset("Canvas account button top", accountButtonBox.y)',
      'assertChromeInset("Canvas account button right", viewportSize.width - (accountButtonBox.x + accountButtonBox.width))',
      'if (Math.abs(zoomOutButtonBox.width - 23) > 1 || Math.abs(zoomOutButtonBox.height - 23) > 1) {',
      '  throw new Error(`Canvas zoom buttons are intentionally excluded from the 40px page chrome round-control contract; expected local 23px size, got ${zoomOutButtonBox.width}x${zoomOutButtonBox.height}`)',
      '}',
      'await page.getByRole("button", { name: "小地图" }).click()',
      'const minimapShell = page.locator(".canvas-minimap-shell").first()',
      'await minimapShell.waitFor({ state: "visible", timeout: 10000 })',
      'const minimapBox = await minimapShell.boundingBox()',
      'if (!minimapBox) throw new Error("Canvas minimap metrics are unavailable")',
      'assertChromeInset("Canvas minimap left", minimapBox.x)',
      'assertChromeInset("Canvas minimap bottom", viewportSize.height - (minimapBox.y + minimapBox.height))',
      'const scrollWidthBefore = await page.evaluate(() => document.documentElement.scrollWidth)',
      'await accountButton.click()',
      'const userMenu = page.getByRole("menu").filter({ hasText: "个人中心" })',
      'await userMenu.waitFor({ state: "visible", timeout: 10000 })',
      'await page.waitForTimeout(120)',
      'const layoutAfter = await layout.boundingBox()',
      'const flowWrapAfter = await flowWrap.boundingBox()',
      'const controlGroupAfter = await controlGroup.boundingBox()',
      'if (!layoutAfter || !flowWrapAfter || !controlGroupAfter) throw new Error("Canvas layout metrics are unavailable after account menu opens")',
      'const scrollWidthAfter = await page.evaluate(() => document.documentElement.scrollWidth)',
      'const assertStableMetric = (name, before, after) => {',
      '  if (Math.abs(before - after) > 1) {',
      '    throw new Error(`${name} shifted after account menu opened: before=${before}, after=${after}`)',
      '  }',
      '}',
      'assertStableMetric("Canvas layout x", layoutBefore.x, layoutAfter.x)',
      'assertStableMetric("Canvas layout width", layoutBefore.width, layoutAfter.width)',
      'assertStableMetric("Canvas flow x", flowWrapBefore.x, flowWrapAfter.x)',
      'assertStableMetric("Canvas flow width", flowWrapBefore.width, flowWrapAfter.width)',
      'assertStableMetric("Canvas control group x", controlGroupBefore.x, controlGroupAfter.x)',
      'assertStableMetric("Canvas document scrollWidth", scrollWidthBefore, scrollWidthAfter)',
      'const canvasMenuStyle = await userMenu.evaluate((element) => {',
      '  const menuElement = element instanceof HTMLElement ? element : null',
      '  const panel = menuElement?.closest(".home-canvas-user-menu")',
      '  const style = panel ? getComputedStyle(panel) : null',
      '  return {',
      '    backgroundColor: style?.backgroundColor ?? "",',
      '    borderTopWidth: style?.borderTopWidth ?? "",',
      '    width: panel instanceof HTMLElement ? panel.getBoundingClientRect().width : 0,',
      '    top: panel instanceof HTMLElement ? panel.getBoundingClientRect().top : 0,',
      '  }',
      '})',
      'if (canvasMenuStyle.width < 112 || canvasMenuStyle.width > 180) {',
      '  throw new Error(`Canvas account menu width should stay compact, got ${canvasMenuStyle.width}`)',
      '}',
      'const verticalGap = canvasMenuStyle.top - (accountButtonBox.y + accountButtonBox.height)',
      'if (verticalGap < 8 || verticalGap > 12) {',
      '  throw new Error(`Canvas account menu vertical gap should be about 10px, got ${verticalGap}`)',
      '}',
      'if (canvasMenuStyle.backgroundColor === "rgba(0, 0, 0, 0)" || canvasMenuStyle.backgroundColor === "transparent") {',
      '  throw new Error("Canvas account menu lost its panel background")',
      '}',
      'if (canvasMenuStyle.borderTopWidth === "0px") {',
      '  throw new Error("Canvas account menu lost its panel border")',
      '}',
      'const adminMenuItem = page.getByText("管理后台", { exact: true }).first()',
      'if (await adminMenuItem.isVisible().catch(() => false)) {',
      '  throw new Error("Regular user should not see admin menu item on canvas page")',
      '}',
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

function assertAdminAccountMenuNavigation(
  playwrightCli: CommandSpec,
  pagePath: string,
  expectedText: string,
) {
  smoke.runCommand(playwrightCli, ['goto', `${smoke.baseUrl}${pagePath}`])
  smoke.runCommand(playwrightCli, [
    'run-code',
    [
      `await page.getByText(${JSON.stringify(expectedText)}).first().waitFor({ state: "visible", timeout: 15000 })`,
      'const accountButton = page.getByRole("button", { name: "账户菜单" }).first()',
      'await accountButton.waitFor({ state: "visible", timeout: 10000 })',
      'await accountButton.click()',
      'const adminMenuItem = page.getByText("管理后台", { exact: true }).first()',
      'await adminMenuItem.waitFor({ state: "visible", timeout: 10000 })',
      'await adminMenuItem.click()',
      'await page.waitForURL((url) => url.pathname.startsWith("/admin"), { timeout: 10000 })',
    ].join('\n'),
  ])
}

function assertAdminAccountMenus(playwrightCli: CommandSpec) {
  setSessionUserRoute(playwrightCli, MOCK_ADMIN)
  assertAdminAccountMenuNavigation(playwrightCli, '/user/boards', MOCK_BOARD_NAME)
  assertAdminAccountMenuNavigation(playwrightCli, `/user/canvas?boardId=${MOCK_BOARD_ID}`, MOCK_BOARD_NAME)
}

function smokePage(playwrightCli: CommandSpec, page: SmokePage) {
  smoke.runCommand(playwrightCli, ['goto', `${smoke.baseUrl}${page.path}`])
  waitForExpectedText(playwrightCli, page)
  const snapshot = smoke.runCommand(playwrightCli, ['snapshot']).stdout
  assertPageReached(snapshot, page)

  if (page.label === 'boards') {
    assertBoardsInteractions(playwrightCli)
  }

  if (page.label === 'home') {
    assertHomeMobileHeroLayout(playwrightCli)
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
    assertAdminAccountMenus(playwrightCli)

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
