import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { performance } from 'node:perf_hooks'
import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { ref, shallowRef } from 'vue'
import type { ImageNodeItem } from '../src/features/canvas-board/domain'
import { readFlowGraphCommandSideEffects } from '../src/features/canvas/composables/flowGraphAdapterCommandSideEffects'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphCommandResult,
} from '../src/features/canvas/composables/useCanvasVueFlowGraphAdapter'
import type { FlowNodeData } from '../src/features/canvas/types/flowTypes'
import { usePendingImageResultExecution } from '../src/features/canvas/execution/result-nodes/usePendingImageResultNodes'
import { createCanvasGraphStore } from '../src/features/canvas/graph-runtime'
import type { ResultMaterializerImageResult } from '../src/features/canvas/execution/result-nodes/resultMaterializer'
import { flowToCanvasBoardSnapshot } from '../src/features/canvas/utils/flowSnapshotAdapter'

type CliOptions = {
  pendingResultCounts: number[]
  outputDir: string
  printJson: boolean
  help: boolean
}

type InteractionCounterName =
  | 'graphCommandCount'
  | 'nativeNodeChangeBatches'
  | 'nativeNodeChangeCount'
  | 'nativeEdgeChangeBatches'
  | 'nativeEdgeChangeCount'
  | 'nodeArrayReplacements'
  | 'edgeArrayReplacements'
  | 'historyCaptureCount'
  | 'historyPushCount'
  | 'autosaveScheduleCount'
  | 'autosaveSaveCount'

type InteractionCounterSnapshot = Record<InteractionCounterName, number> & {
  graphCommands: Record<string, number>
}

type RunningStateMetrics = {
  updateCount: number
  totalMs: number
  maxMs: number
}

type ResolvedTaskImageResult = {
  url: string
  ratio: number
  width?: number
  height?: number
  mimeType?: string
}

type CommandSample = {
  command: string
  didChange: boolean
  recordHistory?: boolean
  markDirty?: boolean
}

type TaskResultPhaseMetrics = {
  durationMs: number
  resultCount: number
  finalNodeCount: number
  finalEdgeCount: number
  counters: InteractionCounterSnapshot
  runningState: RunningStateMetrics
  commands: CommandSample[]
}

type TaskResultProfile = {
  pendingResults: number
  runPendingResults: TaskResultPhaseMetrics & {
    chunkCount: number
  }
  reattachPendingResults: TaskResultPhaseMetrics
  bottleneckSummary: string[]
}

type BenchmarkReport = {
  schemaVersion: 1
  generatedAt: string
  command: string
  fixture: {
    name: 'task-result-heavy'
    pendingResultCounts: number[]
    maxOutputCountPerRun: number
  }
  profiles: TaskResultProfile[]
  recommendation: string
}

type BenchmarkHarness = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  actions: ReturnType<typeof usePendingImageResultExecution>
  counters: InteractionCounterSnapshot
  commands: CommandSample[]
  restoreCounters: () => void
}

const DEFAULT_PENDING_RESULT_COUNTS = [10, 50, 100]
const DEFAULT_OUTPUT_DIR = '.benchmark-reports/canvas-task-results'
const MAX_OUTPUT_COUNT_PER_RUN = 8
const INTERACTION_COUNTER_NAMES: InteractionCounterName[] = [
  'graphCommandCount',
  'nativeNodeChangeBatches',
  'nativeNodeChangeCount',
  'nativeEdgeChangeBatches',
  'nativeEdgeChangeCount',
  'nodeArrayReplacements',
  'edgeArrayReplacements',
  'historyCaptureCount',
  'historyPushCount',
  'autosaveScheduleCount',
  'autosaveSaveCount',
]

const CLI_OPTIONS = readCliOptions(process.argv.slice(2))
if (CLI_OPTIONS.help) {
  printCliHelp()
  process.exit(0)
}

function readCliOptions(args: string[]): CliOptions {
  try {
    return parseCliOptions(args)
  } catch (error) {
    console.error(error instanceof Error ? error.message : error)
    console.error('Run with --help for usage.')
    process.exit(1)
  }
}

