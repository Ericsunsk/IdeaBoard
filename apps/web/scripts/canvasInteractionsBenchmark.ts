/// <reference lib="dom" />

import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'
import { chromium, type BrowserContext, type Page, type Route } from 'playwright'
import { stripUndefined } from '../src/shared/api/stripUndefined'
import {
  BOARD_STATE_SCHEMA_VERSION,
  type BoardState,
  type CanvasConnection,
  type CanvasConnectionPort,
  type CanvasConnectionNodeType,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../src/features/board/domain'
import type { CanvasGraphNodeId } from '../src/features/canvas/domain/graph'
import type {
  BoardStateEnvelope,
  User,
  UserBoardItem,
} from '../src/shared/api/generated-query/model'
import { createMockSmokeRuntime } from './mockSmokeUtils'

type BenchmarkPath =
  'initial-load' | 'node-drag' | 'viewport-pan' | 'viewport-zoom' | 'connect-delete'

type BenchmarkCounters = {
  graphCommandCount?: number
  nativeNodeChangeBatches?: number
  nativeNodeChangeCount?: number
  nativeEdgeChangeBatches?: number
  nativeEdgeChangeCount?: number
  nodeArrayReplacements?: number
  edgeArrayReplacements?: number
  historyCaptureCount?: number
  historyPushCount?: number
  autosaveScheduleCount?: number
  autosaveSaveCount?: number
  handleGeometryLayoutReadCount?: number
  graphCommands?: Record<string, number>
  runtimeTimingMs?: Record<string, RuntimeTimingCounter>
}

type RuntimeTimingCounter = {
  count: number
  totalMs: number
  maxMs: number
}

type RuntimeTimingSummary = RuntimeTimingCounter & {
  avgMs: number
}

type PathBudget = {
  maxP95FrameMs: number
  maxDroppedFrames: number
  maxMutationCount: number
  maxNodeArrayReplacements: number
  maxEdgeArrayReplacements: number
  maxHistoryCaptureCount: number
  maxHistoryPushCount: number
  maxAutosaveScheduleCount: number
  maxAutosaveSaveCount: number
  maxNativeNodeChangeBatches: number
  maxNativeEdgeChangeBatches: number
}

type BenchmarkOptions = {
  fixture: 'structure-heavy'
  nodeCount: number
  paths: BenchmarkPath[]
  output: string
  json: boolean
  viewport: {
    width: number
    height: number
  }
  frameBudgetMs: number
  headed: boolean
}

type FrameProbeSnapshot = {
  frames: number[]
  durationMs: number
  counters: BenchmarkCounters
  longTaskDurationsMs: number[]
  longTaskObserverAvailable: boolean
}

type PathResult = {
  path: BenchmarkPath
  p95FrameMs: number
  droppedFrames: number
  durationMs: number
  avgFrameMs: number
  maxFrameMs: number
  sampleCount: number
  timeToInteractiveMs: number
  longTaskCount: number
  longTaskTotalMs: number
  longTaskP95Ms: number
  longTaskMaxMs: number
  longTaskDurationsMs: number[]
  longTaskObserverAvailable: boolean
  mutationCount: number
  eventCount: number
  counters: BenchmarkCounters
  timings: Record<string, RuntimeTimingSummary>
  positionChanges?: number
}

type BenchmarkReport = {
  schemaVersion: 5
  generatedAt: string
  command: string
  browser: {
    engine: 'chromium'
    source: string
  }
  viewport: {
    width: number
    height: number
  }
  runtime: {
    dragTargetId: number
  }
  fixture: {
    name: 'structure-heavy'
    nodeCount: number
    edgeCount: number
    requestedEdgeDensity: number
    actualEdgeDensity: number
    imageNodes: number
    promptNodes: number
    fusionNodes: number
  }
  smoothnessBudget: {
    frameBudgetMs: number
    ranking: string
    timeToInteractiveDefinition: string
    longTaskSource: string
    runtimeTimingLabels: readonly string[]
  }
  budgets: Record<BenchmarkPath, PathBudget>
  paths: PathResult[]
  worstPath: PathResult | null
  verdict: {
    status: 'pass' | 'fail'
    failures: string[]
  }
}

type FixtureNodeRef = {
  kind: CanvasConnectionNodeType
  id: number
}

type BenchmarkFixture = {
  board: UserBoardItem
  connectDeleteTarget: {
    sourceNodeId: CanvasGraphNodeId
    targetNodeId: CanvasGraphNodeId
    sourcePort: CanvasConnectionPort
    targetPort: CanvasConnectionPort
    deleteNodeId: CanvasGraphNodeId
  }
  reportFixture: BenchmarkReport['fixture']
  dragTargetId: number
}

type BrowserErrorLog = {
  consoleErrors: string[]
  pageErrors: string[]
  unexpectedApiRequests: string[]
}

const WEB_ROOT = fileURLToPath(new URL('..', import.meta.url))
const ALL_BENCHMARK_PATHS: BenchmarkPath[] = [
  'initial-load',
  'node-drag',
  'viewport-pan',
  'viewport-zoom',
  'connect-delete',
]
const DEFAULT_NODE_COUNT = 150
const DEFAULT_EDGE_DENSITY = 1.6
const DEFAULT_VIEWPORT = { width: 1440, height: 900 }
const DEFAULT_FRAME_BUDGET_MS = 16.7
const DEFAULT_OUTPUT = '.benchmark-reports/canvas-interactions'
const BOARD_ID = 'benchmark-canvas-board'
const BOARD_NAME = 'Benchmark Canvas Board'
const MOCK_UPDATED_AT = 1_767_225_600_000
const PNG_1X1 = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMB/axpRFQAAAAASUVORK5CYII=',
  'base64',
)

// These labels are intentionally emitted only while the benchmark counter is installed.
// Keeping absent labels in every report makes pre/post-refactor comparisons mechanically safe.
const EXPECTED_RUNTIME_TIMING_LABELS = [
  'graph-store.rebuild',
  'graph-store.hydrate',
  'graph-transaction.draft',
  'history.capture',
  'vue-flow.projection',
  'node-ui.recompute',
  'handle-proximity.layout-read',
] as const

