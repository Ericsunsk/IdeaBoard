import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref, shallowRef } from 'vue'
import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { TaskResultPendingError } from '../../../../shared/api/taskEvents'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import type { FlowNodeData } from '../../types/flowTypes'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import { useFlowImageDirectActions } from './useFlowImageDirectActions'

const mocks = vi.hoisted(() => ({
  assertTaskRequestReady: vi.fn(),
  submitTaskRequest: vi.fn(),
  waitExistingOrLiveTaskImageResults: vi.fn(),
  resolveTaskImageResultDisplay: vi.fn(),
  prepareFlowLocalInpaintMedia: vi.fn(),
  composeFlowLocalInpaintTaskResult: vi.fn(),
  cleanupFlowLocalInpaintMedia: vi.fn(),
}))

vi.mock('./flowImageDirectActionExecutor', () => ({
  assertTaskRequestReady: mocks.assertTaskRequestReady,
  submitTaskRequest: mocks.submitTaskRequest,
  waitExistingOrLiveTaskImageResults: mocks.waitExistingOrLiveTaskImageResults,
}))

vi.mock('../media/imageSize', () => ({
  loadImageSizeFromUrl: vi.fn(async () => ({ width: 1000, height: 1000 })),
}))

vi.mock('../media/flowLocalInpaint', () => ({
  prepareFlowLocalInpaintMedia: mocks.prepareFlowLocalInpaintMedia,
  cleanupFlowLocalInpaintMedia: mocks.cleanupFlowLocalInpaintMedia,
}))

vi.mock('../results/flowLocalInpaintTaskResult', () => ({
  composeFlowLocalInpaintTaskResult: mocks.composeFlowLocalInpaintTaskResult,
}))