function parseCliOptions(args: string[]): CliOptions {
  const options: CliOptions = {
    pendingResultCounts: [...DEFAULT_PENDING_RESULT_COUNTS],
    outputDir: DEFAULT_OUTPUT_DIR,
    printJson: false,
    help: false,
  }

  for (let index = 0; index < args.length; index += 1) {
    const rawArg = args[index]
    if (!rawArg) continue
    if (rawArg === '--help' || rawArg === '-h') {
      options.help = true
      continue
    }
    if (rawArg === '--json') {
      options.printJson = true
      continue
    }

    const [name, inlineValue] = rawArg.split('=', 2)
    const readValue = () => {
      if (inlineValue !== undefined) return inlineValue
      const nextValue = args[index + 1]
      if (!nextValue || nextValue.startsWith('--')) {
        throw new Error(`Missing value for ${name}`)
      }
      index += 1
      return nextValue
    }

    switch (name) {
      case '--pending-results':
        options.pendingResultCounts = parsePendingResultCounts(readValue())
        break
      case '--output':
        options.outputDir = readValue()
        break
      default:
        throw new Error(`Unknown option ${rawArg}`)
    }
  }

  if (options.pendingResultCounts.length === 0) {
    throw new Error('At least one pending result count is required')
  }
  return options
}

function parsePendingResultCounts(rawValue: string): number[] {
  const counts = rawValue
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
    .map((item) => Number(item))
  const invalid = counts.filter(
    (count) => !Number.isInteger(count) || !Number.isFinite(count) || count < 1,
  )
  if (invalid.length > 0) {
    throw new Error(`--pending-results must be positive integers, got ${rawValue}`)
  }
  return Array.from(new Set(counts)).sort((left, right) => left - right)
}

function printCliHelp() {
  console.log(`Canvas task-result-heavy benchmark

Usage:
  npm run benchmark:canvas-task-results -- [options]

Options:
  --pending-results 10,50,100  Comma-separated pending/result counts.
  --output <dir>               Report directory. Defaults to ${DEFAULT_OUTPUT_DIR}.
  --json                       Print the full JSON report to stdout.
  --help                       Show this help.

Notes:
  This benchmark uses the real Pending Result Node mutation path with mocked task
  submissions/results. It does not call the backend or change workflow contracts.
  Reports are written locally and should not be committed to git.
`)
}

function roundMs(value: number): number {
  return Math.round(value * 10) / 10
}

function createCounters(): InteractionCounterSnapshot {
  const counters = { graphCommands: {} } as InteractionCounterSnapshot
  INTERACTION_COUNTER_NAMES.forEach((name) => {
    counters[name] = 0
  })
  return counters
}

function cloneCounters(counters: InteractionCounterSnapshot): InteractionCounterSnapshot {
  return {
    ...counters,
    graphCommands: { ...counters.graphCommands },
  }
}

function installBenchmarkCounters(counters: InteractionCounterSnapshot): () => void {
  type GlobalWindowCarrier = { window?: unknown }
  const globalWithWindow = globalThis as GlobalWindowCarrier
  const previousWindow = globalWithWindow.window
  const nextWindow = {
    ...((previousWindow && typeof previousWindow === 'object'
      ? previousWindow
      : {}) as Record<string, unknown>),
    __canvasInteractionBenchmarkCounters: counters,
  }
  Reflect.set(globalWithWindow, 'window', nextWindow)
  return () => {
    if (previousWindow !== undefined) {
      Reflect.set(globalWithWindow, 'window', previousWindow)
      return
    }
    Reflect.deleteProperty(globalWithWindow, 'window')
  }
}

function makeAnchorNode(): Node {
  return {
    id: 'prompt-1',
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: {
      kind: 'prompt',
      nodeType: 'prompt',
      title: 'Prompt',
      snapshotNode: {
        id: 1,
        type: 'prompt',
        x: 0,
        y: 0,
        width: 280,
        title: 'Prompt',
        text: 'prompt',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    },
  }
}

function makeImageSnapshot(index: number, overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 10_000 + index,
    type: 'image-node',
    x: 380,
    y: index * 320,
    width: 280,
    title: `Pending ${index}`,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    pendingTaskId: `task-${index}`,
    outputStatus: 'running',
    outputImages: [],
    ...overrides,
  }
}

