import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { shallowRef } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import type { FlowNodeData } from '../../types/flowTypes'
import { readFlowGraphCommandSideEffects } from '../../composables/flowGraphAdapterCommandSideEffects'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphCommandResult,
} from '../../composables/useCanvasVueFlowGraphAdapter'
import { usePendingImageResultExecution } from './usePendingImageResultNodes'

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

function createHarness(initialNodes: Node[], initialEdges: Edge[] = []) {
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

describe('usePendingImageResultExecution', () => {
  beforeEach(() => {
    animationFrameMocks.waitForNextRenderFrame.mockReset()
    animationFrameMocks.waitForNextRenderFrame.mockResolvedValue(undefined)
  })

  it('runs pending result nodes from reservation through materialization', async () => {
    const harness = createHarness([makeAnchorNode()])
    let reservedNodeIds: string[] = []
    const submitTask = createDeferred<{ taskId: string }>()
    const resolveResult = createDeferred<{
      url: string
      ratio: number
      width: number
      height: number
    }>()

    const run = harness.actions.runPendingResults({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 1,
      resultOptions: { targetRatio: 1, targetWidth: 320 },
      submitTask: () => submitTask.promise,
      resolveResult: () => resolveResult.promise,
      isCurrent: () => true,
      isPendingError: () => false,
      afterTaskIdAttached: vi.fn(async () => undefined),
      onReserved: (ids) => {
        reservedNodeIds = ids
      },
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
      ['addNodes', true],
      ['addEdges', false],
      ['updateNode', false],
      ['replaceGraph', false],
    ])
  })

  it('reuses the first valid empty image child and stacks batched pending nodes', async () => {
    const invalidEmptyNode = makeImageNode('invalid-empty', {
      sourceImageUrl: '',
      outputStatus: 'idle',
    })
    const firstReusableNode = makeImageNode('first-reusable', {
      sourceImageUrl: '',
      outputStatus: 'idle',
    })
    const harness = createHarness(
      [makeAnchorNode(), invalidEmptyNode, firstReusableNode],
      [
        makeImageEdge('invalid-empty', { sourceHandle: 'out:prompt' }),
        makeImageEdge('first-reusable'),
      ],
    )
    let reservedNodeIds: string[] = []

    await harness.actions.runPendingResults({
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
      onReserved: (ids) => {
        reservedNodeIds = ids
      },
    })

    expect(reservedNodeIds[0]).toBe('first-reusable')
    expect(harness.nodes.value).toHaveLength(4)
    expect(readImageSnapshot(harness.nodes.value, 'invalid-empty').outputStatus).toBe('idle')
    expect(harness.nodes.value.find((node) => node.id === reservedNodeIds[0])?.position).toEqual({
      x: 0,
      y: 0,
    })
    expect(harness.nodes.value.find((node) => node.id === reservedNodeIds[1])?.position).toEqual({
      x: 0,
      y: 320,
    })
  })

  it('keeps batched pending nodes in a single undoable creation step', async () => {
    const harness = createHarness([makeAnchorNode()])

    const run = harness.actions.runPendingResults({
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
        .filter((result) => result.command === 'addNodes')
        .map(readRecordHistory),
    ).toEqual([true, false])
  })

  it('keeps persisted pending results when task resolution is still pending', async () => {
    const harness = createHarness([makeAnchorNode()])
    const pendingError = new Error('任务进度推送超时，请稍后查看结果')
    let reservedNodeIds: string[] = []

    const summary = await harness.actions.runPendingResults({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 1,
      submitTask: async () => ({ taskId: 'task-1' }),
      resolveResult: async () => {
        throw pendingError
      },
      isCurrent: () => true,
      isPendingError: (error) => error === pendingError,
      onReserved: (ids) => {
        reservedNodeIds = ids
      },
    })

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
  })

  it('rolls back pending results for non-persisted pending task flows', async () => {
    const harness = createHarness([makeAnchorNode()])
    const pendingError = new Error('任务仍在处理中')
    let reservedNodeIds: string[] = []

    const summary = await harness.actions.runPendingResults({
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
      onReserved: (ids) => {
        reservedNodeIds = ids
      },
    })

    expect(summary).toMatchObject({
      successCount: 0,
      pendingCount: 1,
      failureCount: 0,
    })
    expect(harness.nodes.value.some((node) => node.id === reservedNodeIds[0])).toBe(false)
  })

  it('does not materialize a resolved result when the board is no longer current', async () => {
    const harness = createHarness([makeAnchorNode()])
    let isCurrent = true
    let reservedNodeIds: string[] = []

    const summary = await harness.actions.runPendingResults({
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
      onReserved: (ids) => {
        reservedNodeIds = ids
      },
    })

    const resultNodeId = reservedNodeIds[0]
    expect(summary.successCount).toBe(1)
    expect(readImageSnapshot(harness.nodes.value, resultNodeId)).toMatchObject({
      sourceImageUrl: '',
      outputStatus: 'running',
      pendingTaskId: 'task-1',
    })
  })

  it('cancels a failed pending result before the delayed edge append resolves', async () => {
    const nextFrame = createDeferred<void>()
    animationFrameMocks.waitForNextRenderFrame.mockReturnValueOnce(nextFrame.promise)
    const harness = createHarness([makeAnchorNode()])
    let reservedNodeIds: string[] = []

    await harness.actions.runPendingResults({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      outputCount: 1,
      submitTask: async () => {
        throw new Error('submit failed')
      },
      resolveResult: async () => ({ url: 'https://example.com/result.png', ratio: 1 }),
      isCurrent: () => true,
      isPendingError: () => false,
      onReserved: (ids) => {
        reservedNodeIds = ids
      },
    })

    expect(harness.nodes.value.some((node) => node.id === reservedNodeIds[0])).toBe(false)

    nextFrame.resolve(undefined)
    await flushPromises()

    expect(harness.edges.value).toEqual([])
  })

  it('resolves an already attached pending task only while it is still current', async () => {
    const harness = createHarness([
      makeImageNode('pending-node', {
        sourceImageUrl: '',
        outputStatus: 'running',
        pendingTaskId: 'task-1',
      }),
    ])

    const didResolve = await harness.actions.resolveAttachedPendingResult({
      resultNodeId: 'pending-node',
      taskId: 'task-1',
      resolveResult: async () => ({
        url: 'https://example.com/restored.png',
        ratio: 1,
        width: 400,
        height: 400,
      }),
      isCurrent: () => true,
      isAttached: () => true,
    })

    expect(didResolve).toBe(true)
    expect(readImageSnapshot(harness.nodes.value, 'pending-node')).toMatchObject({
      sourceImageUrl: 'https://example.com/restored.png',
      outputStatus: 'ready',
    })
    expect(readImageSnapshot(harness.nodes.value, 'pending-node').pendingTaskId).toBeUndefined()
  })
})
