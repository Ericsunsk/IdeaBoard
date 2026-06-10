import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { IMAGE_NODE_ACTION } from '../types/imageActions'
import type { FlowNodeData } from '../types/flowTypes'
import type { ImageNodeItem } from '../types/canvasNodes'
import { useFlowImageDirectActions } from './useFlowImageDirectActions'

const mocks = vi.hoisted(() => ({
  assertCanvasExtendSubmitReady: vi.fn(),
  assertDirectActionSubmitReady: vi.fn(),
  assertHdUpscaleSubmitReady: vi.fn(),
  assertPromptImageSubmitReady: vi.fn(),
  submitCanvasExtendTask: vi.fn(),
  submitDirectActionTask: vi.fn(),
  submitHdUpscaleTask: vi.fn(),
  submitImageInpaintTask: vi.fn(),
  submitPromptImageGenerationTask: vi.fn(),
  submitPromptTextGenerationTask: vi.fn(),
  waitExistingOrLiveTaskResult: vi.fn(),
  resolveTaskImageRatio: vi.fn(),
}))

vi.mock('../services/flowImageDirectActionExecutor', () => ({
  assertCanvasExtendSubmitReady: mocks.assertCanvasExtendSubmitReady,
  assertDirectActionSubmitReady: mocks.assertDirectActionSubmitReady,
  assertHdUpscaleSubmitReady: mocks.assertHdUpscaleSubmitReady,
  assertPromptImageSubmitReady: mocks.assertPromptImageSubmitReady,
  submitCanvasExtendTask: mocks.submitCanvasExtendTask,
  submitDirectActionTask: mocks.submitDirectActionTask,
  submitHdUpscaleTask: mocks.submitHdUpscaleTask,
  submitImageInpaintTask: mocks.submitImageInpaintTask,
  submitPromptImageGenerationTask: mocks.submitPromptImageGenerationTask,
  submitPromptTextGenerationTask: mocks.submitPromptTextGenerationTask,
  waitExistingOrLiveTaskResult: mocks.waitExistingOrLiveTaskResult,
}))