const DEFAULT_BUDGETS: Record<BenchmarkPath, PathBudget> = {
  'initial-load': {
    maxP95FrameMs: 250,
    maxDroppedFrames: 1000,
    maxMutationCount: 50_000,
    maxNodeArrayReplacements: 10_000,
    maxEdgeArrayReplacements: 10_000,
    maxHistoryCaptureCount: 10_000,
    maxHistoryPushCount: 10_000,
    maxAutosaveScheduleCount: 10_000,
    maxAutosaveSaveCount: 10_000,
    maxNativeNodeChangeBatches: 10_000,
    maxNativeEdgeChangeBatches: 10_000,
  },
  'node-drag': {
    maxP95FrameMs: 75,
    maxDroppedFrames: 120,
    maxMutationCount: 180,
    maxNodeArrayReplacements: 90,
    maxEdgeArrayReplacements: 2,
    maxHistoryCaptureCount: 2,
    maxHistoryPushCount: 1,
    maxAutosaveScheduleCount: 1,
    maxAutosaveSaveCount: 1,
    maxNativeNodeChangeBatches: 90,
    maxNativeEdgeChangeBatches: 2,
  },
  'viewport-pan': {
    maxP95FrameMs: 75,
    maxDroppedFrames: 120,
    maxMutationCount: 120,
    maxNodeArrayReplacements: 40,
    maxEdgeArrayReplacements: 40,
    maxHistoryCaptureCount: 0,
    maxHistoryPushCount: 0,
    maxAutosaveScheduleCount: 40,
    maxAutosaveSaveCount: 0,
    maxNativeNodeChangeBatches: 40,
    maxNativeEdgeChangeBatches: 0,
  },
  'viewport-zoom': {
    maxP95FrameMs: 90,
    maxDroppedFrames: 140,
    maxMutationCount: 120,
    maxNodeArrayReplacements: 40,
    maxEdgeArrayReplacements: 40,
    maxHistoryCaptureCount: 0,
    maxHistoryPushCount: 0,
    maxAutosaveScheduleCount: 40,
    maxAutosaveSaveCount: 0,
    maxNativeNodeChangeBatches: 40,
    maxNativeEdgeChangeBatches: 0,
  },
  'connect-delete': {
    maxP95FrameMs: 120,
    maxDroppedFrames: 220,
    maxMutationCount: 500,
    maxNodeArrayReplacements: 6,
    maxEdgeArrayReplacements: 4,
    maxHistoryCaptureCount: 4,
    maxHistoryPushCount: 2,
    maxAutosaveScheduleCount: 6,
    maxAutosaveSaveCount: 0,
    maxNativeNodeChangeBatches: 4,
    maxNativeEdgeChangeBatches: 4,
  },
}

const MOCK_USER = {
  public_id: '11111111-1111-4111-8111-111111111111',
  display_name: 'Benchmark User',
  storage_namespace: 'UBM001',
  role: 'user',
  avatar: '',
  status: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
} satisfies User

const FRAME_PROBE_SOURCE = String.raw`
(() => {
  if (window.__canvasBenchmarkStartFrameProbe) return

  window.__canvasBenchmarkStartFrameProbe = (frameBudgetMs) => {
    const activeProbe = window.__canvasBenchmarkFrameProbe
    if (activeProbe && activeProbe.running) {
      activeProbe.running = false
      cancelAnimationFrame(activeProbe.rafId)
      activeProbe.longTaskObserver?.disconnect()
    }

    const probe = {
      frameBudgetMs,
      frames: [],
      longTaskDurationsMs: [],
      longTaskObserverAvailable: false,
      lastFrameTime: null,
      running: true,
      startTime: performance.now(),
      rafId: 0,
      longTaskObserver: null,
    }

    if (typeof PerformanceObserver !== 'undefined') {
      try {
        probe.longTaskObserver = new PerformanceObserver((entries) => {
          for (const entry of entries.getEntries()) {
            if (Number.isFinite(entry.duration) && entry.duration > 0) {
              probe.longTaskDurationsMs.push(entry.duration)
            }
          }
        })
        probe.longTaskObserver.observe({ type: 'longtask' })
        probe.longTaskObserverAvailable = true
      } catch {
        // Long Task entries are browser-dependent; frame timings remain available everywhere.
      }
    }

    const tick = (timestamp) => {
      if (!probe.running) return
      if (probe.lastFrameTime !== null) {
        probe.frames.push(timestamp - probe.lastFrameTime)
      }
      probe.lastFrameTime = timestamp
      probe.rafId = requestAnimationFrame(tick)
    }

    probe.rafId = requestAnimationFrame(tick)
    window.__canvasBenchmarkFrameProbe = probe
  }

  window.__canvasBenchmarkStopFrameProbe = () => {
    const probe = window.__canvasBenchmarkFrameProbe
    if (!probe) {
      return {
        frames: [],
        durationMs: 0,
        counters: {},
        longTaskDurationsMs: [],
        longTaskObserverAvailable: false,
      }
    }
    probe.running = false
    cancelAnimationFrame(probe.rafId)
    for (const entry of probe.longTaskObserver?.takeRecords?.() || []) {
      if (Number.isFinite(entry.duration) && entry.duration > 0) {
        probe.longTaskDurationsMs.push(entry.duration)
      }
    }
    probe.longTaskObserver?.disconnect()
    const counters = window.__canvasInteractionBenchmarkCounters || {}
    return {
      frames: probe.frames.slice(),
      durationMs: performance.now() - probe.startTime,
      counters: JSON.parse(JSON.stringify(counters)),
      longTaskDurationsMs: probe.longTaskDurationsMs.slice(),
      longTaskObserverAvailable: probe.longTaskObserverAvailable,
    }
  }
})()
`

const smoke = createMockSmokeRuntime({
  hostEnvName: 'CANVAS_BENCHMARK_HOST',
  portEnvName: 'CANVAS_BENCHMARK_PORT',
  verboseEnvName: 'CANVAS_BENCHMARK_VERBOSE',
  sessionPrefix: 'cib',
})