function makePendingImageNode(index: number): Node<FlowNodeData> {
  const snapshot = makeImageSnapshot(index)
  return {
    id: `image-${snapshot.id}`,
    type: 'canvas',
    position: { x: snapshot.x, y: snapshot.y },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: snapshot.title ?? 'Pending',
      snapshotNode: snapshot,
    },
  }
}

function makePendingImageEdge(index: number): Edge {
  const imageNodeId = `image-${10_000 + index}`
  return {
    id: `connection-${index}`,
    source: 'prompt-1',
    sourceHandle: 'out:image',
    target: imageNodeId,
    targetHandle: 'in:image',
    animated: true,
  }
}

function createHarness(initialNodes: Node[], initialEdges: Edge[] = []): BenchmarkHarness {
  const nodes: Ref<Node[]> = shallowRef([...initialNodes])
  const edges: Ref<Edge[]> = shallowRef([...initialEdges])
  const counters = createCounters()
  const commands: CommandSample[] = []
  const restoreCounters = installBenchmarkCounters(counters)

  const graphAdapter = useCanvasVueFlowGraphAdapter({
    nodes,
    edges,
    beforeCommand: (context) => {
      const sideEffects = readFlowGraphCommandSideEffects(context.metadata)
      if (!sideEffects || sideEffects.recordHistory === false) return null
      counters.historyCaptureCount += 1
      return { captured: true }
    },
    afterCommand: (result: CanvasVueFlowGraphCommandResult, historyBefore) => {
      const sideEffects = readFlowGraphCommandSideEffects(result.metadata)
      commands.push({
        command: result.command,
        didChange: result.didChange,
        recordHistory: sideEffects?.recordHistory,
        markDirty: sideEffects?.markDirty,
      })
      if (!sideEffects || !result.didChange) return
      let didRecordChange = result.didChange
      if (historyBefore) {
        counters.historyPushCount += 1
        didRecordChange = true
      }
      if (sideEffects.markDirty !== false && didRecordChange) {
        counters.autosaveScheduleCount += 1
      }
    },
  })
  const actions = usePendingImageResultExecution({
    nodes,
    edges,
    graphAdapter,
    createCurrentCanvasGraphStore: () =>
      createCanvasGraphStore(
        flowToCanvasBoardSnapshot({
          nodes: nodes.value,
          edges: edges.value as Parameters<typeof flowToCanvasBoardSnapshot>[0]['edges'],
        }),
      ),
  })

  return {
    nodes,
    edges,
    actions,
    counters,
    commands,
    restoreCounters,
  }
}

function createRunningStateTracker() {
  const runningNodeIds = ref<Set<string>>(new Set())
  const metrics: RunningStateMetrics = {
    updateCount: 0,
    totalMs: 0,
    maxMs: 0,
  }

  function setNodeRunning(nodeId: string, running: boolean) {
    const startedAt = performance.now()
    const next = new Set(runningNodeIds.value)
    if (running) {
      next.add(nodeId)
    } else {
      next.delete(nodeId)
    }
    runningNodeIds.value = next
    const durationMs = performance.now() - startedAt
    metrics.updateCount += 1
    metrics.totalMs += durationMs
    metrics.maxMs = Math.max(metrics.maxMs, durationMs)
  }

  function readMetrics(): RunningStateMetrics {
    return {
      updateCount: metrics.updateCount,
      totalMs: roundMs(metrics.totalMs),
      maxMs: roundMs(metrics.maxMs),
    }
  }

  return {
    setNodeRunning,
    readMetrics,
  }
}

async function flushAsyncGraphWork() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

function createResolvedTaskImageResult(index: number): ResolvedTaskImageResult {
  return {
    url: `https://example.com/result-${index}.png`,
    ratio: 1,
    width: 512,
    height: 512,
  }
}

function createPhaseMetrics(
  harness: BenchmarkHarness,
  resultCount: number,
  durationMs: number,
  runningState: RunningStateMetrics,
): TaskResultPhaseMetrics {
  return {
    durationMs: roundMs(durationMs),
    resultCount,
    finalNodeCount: harness.nodes.value.length,
    finalEdgeCount: harness.edges.value.length,
    counters: cloneCounters(harness.counters),
    runningState,
    commands: [...harness.commands],
  }
}

