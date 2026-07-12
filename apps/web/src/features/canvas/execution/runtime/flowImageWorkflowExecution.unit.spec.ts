import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { shallowRef } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import { TaskResultPendingError } from '../../../../shared/api/taskEvents'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import { useCanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import { createCanvasGraphStore } from '../../graph-runtime'
import { flowToCanvasBoardSnapshot } from '../../utils/flowSnapshotAdapter'
import { useFlowImageWorkflowExecution } from './flowImageWorkflowExecution'

const executorMocks = vi.hoisted(() => ({
  assertTaskRequestReady: vi.fn(async () => undefined),
  submitTaskRequest: vi.fn(async () => ({ taskId: 'task-1' })),
  waitExistingOrLiveTaskImageResults: vi.fn(async () => [
    {
      url: 'https://example.com/result.png',
      width: 400,
      height: 200,
    },
  ]),
}))

const imageSizeMocks = vi.hoisted(() => ({
  loadImageSizeFromUrl: vi.fn(async () => ({ width: 400, height: 400 })),
}))

const localInpaintMocks = vi.hoisted(() => ({
  prepareFlowLocalInpaintMedia: vi.fn(async () => ({
    crop: {
      croppedImageUrl: 'https://example.com/cropped-source.png',
      croppedMaskUrl: 'https://example.com/cropped-mask.png',
      bbox: { x: 8, y: 12, width: 64, height: 72 },
      originalSize: { width: 400, height: 400 },
      cropSize: { width: 64, height: 72 },
    },
  })),
  cleanupFlowLocalInpaintMedia: vi.fn(async () => undefined),
}))

const localInpaintResultMocks = vi.hoisted(() => ({
  composeFlowLocalInpaintTaskResult: vi.fn(async () => ({
    url: 'https://example.com/local-inpaint-final.png',
    ratio: 1,
  })),
}))

vi.mock('../task-runner/flowImageDirectActionExecutor', () => executorMocks)
vi.mock('../media/imageSize', () => imageSizeMocks)
vi.mock('../media/flowLocalInpaint', () => localInpaintMocks)
vi.mock('../result-nodes/flowLocalInpaintTaskResult', () => localInpaintResultMocks)
vi.mock('../../utils/animationFrame', () => ({
  waitForNextRenderFrame: vi.fn(async () => undefined),
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
  for (let index = 0; index < 5; index += 1) await Promise.resolve()
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

function makeImageNode(nodeId: string, overrides: Partial<ImageNodeItem> = {}): Node {
  const snapshot = makeImageSnapshot(overrides)
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

function makePendingImageNode(nodeId: string, taskId: string): Node {
  return makeImageNode(nodeId, {
    id: Number(nodeId.slice(nodeId.indexOf('-') + 1)) || 1,
    sourceImageUrl: '',
    pendingTaskId: taskId,
    outputStatus: 'running',
    outputImages: [],
  })
}

function readImageSnapshot(nodes: Node[], nodeId: string): ImageNodeItem {
  const snapshot = nodes.find((node) => node.id === nodeId)?.data?.snapshotNode
  if (!snapshot || typeof snapshot !== 'object') throw new Error(`missing image node ${nodeId}`)
  return snapshot as ImageNodeItem
}

function createExecutionHarness(
  initialNodes: Node[] = [makeImageNode('image-1')],
  initialEdges: Edge[] = [],
) {
  const nodes: Ref<Node[]> = shallowRef(initialNodes)
  const edges: Ref<Edge[]> = shallowRef(initialEdges)
  const graphAdapter = useCanvasVueFlowGraphAdapter({ nodes, edges })
  const flushAutosave = vi.fn(async () => undefined)
  const onSuccess = vi.fn()
  const onWarning = vi.fn()
  const execution = useFlowImageWorkflowExecution({
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
    flushAutosave,
    onSuccess,
    onWarning,
    getCurrentBoardId: () => 'board-a',
  })

  return {
    nodes,
    edges,
    flushAutosave,
    onSuccess,
    onWarning,
    execution,
  }
}

function imageActionPlan() {
  return {
    kind: 'image-action' as const,
    sourceNodeId: 'image-1',
    source: {
      url: 'https://example.com/source.png',
      ratio: 1,
    },
    action: IMAGE_NODE_ACTION.removeBg,
    resultOptions: {
      anchor: {
        x: 0,
        y: 0,
        width: 280,
        outputAnchorNodeId: 'image-1',
      },
    },
  }
}

function imageInpaintPlan() {
  return {
    kind: 'image-inpaint' as const,
    sourceNodeId: 'image-1',
    source: {
      url: 'https://example.com/source.png',
      ratio: 1,
    },
    prompt: 'replace the marked area',
    rawMaskDataUrl: 'data:image/png;base64,mask',
    outputCount: 1,
    referenceImageUrls: [],
    resultOptions: {
      anchor: {
        x: 0,
        y: 0,
        width: 280,
        outputAnchorNodeId: 'image-1',
      },
    },
  }
}

describe('flowImageWorkflowExecution', () => {
  beforeEach(() => {
    executorMocks.assertTaskRequestReady.mockReset()
    executorMocks.submitTaskRequest.mockReset()
    executorMocks.waitExistingOrLiveTaskImageResults.mockReset()
    imageSizeMocks.loadImageSizeFromUrl.mockReset()
    localInpaintMocks.prepareFlowLocalInpaintMedia.mockReset()
    localInpaintMocks.cleanupFlowLocalInpaintMedia.mockReset()
    localInpaintResultMocks.composeFlowLocalInpaintTaskResult.mockReset()
    executorMocks.assertTaskRequestReady.mockResolvedValue(undefined)
    executorMocks.submitTaskRequest.mockResolvedValue({ taskId: 'task-1' })
    executorMocks.waitExistingOrLiveTaskImageResults.mockResolvedValue([
      {
        url: 'https://example.com/result.png',
        width: 400,
        height: 200,
      },
    ])
    imageSizeMocks.loadImageSizeFromUrl.mockResolvedValue({ width: 400, height: 400 })
    localInpaintMocks.prepareFlowLocalInpaintMedia.mockResolvedValue({
      crop: {
        croppedImageUrl: 'https://example.com/cropped-source.png',
        croppedMaskUrl: 'https://example.com/cropped-mask.png',
        bbox: { x: 8, y: 12, width: 64, height: 72 },
        originalSize: { width: 400, height: 400 },
        cropSize: { width: 64, height: 72 },
      },
    })
    localInpaintMocks.cleanupFlowLocalInpaintMedia.mockResolvedValue(undefined)
    localInpaintResultMocks.composeFlowLocalInpaintTaskResult.mockResolvedValue({
      url: 'https://example.com/local-inpaint-final.png',
      ratio: 1,
    })
  })

  it('executes image actions through the unified pending image batch path', async () => {
    const submitTask = createDeferred<{ taskId: string }>()
    const taskResult = createDeferred<Array<{ url: string; width: number; height: number }>>()
    executorMocks.submitTaskRequest.mockImplementationOnce(() => submitTask.promise)
    executorMocks.waitExistingOrLiveTaskImageResults.mockImplementationOnce(
      () => taskResult.promise,
    )
    const { execution, nodes, flushAutosave, onSuccess, onWarning } = createExecutionHarness()

    const run = execution.executeImageWorkflow(imageActionPlan())
    await flushPromises()

    const resultNode = nodes.value.find((node) => node.id !== 'image-1')
    expect(resultNode?.data?.kind).toBe('image')
    expect(execution.isNodeDirectActionRunning(resultNode!.id)).toBe(true)

    submitTask.resolve({ taskId: 'task-1' })
    await flushPromises()

    expect(flushAutosave).toHaveBeenCalledWith({ silent: true })
    expect(readImageSnapshot(nodes.value, resultNode!.id).pendingTaskId).toBe('task-1')

    taskResult.resolve([
      {
        url: 'https://example.com/result.png',
        width: 400,
        height: 200,
      },
    ])
    await run

    expect(executorMocks.assertTaskRequestReady).toHaveBeenCalledTimes(2)
    expect(executorMocks.submitTaskRequest).toHaveBeenCalledTimes(1)
    expect(executorMocks.submitTaskRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        client_action_id: expect.stringMatching(/^canvas:[A-Za-z0-9_.:-]+$/),
      }),
      undefined,
    )
    expect(executorMocks.waitExistingOrLiveTaskImageResults).toHaveBeenCalledWith('task-1')
    expect(onSuccess).toHaveBeenCalledWith('处理完成')
    expect(onWarning).not.toHaveBeenCalled()

    const snapshot = readImageSnapshot(nodes.value, resultNode!.id)
    expect(snapshot).toMatchObject({
      sourceImageUrl: 'https://example.com/result.png',
      sourceImageRatio: 0.5,
      outputStatus: 'ready',
      outputImages: [],
    })
    expect(snapshot.pendingTaskId).toBeUndefined()
    expect(execution.isNodeDirectActionRunning(resultNode!.id)).toBe(false)
  })

  it('keeps local inpaint result nodes alive across pending result timeouts', async () => {
    executorMocks.waitExistingOrLiveTaskImageResults
      .mockRejectedValueOnce(new TaskResultPendingError('任务进度推送超时，请稍后查看结果'))
      .mockResolvedValueOnce([
        {
          url: 'https://example.com/cropped-result.png',
          width: 64,
          height: 72,
        },
      ])
    const { execution, nodes, onSuccess, onWarning } = createExecutionHarness()

    await execution.executeImageWorkflow(imageInpaintPlan())

    const resultNode = nodes.value.find((node) => node.id !== 'image-1')
    expect(resultNode?.data?.kind).toBe('image')
    expect(executorMocks.waitExistingOrLiveTaskImageResults).toHaveBeenCalledTimes(2)
    expect(localInpaintResultMocks.composeFlowLocalInpaintTaskResult).toHaveBeenCalledWith(
      expect.objectContaining({
        originalImageUrl: 'https://example.com/source.png',
        croppedTaskResults: [
          {
            url: 'https://example.com/cropped-result.png',
            width: 64,
            height: 72,
          },
        ],
      }),
    )
    expect(readImageSnapshot(nodes.value, resultNode!.id)).toMatchObject({
      sourceImageUrl: 'https://example.com/local-inpaint-final.png',
      sourceImageRatio: 1,
      outputStatus: 'ready',
      outputImages: [],
    })
    expect(onSuccess).toHaveBeenCalledWith('生成完成')
    expect(onWarning).not.toHaveBeenCalled()
    expect(execution.isNodeDirectActionRunning(resultNode!.id)).toBe(false)
  })

  it('keeps single-image pending timeout copy from the underlying pending error', async () => {
    executorMocks.waitExistingOrLiveTaskImageResults.mockRejectedValueOnce(
      new TaskResultPendingError('任务进度推送超时，请稍后查看结果'),
    )
    const { execution, nodes, onSuccess, onWarning } = createExecutionHarness()

    await execution.executeImageWorkflow(imageActionPlan())

    const resultNode = nodes.value.find((node) => node.id !== 'image-1')
    expect(resultNode?.data?.kind).toBe('image')
    expect(onWarning).toHaveBeenCalledWith('任务进度推送超时，请稍后查看结果')
    expect(onSuccess).not.toHaveBeenCalled()
    expect(readImageSnapshot(nodes.value, resultNode!.id)).toMatchObject({
      pendingTaskId: 'task-1',
      outputStatus: 'running',
    })
    expect(execution.isNodeDirectActionRunning(resultNode!.id)).toBe(false)
  })

  it('keeps the canvas editable without creating result nodes when provider readiness is unavailable', async () => {
    executorMocks.assertTaskRequestReady.mockRejectedValueOnce(
      new Error('暂无可用生成服务，请稍后重试'),
    )
    const { execution, nodes, onSuccess, onWarning } = createExecutionHarness()

    await execution.executeImageWorkflow(imageActionPlan())

    expect(executorMocks.assertTaskRequestReady).toHaveBeenCalledTimes(1)
    expect(executorMocks.submitTaskRequest).not.toHaveBeenCalled()
    expect(nodes.value).toHaveLength(1)
    expect(readImageSnapshot(nodes.value, 'image-1')).toMatchObject({
      sourceImageUrl: 'https://example.com/source.png',
      outputStatus: 'ready',
    })
    expect(onWarning).toHaveBeenCalledWith('暂无可用生成服务，请稍后重试')
    expect(onSuccess).not.toHaveBeenCalled()
  })

  it('reattaches pending image results through the workflow public API', async () => {
    executorMocks.waitExistingOrLiveTaskImageResults.mockImplementation(
      async (...args: unknown[]) => {
        const taskId = String(args[0])
        return [
          {
            url: `https://example.com/${taskId}.png`,
            width: 400,
            height: 200,
          },
        ]
      },
    )
    const { execution, nodes, flushAutosave, onSuccess, onWarning } = createExecutionHarness([
      makePendingImageNode('image-1', 'task-1'),
      makePendingImageNode('image-2.25', 'task-2'),
    ])

    await execution.reattachPendingImageResults()

    expect(executorMocks.waitExistingOrLiveTaskImageResults).toHaveBeenCalledWith('task-1')
    expect(executorMocks.waitExistingOrLiveTaskImageResults).toHaveBeenCalledWith('task-2')
    expect(readImageSnapshot(nodes.value, 'image-1')).toMatchObject({
      sourceImageUrl: 'https://example.com/task-1.png',
      sourceImageRatio: 0.5,
      outputStatus: 'ready',
    })
    expect(readImageSnapshot(nodes.value, 'image-2.25')).toMatchObject({
      sourceImageUrl: 'https://example.com/task-2.png',
      sourceImageRatio: 0.5,
      outputStatus: 'ready',
    })
    expect(readImageSnapshot(nodes.value, 'image-1').pendingTaskId).toBeUndefined()
    expect(readImageSnapshot(nodes.value, 'image-2.25').pendingTaskId).toBeUndefined()
    expect(onSuccess).toHaveBeenCalledWith('已恢复 2 个生成结果')
    expect(onWarning).not.toHaveBeenCalled()
    expect(flushAutosave).toHaveBeenCalledWith({ silent: true })
    expect(execution.isNodeDirectActionRunning('image-1')).toBe(false)
    expect(execution.isNodeDirectActionRunning('image-2.25')).toBe(false)
  })

  it('animates pending image input edges while reattaching running nodes', async () => {
    const taskResult = createDeferred<Array<{ url: string; width: number; height: number }>>()
    executorMocks.waitExistingOrLiveTaskImageResults.mockImplementationOnce(
      () => taskResult.promise,
    )
    const { execution, edges } = createExecutionHarness(
      [makeImageNode('image-10', { id: 10 }), makePendingImageNode('image-1', 'task-1')],
      [
        {
          id: 'connection-1',
          source: 'image-10',
          sourceHandle: 'out:image',
          target: 'image-1',
          targetHandle: 'in:image',
          animated: false,
        },
      ],
    )

    const run = execution.reattachPendingImageResults()
    await flushPromises()

    expect(execution.isNodeDirectActionRunning('image-1')).toBe(true)
    expect(edges.value.find((edge) => edge.id === 'connection-1')?.animated).toBe(true)

    taskResult.resolve([
      {
        url: 'https://example.com/task-1.png',
        width: 400,
        height: 200,
      },
    ])
    await run

    expect(execution.isNodeDirectActionRunning('image-1')).toBe(false)
    expect(edges.value.find((edge) => edge.id === 'connection-1')?.animated).toBe(false)
  })
})