function readArgValue(args: string[], index: number, name: string): string {
  const value = args[index + 1]
  if (!value || value.startsWith('--')) {
    throw new Error(`${name} requires a value`)
  }
  return value
}

function parsePositiveInteger(rawValue: string, name: string): number {
  const value = Number(rawValue)
  if (!Number.isInteger(value) || value <= 0) {
    throw new Error(`${name} must be a positive integer, got ${rawValue}`)
  }
  return value
}

function isBenchmarkPath(value: string): value is BenchmarkPath {
  return ALL_BENCHMARK_PATHS.includes(value as BenchmarkPath)
}

function parsePaths(rawValue: string): BenchmarkPath[] {
  const paths = rawValue
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
  const benchmarkPaths: BenchmarkPath[] = []

  if (paths.length === 0) throw new Error('--paths must include at least one path')

  for (const pathName of paths) {
    if (!isBenchmarkPath(pathName)) {
      throw new Error(
        `Unknown benchmark path "${pathName}". Supported paths: ${ALL_BENCHMARK_PATHS.join(', ')}`,
      )
    }
    benchmarkPaths.push(pathName)
  }

  return [...new Set(benchmarkPaths)]
}

function parseViewport(rawValue: string): BenchmarkOptions['viewport'] {
  const match = rawValue.match(/^(\d+)x(\d+)$/)
  if (!match) throw new Error(`--viewport must use WIDTHxHEIGHT, got ${rawValue}`)
  const rawWidth = match[1]
  const rawHeight = match[2]
  if (rawWidth === undefined || rawHeight === undefined) {
    throw new Error(`--viewport must use WIDTHxHEIGHT, got ${rawValue}`)
  }
  return {
    width: parsePositiveInteger(rawWidth, '--viewport width'),
    height: parsePositiveInteger(rawHeight, '--viewport height'),
  }
}

function parseArgs(args: string[]): BenchmarkOptions {
  const options: BenchmarkOptions = {
    fixture: 'structure-heavy',
    nodeCount: DEFAULT_NODE_COUNT,
    paths: ALL_BENCHMARK_PATHS,
    output: DEFAULT_OUTPUT,
    json: false,
    viewport: DEFAULT_VIEWPORT,
    frameBudgetMs: DEFAULT_FRAME_BUDGET_MS,
    headed: false,
  }

  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index]
    if (arg === '--help' || arg === '-h') {
      printHelp()
      process.exit(0)
    }
    if (arg === '--json') {
      options.json = true
      continue
    }
    if (arg === '--headed') {
      options.headed = true
      continue
    }
    if (arg === '--fixture') {
      const value = readArgValue(args, index, arg)
      if (value !== 'structure-heavy') {
        throw new Error(`Unsupported fixture "${value}". Supported fixture: structure-heavy`)
      }
      options.fixture = value
      index += 1
      continue
    }
    if (arg === '--nodes') {
      options.nodeCount = parsePositiveInteger(readArgValue(args, index, arg), arg)
      index += 1
      continue
    }
    if (arg === '--paths') {
      options.paths = parsePaths(readArgValue(args, index, arg))
      index += 1
      continue
    }
    if (arg === '--output') {
      options.output = readArgValue(args, index, arg)
      index += 1
      continue
    }
    if (arg === '--viewport') {
      options.viewport = parseViewport(readArgValue(args, index, arg))
      index += 1
      continue
    }
    if (arg === '--frame-budget-ms') {
      const value = Number(readArgValue(args, index, arg))
      if (!Number.isFinite(value) || value <= 0) {
        throw new Error(`--frame-budget-ms must be a positive number, got ${args[index + 1]}`)
      }
      options.frameBudgetMs = value
      index += 1
      continue
    }

    throw new Error(`Unknown argument: ${arg}`)
  }

  if (options.nodeCount < 3) {
    throw new Error(
      '--nodes must be at least 3 so connect-delete has source, target, and delete nodes',
    )
  }

  return options
}

function printHelp() {
  console.log(`Canvas interaction benchmark

Usage:
  npm run benchmark:canvas-interactions -- [options]

Options:
  --fixture structure-heavy     Fixture to run. Defaults to structure-heavy.
  --nodes <count>               Number of Canvas nodes. Defaults to ${DEFAULT_NODE_COUNT}.
  --paths <a,b,c>               Paths to run. Defaults to all paths.
  --output <path>               Output directory or .json file. Defaults to ${DEFAULT_OUTPUT}.
  --json                        Print the report JSON to stdout.
  --viewport <width>x<height>   Browser viewport. Defaults to ${DEFAULT_VIEWPORT.width}x${DEFAULT_VIEWPORT.height}.
  --headed                      Run Chromium headed.
`)
}

function flowNodeId(node: FixtureNodeRef): string {
  return `${node.kind}-${node.id}`
}

function createPromptNode(id: number, x: number, y: number): PromptNodeItem {
  return {
    id,
    type: 'prompt',
    x,
    y,
    width: 280,
    title: `Prompt ${id}`,
    text: `Benchmark prompt ${id}`,
    outputCount: 1,
    aspectRatio: '1:1',
    inputImages: [],
  }
}

function createImageNode(id: number, x: number, y: number): ImageNodeItem {
  return {
    id,
    type: 'image-node',
    x,
    y,
    width: 280,
    title: `Image ${id}`,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputCount: 1,
    outputStatus: 'idle',
    outputImages: [],
  }
}

function createFusionNode(id: number, x: number, y: number): FusionNodeItem {
  return {
    id,
    type: 'fusion',
    x,
    y,
    width: 286,
    title: `Fusion ${id}`,
    prompt: `Benchmark fusion ${id}`,
    outputCount: 1,
    aspectRatio: '1:1',
    blendPoint: { x: 0.5, y: 0.5 },
    inputImages: [],
    outputStatus: 'idle',
    outputImages: [],
  }
}

