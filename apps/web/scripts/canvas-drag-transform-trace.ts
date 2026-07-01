import type { User, UserBoardItem } from '../src/shared/api/generated-query/model'
import { createMockSmokeRuntime, type CommandSpec } from './mock-smoke-utils'

const smoke = createMockSmokeRuntime({
  hostEnvName: 'CANVAS_DRAG_TRACE_HOST',
  portEnvName: 'CANVAS_DRAG_TRACE_PORT',
  verboseEnvName: 'CANVAS_DRAG_TRACE_VERBOSE',
  sessionPrefix: 'cdt',
  defaultPort: 5193,
})

const MOCK_USER = {
  id: 1,
  uid: 'USM001',
  username: 'mock-user',
  role: 'user',
  auth_user_id: 'auth-canvas-drag-trace',
  avatar: '',
  status: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
} satisfies User

const MOCK_BOARD_ID = 'mock-drag-trace-board'
const MOCK_BOARD_NAME = 'Mock Drag Trace Board'
const MOCK_UPDATED_AT = 1_764_000_000_000

const AUTH_SESSION = {
  session: { id: 'canvas-drag-trace-session' },
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

const BOARD_UPLOAD = {
  code: 0,
  msg: 'ok',
  data: {
    url: `/uploads/canvas/${MOCK_USER.uid}/${MOCK_BOARD_ID}/content/mock-upload.png`,
    width: 512,
    height: 512,
    size: 1024,
    mime: 'image/png',
  },
}

function setupBrowser(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, ['open', 'about:blank'])
  smoke.routeJson(playwrightCli, '**/api/auth/get-session', AUTH_SESSION)
  smoke.routeJson(playwrightCli, '**/api/settings/branding', SITE_BRANDING_SETTING)
  smoke.routeJson(playwrightCli, '**/api/user/info', USER_INFO)
  smoke.routeJson(playwrightCli, '**/api/user/boards', USER_BOARDS)
  smoke.routeJson(playwrightCli, `**/api/user/boards/${MOCK_BOARD_ID}`, {
    code: 0,
    msg: 'ok',
    data: {
      board: MOCK_BOARD,
    },
  })
  smoke.routeJson(playwrightCli, `**/api/user/boards/${MOCK_BOARD_ID}/uploads`, BOARD_UPLOAD)
}

function runPageCode(playwrightCli: CommandSpec, code: string, timeoutMs = 60_000): string {
  const result = smoke.runCommand(
    playwrightCli,
    [
      'run-code',
      `async (page) => {
${code}
}`,
    ],
    { timeoutMs },
  )
  if (result.stdout.includes('### Error')) {
    throw new Error(result.stdout)
  }
  return result.stdout
}