async function runPendingResultMaterialization(
  pendingResults: number,
): Promise<TaskResultProfile['runPendingResults']> {
  const harness = createHarness([makeAnchorNode()])
  const runningState = createRunningStateTracker()
  let remaining = pendingResults
  let completed = 0
  let chunkCount = 0
  const startedAt = performance.now()

  try {
    while (remaining > 0) {
      const outputCount = Math.min(MAX_OUTPUT_COUNT_PER_RUN, remaining)
      chunkCount += 1
      const reservedNodeIds: string[] = []
      await harness.actions.runPendingResults({
        source: { url: 'https://example.com/source.png', ratio: 1 },
        sourceNodeId: 'prompt-1',
        outputCount,
        resultOptions: { targetRatio: 1, targetWidth: 280 },
        submitTask: async (resultNodeId) => ({ taskId: `task-${resultNodeId}` }),
        resolveResult: async () => {
          completed += 1
          return createResolvedTaskImageResult(completed)
        },
        isCurrent: () => true,
        isPendingError: () => false,
        onReserved: (ids) => {
          reservedNodeIds.push(...ids)
          ids.forEach((resultNodeId) => runningState.setNodeRunning(resultNodeId, true))
        },
      })
      reservedNodeIds.forEach((resultNodeId) => runningState.setNodeRunning(resultNodeId, false))
      remaining -= outputCount
    }
    await flushAsyncGraphWork()
    const durationMs = performance.now() - startedAt
    return {
      ...createPhaseMetrics(harness, pendingResults, durationMs, runningState.readMetrics()),
      chunkCount,
    }
  } finally {
    harness.restoreCounters()
  }
}

async function runAttachedPendingResultReattach(
  pendingResults: number,
): Promise<TaskResultPhaseMetrics> {
  const pendingNodes = Array.from({ length: pendingResults }, (_, index) =>
    makePendingImageNode(index + 1),
  )
  const pendingEdges = Array.from({ length: pendingResults }, (_, index) =>
    makePendingImageEdge(index + 1),
  )
  const harness = createHarness([makeAnchorNode(), ...pendingNodes], pendingEdges)
  const runningState = createRunningStateTracker()
  const startedAt = performance.now()

  try {
    const resolvedResults: ResultMaterializerImageResult[] = []
    await Promise.allSettled(
      pendingNodes.map(async (node, index) => {
        const taskId = `task-${index + 1}`
        runningState.setNodeRunning(node.id, true)
        try {
          resolvedResults.push({
            ...createResolvedTaskImageResult(index + 1),
            resultNodeId: node.id as ResultMaterializerImageResult['resultNodeId'],
            taskId,
          })
        } finally {
          runningState.setNodeRunning(node.id, false)
        }
      }),
    )
    harness.actions.materializeResolvedAttachedPendingResults({
      results: resolvedResults,
      isAttached: () => true,
    })
    await flushAsyncGraphWork()
    const durationMs = performance.now() - startedAt
    return createPhaseMetrics(harness, pendingResults, durationMs, runningState.readMetrics())
  } finally {
    harness.restoreCounters()
  }
}

function summarizeProfile(profile: Omit<TaskResultProfile, 'bottleneckSummary'>): string[] {
  const batchCounters = profile.runPendingResults.counters
  const reattachCounters = profile.reattachPendingResults.counters
  return [
    `runPendingResults used ${profile.runPendingResults.chunkCount} chunks for ${profile.pendingResults} results.`,
    `runPendingResults scheduled ${batchCounters.autosaveScheduleCount} autosaves and ${batchCounters.historyPushCount} history pushes.`,
    `reattachPendingResults issued ${reattachCounters.graphCommands.replaceGraph ?? 0} replaceGraph commands for ${profile.pendingResults} results.`,
    `running state updates cloned Sets ${profile.runPendingResults.runningState.updateCount + profile.reattachPendingResults.runningState.updateCount} times.`,
  ]
}

async function runProfile(pendingResults: number): Promise<TaskResultProfile> {
  const runPendingResults = await runPendingResultMaterialization(pendingResults)
  const reattachPendingResults = await runAttachedPendingResultReattach(pendingResults)
  const profile = {
    pendingResults,
    runPendingResults,
    reattachPendingResults,
  }
  return {
    ...profile,
    bottleneckSummary: summarizeProfile(profile),
  }
}