function createStructureHeavyFixture(nodeCount: number): BenchmarkFixture {
  const columns = Math.max(3, Math.ceil(Math.sqrt(nodeCount * 1.35)))
  const imageNodes: ImageNodeItem[] = []
  const promptNodes: PromptNodeItem[] = []
  const fusionNodes: FusionNodeItem[] = []
  const refs: FixtureNodeRef[] = []

  for (let index = 0; index < nodeCount; index += 1) {
    const id = index + 1
    const column = index % columns
    const row = Math.floor(index / columns)
    const x = 160 + column * 360
    const y = 140 + row * 300
    const slot = index % 3

    if (slot === 0) {
      imageNodes.push(createImageNode(id, x, y))
      refs.push({ kind: 'image', id })
      continue
    }

    if (slot === 1) {
      promptNodes.push(createPromptNode(id, x, y))
      refs.push({ kind: 'prompt', id })
      continue
    }

    fusionNodes.push(createFusionNode(id, x, y))
    refs.push({ kind: 'fusion', id })
  }

  const connections = createStructureHeavyConnections(
    refs,
    Math.floor(nodeCount * DEFAULT_EDGE_DENSITY),
  )
  const state: BoardState = {
    schemaVersion: BOARD_STATE_SCHEMA_VERSION,
    imageNodes,
    promptNodes,
    fusionNodes,
    connections,
    groups: [],
    updatedAt: MOCK_UPDATED_AT,
  }
  const board = {
    board_id: BOARD_ID,
    name: BOARD_NAME,
    state: toBoardStateEnvelope(state),
    access_role: 'owner',
    revision: 1,
    state_schema_version: BOARD_STATE_SCHEMA_VERSION,
    created_at: MOCK_UPDATED_AT - 86_400_000,
    updated_at: MOCK_UPDATED_AT,
  } satisfies UserBoardItem

  return {
    board,
    connectDeleteTarget: {
      sourceNodeId: 'image-1',
      targetNodeId: 'prompt-2',
      sourcePort: 'image',
      targetPort: 'image',
      deleteNodeId: 'fusion-3',
    },
    dragTargetId: 1,
    reportFixture: {
      name: 'structure-heavy',
      nodeCount,
      edgeCount: connections.length,
      requestedEdgeDensity: DEFAULT_EDGE_DENSITY,
      actualEdgeDensity: roundMetric(connections.length / nodeCount, 2),
      imageNodes: imageNodes.length,
      promptNodes: promptNodes.length,
      fusionNodes: fusionNodes.length,
    },
  }
}

function toBoardStateEnvelope(state: BoardState): BoardStateEnvelope {
  return stripUndefined({
    schemaVersion: state.schemaVersion,
    imageNodes: state.imageNodes.map((node) => ({ ...node })),
    promptNodes: state.promptNodes.map((node) => ({ ...node })),
    fusionNodes: state.fusionNodes.map((node) => ({ ...node })),
    connections: state.connections.map((connection) => ({ ...connection })),
    groups: state.groups.map((group) => ({ ...group, nodeIds: [...group.nodeIds] })),
    mediaRefs: state.mediaRefs?.map((ref) => stripUndefined({ ...ref })),
    previewSnapshotDataUrl: state.previewSnapshotDataUrl,
    previewSnapshotUpdatedAt: state.previewSnapshotUpdatedAt,
    previewSnapshotVersion: state.previewSnapshotVersion,
    updatedAt: state.updatedAt,
  })
}

function createStructureHeavyConnections(
  refs: FixtureNodeRef[],
  requestedEdgeCount: number,
): CanvasConnection[] {
  const reservedNodeIds = new Set([1, 2, 3])
  const sources = refs.filter((node) => !reservedNodeIds.has(node.id))
  const targets = refs.filter(
    (node) => !reservedNodeIds.has(node.id) && (node.kind === 'prompt' || node.kind === 'fusion'),
  )
  const targetInputCounts = new Map<string, number>()
  const connectionKeys = new Set<string>()
  const connections: CanvasConnection[] = []

  if (sources.length === 0 || targets.length === 0) return connections

  let cursor = 0
  let attempts = 0
  const maxAttempts = Math.max(200, requestedEdgeCount * 40)

  while (connections.length < requestedEdgeCount && attempts < maxAttempts) {
    attempts += 1
    const source = sources[cursor % sources.length]
    const target = targets[(cursor * 7 + Math.floor(cursor / sources.length)) % targets.length]
    cursor += 1
    if (!source || !target) break

    if (flowNodeId(source) === flowNodeId(target)) continue

    const targetKey = flowNodeId(target)
    const targetInputCount = targetInputCounts.get(targetKey) ?? 0
    if (targetInputCount >= 4) continue

    const connectionKey = `${flowNodeId(source)}:image:${targetKey}:image`
    if (connectionKeys.has(connectionKey)) continue

    connectionKeys.add(connectionKey)
    targetInputCounts.set(targetKey, targetInputCount + 1)
    connections.push({
      id: connections.length + 1,
      fromNodeType: source.kind,
      fromNodeId: source.id,
      fromPort: 'image',
      toNodeType: target.kind,
      toNodeId: target.id,
      toPort: 'image',
    })
  }

  return connections
}