function readEvalJson<T>(playwrightCli: CommandSpec, expression: string): T {
  const output = smoke.runCommand(playwrightCli, ['eval', `JSON.stringify(${expression})`]).stdout
  const match = output.match(/### Result\n([\s\S]*?)\n### Ran Playwright code/)
  if (!match) {
    throw new Error(`Eval result not found in output\n${output}`)
  }
  return JSON.parse(JSON.parse(match[1].trim())) as T
}

function prepareOriginStorage(playwrightCli: CommandSpec) {
  smoke.runCommand(playwrightCli, ['goto', `${smoke.baseUrl}/user/home`])
  runPageCode(
    playwrightCli,
    `
await page.evaluate(async (user) => {
  localStorage.clear()
  sessionStorage.clear()
  localStorage.setItem('user', JSON.stringify(user))
  if (!('indexedDB' in window)) return
  const databases = typeof indexedDB.databases === 'function' ? await indexedDB.databases() : []
  await Promise.all(databases.map((db) => new Promise((resolve) => {
    if (!db.name) return resolve(undefined)
    const request = indexedDB.deleteDatabase(db.name)
    request.onsuccess = request.onerror = request.onblocked = () => resolve(undefined)
  })))
}, ${JSON.stringify(MOCK_USER)})
`,
  )
}

function installTraceHooks(playwrightCli: CommandSpec) {
  runPageCode(
    playwrightCli,
    `
await page.evaluate(() => {
  window.__canvasDragTrace = {
    raf: [],
    mutations: [],
    events: [],
  }

  const parseTranslate = (transform) => {
    const matrix = new DOMMatrixReadOnly(transform === 'none' ? undefined : transform)
    return { x: Math.round(matrix.m41 * 1000) / 1000, y: Math.round(matrix.m42 * 1000) / 1000 }
  }

  const readNode = () => {
    const node = document.querySelector('.vue-flow__node')
    if (!node) return null
    const style = window.getComputedStyle(node)
    const transform = style.transform
    const rect = node.getBoundingClientRect()
    const translate = parseTranslate(transform)
    return {
      t: Math.round(performance.now() * 1000) / 1000,
      id: node.getAttribute('data-id'),
      className: node.className,
      inlineTransform: node.style.transform,
      transform,
      translate,
      rect: {
        x: Math.round(rect.x * 1000) / 1000,
        y: Math.round(rect.y * 1000) / 1000,
        width: Math.round(rect.width * 1000) / 1000,
        height: Math.round(rect.height * 1000) / 1000,
      },
    }
  }

  window.__canvasDragTrace.readNode = readNode
  const observer = new MutationObserver((mutations) => {
    const snapshot = readNode()
    if (!snapshot) return
    window.__canvasDragTrace.mutations.push({
      ...snapshot,
      mutationTypes: mutations.map((mutation) => mutation.type),
      mutationAttributes: mutations.map((mutation) => mutation.attributeName),
    })
  })
  const node = document.querySelector('.vue-flow__node')
  if (node) observer.observe(node, { attributes: true, attributeFilter: ['style', 'class'] })
  window.__canvasDragTrace.observer = observer
  document.querySelector('.vue-flow')?.addEventListener(
    'mousemove',
    () => {
      window.__canvasDragTrace.events.push({ t: performance.now(), type: 'mousemove' })
    },
    { passive: true },
  )
})
`,
  )
}

function traceDrag(playwrightCli: CommandSpec): unknown {
  runPageCode(
    playwrightCli,
    `
await page.waitForSelector('.vue-flow__node', { timeout: 15000 })
`,
  )
  installTraceHooks(playwrightCli)
  runPageCode(
    playwrightCli,
    `
const result = await page.evaluate(async () => {
  const trace = window.__canvasDragTrace
  const node = document.querySelector('.vue-flow__node')
  if (!node || !trace?.readNode) throw new Error('Missing canvas node trace target')
  const header = node.querySelector('.workflow-node-header')
  if (!header) throw new Error('Missing prompt node drag header')
  const before = trace.readNode()
  const box = header.getBoundingClientRect()
  const start = { x: box.right - Math.min(28, box.width / 3), y: box.top + box.height / 2 }
  const end = { x: start.x + 260, y: start.y + 120 }
  trace.raf.length = 0
  trace.mutations.length = 0
  trace.events.length = 0
  trace.before = before
  trace.drag = { start, end }
  trace.isSampling = true
  const sample = () => {
    const snapshot = trace.readNode()
    if (snapshot) trace.raf.push(snapshot)
    if (trace.isSampling) requestAnimationFrame(sample)
  }
  requestAnimationFrame(sample)
  return trace.drag
})
await page.mouse.move(result.start.x, result.start.y)
await page.mouse.down()
await page.mouse.move(result.end.x, result.end.y, { steps: 18 })
await page.mouse.up()
await page.waitForTimeout(2500)
await page.evaluate(() => {
  const trace = window.__canvasDragTrace
  trace.isSampling = false
  trace.after = trace.readNode()
})
`,
    90_000,
  )
  runPageCode(
    playwrightCli,
    `
await page.evaluate(() => {
  const trace = window.__canvasDragTrace
  trace.observer?.disconnect()
  trace.result = {
    before: trace.before,
    after: trace.after,
    drag: trace.drag,
    raf: trace.raf,
    positionChanges: trace.raf.filter((sample, index, samples) => {
      const prev = samples[index - 1]
      return !prev || prev.translate.x !== sample.translate.x || prev.translate.y !== sample.translate.y
    }),
    mutations: trace.mutations,
    events: trace.events,
  }
})
`,
  )
  return readEvalJson(playwrightCli, 'window.__canvasDragTrace.result')
}

async function main() {
  let playwrightCli: CommandSpec | null = null
  try {
    playwrightCli = smoke.resolvePlaywrightCli()
    await smoke.startDevServer()
    setupBrowser(playwrightCli)
    prepareOriginStorage(playwrightCli)
    smoke.runCommand(playwrightCli, ['goto', `${smoke.baseUrl}/user/canvas?boardId=${MOCK_BOARD_ID}`])
    runPageCode(
      playwrightCli,
      `
await page.getByText(${JSON.stringify(MOCK_BOARD_NAME)}).waitFor({ timeout: 15000 })
await page.waitForSelector('.vue-flow__node', { timeout: 15000 })
`,
    )

    const trace = traceDrag(playwrightCli)
    console.log(JSON.stringify(trace, null, 2))
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