function createRecommendation(profiles: readonly TaskResultProfile[]): string {
  const largest = profiles.at(-1)
  if (!largest) return 'No profiles were recorded.'
  const batchAutosaves = largest.runPendingResults.counters.autosaveScheduleCount
  const reattachReplaceGraph = largest.reattachPendingResults.counters.graphCommands.replaceGraph ?? 0
  if (batchAutosaves >= largest.pendingResults || reattachReplaceGraph >= largest.pendingResults) {
    return [
      'Move Pending/Result writeback toward a transaction-backed batch path.',
      'The current path scales linearly in autosave schedules and per-result replaceGraph commands.',
      'A good target is one transaction summary per user action or reattach batch, with one dirty/autosave schedule.',
    ].join(' ')
  }
  return 'Keep the current writeback path; this benchmark did not show linear command or autosave pressure.'
}

async function createReport(): Promise<BenchmarkReport> {
  const profiles: TaskResultProfile[] = []
  for (const pendingResults of CLI_OPTIONS.pendingResultCounts) {
    profiles.push(await runProfile(pendingResults))
  }
  return {
    schemaVersion: 1,
    generatedAt: new Date().toISOString(),
    command: createReportCommand(),
    fixture: {
      name: 'task-result-heavy',
      pendingResultCounts: CLI_OPTIONS.pendingResultCounts,
      maxOutputCountPerRun: MAX_OUTPUT_COUNT_PER_RUN,
    },
    profiles,
    recommendation: createRecommendation(profiles),
  }
}

function createReportCommand(): string {
  const args = process.argv.slice(2)
  return ['cd apps/web && npm run benchmark:canvas-task-results', args.length ? '--' : '', ...args]
    .filter(Boolean)
    .join(' ')
}

function writeBenchmarkReport(report: BenchmarkReport, options: CliOptions): string {
  const outputDir = path.resolve(process.cwd(), options.outputDir)
  mkdirSync(outputDir, { recursive: true })
  const timestamp = report.generatedAt.replace(/[:.]/g, '-')
  const fileName = ['canvas-task-results', 'task-result-heavy', timestamp].join('-')
  const reportPath = path.join(outputDir, `${fileName}.json`)
  writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`, 'utf8')
  return reportPath
}

function printCompactSummary(report: BenchmarkReport, reportPath: string) {
  const lines = [
    'Canvas task-result-heavy benchmark',
    `Report: ${reportPath}`,
    `Pending result counts: ${report.fixture.pendingResultCounts.join(', ')}`,
    '',
    'Profile summary:',
    ...report.profiles.flatMap((profile) => [
      `- ${profile.pendingResults} results / runPendingResults: duration=${profile.runPendingResults.durationMs}ms chunks=${profile.runPendingResults.chunkCount} autosaves=${profile.runPendingResults.counters.autosaveScheduleCount} history=${profile.runPendingResults.counters.historyPushCount} nodeReplacements=${profile.runPendingResults.counters.nodeArrayReplacements} edgeReplacements=${profile.runPendingResults.counters.edgeArrayReplacements}`,
      `- ${profile.pendingResults} results / reattachPendingResults: duration=${profile.reattachPendingResults.durationMs}ms replaceGraph=${profile.reattachPendingResults.counters.graphCommands.replaceGraph ?? 0} autosaves=${profile.reattachPendingResults.counters.autosaveScheduleCount} nodeReplacements=${profile.reattachPendingResults.counters.nodeArrayReplacements} edgeReplacements=${profile.reattachPendingResults.counters.edgeArrayReplacements}`,
    ]),
    '',
    `Recommendation: ${report.recommendation}`,
  ]
  console.log(lines.join('\n'))
}

async function main() {
  try {
    const report = await createReport()
    const reportPath = writeBenchmarkReport(report, CLI_OPTIONS)
    if (CLI_OPTIONS.printJson) {
      console.log(JSON.stringify(report, null, 2))
      return
    }
    printCompactSummary(report, reportPath)
  } catch (error) {
    console.error(error instanceof Error ? error.message : error)
    process.exitCode = 1
  }
}

void main()