async function installMockApiRoutes(context: BrowserContext, fixture: BenchmarkFixture) {
  let currentBoard = fixture.board

  await context.route('**/*', async (route) => {
    const request = route.request()
    const requestUrl = new URL(request.url())
    const pathname = requestUrl.pathname

    if (pathname === '/api/auth/get-session') {
      await fulfillJson(route, {
        session: { id: 'canvas-benchmark-session' },
        user: { id: MOCK_USER.public_id, name: MOCK_USER.display_name, role: MOCK_USER.role },
      })
      return
    }

    if (pathname === '/api/settings/branding') {
      await fulfillJson(route, {
        code: 0,
        msg: 'ok',
        data: { project_name: 'IdeaBoard', logo_url: '' },
      })
      return
    }

    if (pathname === '/api/user/info') {
      await fulfillJson(route, { code: 0, msg: 'ok', data: { user: MOCK_USER } })
      return
    }

    if (pathname === '/api/user/credits/wallet') {
      await fulfillJson(route, {
        code: 0,
        msg: 'ok',
        data: {
          public_id: MOCK_USER.public_id,
          display_name: MOCK_USER.display_name,
          available_credits: 1280,
          reserved_credits: 0,
        },
      })
      return
    }

    if (pathname === '/api/user/tasks/submit/readiness') {
      await fulfillJson(route, {
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
            available_credits: 1280,
            reserved_credits: 0,
            sufficient: true,
          },
        },
      })
      return
    }

    if (pathname === '/api/user/boards') {
      await fulfillJson(route, { code: 0, msg: 'ok', data: { list: [currentBoard] } })
      return
    }

    if (pathname === `/api/user/boards/${BOARD_ID}`) {
      if (request.method() !== 'GET') {
        currentBoard = applyBoardMutation(currentBoard, request.postData())
      }
      await fulfillJson(route, { code: 0, msg: 'ok', data: { board: currentBoard } })
      return
    }

    if (pathname === `/api/user/boards/${BOARD_ID}/uploads`) {
      await fulfillJson(route, {
        code: 0,
        msg: 'ok',
        data: {
          url: `/uploads/canvas/${MOCK_USER.storage_namespace}/${BOARD_ID}/content/benchmark-preview.png`,
          width: 512,
          height: 512,
          size: PNG_1X1.length,
          mime: 'image/png',
        },
      })
      return
    }

    if (pathname.startsWith('/uploads/')) {
      await route.fulfill({
        status: 200,
        contentType: 'image/png',
        body: PNG_1X1,
      })
      return
    }

    if (pathname.startsWith('/api/')) {
      await fulfillJson(
        route,
        { code: 599, msg: `canvas benchmark blocked unexpected API request: ${pathname}` },
        599,
      )
      return
    }

    await route.continue()
  })
}

function applyBoardMutation(board: UserBoardItem, bodyText: string | null): UserBoardItem {
  if (!bodyText) return board

  try {
    const payload = JSON.parse(bodyText) as {
      name?: unknown
      state?: unknown
      revision?: unknown
    }
    return {
      ...board,
      ...(typeof payload.name === 'string' ? { name: payload.name } : {}),
      ...(isBoardState(payload.state) ? { state: toBoardStateEnvelope(payload.state) } : {}),
      ...(typeof payload.revision === 'number' ? { revision: payload.revision } : {}),
      updated_at: Date.now(),
    }
  } catch (error: unknown) {
    console.warn(
      `Ignoring invalid benchmark board mutation payload: ${error instanceof Error ? error.message : String(error)}`,
    )
    return board
  }
}

function isBoardState(value: unknown): value is BoardState {
  if (!value || typeof value !== 'object') return false
  const record = value as Partial<BoardState>
  return (
    record.schemaVersion === BOARD_STATE_SCHEMA_VERSION &&
    Array.isArray(record.imageNodes) &&
    Array.isArray(record.promptNodes) &&
    Array.isArray(record.fusionNodes) &&
    Array.isArray(record.connections) &&
    Array.isArray(record.groups)
  )
}

async function fulfillJson(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  })
}

function recordBrowserErrors(page: Page, errors: BrowserErrorLog) {
  page.on('console', (message) => {
    if (message.type() === 'error') {
      errors.consoleErrors.push(message.text())
    }
  })
  page.on('pageerror', (error) => {
    errors.pageErrors.push(error.message)
  })
  page.on('requestfinished', async (request) => {
    const response = await request.response()
    const requestUrl = new URL(request.url())
    if (requestUrl.pathname.startsWith('/api/') && response?.status() === 599) {
      errors.unexpectedApiRequests.push(`${request.method()} ${requestUrl.pathname}`)
    }
  })
}

async function createBenchmarkPage(
  context: BrowserContext,
  options: BenchmarkOptions,
  startProbeOnDocumentStart: boolean,
): Promise<{ page: Page; errors: BrowserErrorLog }> {
  const page = await context.newPage()
  const errors: BrowserErrorLog = {
    consoleErrors: [],
    pageErrors: [],
    unexpectedApiRequests: [],
  }
  recordBrowserErrors(page, errors)

  await page.addInitScript({
    content: startProbeOnDocumentStart
      ? `${FRAME_PROBE_SOURCE}
window.__canvasInteractionBenchmarkCounters = {}
window.__canvasBenchmarkStartFrameProbe(${JSON.stringify(options.frameBudgetMs)})
`
      : FRAME_PROBE_SOURCE,
  })

  return { page, errors }
}

async function gotoBenchmarkCanvas(
  page: Page,
  baseUrl: string,
): Promise<{ timeToInteractiveMs: number }> {
  const url = `${baseUrl}/user/canvas?boardId=${encodeURIComponent(BOARD_ID)}&canvasBenchmarkActions=1`
  await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 60_000 })
  await page.waitForSelector('.vue-flow', { state: 'visible', timeout: 30_000 })
  await page.getByText(BOARD_NAME).waitFor({ state: 'visible', timeout: 30_000 })
  await page.waitForFunction(
    () => document.querySelectorAll('.vue-flow__node').length > 0,
    undefined,
    { timeout: 30_000 },
  )
  await page.waitForTimeout(250)
  const timeToInteractiveMs = await page.evaluate(() => performance.now())
  return { timeToInteractiveMs }
}

async function startFrameProbe(page: Page, frameBudgetMs: number) {
  await page.evaluate((budget) => {
    const benchmarkWindow = window as typeof window & {
      __canvasInteractionBenchmarkCounters?: BenchmarkCounters
      __canvasBenchmarkStartFrameProbe?: (frameBudgetMs: number) => void
    }
    benchmarkWindow.__canvasInteractionBenchmarkCounters = {}
    benchmarkWindow.__canvasBenchmarkStartFrameProbe?.(budget)
  }, frameBudgetMs)
}

