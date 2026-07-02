/// <reference lib="dom" />

import type { User, UserBoardItem } from '../src/shared/api/generated-query/model'
import type {
  CanvasBoardState,
  CanvasConnection,
  FusionNodeItem,
  ImageNodeItem,
  PromptNodeItem,
} from '../src/features/canvas-board/domain'
import { createMockSmokeRuntime, type CommandSpec } from './mock-smoke-utils'

type BenchmarkPath = 'node-drag' | 'viewport-pan' | 'viewport-zoom'

type TargetSnapshot = {
  t: number
  inlineTransform: string
  transform: string
  translate: {
    x: number
    y: number
  }
  scale: {
    x: number
    y: number
  }
  rect: {
    x: number
    y: number
    width: number
    height: number
  }
}

type RawInteractionTrace = {
  path: BenchmarkPath
  durationMs: number
  frameDeltas: number[]
  mutationCount: number
  eventCount: number
  eventsByType: Record<string, number>
  samples: TargetSnapshot[]
  before: TargetSnapshot | null
  after: TargetSnapshot | null
}

type InteractionMetrics = {
  path: BenchmarkPath
  p95FrameMs: number
  droppedFrames: number
  durationMs: number
  avgFrameMs: number
  maxFrameMs: number
  sampleCount: number
  mutationCount: number
  eventCount: number
  positionChanges?: number
  viewportChanges?: number
}

type BenchmarkReport = {
  schemaVersion: 1
  generatedAt: string
  command: string
  browser: {
    engine: 'chromium'
    source: string
  }
  viewport: typeof BENCHMARK_VIEWPORT
  fixture: {
    name: string
    nodeCount: number
    edgeCount: number
    imageNodes: number
    promptNodes: number
    fusionNodes: number
  }
  smoothnessBudget: {
    frameBudgetMs: number
    ranking: string
  }
  paths: InteractionMetrics[]
  worstPath: InteractionMetrics
}

const BENCHMARK_VIEWPORT = {
  width: 1440,
  height: 900,
} as const
const FRAME_BUDGET_MS = 1000 / 60
const NODE_COUNT_BY_KIND = 50
const GRID_COLUMNS = 15
const GRID_X_GAP = 340
const GRID_Y_GAP = 320
const NODE_DRAG_TARGET_ID = 20
const MOCK_UPDATED_AT = 1_764_000_000_000

const smoke = createMockSmokeRuntime({
  hostEnvName: 'CANVAS_INTERACTION_BENCHMARK_HOST',
  portEnvName: 'CANVAS_INTERACTION_BENCHMARK_PORT',
  verboseEnvName: 'CANVAS_INTERACTION_BENCHMARK_VERBOSE',
  sessionPrefix: 'cib',
  defaultPort: 5194,
})

const MOCK_USER = {
  id: 1,
  uid: 'USM001',
  username: 'mock-user',
  role: 'user',
  auth_user_id: 'auth-canvas-interaction-benchmark',
  avatar: '',
  status: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
} satisfies User

const MOCK_BOARD_ID = 'mock-canvas-interaction-benchmark'
const MOCK_BOARD_NAME = 'Canvas Interaction Benchmark Board'
const MOCK_BOARD_STATE = createMediumLargeCanvasBoardState()

