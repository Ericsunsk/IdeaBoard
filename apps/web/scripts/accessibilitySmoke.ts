import AxeBuilder from '@axe-core/playwright'
import type { Page } from 'playwright'
import type {
  BoardStateEnvelope,
  User,
  UserBoardItem,
} from '../src/shared/api/generated-query/model'
import { CANVAS_BOARD_STATE_SCHEMA_VERSION } from '../src/features/canvas-board/domain'
import { createBrowserSmokeRuntime } from './browserSmokeRuntime'
import { assertNoConsoleErrors } from './mockSmokeUtils'

const smoke = createBrowserSmokeRuntime({
  hostEnvName: 'A11Y_SMOKE_HOST',
  portEnvName: 'A11Y_SMOKE_PORT',
  verboseEnvName: 'A11Y_SMOKE_VERBOSE',
})

const MOCK_USER: User = {
  public_id: '11111111-1111-4111-8111-111111111111',
  display_name: 'A11y Test User',
  storage_namespace: 'USM001',
  role: 'user',
  avatar:
    'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mPs6+v7DwAFgwJ/lk5c7wAAAABJRU5ErkJggg==',
  status: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

const MOCK_ADMIN: User = {
  ...MOCK_USER,
  public_id: '22222222-2222-4222-8222-222222222222',
  display_name: 'A11y Test Admin',
  role: 'admin',
}

const MOCK_BOARD_ID = 'mock-board-a11y'
const MOCK_BOARD_NAME = 'Accessibility Smoke Canvas'
const MOCK_UPDATED_AT = 1_764_000_000_000

const MOCK_BOARD_STATE: BoardStateEnvelope = {
  schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
  imageNodes: [],
  promptNodes: [
    {
      id: 1,
      type: 'prompt',
      x: 160,
      y: 140,
      width: 320,
      title: 'A11y Prompt',
      text: 'A test prompt for accessibility checking',
      outputCount: 1,
      aspectRatio: '1:1',
      inputImages: [],
    },
  ],
  fusionNodes: [],
  connections: [],
  groups: [],
  updatedAt: MOCK_UPDATED_AT,
}

const MOCK_BOARD: UserBoardItem = {
  board_id: MOCK_BOARD_ID,
  name: MOCK_BOARD_NAME,
  state: MOCK_BOARD_STATE,
  access_role: 'owner',
  revision: 1,
  state_schema_version: CANVAS_BOARD_STATE_SCHEMA_VERSION,
  created_at: MOCK_UPDATED_AT - 86_400_000,
  updated_at: MOCK_UPDATED_AT,
}

function createAuthSession(user: User | null) {
  if (!user) return { session: null, user: null }
  return {
    session: { id: `a11y-smoke-session-${user.public_id}` },
    user: { id: user.public_id, name: user.display_name, role: user.role },
  }
}

function createUserInfo(user: User | null) {
  return {
    code: 0,
    msg: 'ok',
    data: user ? { user } : null,
  }
}

async function runAxeCheck(page: Page, contextName: string, options: { exclude?: string[] } = {}) {
  let builder = new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa'])
  if (options.exclude) {
    for (const selector of options.exclude) {
      builder = builder.exclude(selector)
    }
  }

  const results = await builder.analyze()
  const severeViolations = results.violations.filter(
    (v) => v.impact === 'critical' || v.impact === 'serious',
  )

  if (severeViolations.length > 0) {
    const report = severeViolations
      .map(
        (v) =>
          `  [${v.impact?.toUpperCase()}] rule: "${v.id}" - ${v.help} (${v.helpUrl})\n` +
          v.nodes
            .map((n) => `    - Element: ${n.html}\n      Summary: ${n.failureSummary}`)
            .join('\n'),
      )
      .join('\n\n')
    throw new Error(`Accessibility violations found in [${contextName}]:\n${report}`)
  }

  return results.violations
}

async function setupRoutes(page: Page, currentUser: User | null) {
  await page.unroute('**/*').catch(() => undefined)

  await smoke.routeJson('**/api/settings/branding', {
    code: 0,
    msg: 'ok',
    data: { project_name: 'IdeaBoard', logo_url: '' },
  })
  await smoke.routeJson('**/api/auth/capabilities', {
    code: 0,
    msg: 'ok',
    data: {
      password_enabled: true,
      oauth_providers: ['google', 'github'],
    },
  })
  await smoke.routeJson('**/api/auth/get-session', createAuthSession(currentUser))
  await smoke.routeJson('**/api/user/info', createUserInfo(currentUser))
  await smoke.routeJson('**/api/user/credits/wallet', {
    code: 0,
    msg: 'ok',
    data: {
      public_id: currentUser?.public_id || '',
      display_name: currentUser?.display_name || '',
      available_credits: 500,
      reserved_credits: 0,
    },
  })
  await smoke.routeJson('**/api/user/billing/records**', {
    code: 0,
    msg: 'ok',
    data: { records: [] },
  })
  await smoke.routeJson('**/api/user/boards', {
    code: 0,
    msg: 'ok',
    data: { list: [MOCK_BOARD] },
  })
  await smoke.routeJson(`**/api/user/boards/${MOCK_BOARD_ID}`, {
    code: 0,
    msg: 'ok',
    data: { board: MOCK_BOARD },
  })
  await smoke.routeJson('**/api/user/tasks/submit/readiness', {
    code: 0,
    msg: 'ok',
    data: {
      ready: true,
      feature: 'render',
      billing: {
        sku: 'ai.image.render',
        display_name: 'Sketch to image',
        price_version_id: 1,
        price_version: 1,
        unit_credits: 4,
        quantity: 1,
        total_credits: 4,
        quoted_at: '2026-07-12T00:00:00.000Z',
        available_credits: 500,
        reserved_credits: 0,
        sufficient: true,
      },
    },
  })
  await smoke.routeJson('**/api/admin/users**', {
    code: 0,
    msg: 'ok',
    data: {
      list: [
        {
          id: 1,
          public_id: MOCK_USER.public_id,
          display_name: MOCK_USER.display_name,
          role: 'user',
          status: 1,
          storage_namespace: 'USM001',
          created_at: '2026-01-01T00:00:00Z',
        },
      ],
      total: 1,
    },
  })
  await smoke.routeJson('**/api/admin/monitor**', {
    code: 0,
    msg: 'ok',
    data: { monitor: {}, stats: {} },
  })
  await smoke.routeJson('**/api/admin/statistics**', {
    code: 0,
    msg: 'ok',
    data: {
      model_health: [],
      slow_models: [],
      quality_trend: [],
      bottleneck_sources: [],
      failure_categories: [],
    },
  })
  await smoke.routeJson('**/api/admin/brand-logo-overrides**', {
    code: 0,
    msg: 'ok',
    data: { overrides: [] },
  })
}

async function verifyAuthenticationJourney(page: Page) {
  console.log('Testing Journey 1: Authentication & Guest Entry...')
  await setupRoutes(page, null)
  await page.goto(`${smoke.baseUrl}/user/login`)
  await page.waitForLoadState('networkidle')

  const authDialog = page.locator('[role="dialog"]').first()
  await authDialog.waitFor({ state: 'visible', timeout: 10000 })

  // Verify form labels and accessible inputs
  const emailInput = authDialog.locator('input#login-identifier').first()
  await emailInput.waitFor({ state: 'visible', timeout: 5000 })
  const passwordInput = authDialog.locator('input[type="password"]').first()
  await passwordInput.waitFor({ state: 'visible', timeout: 5000 })

  // Keyboard navigation check
  await emailInput.focus()
  await page.keyboard.type('test@example.com')
  await page.keyboard.press('Tab')
  const forgotBtn = authDialog.getByRole('button', { name: /Forgot|忘记/i }).first()
  const isForgotFocused = await forgotBtn.evaluate((el) => el === document.activeElement)
  if (!isForgotFocused) {
    throw new Error('Keyboard tab navigation failed: focus did not move to forgot password button')
  }
  await page.keyboard.press('Tab')
  const focusedPassword = await passwordInput.evaluate((el) => el === document.activeElement)
  if (!focusedPassword) {
    throw new Error('Keyboard tab navigation failed: focus did not move to password input')
  }

  // Run Axe accessibility scan
  await runAxeCheck(page, 'Authentication Sign In Dialog')
  console.log('✓ Journey 1 (Authentication) passed.')
}

async function verifyBoardsJourney(page: Page) {
  console.log('Testing Journey 2: Boards & Navigation...')
  await setupRoutes(page, MOCK_USER)
  await page.goto(`${smoke.baseUrl}/user/boards`)
  await page.waitForLoadState('networkidle')

  await page.getByText(MOCK_BOARD_NAME).first().waitFor({ state: 'visible', timeout: 15000 })
  const newCanvasBtn = page.getByRole('button', { name: 'New Canvas' }).first()
  await newCanvasBtn.waitFor({ state: 'visible', timeout: 5000 })

  // Keyboard navigation check
  await newCanvasBtn.focus()
  const isNewCanvasFocused = await newCanvasBtn.evaluate((el) => el === document.activeElement)
  if (!isNewCanvasFocused) {
    throw new Error('Keyboard focus failed on "New Canvas" button')
  }

  await runAxeCheck(page, 'Boards Dashboard')
  console.log('✓ Journey 2 (Boards) passed.')
}

async function verifyCanvasAndMediaGenerationJourney(page: Page) {
  console.log('Testing Journey 3: Canvas Workspace & Media Generation...')
  await setupRoutes(page, MOCK_USER)
  await page.goto(`${smoke.baseUrl}/user/canvas?boardId=${MOCK_BOARD_ID}`)
  await page.waitForLoadState('networkidle')

  await page.locator('.vue-flow').first().waitFor({ state: 'visible', timeout: 15000 })
  await page.getByText(MOCK_BOARD_NAME).first().waitFor({ state: 'visible', timeout: 10000 })

  // Prompt composer node checks
  const promptTextarea = page.locator('.vue-flow__node textarea').first()
  await promptTextarea.waitFor({ state: 'visible', timeout: 10000 })

  // Check interactive controls have accessible names
  const zoomInBtn = page.getByRole('button', { name: 'Zoom In' }).first()
  const zoomOutBtn = page.getByRole('button', { name: 'Zoom Out' }).first()
  await zoomInBtn.waitFor({ state: 'visible' })
  await zoomOutBtn.waitFor({ state: 'visible' })

  // Run Axe accessibility on Canvas
  await runAxeCheck(page, 'Canvas Workspace')
  console.log('✓ Journey 3 (Canvas & Media Generation) passed.')
}

async function verifyBillingDialogJourney(page: Page) {
  console.log('Testing Journey 4: Credits and Billing Focus Trap...')
  await setupRoutes(page, MOCK_USER)
  await page.goto(`${smoke.baseUrl}/user/boards`)
  await page.waitForLoadState('networkidle')

  const accountButton = page.getByRole('button', { name: 'Account menu' }).first()
  await accountButton.waitFor({ state: 'visible', timeout: 10000 })
  await accountButton.click()

  const balanceButton = page.getByRole('button', { name: /Ideas balance/ }).first()
  await balanceButton.waitFor({ state: 'visible', timeout: 10000 })
  await balanceButton.click()

  const billingDialog = page.getByRole('dialog', { name: 'Account' })
  await billingDialog.waitFor({ state: 'visible', timeout: 10000 })

  const transactionsTab = billingDialog.getByRole('button', { name: 'Transactions' }).first()
  await transactionsTab.waitFor({ state: 'visible', timeout: 10000 })

  // Focus trap verification: tab navigates inside the dialog
  await page.keyboard.press('Tab')
  const isFocusInside = await billingDialog.evaluate((dialog) =>
    dialog.contains(document.activeElement),
  )
  if (!isFocusInside) {
    throw new Error(
      'Focus management failed: active element is not contained within the billing dialog',
    )
  }

  // Wait for dialog enter transition to complete and opacity to settle at 1
  await page.waitForTimeout(400)

  // Run Axe accessibility scan on Billing Dialog
  await runAxeCheck(page, 'Billing & Account Dialog')

  // Test Escape key closes dialog
  await page.keyboard.press('Escape')
  await billingDialog.waitFor({ state: 'hidden', timeout: 10000 })
  console.log('✓ Journey 4 (Credits & Billing Focus Trap) passed.')
}

async function verifyAdminJourney(page: Page) {
  console.log('Testing Journey 5: Admin Console...')
  await setupRoutes(page, MOCK_ADMIN)
  await page.goto(`${smoke.baseUrl}/admin/users`)
  await page.waitForLoadState('networkidle')

  await page.getByText(MOCK_USER.display_name).first().waitFor({ state: 'visible', timeout: 15000 })
  await runAxeCheck(page, 'Admin Users Table')
  console.log('✓ Journey 5 (Admin Console) passed.')
}

async function main() {
  console.log('=== Starting Accessibility & Critical User Journey Smoke Tests ===\n')
  await smoke.startDevServer()
  await smoke.startBrowser()
  const page = smoke.getPage()

  try {
    await verifyAuthenticationJourney(page)
    await verifyBoardsJourney(page)
    await verifyCanvasAndMediaGenerationJourney(page)
    await verifyBillingDialogJourney(page)
    await verifyAdminJourney(page)

    const consoleOutput = (await smoke.runCommand(['console', 'error'])).stdout
    assertNoConsoleErrors(consoleOutput, 'accessibility-smoke')

    console.log(
      '\n✓ All 5 critical user journeys passed accessibility, keyboard, and focus smoke checks.',
    )
  } finally {
    await smoke.closeBrowser()
    await smoke.stopDevServer()
  }
}

main().catch((error: unknown) => {
  console.error('\n✗ Accessibility smoke test failed:\n', error)
  process.exit(1)
})