async function stopFrameProbe(page: Page): Promise<FrameProbeSnapshot> {
  return await page.evaluate(() => {
    const benchmarkWindow = window as typeof window & {
      __canvasBenchmarkStopFrameProbe?: () => FrameProbeSnapshot
    }
    return (
      benchmarkWindow.__canvasBenchmarkStopFrameProbe?.() ?? {
        frames: [],
        durationMs: 0,
        counters: {},
        longTaskDurationsMs: [],
        longTaskObserverAvailable: false,
      }
    )
  })
}

async function runPath(
  context: BrowserContext,
  baseUrl: string,
  fixture: BenchmarkFixture,
  options: BenchmarkOptions,
  pathName: BenchmarkPath,
): Promise<PathResult> {
  const { page, errors } = await createBenchmarkPage(context, options, pathName === 'initial-load')

  try {
    const readiness = await gotoBenchmarkCanvas(page, baseUrl)

    let interaction: { eventCount: number; positionChanges?: number } = {
      eventCount: 0,
    }

    if (pathName !== 'initial-load') {
      await startFrameProbe(page, options.frameBudgetMs)
      interaction = await runInteraction(page, fixture, pathName)
      await delay(300)
    }

    const probe = await stopFrameProbe(page)
    assertBrowserClean(errors, pathName)
    return summarizePath(pathName, probe, options.frameBudgetMs, interaction, readiness)
  } finally {
    await page.close()
  }
}

async function runInteraction(
  page: Page,
  fixture: BenchmarkFixture,
  pathName: Exclude<BenchmarkPath, 'initial-load'>,
): Promise<{ eventCount: number; positionChanges?: number }> {
  if (pathName === 'node-drag') return await runNodeDrag(page)
  if (pathName === 'viewport-pan') return await runViewportPan(page)
  if (pathName === 'viewport-zoom') return await runViewportZoom(page)
  return await runConnectDelete(page, fixture)
}

async function runNodeDrag(page: Page): Promise<{ eventCount: number; positionChanges: number }> {
  await page.getByRole('button', { name: 'Select (V)' }).click()
  const node = page.locator('.vue-flow__node').first()
  await node.waitFor({ state: 'visible', timeout: 10_000 })
  const box = await node.boundingBox()
  if (!box) throw new Error('node-drag target node has no bounding box')

  const steps = 36
  const startX = box.x + Math.min(80, box.width / 2)
  const startY = box.y + Math.min(80, box.height / 2)
  await page.mouse.move(startX, startY)
  await page.mouse.down()
  for (let step = 1; step <= steps; step += 1) {
    await page.mouse.move(startX + step * 4, startY + Math.sin(step / 4) * 24, { steps: 1 })
    await page.waitForTimeout(16)
  }
  await page.mouse.up()

  return { eventCount: steps + 3, positionChanges: steps }
}

async function runViewportPan(page: Page): Promise<{ eventCount: number }> {
  await page.getByRole('button', { name: 'Hand (H)' }).click()
  await page.waitForTimeout(50)
  const paneBox = await getPaneBox(page)
  const steps = 30
  const startX = paneBox.x + paneBox.width / 2
  const startY = paneBox.y + paneBox.height / 2
  await page.mouse.move(startX, startY)
  await page.mouse.down()
  for (let step = 1; step <= steps; step += 1) {
    await page.mouse.move(startX + step * 9, startY + step * 4, { steps: 1 })
    await page.waitForTimeout(16)
  }
  await page.mouse.up()

  return { eventCount: steps + 3 }
}

async function runViewportZoom(page: Page): Promise<{ eventCount: number }> {
  const zoomIn = page.getByRole('button', { name: 'Zoom In' })
  const zoomOut = page.getByRole('button', { name: 'Zoom Out' })
  const clicksPerDirection = 6

  for (let index = 0; index < clicksPerDirection; index += 1) {
    await zoomIn.click()
    await page.waitForTimeout(50)
  }
  for (let index = 0; index < clicksPerDirection; index += 1) {
    await zoomOut.click()
    await page.waitForTimeout(50)
  }

  return { eventCount: clicksPerDirection * 2 }
}

async function runConnectDelete(
  page: Page,
  fixture: BenchmarkFixture,
): Promise<{ eventCount: number }> {
  await page.waitForFunction(
    () => {
      const benchmarkWindow = window as typeof window & {
        __ideaboardCanvasBenchmarkActions?: {
          connectDelete?: unknown
        }
      }
      return typeof benchmarkWindow.__ideaboardCanvasBenchmarkActions?.connectDelete === 'function'
    },
    undefined,
    { timeout: 10_000 },
  )
  await page.evaluate((target) => {
    const benchmarkWindow = window as typeof window & {
      __ideaboardCanvasBenchmarkActions?: {
        connectDelete: (payload: typeof target) => void
      }
    }
    benchmarkWindow.__ideaboardCanvasBenchmarkActions?.connectDelete(target)
  }, fixture.connectDeleteTarget)

  return { eventCount: 2 }
}

async function getPaneBox(page: Page) {
  const pane = page.locator('.vue-flow__pane').first()
  await pane.waitFor({ state: 'visible', timeout: 10_000 })
  const paneBox = await pane.boundingBox()
  if (!paneBox) throw new Error('Canvas pane has no bounding box')
  return paneBox
}

function assertBrowserClean(errors: BrowserErrorLog, pathName: BenchmarkPath) {
  const failures = [
    ...errors.consoleErrors.map((message) => `[console] ${message}`),
    ...errors.pageErrors.map((message) => `[pageerror] ${message}`),
    ...errors.unexpectedApiRequests.map((message) => `[api] ${message}`),
  ]
  if (failures.length > 0) {
    throw new Error(`${pathName} produced browser errors:\n${failures.join('\n')}`)
  }
}

