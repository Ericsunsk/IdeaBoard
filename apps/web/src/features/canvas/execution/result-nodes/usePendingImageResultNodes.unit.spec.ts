import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { shallowRef } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  TaskCancelledError,
  TaskResultPendingError,
  isTaskResultPendingError,
} from '../../../../shared/api/taskEvents'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import type { FlowNodeData } from '../../types/flowTypes'
import { readFlowGraphCommandSideEffects } from '../../composables/flowGraphAdapterCommandSideEffects'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphCommandResult,
} from '../../composables/useCanvasVueFlowGraphAdapter'
import { createCanvasGraphStore } from '../../graph-runtime'
import { flowToCanvasBoardSnapshot } from '../../utils/flowSnapshotAdapter'
import {
  usePendingImageResultExecution,
  type PendingImageResultExecution,
  type PendingImageResultReservationParams,
  type RunReservedPendingImageResultsParams,
} from './usePendingImageResultNodes'

const animationFrameMocks = vi.hoisted(() => ({
  waitForNextRenderFrame: vi.fn<() => Promise<void>>(async () => undefined),
}))
vi.mock('../../utils/animationFrame', () => ({
  waitForNextRenderFrame: animationFrameMocks.waitForNextRenderFrame,
}))

type Deferred<T> = {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (reason?: unknown) => void
}

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

function makeAnchorNode(id = 'anchor'): Node {
  return {
    id,
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: {
      kind: 'prompt',
      snapshotNode: {
        width: 280,
      },
    },
  }
}