vi.mock('../utils/flowImageDirectActionTaskResult', () => ({
  normalizeTaskImageUrls: (urls: string[]) => urls.map((url) => url.trim()).filter(Boolean),
  pickFirstValidUrl: (urls: string[]) => urls.map((url) => url.trim()).find(Boolean) ?? null,
  resolveTaskImageRatio: mocks.resolveTaskImageRatio,
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
}

function makeImageSnapshot(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: 'https://example.com/source.png',
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [],
    ...overrides,
  }
}

function makeImageNode(nodeId: string, snapshot: ImageNodeItem): Node<FlowNodeData> {
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

function createHarness(initialNodes: Node<FlowNodeData>[], initialBoardId = 'board-a') {
  const boardId = ref(initialBoardId)
  const nodes = ref<Node[]>(initialNodes as Node[])
  const edges = ref<Edge[]>([])
  const flushAutosave = vi.fn(async () => undefined)
  const onSuccess = vi.fn()
  const onWarning = vi.fn()
  const commitDirty = vi.fn()

  const actions = useFlowImageDirectActions({
    nodes,
    edges,
    commitGraphMutation: (mutator) => {
      const next = mutator(nodes.value, edges.value)
      nodes.value = next.nodes
      edges.value = next.edges
      commitDirty()
    },
    commitNodeMutation: (mutator) => {
      const next = mutator(nodes.value)
      if (next === nodes.value) return
      nodes.value = next
      commitDirty()
    },
    commitEdgeMutation: (mutator) => {
      const next = mutator(edges.value)
      if (next === edges.value) return
      edges.value = next
      commitDirty()
    },
    commitDirty,
    flushAutosave,
    getCurrentBoardId: () => boardId.value,
    onSuccess,
    onWarning,
  })

  return {
    actions,
    boardId,
    nodes,
    edges,
    flushAutosave,
    onSuccess,
    onWarning,
  }
}

function findImageNode(nodes: Node[], nodeId: string): ImageNodeItem {
  const snapshot = nodes.find((node) => node.id === nodeId)?.data?.snapshotNode
  if (!snapshot || typeof snapshot !== 'object') throw new Error(`missing node ${nodeId}`)
  return snapshot as ImageNodeItem
}

function findPendingResultNode(nodes: Node[]): Node {
  const node = nodes.find((item) => {
    const snapshot = item.data?.snapshotNode as ImageNodeItem | undefined
    return snapshot?.type === 'image-node' && snapshot.outputStatus === 'running'
  })
  if (!node) throw new Error('missing pending result node')
  return node
}

describe('useFlowImageDirectActions pending task recovery', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.assertDirectActionSubmitReady.mockResolvedValue(undefined)
    mocks.assertHdUpscaleSubmitReady.mockResolvedValue(undefined)
    mocks.resolveTaskImageRatio.mockImplementation(async (_url: string, fallback: number) =>
      fallback > 0 ? fallback : 1,
    )
  })

  it('persists task id after submit and clears it after completion', async () => {
    const submitTask = createDeferred<string>()
    const waitTask = createDeferred<string[]>()
    mocks.submitDirectActionTask.mockReturnValue(submitTask.promise)
    mocks.waitExistingOrLiveTaskResult.mockReturnValue(waitTask.promise)
    const harness = createHarness([makeImageNode('image-1', makeImageSnapshot())])

    const run = harness.actions.runImageDirectAction('image-1', IMAGE_NODE_ACTION.removeBg)
    await flushPromises()
    const pendingNode = findPendingResultNode(harness.nodes.value)
    expect((pendingNode.data?.snapshotNode as ImageNodeItem).pendingTaskId).toBeUndefined()

    submitTask.resolve('task-1')
    await flushPromises()
    expect(findImageNode(harness.nodes.value, pendingNode.id).pendingTaskId).toBe('task-1')
    expect(harness.flushAutosave).toHaveBeenCalledWith({ silent: true })

    waitTask.resolve(['https://example.com/result.png'])
    await run

    const completed = findImageNode(harness.nodes.value, pendingNode.id)
    expect(completed.sourceImageUrl).toBe('https://example.com/result.png')
    expect(completed.outputStatus).toBe('ready')
    expect(completed.pendingTaskId).toBeUndefined()
    expect(harness.onSuccess).toHaveBeenCalledWith('处理完成')
  })

  it('reattaches a loaded pending result once and saves the recovered image', async () => {
    const waitTask = createDeferred<string[]>()
    mocks.waitExistingOrLiveTaskResult.mockReturnValue(waitTask.promise)
    const harness = createHarness([
      makeImageNode(
        'image-2',
        makeImageSnapshot({
          id: 2,
          sourceImageUrl: '',
          pendingTaskId: 'task-2',
          outputStatus: 'running',
          outputImages: [],
        }),
      ),
    ])

    const first = harness.actions.reattachPendingImageResults()
    const second = harness.actions.reattachPendingImageResults()
    await flushPromises()
    expect(mocks.waitExistingOrLiveTaskResult).toHaveBeenCalledTimes(1)

    waitTask.resolve(['https://example.com/recovered.png'])
    await Promise.all([first, second])

    const recovered = findImageNode(harness.nodes.value, 'image-2')
    expect(recovered.sourceImageUrl).toBe('https://example.com/recovered.png')
    expect(recovered.outputStatus).toBe('ready')
    expect(recovered.pendingTaskId).toBeUndefined()
    expect(harness.flushAutosave).toHaveBeenCalledWith({ silent: true })
    expect(harness.onSuccess).toHaveBeenCalledWith('已恢复生成结果')
  })

  it('does not overwrite the current board when reattach resolves after a board switch', async () => {
    const waitTask = createDeferred<string[]>()
    mocks.waitExistingOrLiveTaskResult.mockReturnValue(waitTask.promise)
    const harness = createHarness([
      makeImageNode(
        'image-3',
        makeImageSnapshot({
          id: 3,
          sourceImageUrl: '',
          pendingTaskId: 'task-3',
          outputStatus: 'running',
          outputImages: [],
        }),
      ),
    ])

    const reattach = harness.actions.reattachPendingImageResults()
    await flushPromises()
    harness.boardId.value = 'board-b'
    waitTask.resolve(['https://example.com/stale.png'])
    await reattach

    const stale = findImageNode(harness.nodes.value, 'image-3')
    expect(stale.sourceImageUrl).toBe('')
    expect(stale.outputStatus).toBe('running')
    expect(stale.pendingTaskId).toBe('task-3')
    expect(harness.onSuccess).not.toHaveBeenCalled()
  })
})