function summarizePath(
  pathName: BenchmarkPath,
  probe: FrameProbeSnapshot,
  frameBudgetMs: number,
  interaction: { eventCount: number; positionChanges?: number },
  readiness: { timeToInteractiveMs: number },
): PathResult {
  const frames = probe.frames.filter((value) => Number.isFinite(value) && value >= 0)
  const longTaskDurationsMs = probe.longTaskDurationsMs
    .filter((value) => Number.isFinite(value) && value > 0)
    .map((value) => roundMetric(value))
  const counters = normalizeCounters(probe.counters)
  const result: PathResult = {
    path: pathName,
    p95FrameMs: roundMetric(percentile(frames, 0.95)),
    droppedFrames: countDroppedFrames(frames, frameBudgetMs),
    durationMs: roundMetric(probe.durationMs),
    avgFrameMs: roundMetric(average(frames)),
    maxFrameMs: roundMetric(frames.length > 0 ? Math.max(...frames) : 0),
    sampleCount: frames.length,
    timeToInteractiveMs: roundMetric(readiness.timeToInteractiveMs),
    longTaskCount: longTaskDurationsMs.length,
    longTaskTotalMs: roundMetric(
      longTaskDurationsMs.reduce((total, durationMs) => total + durationMs, 0),
    ),
    longTaskP95Ms: roundMetric(percentile(longTaskDurationsMs, 0.95)),
    longTaskMaxMs: roundMetric(
      longTaskDurationsMs.length > 0 ? Math.max(...longTaskDurationsMs) : 0,
    ),
    longTaskDurationsMs,
    longTaskObserverAvailable: probe.longTaskObserverAvailable,
    mutationCount: counters.graphCommandCount ?? 0,
    eventCount: interaction.eventCount,
    counters,
    timings: summarizeRuntimeTimings(counters.runtimeTimingMs),
  }

  if (typeof interaction.positionChanges === 'number') {
    result.positionChanges = interaction.positionChanges
  }

  return result
}

function normalizeCounters(counters: BenchmarkCounters): BenchmarkCounters {
  return {
    graphCommandCount: counters.graphCommandCount ?? 0,
    nativeNodeChangeBatches: counters.nativeNodeChangeBatches ?? 0,
    nativeNodeChangeCount: counters.nativeNodeChangeCount ?? 0,
    nativeEdgeChangeBatches: counters.nativeEdgeChangeBatches ?? 0,
    nativeEdgeChangeCount: counters.nativeEdgeChangeCount ?? 0,
    nodeArrayReplacements: counters.nodeArrayReplacements ?? 0,
    edgeArrayReplacements: counters.edgeArrayReplacements ?? 0,
    historyCaptureCount: counters.historyCaptureCount ?? 0,
    historyPushCount: counters.historyPushCount ?? 0,
    autosaveScheduleCount: counters.autosaveScheduleCount ?? 0,
    autosaveSaveCount: counters.autosaveSaveCount ?? 0,
    handleGeometryLayoutReadCount: counters.handleGeometryLayoutReadCount ?? 0,
    graphCommands: counters.graphCommands ?? {},
    runtimeTimingMs: normalizeRuntimeTimingCounters(counters.runtimeTimingMs),
  }
}

function normalizeRuntimeTimingCounters(
  timings: BenchmarkCounters['runtimeTimingMs'],
): Record<string, RuntimeTimingCounter> {
  const normalized: Record<string, RuntimeTimingCounter> = {}

  for (const label of [...EXPECTED_RUNTIME_TIMING_LABELS, ...Object.keys(timings ?? {})]) {
    if (normalized[label]) continue
    const current = timings?.[label]
    const count = Number.isFinite(current?.count) && current!.count > 0 ? current!.count : 0
    const totalMs = Number.isFinite(current?.totalMs) && current!.totalMs > 0 ? current!.totalMs : 0
    const maxMs = Number.isFinite(current?.maxMs) && current!.maxMs > 0 ? current!.maxMs : 0
    normalized[label] = { count, totalMs, maxMs }
  }

  return normalized
}

function summarizeRuntimeTimings(
  timings: BenchmarkCounters['runtimeTimingMs'],
): Record<string, RuntimeTimingSummary> {
  return Object.fromEntries(
    Object.entries(timings ?? {}).map(([label, timing]) => [
      label,
      {
        count: timing.count,
        totalMs: roundMetric(timing.totalMs),
        avgMs: roundMetric(timing.count > 0 ? timing.totalMs / timing.count : 0),
        maxMs: roundMetric(timing.maxMs),
      },
    ]),
  )
}

function percentile(values: number[], percentileValue: number): number {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  const index = Math.min(
    sorted.length - 1,
    Math.max(0, Math.ceil(sorted.length * percentileValue) - 1),
  )
  return sorted[index] ?? 0
}

function average(values: number[]): number {
  if (values.length === 0) return 0
  return values.reduce((sum, value) => sum + value, 0) / values.length
}

function countDroppedFrames(frames: number[], frameBudgetMs: number): number {
  return frames.reduce(
    (sum, frameMs) => sum + Math.max(0, Math.round(frameMs / frameBudgetMs) - 1),
    0,
  )
}

function roundMetric(value: number, digits = 1): number {
  const scale = 10 ** digits
  return Math.round(value * scale) / scale
}

function buildReport(
  fixture: BenchmarkFixture,
  options: BenchmarkOptions,
  generatedAt: string,
  pathResults: PathResult[],
): BenchmarkReport {
  const failures = evaluateBudgetFailures(pathResults, DEFAULT_BUDGETS)

  return {
    schemaVersion: 5,
    generatedAt,
    command:
      `cd apps/web && npm run benchmark:canvas-interactions -- ${process.argv.slice(2).join(' ')}`.trim(),
    browser: {
      engine: 'chromium',
      source: 'playwright chromium browser launched by canvas-interactions-benchmark.ts',
    },
    viewport: options.viewport,
    runtime: {
      dragTargetId: fixture.dragTargetId,
    },
    fixture: fixture.reportFixture,
    smoothnessBudget: {
      frameBudgetMs: options.frameBudgetMs,
      ranking: 'Higher longTaskMaxMs, maxFrameMs, p95FrameMs, then droppedFrames is worse.',
      timeToInteractiveDefinition:
        'Navigation start to Canvas Board label, visible Vue Flow node, and a 250ms settle.',
      longTaskSource:
        'PerformanceObserver longtask entries during the measured path; inspect each path longTaskObserverAvailable before interpreting zero values.',
      runtimeTimingLabels: EXPECTED_RUNTIME_TIMING_LABELS,
    },
    budgets: DEFAULT_BUDGETS,
    paths: pathResults,
    worstPath: findWorstPath(pathResults),
    verdict: {
      status: failures.length > 0 ? 'fail' : 'pass',
      failures,
    },
  }
}

