import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import type { FlowNodeData } from '../../types/flowTypes'
import { readFlowGraphCommandSideEffects } from '../../composables/flowGraphAdapterCommandSideEffects'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphCommandResult,
} from '../../composables/useCanvasVueFlowGraphAdapter'
import { usePendingImageResultExecution } from './usePendingImageResultNodes'

type Deferred<T> = {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (reason?: unknown) => void
}

const ANCHOR_NODE_ID = 'prompt-1'

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

function makeAnchorNode(id = ANCHOR_NODE_ID): Node {
  return {
    id,
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
        text: '',
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

function makeImageEdge(target: string, overrides: Partial<Edge> = {}): Edge {
  return {
    id: `edge-${target}`,
    source: ANCHOR_NODE_ID,
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

function readMarkDirty(result: CanvasVueFlowGraphCommandResult): boolean | undefined {
  return readFlowGraphCommandSideEffects(result.metadata)?.markDirty
}

function readTransactionSummary(result: CanvasVueFlowGraphCommandResult):
  | {
      reason?: string
      didChange?: boolean
      shouldRecordHistory?: boolean
      shouldMarkDirty?: boolean
    }
  | undefined {
  return readFlowGraphCommandSideEffects(result.metadata)?.transactionSummary as
    | {
        reason?: string
        didChange?: boolean
        shouldRecordHistory?: boolean
        shouldMarkDirty?: boolean
      }
    | undefined
}

describe('usePendingImageResultExecution', () => {
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
      sourceNodeId: ANCHOR_NODE_ID,
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
      ['replaceGraph', true],
      ['replaceNodes', false],
      ['replaceGraph', false],
    ])
  })

  it('batches multi-output reservation, task attachment, and materialization writeback', async () => {
    const harness = createHarness([makeAnchorNode()])

    const summary = await harness.actions.runPendingResults({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: ANCHOR_NODE_ID,
      outputCount: 3,
      resultOptions: { targetRatio: 1, targetWidth: 280 },
      submitTask: async (resultNodeId) => ({ taskId: `task-${resultNodeId}` }),
      resolveResult: async (_taskId, resultNodeId) => ({
        url: `https://example.com/${resultNodeId}.png`,
        ratio: 1,
        width: 400,
        height: 400,
      }),
      isCurrent: () => true,
      isPendingError: () => false,
    })
    await flushPromises()

    expect(summary).toMatchObject({
      totalCount: 3,
      successCount: 3,
      pendingCount: 0,
      failureCount: 0,
    })
    expect(summary.resultNodeIds).toHaveLength(3)
    expect(
      summary.resultNodeIds.map((nodeId) => readImageSnapshot(harness.nodes.value, nodeId)),
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ sourceImageUrl: expect.stringContaining('/image-') }),
      ]),
    )
    expect(
      harness.commandResults.map((result) => [
        result.command,
        readRecordHistory(result),
        readMarkDirty(result),
      ]),
    ).toEqual([
      ['replaceGraph', true, false],
      ['replaceNodes', false, false],
      ['replaceGraph', false, true],
    ])
    expect(harness.commandResults.map((result) => readTransactionSummary(result)?.reason)).toEqual([
      'result-materialize',
      'result-materialize',
      'result-materialize',
    ])
    expect(
      harness.commandResults.map((result) => readTransactionSummary(result)?.didChange),
    ).toEqual([true, true, true])
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
      sourceNodeId: ANCHOR_NODE_ID,
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
      sourceNodeId: ANCHOR_NODE_ID,
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

    expect(harness.commandResults.map(readRecordHistory).filter(Boolean)).toEqual([true])
  })

  it('keeps persisted pending results when task resolution is still pending', async () => {
    const harness = createHarness([makeAnchorNode()])
    const pendingError = new Error('任务进度推送超时，请稍后查看结果')
    let reservedNodeIds: string[] = []

    const summary = await harness.actions.runPendingResults({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: ANCHOR_NODE_ID,
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
      sourceNodeId: ANCHOR_NODE_ID,
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

  it('batches multi-output partial failure and cancellation cleanup', async () => {
    const harness = createHarness([makeAnchorNode()])
    const pendingError = new Error('任务仍在处理中')
    const hardError = new Error('submit failed')
    let submitIndex = 0
    let reservedNodeIds: string[] = []

    const summary = await harness.actions.runPendingResults({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: ANCHOR_NODE_ID,
      outputCount: 3,
      persistPendingTaskIds: false,
      submitTask: async () => {
        submitIndex += 1
        return { taskId: `task-${submitIndex}` }
      },
      resolveResult: async (taskId) => {
        if (taskId === 'task-1') return { url: 'https://example.com/result.png', ratio: 1 }
        if (taskId === 'task-2') throw pendingError
        throw hardError
      },
      isCurrent: () => true,
      isPendingError: (error) => error === pendingError,
      onReserved: (ids) => {
        reservedNodeIds = ids
      },
    })

    expect(summary).toMatchObject({
      successCount: 1,
      pendingCount: 1,
      failureCount: 1,
      firstFailure: pendingError,
    })
    expect(readImageSnapshot(harness.nodes.value, reservedNodeIds[0])).toMatchObject({
      sourceImageUrl: 'https://example.com/result.png',
      outputStatus: 'ready',
    })
    expect(harness.nodes.value.some((node) => node.id === reservedNodeIds[1])).toBe(false)
    expect(harness.nodes.value.some((node) => node.id === reservedNodeIds[2])).toBe(false)
    expect(harness.commandResults.map((result) => result.command)).toEqual([
      'replaceGraph',
      'replaceGraph',
      'removeNodes',
    ])
  })

  it('does not materialize a resolved result when the board is no longer current', async () => {
    const harness = createHarness([makeAnchorNode()])
    let isCurrent = true
    let reservedNodeIds: string[] = []

    const summary = await harness.actions.runPendingResults({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: ANCHOR_NODE_ID,
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

  it('cancels a failed pending result from the reservation batch', async () => {
    const harness = createHarness([makeAnchorNode()])
    let reservedNodeIds: string[] = []

    await harness.actions.runPendingResults({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: ANCHOR_NODE_ID,
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

  it('materializes attached pending tasks in one batch writeback', async () => {
    const harness = createHarness(
      [
        makeAnchorNode(),
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
      ],
      [makeImageEdge('image-1', { animated: true }), makeImageEdge('image-2', { animated: true })],
    )

    const summary = await harness.actions.resolveAttachedPendingResults({
      tasks: [
        { resultNodeId: 'image-1', taskId: 'task-1' },
        { resultNodeId: 'image-2', taskId: 'task-2' },
      ],
      resolveResult: async (taskId) => ({
        url: `https://example.com/${taskId}.png`,
        ratio: 1,
        width: 400,
        height: 400,
      }),
      isCurrent: () => true,
      isAttached: () => true,
    })

    expect(summary).toMatchObject({
      successCount: 2,
      failureCount: 0,
      firstFailure: null,
    })
    expect(readImageSnapshot(harness.nodes.value, 'image-1')).toMatchObject({
      sourceImageUrl: 'https://example.com/task-1.png',
      outputStatus: 'ready',
      pendingTaskId: undefined,
    })
    expect(readImageSnapshot(harness.nodes.value, 'image-2')).toMatchObject({
      sourceImageUrl: 'https://example.com/task-2.png',
      outputStatus: 'ready',
      pendingTaskId: undefined,
    })
    expect(harness.edges.value.every((edge) => edge.animated === false)).toBe(true)
    expect(
      harness.commandResults.map((result) => [
        result.command,
        readRecordHistory(result),
        readMarkDirty(result),
      ]),
    ).toEqual([['replaceGraph', false, true]])
    expect(readTransactionSummary(harness.commandResults[0]!)).toMatchObject({
      reason: 'result-materialize',
      didChange: true,
      shouldMarkDirty: true,
    })
  })
})