vi.mock('../results/flowImageDirectActionTaskResult', () => ({
  resolveFirstTaskImageResultDisplay: async (
    results: Array<{ url: string; width?: number; height?: number }>,
    fallback: number,
    loadImageSize: (url: string) => Promise<{ width: number; height: number } | null>,
  ) => {
    const result = results.find((item) => item.url.trim())
    if (!result) throw new Error('任务完成但未返回图片')
    const resolved = await mocks.resolveTaskImageResultDisplay(result, fallback, loadImageSize)
    if (!resolved) throw new Error('任务完成但未返回图片')
    return resolved
  },
  resolveTaskImageResultDisplay: mocks.resolveTaskImageResultDisplay,
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
  const nodes: Ref<Node[]> = shallowRef([...initialNodes] as Node[])
  const edges: Ref<Edge[]> = shallowRef([])
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
    mocks.assertTaskRequestReady.mockResolvedValue(undefined)
    mocks.resolveTaskImageResultDisplay.mockImplementation(
      async (result: { url: string; width?: number; height?: number }, fallback: number) => {
        const hasSize = Number(result.width) > 0 && Number(result.height) > 0
        const ratio = hasSize
          ? Number(result.height) / Number(result.width)
          : fallback > 0
            ? fallback
            : 1
        return {
          url: result.url,
          ratio,
          ...(hasSize ? { width: Number(result.width), height: Number(result.height) } : {}),
        }
      },
    )
    mocks.prepareFlowLocalInpaintMedia.mockResolvedValue({
      crop: {
        croppedImageUrl: 'https://example.com/cropped.png',
        croppedMaskUrl: 'https://example.com/cropped-mask.png',
        bbox: { x: 0, y: 0, width: 64, height: 64 },
        originalSize: { width: 1000, height: 1000 },
        cropSize: { width: 64, height: 64 },
      },
      temporaryMaskUrl: 'https://example.com/temp-mask.png',
    })
    mocks.composeFlowLocalInpaintTaskResult.mockResolvedValue({
      url: 'https://example.com/inpaint.png',
      ratio: 1,
    })
    mocks.cleanupFlowLocalInpaintMedia.mockResolvedValue(undefined)
  })

  it('routes direct action readiness failures through warning without creating a pending node', async () => {
    const readinessError = new Error('缺少提示词，请填写 Prompt 或配置后台 system prompt')
    mocks.assertTaskRequestReady.mockRejectedValueOnce(readinessError)
    const harness = createHarness([makeImageNode('image-1', makeImageSnapshot())])

    await expect(
      harness.actions.runImageDirectAction('image-1', IMAGE_NODE_ACTION.removeBg),
    ).resolves.toBeUndefined()

    expect(harness.onWarning).toHaveBeenCalledWith(readinessError.message)
    expect(mocks.submitTaskRequest).not.toHaveBeenCalled()
    expect(harness.flushAutosave).not.toHaveBeenCalled()
    expect(harness.nodes.value).toHaveLength(1)
    expect(
      harness.nodes.value.some((node) => {
        const snapshot = node.data?.snapshotNode as ImageNodeItem | undefined
        return snapshot?.outputStatus === 'running'
      }),
    ).toBe(false)
  })

  it('persists task id after submit and clears it after completion', async () => {
    const submitTask = createDeferred<{ taskId: string }>()
    const waitTask = createDeferred<Array<{ url: string }>>()
    mocks.submitTaskRequest.mockReturnValue(submitTask.promise)
    mocks.waitExistingOrLiveTaskImageResults.mockReturnValue(waitTask.promise)
    const harness = createHarness([makeImageNode('image-1', makeImageSnapshot())])

    const run = harness.actions.runImageDirectAction('image-1', IMAGE_NODE_ACTION.removeBg)
    await flushPromises()
    const pendingNode = findPendingResultNode(harness.nodes.value)
    expect((pendingNode.data?.snapshotNode as ImageNodeItem).pendingTaskId).toBeUndefined()

    submitTask.resolve({ taskId: 'task-1' })
    await flushPromises()
    expect(findImageNode(harness.nodes.value, pendingNode.id).pendingTaskId).toBe('task-1')
    expect(harness.flushAutosave).toHaveBeenCalledWith({ silent: true })

    waitTask.resolve([{ url: 'https://example.com/result.png' }])
    await run

    const completed = findImageNode(harness.nodes.value, pendingNode.id)
    expect(completed.sourceImageUrl).toBe('https://example.com/result.png')
    expect(completed.sourceImageRatio).toBe(1)
    expect(completed.width).toBe(260)
    expect(completed.outputStatus).toBe('ready')
    expect(completed.pendingTaskId).toBeUndefined()
    expect(harness.onSuccess).toHaveBeenCalledWith('处理完成')
  })

  it('reattaches a loaded pending result once and saves the recovered image', async () => {
    const waitTask = createDeferred<Array<{ url: string }>>()
    mocks.waitExistingOrLiveTaskImageResults.mockReturnValue(waitTask.promise)
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
    expect(mocks.waitExistingOrLiveTaskImageResults).toHaveBeenCalledTimes(1)

    waitTask.resolve([{ url: 'https://example.com/recovered.png' }])
    await Promise.all([first, second])

    const recovered = findImageNode(harness.nodes.value, 'image-2')
    expect(recovered.sourceImageUrl).toBe('https://example.com/recovered.png')
    expect(recovered.outputStatus).toBe('ready')
    expect(recovered.pendingTaskId).toBeUndefined()
    expect(harness.flushAutosave).toHaveBeenCalledWith({ silent: true })
    expect(harness.onSuccess).toHaveBeenCalledWith('已恢复生成结果')
  })

  it('does not overwrite the current board when reattach resolves after a board switch', async () => {
    const waitTask = createDeferred<Array<{ url: string }>>()
    mocks.waitExistingOrLiveTaskImageResults.mockReturnValue(waitTask.promise)
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
    waitTask.resolve([{ url: 'https://example.com/stale.png' }])
    await reattach

    const stale = findImageNode(harness.nodes.value, 'image-3')
    expect(stale.sourceImageUrl).toBe('')
    expect(stale.outputStatus).toBe('running')
    expect(stale.pendingTaskId).toBe('task-3')
    expect(harness.onSuccess).not.toHaveBeenCalled()
  })

  it('uses completed image dimensions when resolving canvas extend result display', async () => {
    mocks.submitTaskRequest.mockResolvedValue({ taskId: 'task-extend' })
    mocks.waitExistingOrLiveTaskImageResults.mockResolvedValue([
      { url: 'https://example.com/extended.png', width: 400, height: 100 },
    ])
    const harness = createHarness([makeImageNode('image-1', makeImageSnapshot())])

    await harness.actions.runCanvasExtendAction('image-1', {
      originalImageRect: { left: 0, top: 0, width: 100, height: 100 },
      frameRect: { left: 0, top: 0, width: 100, height: 200 },
      outputCount: 1,
    })

    const completed = harness.nodes.value
      .map((node) => node.data?.snapshotNode as ImageNodeItem | undefined)
      .find((snapshot) => snapshot?.sourceImageUrl === 'https://example.com/extended.png')
    expect(completed?.sourceImageRatio).toBe(0.25)
    expect(mocks.resolveTaskImageResultDisplay).toHaveBeenCalledWith(
      expect.objectContaining({ width: 400, height: 100 }),
      2,
      expect.any(Function),
    )
  })

  it('resizes completed canvas extend result nodes from the actual output dimensions', async () => {
    mocks.submitTaskRequest.mockResolvedValue({ taskId: 'task-extend' })
    mocks.waitExistingOrLiveTaskImageResults.mockResolvedValue([
      { url: 'https://example.com/extended-wide.png', width: 400, height: 200 },
    ])
    const harness = createHarness([makeImageNode('image-1', makeImageSnapshot({ width: 280 }))])

    await harness.actions.runCanvasExtendAction('image-1', {
      originalImageRect: { left: 0, top: 0, width: 100, height: 100 },
      frameRect: { left: -50, top: 0, width: 200, height: 100 },
      outputCount: 1,
    })

    const completed = harness.nodes.value
      .map((node) => node.data?.snapshotNode as ImageNodeItem | undefined)
      .find((snapshot) => snapshot?.sourceImageUrl === 'https://example.com/extended-wide.png')
    expect(completed?.width).toBe(520)
    expect(completed?.sourceImageRatio).toBe(0.5)
  })

  it('removes local inpaint pending nodes when composed results cannot be recovered later', async () => {
    mocks.submitTaskRequest.mockResolvedValue({ taskId: 'task-inpaint' })
    mocks.waitExistingOrLiveTaskImageResults.mockRejectedValue(
      new TaskResultPendingError('任务进度推送超时，请稍后查看结果'),
    )
    const harness = createHarness([makeImageNode('image-1', makeImageSnapshot())])

    await harness.actions.runImageInpaintAction('image-1', {
      prompt: 'replace selected area',
      maskDataUrl: 'data:image/png;base64,mask',
      outputCount: 1,
    })

    expect(mocks.prepareFlowLocalInpaintMedia).toHaveBeenCalledWith({
      originalImageUrl: 'https://example.com/source.png',
      rawMaskDataUrl: 'data:image/png;base64,mask',
      boardId: 'board-a',
    })
    expect(mocks.cleanupFlowLocalInpaintMedia).toHaveBeenCalledWith(
      expect.objectContaining({
        crop: expect.objectContaining({
          croppedImageUrl: 'https://example.com/cropped.png',
        }),
      }),
      'board-a',
    )
    expect(harness.onWarning).toHaveBeenCalledWith('局部重绘仍在处理中，请稍后重试')
    expect(harness.flushAutosave).not.toHaveBeenCalled()
    expect(harness.nodes.value).toHaveLength(1)
    expect(
      harness.nodes.value.some((node) => {
        const snapshot = node.data?.snapshotNode as ImageNodeItem | undefined
        return snapshot?.pendingTaskId === 'task-inpaint'
      }),
    ).toBe(false)
    expect(
      harness.nodes.value.some((node) => {
        const snapshot = node.data?.snapshotNode as ImageNodeItem | undefined
        return snapshot?.outputStatus === 'running' || !!snapshot?.pendingTaskId
      }),
    ).toBe(false)
  })
})