function evaluateBudgetFailures(
  results: PathResult[],
  budgets: Record<BenchmarkPath, PathBudget>,
): string[] {
  const failures: string[] = []

  for (const result of results) {
    const budget = budgets[result.path]
    const checks: Array<[string, number, number]> = [
      ['p95FrameMs', result.p95FrameMs, budget.maxP95FrameMs],
      ['droppedFrames', result.droppedFrames, budget.maxDroppedFrames],
      ['mutationCount', result.mutationCount, budget.maxMutationCount],
      [
        'nodeArrayReplacements',
        result.counters.nodeArrayReplacements ?? 0,
        budget.maxNodeArrayReplacements,
      ],
      [
        'edgeArrayReplacements',
        result.counters.edgeArrayReplacements ?? 0,
        budget.maxEdgeArrayReplacements,
      ],
      [
        'historyCaptureCount',
        result.counters.historyCaptureCount ?? 0,
        budget.maxHistoryCaptureCount,
      ],
      ['historyPushCount', result.counters.historyPushCount ?? 0, budget.maxHistoryPushCount],
      [
        'autosaveScheduleCount',
        result.counters.autosaveScheduleCount ?? 0,
        budget.maxAutosaveScheduleCount,
      ],
      ['autosaveSaveCount', result.counters.autosaveSaveCount ?? 0, budget.maxAutosaveSaveCount],
      [
        'nativeNodeChangeBatches',
        result.counters.nativeNodeChangeBatches ?? 0,
        budget.maxNativeNodeChangeBatches,
      ],
      [
        'nativeEdgeChangeBatches',
        result.counters.nativeEdgeChangeBatches ?? 0,
        budget.maxNativeEdgeChangeBatches,
      ],
    ]

    for (const [metric, actual, max] of checks) {
      if (actual > max) {
        failures.push(`${result.path} ${metric} ${actual} exceeded budget ${max}`)
      }
    }
  }

  return failures
}

function findWorstPath(results: PathResult[]): PathResult | null {
  if (results.length === 0) return null
  const ranked = [...results].sort((left, right) => {
    if (right.longTaskMaxMs !== left.longTaskMaxMs) {
      return right.longTaskMaxMs - left.longTaskMaxMs
    }
    if (right.maxFrameMs !== left.maxFrameMs) return right.maxFrameMs - left.maxFrameMs
    if (right.p95FrameMs !== left.p95FrameMs) return right.p95FrameMs - left.p95FrameMs
    if (right.droppedFrames !== left.droppedFrames) return right.droppedFrames - left.droppedFrames
    return right.timeToInteractiveMs - left.timeToInteractiveMs
  })
  return ranked[0] ?? null
}

function writeReport(report: BenchmarkReport, output: string): string {
  const outputPath = path.resolve(WEB_ROOT, output)
  const filePath =
    path.extname(outputPath).toLowerCase() === '.json'
      ? outputPath
      : path.join(
          outputPath,
          `canvas-interactions-${report.fixture.name}-${report.fixture.nodeCount}-nodes-${report.generatedAt.replace(/[:.]/g, '-')}.json`,
        )

  mkdirSync(path.dirname(filePath), { recursive: true })
  writeFileSync(filePath, `${JSON.stringify(report, null, 2)}\n`, 'utf8')
  return filePath
}

async function main() {
  const options = parseArgs(process.argv.slice(2))
  const fixture = createStructureHeavyFixture(options.nodeCount)
  const generatedAt = new Date().toISOString()
  const pathResults: PathResult[] = []

  let browser: Awaited<ReturnType<typeof chromium.launch>> | null = null
  let context: BrowserContext | null = null

  try {
    await smoke.startDevServer()
    browser = await chromium.launch({ headless: !options.headed })
    context = await browser.newContext({
      viewport: options.viewport,
      deviceScaleFactor: 1,
    })
    context.setDefaultTimeout(30_000)
    await installMockApiRoutes(context, fixture)

    for (const pathName of options.paths) {
      pathResults.push(await runPath(context, smoke.baseUrl, fixture, options, pathName))
    }

    const report = buildReport(fixture, options, generatedAt, pathResults)
    const reportPath = writeReport(report, options.output)

    if (options.json) {
      console.log(JSON.stringify(report, null, 2))
    } else {
      console.log(
        `canvas interactions benchmark ${report.verdict.status}: ${options.paths.length} paths, report ${path.relative(WEB_ROOT, reportPath)}`,
      )
      for (const result of report.paths) {
        console.log(
          `${result.path}: tti=${result.timeToInteractiveMs}ms p95=${result.p95FrameMs}ms max=${result.maxFrameMs}ms longTaskMax=${result.longTaskMaxMs}ms longTasks=${result.longTaskCount} dropped=${result.droppedFrames} duration=${result.durationMs}ms`,
        )
      }
      report.verdict.failures.forEach((failure) => console.error(failure))
    }

    if (report.verdict.status === 'fail') {
      process.exitCode = 1
    }
  } catch (error) {
    console.error(error instanceof Error ? error.message : error)
    if (smoke.devServerLogs.length > 0) {
      console.error(smoke.devServerLogs.join('\n'))
    }
    process.exitCode = 1
  } finally {
    await context?.close().catch((error: unknown) => {
      console.warn(
        `Failed to close benchmark browser context: ${error instanceof Error ? error.message : String(error)}`,
      )
    })
    await browser?.close().catch((error: unknown) => {
      console.warn(
        `Failed to close benchmark browser: ${error instanceof Error ? error.message : String(error)}`,
      )
    })
    await smoke.stopDevServer()
  }
}

void main()