const AUTH_SESSION = {
  session: { id: 'canvas-interaction-benchmark-session' },
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

const MOCK_BOARD = {
  board_id: MOCK_BOARD_ID,
  name: MOCK_BOARD_NAME,
  state: toApiBoardState(MOCK_BOARD_STATE),
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

function createMediumLargeCanvasBoardState(): CanvasBoardState {
  const imageNodes: ImageNodeItem[] = []
  const promptNodes: PromptNodeItem[] = []
  const fusionNodes: FusionNodeItem[] = []

  for (let index = 0; index < NODE_COUNT_BY_KIND; index += 1) {
    const id = index + 1
    imageNodes.push(createImageNode(id, index))
    promptNodes.push(createPromptNode(id, index + NODE_COUNT_BY_KIND))
    fusionNodes.push(createFusionNode(id, index + NODE_COUNT_BY_KIND * 2))
  }

  return {
    schemaVersion: 1,
    imageNodes,
    promptNodes,
    fusionNodes,
    connections: createBenchmarkConnections(),
    viewport: {
      canvasX: 72,
      canvasY: 34,
      zoomLevel: 50,
    },
    updatedAt: MOCK_UPDATED_AT,
  }
}

function createImageNode(id: number, gridIndex: number): ImageNodeItem {
  const position = getGridPosition(gridIndex)
  return {
    id,
    type: 'image-node',
    x: position.x,
    y: position.y,
    width: 280,
    title: `Benchmark Image ${id}`,
    sourceImageUrl: createImageDataUrl(id),
    sourceImageRatio: 1,
    outputCount: 1,
    outputStatus: 'ready',
    outputImages: [{ url: createImageDataUrl(id + 100), ratio: 1 }],
  }
}

function createPromptNode(id: number, gridIndex: number): PromptNodeItem {
  const position = getGridPosition(gridIndex)
  return {
    id,
    type: 'prompt',
    x: position.x,
    y: position.y,
    width: 280,
    title: `Benchmark Prompt ${id}`,
    text: `Prompt fixture ${id}: build a layered visual idea with repeatable Canvas Board state.`,
    outputCount: 1,
    aspectRatio: '1:1',
    inputImages: [
      {
        id: `prompt-input-${id}`,
        sourceType: 'node',
        sourceId: `image-${id}`,
        sourceNodeId: `image-${id}`,
        url: createImageDataUrl(id),
        ratio: 1,
      },
    ],
  }
}

function createFusionNode(id: number, gridIndex: number): FusionNodeItem {
  const position = getGridPosition(gridIndex)
  return {
    id,
    type: 'fusion',
    x: position.x,
    y: position.y,
    width: 286,
    title: `Benchmark Fusion ${id}`,
    prompt: `Fusion fixture ${id}`,
    outputCount: 1,
    aspectRatio: '1:1',
    blendPoint: { x: 0.5, y: 0.5 },
    inputImages: [0, 1, 2, 3].map((offset) => {
      const sourceId = wrapNodeId(id + offset)
      return {
        id: `fusion-input-${id}-${offset}`,
        sourceType: 'node',
        sourceId: `image-${sourceId}`,
        sourceNodeId: `image-${sourceId}`,
        url: createImageDataUrl(sourceId),
        ratio: 1,
      }
    }),
    outputStatus: 'ready',
    outputImages: [{ url: createImageDataUrl(id + 200), ratio: 1 }],
  }
}

function createBenchmarkConnections(): CanvasConnection[] {
  const connections: CanvasConnection[] = []
  let connectionId = 1
  for (let id = 1; id <= NODE_COUNT_BY_KIND; id += 1) {
    connections.push({
      id: connectionId,
      fromNodeType: 'image',
      fromNodeId: id,
      fromPort: 'image',
      toNodeType: 'prompt',
      toNodeId: id,
      toPort: 'image',
    })
    connectionId += 1
    connections.push({
      id: connectionId,
      fromNodeType: 'prompt',
      fromNodeId: id,
      fromPort: 'prompt',
      toNodeType: 'fusion',
      toNodeId: id,
      toPort: 'prompt',
    })
    connectionId += 1
    for (let offset = 0; offset < 2; offset += 1) {
      connections.push({
        id: connectionId,
        fromNodeType: 'image',
        fromNodeId: wrapNodeId(id + offset),
        fromPort: 'image',
        toNodeType: 'fusion',
        toNodeId: id,
        toPort: 'image',
      })
      connectionId += 1
    }
    if (id <= 40) {
      connections.push({
        id: connectionId,
        fromNodeType: 'image',
        fromNodeId: wrapNodeId(id + 2),
        fromPort: 'image',
        toNodeType: 'fusion',
        toNodeId: id,
        toPort: 'image',
      })
      connectionId += 1
    }
  }
  return connections
}

function getGridPosition(index: number) {
  const column = index % GRID_COLUMNS
  const row = Math.floor(index / GRID_COLUMNS)
  return {
    x: column * GRID_X_GAP,
    y: row * GRID_Y_GAP,
  }
}

function wrapNodeId(id: number) {
  return ((id - 1) % NODE_COUNT_BY_KIND) + 1
}

function toApiBoardState(state: CanvasBoardState): UserBoardItem['state'] {
  return {
    ...state,
    imageNodes: state.imageNodes.map((node) => ({ ...node })),
    promptNodes: state.promptNodes.map((node) => ({ ...node })),
    fusionNodes: state.fusionNodes.map((node) => ({ ...node })),
    connections: state.connections.map((connection) => ({ ...connection })),
    viewport: { ...state.viewport },
  }
}

function createImageDataUrl(seed: number): string {
  const hue = (seed * 37) % 360
  const svg = [
    `<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 96 96">`,
    `<rect width="96" height="96" rx="16" fill="hsl(${hue} 72% 64%)"/>`,
    `<circle cx="28" cy="32" r="18" fill="white" opacity=".34"/>`,
    `<path d="M14 76 39 48l17 16 12-13 18 25Z" fill="white" opacity=".58"/>`,
    `</svg>`,
  ].join('')
  return `data:image/svg+xml,${encodeURIComponent(svg)}`
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

function loadBenchmarkCanvas(playwrightCli: CommandSpec) {
  prepareOriginStorage(playwrightCli)
  smoke.runCommand(playwrightCli, ['goto', `${smoke.baseUrl}/user/canvas?boardId=${MOCK_BOARD_ID}`])
  runPageCode(
    playwrightCli,
    `
await page.setViewportSize(${JSON.stringify(BENCHMARK_VIEWPORT)})
const browserUserAgent = await page.evaluate(() => navigator.userAgent)
if (!/Chrome|Chromium/.test(browserUserAgent)) {
  throw new Error('Canvas interaction benchmark requires Chromium, got: ' + browserUserAgent)
}
await page.getByText(${JSON.stringify(MOCK_BOARD_NAME)}).waitFor({ timeout: 15000 })
await page.waitForSelector('.vue-flow__transformationpane', { timeout: 15000 })
await page.waitForSelector(${JSON.stringify(`.vue-flow__node[data-id="image-${NODE_DRAG_TARGET_ID}"] .workflow-node-header`)}, { timeout: 15000 })
await page.waitForFunction(() => document.querySelectorAll('.vue-flow__node').length >= 20, null, { timeout: 20000 })
await page.waitForTimeout(400)
`,
  )
}

function getInstallBenchmarkHarnessCode(): string {
  return `
await page.evaluate(() => {
  const round = (value) => Math.round(value * 10) / 10
  const roundCoord = (value) => Math.round(value * 1000) / 1000
  const parseTransform = (transform) => {
    const matrix = new DOMMatrixReadOnly(transform === 'none' ? undefined : transform)
    return {
      translate: { x: roundCoord(matrix.m41), y: roundCoord(matrix.m42) },
      scale: { x: roundCoord(matrix.a), y: roundCoord(matrix.d) },
    }
  }
  const readSnapshot = (target) => {
    const style = window.getComputedStyle(target)
    const transform = style.transform
    const parsed = parseTransform(transform)
    const rect = target.getBoundingClientRect()
    return {
      t: roundCoord(performance.now()),
      inlineTransform: target instanceof HTMLElement ? target.style.transform : '',
      transform,
      translate: parsed.translate,
      scale: parsed.scale,
      rect: {
        x: roundCoord(rect.x),
        y: roundCoord(rect.y),
        width: roundCoord(rect.width),
        height: roundCoord(rect.height),
      },
    }
  }

  window.__canvasInteractionBenchmark = {
    active: null,
    traces: {},
    begin(path, targetSelector) {
      if (this.active) this.end()
      const target = document.querySelector(targetSelector)
      if (!target) throw new Error('Missing benchmark target: ' + targetSelector)
      const eventTarget = document
      const frameDeltas = []
      const samples = []
      const mutations = []
      const eventsByType = {}
      let lastFrameAt
      let frameId
      let isSampling = true
      const sample = (timestamp) => {
        if (!isSampling) return
        if (lastFrameAt !== undefined) {
          const delta = timestamp - lastFrameAt
          if (Number.isFinite(delta) && delta >= 0) frameDeltas.push(delta)
        }
        lastFrameAt = timestamp
        samples.push(readSnapshot(target))
        frameId = requestAnimationFrame(sample)
      }
      const observer = new MutationObserver((entries) => {
        mutations.push(...entries.map((entry) => ({
          type: entry.type,
          attributeName: entry.attributeName,
          t: performance.now(),
        })))
      })
      const onEvent = (event) => {
        eventsByType[event.type] = (eventsByType[event.type] || 0) + 1
      }
      const eventTypes = ['pointerdown', 'pointermove', 'pointerup', 'mousemove', 'wheel', 'click']
      eventTypes.forEach((type) => {
        eventTarget.addEventListener(type, onEvent, { capture: true, passive: true })
      })
      observer.observe(target, { attributes: true, attributeFilter: ['style', 'class'] })
      const startedAt = performance.now()
      const before = readSnapshot(target)
      frameId = requestAnimationFrame(sample)
      this.active = {
        path,
        target,
        eventTarget,
        startedAt,
        before,
        frameDeltas,
        samples,
        mutations,
        eventsByType,
        eventTypes,
        onEvent,
        observer,
        get frameId() {
          return frameId
        },
        stopSampling() {
          isSampling = false
        },
      }
    },
    end() {
      const active = this.active
      if (!active) throw new Error('No active canvas interaction benchmark')
      active.stopSampling()
      if (active.frameId !== undefined) cancelAnimationFrame(active.frameId)
      active.observer.disconnect()
      active.eventTypes.forEach((type) => {
        active.eventTarget.removeEventListener(type, active.onEvent, { capture: true })
      })
      const endedAt = performance.now()
      const trace = {
        path: active.path,
        durationMs: round(endedAt - active.startedAt),
        frameDeltas: active.frameDeltas.map(round),
        mutationCount: active.mutations.length,
        eventCount: Object.values(active.eventsByType).reduce((total, count) => total + count, 0),
        eventsByType: active.eventsByType,
        samples: active.samples,
        before: active.before,
        after: readSnapshot(active.target),
      }
      this.traces[active.path] = trace
      this.active = null
      return trace
    },
  }
})
`
}

function runNodeDragBenchmark(playwrightCli: CommandSpec): RawInteractionTrace {
  loadBenchmarkCanvas(playwrightCli)
  runPageCode(
    playwrightCli,
    `
${getInstallBenchmarkHarnessCode()}
await page.getByRole('button', { name: '选择 (V)' }).click()
const drag = await page.evaluate(() => {
  const header = document.querySelector(${JSON.stringify(`.vue-flow__node[data-id="image-${NODE_DRAG_TARGET_ID}"] .workflow-node-header`)})
  if (!header) throw new Error('Missing node drag benchmark header')
  const box = header.getBoundingClientRect()
  const x = box.right - Math.min(28, box.width / 3)
  const y = box.top + box.height / 2
  return {
    start: { x, y },
    end: { x: x + 260, y: y + 120 },
  }
})
await page.evaluate(() => {
  window.__canvasInteractionBenchmark.begin('node-drag', ${JSON.stringify(`.vue-flow__node[data-id="image-${NODE_DRAG_TARGET_ID}"]`)})
})
await page.mouse.move(drag.start.x, drag.start.y)
await page.mouse.down()
await page.mouse.move(drag.end.x, drag.end.y, { steps: 36 })
await page.mouse.up()
await page.waitForTimeout(300)
await page.evaluate(() => window.__canvasInteractionBenchmark.end())
`,
    90_000,
  )
  return readEvalJson(playwrightCli, `window.__canvasInteractionBenchmark.traces['node-drag']`)
}

function runViewportPanBenchmark(playwrightCli: CommandSpec): RawInteractionTrace {
  loadBenchmarkCanvas(playwrightCli)
  runPageCode(
    playwrightCli,
    `
${getInstallBenchmarkHarnessCode()}
await page.getByRole('button', { name: '抓手 (H)' }).click()
const pan = await page.evaluate(() => {
  const pane = document.querySelector('.vue-flow__pane')
  if (!pane) throw new Error('Missing viewport pan benchmark pane')
  const box = pane.getBoundingClientRect()
  let start = null
  for (let y = box.top + 90; y < box.bottom - 90 && !start; y += 32) {
    for (let x = box.left + 120; x < box.right - 120; x += 32) {
      const element = document.elementFromPoint(x, y)
      if (!element) continue
      if (element.closest('.vue-flow__node, .canvas-page-chrome')) continue
      if (!pane.contains(element) && element !== pane) continue
      start = { x, y }
      break
    }
  }
  if (!start) throw new Error('Missing blank viewport pan start point')
  return {
    start,
    end: { x: start.x - 280, y: start.y - 140 },
  }
})
await page.evaluate(() => {
  window.__canvasInteractionBenchmark.begin('viewport-pan', '.vue-flow__transformationpane')
})
await page.mouse.move(pan.start.x, pan.start.y)
await page.mouse.down()
await page.mouse.move(pan.end.x, pan.end.y, { steps: 36 })
await page.mouse.up()
await page.waitForTimeout(300)
await page.evaluate(() => window.__canvasInteractionBenchmark.end())
`,
    90_000,
  )
  return readEvalJson(playwrightCli, `window.__canvasInteractionBenchmark.traces['viewport-pan']`)
}

function runViewportZoomBenchmark(playwrightCli: CommandSpec): RawInteractionTrace {
  loadBenchmarkCanvas(playwrightCli)
  runPageCode(
    playwrightCli,
    `
${getInstallBenchmarkHarnessCode()}
await page.evaluate(() => {
  window.__canvasInteractionBenchmark.begin('viewport-zoom', '.vue-flow__transformationpane')
})
const zoomPoint = await page.evaluate(() => {
  const pane = document.querySelector('.vue-flow__pane')
  if (!pane) throw new Error('Missing viewport zoom benchmark pane')
  const box = pane.getBoundingClientRect()
  return { x: box.left + box.width * 0.54, y: box.top + box.height * 0.52 }
})
await page.mouse.move(zoomPoint.x, zoomPoint.y)
await page.keyboard.down('Control')
try {
  for (let index = 0; index < 8; index += 1) {
    await page.mouse.wheel(0, -220)
    await page.waitForTimeout(70)
  }
} finally {
  await page.keyboard.up('Control')
}
await page.waitForTimeout(400)
await page.evaluate(() => window.__canvasInteractionBenchmark.end())
`,
    90_000,
  )
  return readEvalJson(playwrightCli, `window.__canvasInteractionBenchmark.traces['viewport-zoom']`)
}

function summarizeTrace(trace: RawInteractionTrace): InteractionMetrics {
  const sortedFrameDeltas = [...trace.frameDeltas].sort((left, right) => left - right)
  const sampleCount = trace.frameDeltas.length
  const totalFrameMs = trace.frameDeltas.reduce((total, deltaMs) => total + deltaMs, 0)
  const metrics: InteractionMetrics = {
    path: trace.path,
    p95FrameMs: roundMs(percentile(sortedFrameDeltas, 0.95)),
    droppedFrames: countDroppedFrames(trace.frameDeltas),
    durationMs: roundMs(trace.durationMs),
    avgFrameMs: sampleCount > 0 ? roundMs(totalFrameMs / sampleCount) : 0,
    maxFrameMs: roundMs(sortedFrameDeltas.at(-1) ?? 0),
    sampleCount,
    mutationCount: trace.mutationCount,
    eventCount: trace.eventCount,
  }

  if (trace.path === 'node-drag') {
    metrics.positionChanges = countPositionChanges(trace.samples)
    if (metrics.positionChanges <= 1) {
      throw new Error(
        `node-drag benchmark did not observe node movement ${formatTraceDebug(trace)}`,
      )
    }
  } else {
    metrics.viewportChanges = countViewportChanges(trace.samples)
    if (metrics.viewportChanges <= 1) {
      throw new Error(
        `${trace.path} benchmark did not observe viewport movement ${formatTraceDebug(trace)}`,
      )
    }
  }

  return metrics
}

function formatTraceDebug(trace: RawInteractionTrace): string {
  return JSON.stringify({
    before: trace.before?.rect,
    after: trace.after?.rect,
    samples: trace.samples.length,
    mutationCount: trace.mutationCount,
    eventsByType: trace.eventsByType,
  })
}

function roundMs(value: number): number {
  return Math.round(value * 10) / 10
}

function percentile(sortedValues: number[], percentileValue: number): number {
  if (sortedValues.length === 0) return 0
  const index = Math.ceil(sortedValues.length * percentileValue) - 1
  return sortedValues[Math.max(0, Math.min(index, sortedValues.length - 1))]
}

function countDroppedFrames(frameDeltas: readonly number[]): number {
  return frameDeltas.reduce((total, deltaMs) => {
    const expectedFrames = Math.max(1, Math.round(deltaMs / FRAME_BUDGET_MS))
    return total + expectedFrames - 1
  }, 0)
}

function countPositionChanges(samples: readonly TargetSnapshot[]): number {
  return countSampleChanges(samples, (sample) => `${sample.rect.x}:${sample.rect.y}`)
}

function countViewportChanges(samples: readonly TargetSnapshot[]): number {
  return countSampleChanges(
    samples,
    (sample) => `${sample.translate.x}:${sample.translate.y}:${sample.scale.x}:${sample.scale.y}`,
  )
}

function countSampleChanges(
  samples: readonly TargetSnapshot[],
  getKey: (sample: TargetSnapshot) => string,
) {
  let changes = 0
  let previousKey: string | null = null
  samples.forEach((sample) => {
    const key = getKey(sample)
    if (key !== previousKey) {
      changes += 1
      previousKey = key
    }
  })
  return changes
}

function pickWorstPath(paths: readonly InteractionMetrics[]): InteractionMetrics {
  const [firstPath] = paths
  if (!firstPath) {
    throw new Error('No canvas interaction benchmark paths were recorded')
  }
  return paths.reduce((worst, current) => {
    if (current.p95FrameMs !== worst.p95FrameMs) {
      return current.p95FrameMs > worst.p95FrameMs ? current : worst
    }
    if (current.droppedFrames !== worst.droppedFrames) {
      return current.droppedFrames > worst.droppedFrames ? current : worst
    }
    return current.maxFrameMs > worst.maxFrameMs ? current : worst
  }, firstPath)
}

function createReport(paths: InteractionMetrics[]): BenchmarkReport {
  const fixtureNodeCount =
    MOCK_BOARD_STATE.imageNodes.length +
    MOCK_BOARD_STATE.promptNodes.length +
    MOCK_BOARD_STATE.fusionNodes.length
  return {
    schemaVersion: 1,
    generatedAt: new Date().toISOString(),
    command: 'cd apps/web && npm run benchmark:canvas-interactions',
    browser: {
      engine: 'chromium',
      source: 'playwright-cli default browser session',
    },
    viewport: BENCHMARK_VIEWPORT,
    fixture: {
      name: 'medium-large-canvas-board',
      nodeCount: fixtureNodeCount,
      edgeCount: MOCK_BOARD_STATE.connections.length,
      imageNodes: MOCK_BOARD_STATE.imageNodes.length,
      promptNodes: MOCK_BOARD_STATE.promptNodes.length,
      fusionNodes: MOCK_BOARD_STATE.fusionNodes.length,
    },
    smoothnessBudget: {
      frameBudgetMs: roundMs(FRAME_BUDGET_MS),
      ranking: 'Higher p95FrameMs, droppedFrames, then maxFrameMs is worse.',
    },
    paths,
    worstPath: pickWorstPath(paths),
  }
}

async function main() {
  let playwrightCli: CommandSpec | null = null
  try {
    playwrightCli = smoke.resolvePlaywrightCli()
    await smoke.startDevServer()
    setupBrowser(playwrightCli)

    const traces = [
      runNodeDragBenchmark(playwrightCli),
      runViewportPanBenchmark(playwrightCli),
      runViewportZoomBenchmark(playwrightCli),
    ]
    const report = createReport(traces.map(summarizeTrace))
    console.log(JSON.stringify(report, null, 2))
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