function makePromptNode(nodeId = 'prompt-1'): Node {
  return {
    id: nodeId,
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: {
      kind: 'prompt',
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

function makeImageSnapshot(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: 'https://example.com/image.png',
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [],
    ...overrides,
  }
}

function makeImageNode(
  nodeId: string,
  snapshotOverrides: Partial<ImageNodeItem> = {},
): Node<FlowNodeData> {
  const snapshot = makeImageSnapshot(snapshotOverrides)
  return {
    id: nodeId,
    type: 'canvas',
    position: { x: snapshot.x, y: snapshot.y },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: 'Image',
      snapshotNode: snapshot,
    },
  }
}

function setNodeDimensions(node: Node, dimensions: { width: number; height: number }) {
  Object.assign(node, { dimensions })
}

function makeImageEdge(target: string, overrides: Partial<Edge> = {}): Edge {
  return {
    id: `edge-${target}`,
    source: 'anchor',
    sourceHandle: 'out:image',
    target,
    targetHandle: 'in:image',
    ...overrides,
  }
}

function createHarness(
  initialNodes: Node[],
  initialEdges: Edge[] = [],
  isNodeExecutionActive: (nodeId: string) => boolean = () => false,
) {
  const nodes: Ref<Node[]> = shallowRef([...initialNodes])
  const edges: Ref<Edge[]> = shallowRef([...initialEdges])
  const commandResults: CanvasVueFlowGraphCommandResult[] = []
  const graphAdapter = useCanvasVueFlowGraphAdapter({
    nodes,
    edges,
    afterCommand: (result) => {
      commandResults.push(result)
    },
  })

  const actions = usePendingImageResultExecution({
    nodes,
    edges,
    graphAdapter,
    createCurrentCanvasGraphStore: (snapshotOptions) =>
      createCanvasGraphStore(
        flowToCanvasBoardSnapshot(
          {
            nodes: nodes.value,
            edges: edges.value as Parameters<typeof flowToCanvasBoardSnapshot>[0]['edges'],
          },
          undefined,
          snapshotOptions,
        ),
      ),
    isNodeExecutionActive,
  })

  return {
    actions,
    nodes,
    edges,
    commandResults,
  }
}

function readImageSnapshot(nodes: Node[], nodeId: string): ImageNodeItem {
  const snapshot = nodes.find((node) => node.id === nodeId)?.data?.snapshotNode
  if (!snapshot || typeof snapshot !== 'object') throw new Error(`missing image node ${nodeId}`)
  return snapshot as ImageNodeItem
}

function readRecordHistory(result: CanvasVueFlowGraphCommandResult): boolean | undefined {
  return readFlowGraphCommandSideEffects(result.metadata)?.recordHistory
}

type PendingResultTestRunParams = PendingImageResultReservationParams &
  Omit<RunReservedPendingImageResultsParams, 'resultNodeIds' | 'isResultExecutionCurrent'> & {
    isResultExecutionCurrent?: RunReservedPendingImageResultsParams['isResultExecutionCurrent']
  }

function startPendingResultRun(
  actions: PendingImageResultExecution,
  params: PendingResultTestRunParams,
) {
  const { source, sourceNodeId, outputCount, resultOptions, ...runParams } = params
  const reservation = actions.reservePendingResults({
    source,
    sourceNodeId,
    outputCount,
    resultOptions,
  })
  return {
    resultNodeIds: reservation.resultNodeIds,
    run: actions.runReservedPendingResults({
      ...runParams,
      resultNodeIds: reservation.resultNodeIds,
      isResultExecutionCurrent: runParams.isResultExecutionCurrent ?? (() => true),
    }),
  }
}

describe('usePendingImageResultExecution', () => {
  beforeEach(() => {
    animationFrameMocks.waitForNextRenderFrame.mockReset()
    animationFrameMocks.waitForNextRenderFrame.mockResolvedValue(undefined)
  })

  it('runs pending result nodes from reservation through materialization', async () => {
    const harness = createHarness([makeAnchorNode()])
    const submitTask = createDeferred<{ taskId: string }>()
    const resolveResult = createDeferred<{
      url: string
      ratio: number
      width: number
      height: number
    }>()

    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 1,
      resultOptions: { targetRatio: 1, targetWidth: 320 },
      submitTask: () => submitTask.promise,
      resolveResult: () => resolveResult.promise,
      isCurrent: () => true,
      isPendingError: () => false,
      afterTaskIdAttached: vi.fn(async () => undefined),
    })
    await flushPromises()

    const resultNodeId = reservedNodeIds[0]
    expect(resultNodeId).toBeTruthy()
    expect(readImageSnapshot(harness.nodes.value, resultNodeId)).toMatchObject({
      sourceImageUrl: '',
      sourceImageRatio: 1,
      outputStatus: 'running',
      outputImages: [],
      width: 320,
    })

    submitTask.resolve({ taskId: 'task-1' })
    await flushPromises()
    expect(readImageSnapshot(harness.nodes.value, resultNodeId).pendingTaskId).toBe('task-1')

    resolveResult.resolve({
      url: 'https://example.com/result.png',
      ratio: 1,
      width: 400,
      height: 200,
    })
    const summary = await run

    expect(summary).toMatchObject({
      resultNodeIds: [resultNodeId],
      totalCount: 1,
      successCount: 1,
      pendingCount: 0,
      failureCount: 0,
    })
    expect(readImageSnapshot(harness.nodes.value, resultNodeId)).toMatchObject({
      sourceImageUrl: 'https://example.com/result.png',
      sourceImageRatio: 0.5,
      outputStatus: 'ready',
      outputImages: [],
      width: 520,
    })
    expect(readImageSnapshot(harness.nodes.value, resultNodeId).pendingTaskId).toBeUndefined()
    expect(
      harness.commandResults.map((result) => [result.command, readRecordHistory(result)]),
    ).toEqual([
      ['replaceGraph', false],
      ['addEdges', false],
      ['updateNode', false],
      ['replaceGraph', false],
    ])
  })

  it('reuses the first valid empty image child and places additional batch results locally', async () => {
    const invalidEmptyNode = makeImageNode('image-1', {
      id: 1,
      sourceImageUrl: '',
      outputStatus: 'idle',
    })
    const firstReusableNode = makeImageNode('image-2', {
      id: 2,
      sourceImageUrl: '',
      outputStatus: 'idle',
    })
    const harness = createHarness(
      [makePromptNode('prompt-1'), invalidEmptyNode, firstReusableNode],
      [
        makeImageEdge('image-1', { source: 'prompt-1', sourceHandle: 'out:prompt' }),
        makeImageEdge('image-2', { source: 'prompt-1' }),
      ],
    )
    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'prompt-1',
      outputCount: 2,
      resultOptions: { targetRatio: 1, targetWidth: 280 },
      submitTask: async (resultNodeId) => ({ taskId: `task-${resultNodeId}` }),
      resolveResult: async (_taskId, resultNodeId) => ({
        url: `https://example.com/${resultNodeId}.png`,
        ratio: 1,
      }),
      isCurrent: () => true,
      isPendingError: () => false,
    })
    await run

    expect(reservedNodeIds[0]).toBe('image-2')
    expect(harness.nodes.value).toHaveLength(4)
    expect(readImageSnapshot(harness.nodes.value, 'image-1').outputStatus).toBe('idle')
    expect(harness.nodes.value.find((node) => node.id === reservedNodeIds[0])?.position).toEqual({
      x: 0,
      y: 0,
    })
    expect(harness.nodes.value.find((node) => node.id === reservedNodeIds[1])?.position).toEqual({
      x: 380,
      y: 0,
    })
  })

  it('places a derived node to the right of its current trigger without moving existing nodes', async () => {
    const anchor = makeImageNode('image-1', { id: 1, x: 120, y: 80, width: 300 })
    const occupied = makeImageNode('image-2', { id: 2, x: 900, y: 80, width: 280 })
    setNodeDimensions(occupied, { width: 280, height: 306 })
    const harness = createHarness([anchor, occupied])
    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      outputCount: 1,
      resultOptions: { targetRatio: 1, targetWidth: 280 },
      submitTask: async () => ({ taskId: 'task-1' }),
      resolveResult: async () => ({ url: 'https://example.com/result.png', ratio: 1 }),
      isCurrent: () => true,
      isPendingError: () => false,
    })
    await run

    expect(harness.nodes.value.find((node) => node.id === reservedNodeIds[0])?.position).toEqual({
      x: 520,
      y: 80,
    })
    expect(occupied.position).toEqual({ x: 900, y: 80 })
  })

  it('uses a stable nearby fallback when the preferred right-side slot is occupied', async () => {
    const anchor = makeImageNode('image-1', { id: 1, x: 0, y: 0, width: 280 })
    const occupied = makeImageNode('image-2', { id: 2, x: 380, y: 0, width: 280 })
    setNodeDimensions(occupied, { width: 280, height: 306 })
    const harness = createHarness([anchor, occupied])
    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      outputCount: 1,
      resultOptions: { targetRatio: 1, targetWidth: 280 },
      submitTask: async () => ({ taskId: 'task-1' }),
      resolveResult: async () => ({ url: 'https://example.com/result.png', ratio: 1 }),
      isCurrent: () => true,
      isPendingError: () => false,
    })
    await run

    expect(harness.nodes.value.find((node) => node.id === reservedNodeIds[0])?.position).toEqual({
      x: 380,
      y: 320,
    })
  })

  it('derives from a manually moved image node using its current measured bounds', async () => {
    const movedAnchor = makeImageNode('image-1', { id: 1, x: 0, y: 0, width: 280 })
    movedAnchor.position = { x: 1_000, y: 500 }
    setNodeDimensions(movedAnchor, { width: 350, height: 260 })
    const harness = createHarness([movedAnchor])
    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      outputCount: 1,
      resultOptions: { targetRatio: 1, targetWidth: 280 },
      submitTask: async () => ({ taskId: 'task-1' }),
      resolveResult: async () => ({ url: 'https://example.com/result.png', ratio: 1 }),
      isCurrent: () => true,
      isPendingError: () => false,
    })
    await run

    expect(harness.nodes.value.find((node) => node.id === reservedNodeIds[0])?.position).toEqual({
      x: 1_450,
      y: 500,
    })
  })

  it('keeps a large derived batch collision-free when nearby preferred slots are occupied', async () => {
    const anchor = makeImageNode('image-1', { id: 1, x: 0, y: 0, width: 280 })
    const firstOccupied = makeImageNode('image-2', { id: 2, x: 380, y: 0, width: 280 })
    const secondOccupied = makeImageNode('image-3', { id: 3, x: 380, y: 320, width: 280 })
    setNodeDimensions(firstOccupied, { width: 280, height: 306 })
    setNodeDimensions(secondOccupied, { width: 280, height: 306 })
    const harness = createHarness([anchor, firstOccupied, secondOccupied])
    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      outputCount: 4,
      resultOptions: { targetRatio: 1, targetWidth: 280 },
      submitTask: async (resultNodeId) => ({ taskId: `task-${resultNodeId}` }),
      resolveResult: async (_taskId, resultNodeId) => ({
        url: `https://example.com/${resultNodeId}.png`,
        ratio: 1,
      }),
      isCurrent: () => true,
      isPendingError: () => false,
    })
    await run

    const positions = reservedNodeIds.map((nodeId) => {
      const position = harness.nodes.value.find((node) => node.id === nodeId)?.position
      if (!position) throw new Error(`missing derived node ${nodeId}`)
      return position
    })
    expect(positions).toHaveLength(4)
    for (let leftIndex = 0; leftIndex < positions.length; leftIndex += 1) {
      for (let rightIndex = leftIndex + 1; rightIndex < positions.length; rightIndex += 1) {
        const left = positions[leftIndex]
        const right = positions[rightIndex]
        expect(Math.abs(left.x - right.x) >= 280 || Math.abs(left.y - right.y) >= 306).toBe(true)
      }
    }
  })

  it('prefers a moved derived node current bounds over its stale planned anchor hint', async () => {
    const harness = createHarness([makeImageNode('image-1', { id: 1, width: 280 })])
    const { resultNodeIds: firstResultNodeIds, run: firstRun } = startPendingResultRun(
      harness.actions,
      {
        source: { url: 'https://example.com/source.png', ratio: 1 },
        sourceNodeId: 'image-1',
        outputCount: 1,
        resultOptions: { targetRatio: 1, targetWidth: 280 },
        submitTask: async () => ({ taskId: 'task-1' }),
        resolveResult: async () => ({ url: 'https://example.com/first-result.png', ratio: 1 }),
        isCurrent: () => true,
        isPendingError: () => false,
      },
    )
    await firstRun
    const firstResultNodeId = firstResultNodeIds[0]

    const movedDerivedNode = harness.nodes.value.find((node) => node.id === firstResultNodeId)
    if (!movedDerivedNode) throw new Error('missing first derived node')
    movedDerivedNode.position = { x: 1_000, y: 500 }
    setNodeDimensions(movedDerivedNode, { width: 350, height: 260 })
    const { resultNodeIds: secondResultNodeIds, run: secondRun } = startPendingResultRun(
      harness.actions,
      {
        source: { url: 'https://example.com/first-result.png', ratio: 1 },
        sourceNodeId: firstResultNodeId,
        outputCount: 1,
        resultOptions: {
          targetRatio: 1,
          targetWidth: 280,
          anchor: { x: 380, y: 0, width: 280, outputAnchorNodeId: firstResultNodeId },
        },
        submitTask: async () => ({ taskId: 'task-2' }),
        resolveResult: async () => ({ url: 'https://example.com/second-result.png', ratio: 1 }),
        isCurrent: () => true,
        isPendingError: () => false,
      },
    )
    await secondRun
    const secondResultNodeId = secondResultNodeIds[0]

    expect(harness.nodes.value.find((node) => node.id === secondResultNodeId)?.position).toEqual({
      x: 1_450,
      y: 500,
    })
  })

  it('leaves reused and appended edge projection to CanvasExecutionRuntime', async () => {
    const reusableNode = makeImageNode('image-1', {
      id: 1,
      sourceImageUrl: '',
      outputStatus: 'idle',
    })
    const harness = createHarness(
      [makePromptNode('prompt-1'), reusableNode],
      [makeImageEdge('image-1', { id: 'connection-1', source: 'prompt-1', animated: false })],
    )
    const pendingResult = createDeferred<{
      url: string
      ratio: number
    }>()

    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'prompt-1',
      outputCount: 2,
      resultOptions: { targetRatio: 1, targetWidth: 280 },
      submitTask: async (resultNodeId) => ({ taskId: `task-${resultNodeId}` }),
      resolveResult: () => pendingResult.promise,
      isCurrent: () => true,
      isPendingError: (error) => error instanceof Error && error.message === 'pending',
    })
    await flushPromises()

    expect(reservedNodeIds[0]).toBe('image-1')
    const animatedByTarget = new Map(
      harness.edges.value.map((edge) => [edge.target, edge.animated]),
    )
    expect(animatedByTarget.get('image-1')).toBe(false)
    expect(animatedByTarget.get(reservedNodeIds[1])).toBeUndefined()

    pendingResult.reject(new Error('pending'))
    await run
  })

  it('keeps batched pending reservation outside semantic graph history', async () => {
    const harness = createHarness([makeAnchorNode()])

    const { run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 2,
      resultOptions: { targetRatio: 1, targetWidth: 280 },
      submitTask: async (resultNodeId) => ({ taskId: `task-${resultNodeId}` }),
      resolveResult: async (_taskId, resultNodeId) => ({
        url: `https://example.com/${resultNodeId}.png`,
        ratio: 1,
      }),
      isCurrent: () => true,
      isPendingError: () => false,
    })
    await flushPromises()
    await run

    expect(
      harness.commandResults
        .filter((result) => readRecordHistory(result) === true)
        .map((result) => result.command),
    ).toEqual([])
    expect(
      harness.commandResults
        .filter((result) => result.command === 'addNodes')
        .map(readRecordHistory),
    ).toEqual([])
    expect(
      harness.commandResults
        .filter((result) => result.command === 'addEdges')
        .map(readRecordHistory),
    ).toEqual([false])
  })

  it('batch materializes completed run pending results with one graph replacement', async () => {
    const harness = createHarness([makePromptNode('prompt-1')])

    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'prompt-1',
      outputCount: 3,
      resultOptions: { targetRatio: 1, targetWidth: 280 },
      submitTask: async (resultNodeId) => ({ taskId: `task-${resultNodeId}` }),
      resolveResult: async (_taskId, resultNodeId) => ({
        url: `https://example.com/${resultNodeId}.png`,
        ratio: 1,
        width: 400,
        height: 200,
      }),
      isCurrent: () => true,
      isPendingError: () => false,
    })
    const summary = await run
    await flushPromises()

    expect(summary).toMatchObject({
      successCount: 3,
      pendingCount: 0,
      failureCount: 0,
    })
    expect(
      harness.commandResults.filter(
        (command) => command.command === 'replaceGraph' && readRecordHistory(command) === false,
      ),
    ).toHaveLength(2)
    expect(reservedNodeIds.map((nodeId) => readImageSnapshot(harness.nodes.value, nodeId))).toEqual(
      reservedNodeIds.map((nodeId) =>
        expect.objectContaining({
          sourceImageUrl: `https://example.com/${nodeId}.png`,
          sourceImageRatio: 0.5,
          outputStatus: 'ready',
          outputImages: [],
          width: 520,
        }),
      ),
    )
    reservedNodeIds.forEach((nodeId) => {
      expect(readImageSnapshot(harness.nodes.value, nodeId).pendingTaskId).toBeUndefined()
    })
    expect(summary.outcomes).toEqual(
      reservedNodeIds.map((nodeId) => ({
        resultNodeId: nodeId,
        status: 'succeeded',
        taskId: `task-${nodeId}`,
      })),
    )
  })

  it('keeps persisted pending results when task resolution is still pending', async () => {
    const harness = createHarness([makeAnchorNode()])
    const pendingError = new Error('任务进度推送超时，请稍后查看结果')

    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 1,
      submitTask: async () => ({ taskId: 'task-1' }),
      resolveResult: async () => {
        throw pendingError
      },
      isCurrent: () => true,
      isPendingError: (error) => error === pendingError,
    })
    const summary = await run

    const resultNodeId = reservedNodeIds[0]
    expect(summary).toMatchObject({
      successCount: 0,
      pendingCount: 1,
      failureCount: 0,
      firstFailure: pendingError,
    })
    expect(readImageSnapshot(harness.nodes.value, resultNodeId)).toMatchObject({
      sourceImageUrl: '',
      outputStatus: 'running',
      pendingTaskId: 'task-1',
    })
    expect(summary.outcomes).toEqual([{ resultNodeId, status: 'pending', taskId: 'task-1' }])
  })

  it('starts a clean reservation instead of reusing a persisted pending task', async () => {
    const activeNodeIds = new Set<string>()
    const harness = createHarness([makeAnchorNode()], [], (nodeId) => activeNodeIds.has(nodeId))
    const pendingError = new Error('任务进度推送超时，请稍后查看结果')

    const { resultNodeIds: firstReservedNodeIds, run: firstRun } = startPendingResultRun(
      harness.actions,
      {
        source: { url: 'https://example.com/source.png', ratio: 1 },
        sourceNodeId: 'anchor',
        outputCount: 1,
        submitTask: async () => ({ taskId: 'task-1' }),
        resolveResult: async () => {
          throw pendingError
        },
        isCurrent: () => true,
        isPendingError: (error) => error === pendingError,
      },
    )
    await firstRun

    const firstResultNodeId = firstReservedNodeIds[0]
    activeNodeIds.add(firstResultNodeId)
    const submitTask = vi.fn(async () => ({ taskId: 'task-2' }))
    const resolvedTaskIds: string[] = []

    const { resultNodeIds: secondReservedNodeIds, run: secondRun } = startPendingResultRun(
      harness.actions,
      {
        source: { url: 'https://example.com/source.png', ratio: 1 },
        sourceNodeId: 'anchor',
        outputCount: 1,
        submitTask,
        resolveResult: async (taskId) => {
          resolvedTaskIds.push(taskId)
          throw pendingError
        },
        isCurrent: () => true,
        isPendingError: (error) => error === pendingError,
      },
    )
    await secondRun

    expect(secondReservedNodeIds[0]).not.toBe(firstResultNodeId)
    expect(submitTask).toHaveBeenCalledTimes(1)
    expect(resolvedTaskIds).toEqual(['task-2'])
    expect(harness.nodes.value.filter((node) => node.data?.kind === 'image')).toHaveLength(2)
  })

  it('keeps storage materialization failures as pending retry result nodes', async () => {
    const harness = createHarness([makeAnchorNode()])
    const storageFailure = new TaskResultPendingError('图片已生成，但结果保存暂时失败，请稍后重试')

    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 1,
      submitTask: async () => ({ taskId: 'task-storage' }),
      resolveResult: async () => {
        throw storageFailure
      },
      isCurrent: () => true,
      isPendingError: isTaskResultPendingError,
    })
    const summary = await run

    const resultNodeId = reservedNodeIds[0]
    expect(summary).toMatchObject({
      successCount: 0,
      pendingCount: 1,
      failureCount: 0,
      firstFailure: storageFailure,
    })
    expect(readImageSnapshot(harness.nodes.value, resultNodeId)).toMatchObject({
      sourceImageUrl: '',
      outputStatus: 'running',
      pendingTaskId: 'task-storage',
    })
  })

  it('rolls back pending results for non-persisted pending task flows', async () => {
    const harness = createHarness([makeAnchorNode()])
    const pendingError = new Error('任务仍在处理中')

    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 1,
      persistPendingTaskIds: false,
      submitTask: async () => ({ taskId: 'task-1' }),
      resolveResult: async () => {
        throw pendingError
      },
      isCurrent: () => true,
      isPendingError: (error) => error === pendingError,
    })
    const summary = await run

    expect(summary).toMatchObject({
      successCount: 0,
      pendingCount: 1,
      failureCount: 0,
    })
    expect(harness.nodes.value.some((node) => node.id === reservedNodeIds[0])).toBe(false)
    expect(summary.outcomes).toEqual([
      { resultNodeId: reservedNodeIds[0], status: 'cancelled', taskId: 'task-1' },
    ])
  })

  it('reports a cancelled task as a cancelled execution terminal state', async () => {
    const harness = createHarness([makeAnchorNode()])

    const { resultNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 1,
      submitTask: async () => ({ taskId: 'task-cancelled' }),
      resolveResult: async () => {
        throw new TaskCancelledError('用户取消')
      },
      isCurrent: () => true,
      isPendingError: () => false,
    })
    const summary = await run
    const resultNodeId = resultNodeIds[0]

    expect(summary.outcomes).toEqual([
      {
        resultNodeId,
        status: 'cancelled',
        taskId: 'task-cancelled',
      },
    ])
  })

  it('does not materialize a resolved result when the board is no longer current', async () => {
    const harness = createHarness([makeAnchorNode()])
    let isCurrent = true

    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 1,
      submitTask: async () => ({ taskId: 'task-1' }),
      resolveResult: async () => {
        isCurrent = false
        return { url: 'https://example.com/result.png', ratio: 1 }
      },
      isCurrent: () => isCurrent,
      isPendingError: () => false,
    })
    const summary = await run

    const resultNodeId = reservedNodeIds[0]
    expect(summary.successCount).toBe(0)
    expect(summary.outcomes).toEqual([{ resultNodeId, status: 'cancelled', taskId: 'task-1' }])
    expect(readImageSnapshot(harness.nodes.value, resultNodeId)).toMatchObject({
      sourceImageUrl: '',
      outputStatus: 'running',
      pendingTaskId: 'task-1',
    })
  })

  it('does not materialize a resolved result after its exact execution lease is cancelled', async () => {
    const harness = createHarness([makeAnchorNode()])
    let isLeaseCurrent = true

    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 1,
      submitTask: async () => ({ taskId: 'task-lease' }),
      resolveResult: async () => {
        isLeaseCurrent = false
        return { url: 'https://example.com/stale.png', ratio: 1 }
      },
      isCurrent: () => true,
      isResultExecutionCurrent: () => isLeaseCurrent,
      isPendingError: () => false,
    })
    const summary = await run

    const resultNodeId = reservedNodeIds[0]
    expect(summary.successCount).toBe(0)
    expect(summary.outcomes).toEqual([{ resultNodeId, status: 'cancelled', taskId: 'task-lease' }])
    expect(readImageSnapshot(harness.nodes.value, resultNodeId)).toMatchObject({
      sourceImageUrl: '',
      outputStatus: 'running',
      pendingTaskId: 'task-lease',
    })
  })

  it('cancels a failed pending result before the delayed edge append resolves', async () => {
    const nextFrame = createDeferred<void>()
    animationFrameMocks.waitForNextRenderFrame.mockReturnValueOnce(nextFrame.promise)
    const harness = createHarness([makeAnchorNode()])

    const { resultNodeIds: reservedNodeIds, run } = startPendingResultRun(harness.actions, {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 1,
      submitTask: async () => {
        throw new Error('submit failed')
      },
      resolveResult: async () => ({ url: 'https://example.com/result.png', ratio: 1 }),
      isCurrent: () => true,
      isPendingError: () => false,
    })
    await run

    expect(harness.nodes.value.some((node) => node.id === reservedNodeIds[0])).toBe(false)

    nextFrame.resolve(undefined)
    await flushPromises()

    expect(harness.edges.value).toEqual([])
  })

  it('batch materializes resolved attached pending tasks with one graph replacement', () => {
    const harness = createHarness(
      [
        makePromptNode('prompt-1'),
        makeImageNode('image-1', {
          id: 1,
          sourceImageUrl: '',
          outputStatus: 'running',
          pendingTaskId: 'task-1',
        }),
        makeImageNode('image-2', {
          id: 2,
          sourceImageUrl: '',
          outputStatus: 'running',
          pendingTaskId: 'task-2',
        }),
        makeImageNode('image-3', {
          id: 3,
          sourceImageUrl: '',
          outputStatus: 'running',
          pendingTaskId: 'task-3',
        }),
      ],
      [
        makeImageEdge('image-1', { id: 'connection-1', source: 'prompt-1', animated: true }),
        makeImageEdge('image-2', { id: 'connection-2', source: 'prompt-1', animated: true }),
        makeImageEdge('image-3', { id: 'connection-3', source: 'prompt-1', animated: true }),
      ],
    )

    const result = harness.actions.materializeResolvedAttachedPendingResults({
      results: [
        {
          resultNodeId: 'image-1',
          taskId: 'task-1',
          url: 'https://example.com/restored-1.png',
          ratio: 1,
        },
        {
          resultNodeId: 'image-2',
          taskId: 'task-2',
          url: 'https://example.com/restored-2.png',
          ratio: 1,
        },
        {
          resultNodeId: 'image-3',
          taskId: 'task-3',
          url: 'https://example.com/restored-3.png',
          ratio: 1,
        },
      ],
      isAttached: (item) => item.resultNodeId !== 'image-3',
    })

    expect(result.materializedNodeIds).toEqual(['image-1', 'image-2'])
    expect(result.skipped).toEqual([{ resultNodeId: 'image-3', reason: 'detached-node' }])
    expect(
      harness.commandResults.filter((command) => command.command === 'replaceGraph'),
    ).toHaveLength(1)
    expect(readImageSnapshot(harness.nodes.value, 'image-1')).toMatchObject({
      sourceImageUrl: 'https://example.com/restored-1.png',
      outputStatus: 'ready',
    })
    expect(readImageSnapshot(harness.nodes.value, 'image-2')).toMatchObject({
      sourceImageUrl: 'https://example.com/restored-2.png',
      outputStatus: 'ready',
    })
    expect(readImageSnapshot(harness.nodes.value, 'image-3')).toMatchObject({
      sourceImageUrl: '',
      outputStatus: 'running',
      pendingTaskId: 'task-3',
    })
    expect(harness.edges.value.map((edge) => [edge.id, edge.animated])).toEqual([
      ['connection-1', undefined],
      ['connection-2', undefined],
      ['connection-3', undefined],
    ])
  })
})
