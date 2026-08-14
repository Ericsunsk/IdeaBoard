import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { shallowRef } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import { TaskResultPendingError } from '../../../../shared/api/taskEvents'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import { CanvasBillingPreflightError } from '../task-runner/canvasBillingPreflight'
import { useCanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import { createCanvasGraphStore } from '../../graph-runtime'
import { flowToCanvasBoardSnapshot } from '../../utils/flowSnapshotAdapter'
import { useFlowImageWorkflowExecution } from './flowImageWorkflowExecution'
import { createCanvasExecutionRuntime } from './CanvasExecutionRuntime'
import { useCanvasExecutionWorkspace } from '../workspace/useCanvasExecutionWorkspace'

type MockTaskImageResult = {
  url: string
  width: number
  height: number
  resultIndex?: number
}

const executorMocks = vi.hoisted(() => ({
  assertTaskRequestReady: vi.fn(async () => undefined),
  submitTaskRequest: vi.fn(async () => ({ taskId: 'task-1' })),
  waitExistingOrLiveTaskImageResults: vi.fn<() => Promise<MockTaskImageResult[]>>(async () => [
    {
      url: 'https://example.com/result.png',
      width: 400,
      height: 200,
      resultIndex: 0,
    },
  ]),
}))

const imageSizeMocks = vi.hoisted(() => ({
  loadImageSizeFromUrl: vi.fn(async () => ({ width: 400, height: 400 })),
}))

const localInpaintMaskMocks = vi.hoisted(() => ({
  prepareFlowLocalInpaintMask: vi.fn(async () => 'https://example.com/mask.png'),
}))

vi.mock('../task-runner/flowImageDirectActionExecutor', () => executorMocks)
vi.mock('../media/imageSize', () => imageSizeMocks)
vi.mock('../media/flowLocalInpaintMask', () => localInpaintMaskMocks)
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
  const currentBoardId = shallowRef('board-a')
  const executionRuntime = createCanvasExecutionRuntime()
  const attachment = executionRuntime.attachBoard(currentBoardId.value)
  nodes.value.forEach((node) => {
    const snapshot = node.data?.snapshotNode as ImageNodeItem | undefined
    const taskId = snapshot?.pendingTaskId?.trim()
    if (snapshot?.type === 'image-node' && snapshot.outputStatus === 'running' && taskId) {
      executionRuntime.hydratePending(attachment, node.id, taskId)
    }
  })
  const cancelTasks = vi.fn()
  const createCurrentCanvasGraphStore = (
    snapshotOptions?: Parameters<typeof flowToCanvasBoardSnapshot>[2],
  ) =>
    createCanvasGraphStore(
      flowToCanvasBoardSnapshot(
        {
          nodes: nodes.value,
          edges: edges.value as Parameters<typeof flowToCanvasBoardSnapshot>[0]['edges'],
        },
        undefined,
        snapshotOptions,
      ),
    )
  const executionWorkspace = useCanvasExecutionWorkspace({
    nodes,
    edges,
    graphAdapter,
    createCurrentCanvasGraphStore,
    flushAutosave,
    executionRuntime,
    cancelTasks,
    onSuccess,
    onWarning,
  })
  const execution = useFlowImageWorkflowExecution({
    nodes,
    edges,
    graphAdapter,
    createCurrentCanvasGraphStore,
    flushAutosave,
    onSuccess,
    onWarning,
    executionWorkspace,
    cancelTasks,
  })

  return {
    nodes,
    edges,
    flushAutosave,
    onSuccess,
    onWarning,
    currentBoardId,
    executionRuntime,
    execution,
  }
}

function imageActionPlan() {
  return {
    activationId: 'canvas:test-activation',
    kind: 'image-action' as const,
    sourceNodeId: 'image-1',
    source: {
      url: 'https://example.com/source.png',
      ratio: 1,
    },
    action: IMAGE_NODE_ACTION.removeBg,
    prompt: '',
    outputCount: 1,
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
    activationId: 'canvas:test-activation',
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
    localInpaintMaskMocks.prepareFlowLocalInpaintMask.mockReset()
    executorMocks.assertTaskRequestReady.mockResolvedValue(undefined)
    executorMocks.submitTaskRequest.mockResolvedValue({ taskId: 'task-1' })
    executorMocks.waitExistingOrLiveTaskImageResults.mockResolvedValue([
      {
        url: 'https://example.com/result.png',
        width: 400,
        height: 200,
        resultIndex: 0,
      },
    ])
    imageSizeMocks.loadImageSizeFromUrl.mockResolvedValue({ width: 400, height: 400 })
    localInpaintMaskMocks.prepareFlowLocalInpaintMask.mockResolvedValue(
      'https://example.com/mask.png',
    )
  })

  it('executes image actions through the unified pending image batch path', async () => {
    const submitTask = createDeferred<{ taskId: string }>()
    const taskResult = createDeferred<MockTaskImageResult[]>()
    executorMocks.submitTaskRequest.mockImplementationOnce(() => submitTask.promise)
    executorMocks.waitExistingOrLiveTaskImageResults.mockImplementationOnce(
      () => taskResult.promise,
    )
    const { execution, executionRuntime, nodes, edges, flushAutosave, onSuccess, onWarning } =
      createExecutionHarness()

    const run = execution.executeImageWorkflow(imageActionPlan())
    await flushPromises()

    const resultNode = nodes.value.find((node) => node.id !== 'image-1')
    expect(resultNode?.data?.kind).toBe('image')
    expect(execution.isNodeExecutionActive(resultNode!.id)).toBe(true)
    expect(executionRuntime.getNodeState(resultNode!.id)).toEqual({ status: 'submitting' })
    expect(edges.value.find((edge) => edge.target === resultNode!.id)?.animated).toBeUndefined()

    submitTask.resolve({ taskId: 'task-1' })
    await flushPromises()

    expect(flushAutosave).toHaveBeenCalledWith({ silent: true })
    expect(readImageSnapshot(nodes.value, resultNode!.id).pendingTaskId).toBe('task-1')
    expect(executionRuntime.getNodeState(resultNode!.id)).toEqual({
      status: 'running',
      taskId: 'task-1',
    })

    taskResult.resolve([
      {
        url: 'https://example.com/result.png',
        width: 400,
        height: 200,
        resultIndex: 0,
      },
    ])
    await run

    expect(executorMocks.assertTaskRequestReady).toHaveBeenCalledOnce()
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
    expect(execution.isNodeExecutionActive(resultNode!.id)).toBe(false)
    expect(executionRuntime.getNodeState(resultNode!.id)).toEqual({
      status: 'succeeded',
      taskId: 'task-1',
    })
    expect(edges.value.find((edge) => edge.target === resultNode!.id)?.animated).toBeUndefined()
  })

  it('cancels active execution state when the board changes before a result resolves', async () => {
    const taskResult = createDeferred<MockTaskImageResult[]>()
    executorMocks.waitExistingOrLiveTaskImageResults.mockImplementationOnce(
      () => taskResult.promise,
    )
    const { execution, nodes, currentBoardId, executionRuntime } = createExecutionHarness()

    const run = execution.executeImageWorkflow(imageActionPlan())
    await flushPromises()

    const resultNode = nodes.value.find((node) => node.id !== 'image-1')
    await vi.waitFor(() => {
      expect(executionRuntime.getNodeState(resultNode!.id)?.status).toBe('running')
    })

    currentBoardId.value = 'board-b'
    executionRuntime.attachBoard('board-b')
    taskResult.resolve([
      {
        url: 'https://example.com/task-1.png',
        width: 400,
        height: 200,
      },
    ])
    await run

    expect(executionRuntime.getNodeState(resultNode!.id)).toBeNull()
  })

  it('cancels active execution state when a result rejects after the board changes', async () => {
    const taskResult = createDeferred<MockTaskImageResult[]>()
    executorMocks.waitExistingOrLiveTaskImageResults.mockImplementationOnce(
      () => taskResult.promise,
    )
    const { execution, nodes, currentBoardId, executionRuntime } = createExecutionHarness()

    const run = execution.executeImageWorkflow(imageActionPlan())
    await flushPromises()

    const resultNode = nodes.value.find((node) => node.id !== 'image-1')
    await vi.waitFor(() => {
      expect(executionRuntime.getNodeState(resultNode!.id)?.status).toBe('running')
    })

    currentBoardId.value = 'board-b'
    executionRuntime.attachBoard('board-b')
    taskResult.reject(new Error('provider failed after navigation'))
    await run

    expect(executionRuntime.getNodeState(resultNode!.id)).toBeNull()
  })

  it('uses backend materialized local inpaint results by stable result index', async () => {
    executorMocks.waitExistingOrLiveTaskImageResults.mockResolvedValueOnce([
      {
        url: 'https://example.com/local-inpaint-final-0.png',
        width: 400,
        height: 400,
        resultIndex: 0,
      },
      {
        url: 'https://example.com/local-inpaint-final-1.png',
        width: 400,
        height: 200,
        resultIndex: 1,
      },
    ])
    const { execution, nodes, onSuccess, onWarning } = createExecutionHarness()

    await execution.executeImageWorkflow({ ...imageInpaintPlan(), outputCount: 2 })

    const resultNodes = nodes.value.filter((node) => node.id !== 'image-1')
    expect(resultNodes).toHaveLength(2)
    expect(localInpaintMaskMocks.prepareFlowLocalInpaintMask).toHaveBeenCalledWith(
      'data:image/png;base64,mask',
      'board-a',
    )
    expect(executorMocks.submitTaskRequest).toHaveBeenCalledTimes(1)
    expect(executorMocks.submitTaskRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        feature: 'local-inpaint',
        params: expect.objectContaining({
          original_image_url: 'https://example.com/source.png',
          mask_image_url: 'https://example.com/mask.png',
          output_count: 2,
        }),
      }),
      '提交局部重绘任务失败',
    )
    expect(executorMocks.waitExistingOrLiveTaskImageResults).toHaveBeenCalledTimes(1)
    expect(resultNodes.map((node) => readImageSnapshot(nodes.value, node.id))).toMatchObject([
      {
        sourceImageUrl: 'https://example.com/local-inpaint-final-0.png',
        sourceImageRatio: 1,
        outputStatus: 'ready',
      },
      {
        sourceImageUrl: 'https://example.com/local-inpaint-final-1.png',
        sourceImageRatio: 0.5,
        outputStatus: 'ready',
      },
    ])
    expect(onSuccess).toHaveBeenCalledWith('生成完成（2 张）')
    expect(onWarning).not.toHaveBeenCalled()
    expect(resultNodes.every((node) => !execution.isNodeExecutionActive(node.id))).toBe(true)
  })

  it('keeps single-image pending timeout copy from the underlying pending error', async () => {
    executorMocks.waitExistingOrLiveTaskImageResults.mockRejectedValueOnce(
      new TaskResultPendingError('任务进度推送超时，请稍后查看结果'),
    )
    const { execution, executionRuntime, nodes, onSuccess, onWarning } = createExecutionHarness()

    await execution.executeImageWorkflow(imageActionPlan())

    const resultNode = nodes.value.find((node) => node.id !== 'image-1')
    expect(resultNode?.data?.kind).toBe('image')
    expect(onWarning).toHaveBeenCalledWith('任务进度推送超时，请稍后查看结果')
    expect(onSuccess).not.toHaveBeenCalled()
    expect(readImageSnapshot(nodes.value, resultNode!.id)).toMatchObject({
      pendingTaskId: 'task-1',
      outputStatus: 'running',
    })
    expect(execution.isNodeExecutionActive(resultNode!.id)).toBe(true)
    expect(executionRuntime.getNodeState(resultNode!.id)).toEqual({
      status: 'running',
      taskId: 'task-1',
    })
  })

  it('keeps identical activations as distinct pending tasks and idempotency requests', async () => {
    executorMocks.submitTaskRequest
      .mockResolvedValueOnce({ taskId: 'task-first' })
      .mockResolvedValueOnce({ taskId: 'task-second' })
    executorMocks.waitExistingOrLiveTaskImageResults.mockRejectedValue(
      new TaskResultPendingError('任务进度推送超时，请稍后查看结果'),
    )
    const { execution, nodes } = createExecutionHarness()

    await execution.executeImageWorkflow({
      ...imageActionPlan(),
      activationId: 'canvas:activation-1',
    })
    await execution.executeImageWorkflow({
      ...imageActionPlan(),
      activationId: 'canvas:activation-2',
    })

    const pendingResultNodes = nodes.value.filter(
      (node) => node.id !== 'image-1' && node.data?.kind === 'image',
    )
    expect(pendingResultNodes).toHaveLength(2)
    expect(
      pendingResultNodes.map((node) => readImageSnapshot(nodes.value, node.id).pendingTaskId),
    ).toEqual(['task-first', 'task-second'])
    expect(executorMocks.submitTaskRequest).toHaveBeenCalledTimes(2)
    const submitCalls = executorMocks.submitTaskRequest.mock.calls as unknown as Array<
      [{ client_action_id?: string }]
    >
    const submittedIds = submitCalls.map(([request]) => request.client_action_id)
    expect(submittedIds[0]).toMatch(/^canvas:/)
    expect(submittedIds[1]).toMatch(/^canvas:/)
    expect(submittedIds[0]).not.toBe(submittedIds[1])
  })

  it('retries one network submit within an activation without creating another pending task', async () => {
    executorMocks.submitTaskRequest
      .mockRejectedValueOnce(new CanvasBillingPreflightError('network', '网络错误，请重试'))
      .mockResolvedValueOnce({ taskId: 'task-retried' })
    executorMocks.waitExistingOrLiveTaskImageResults.mockRejectedValueOnce(
      new TaskResultPendingError('任务进度推送超时，请稍后查看结果'),
    )
    const { execution, nodes } = createExecutionHarness()

    await execution.executeImageWorkflow({
      ...imageActionPlan(),
      activationId: 'canvas:activation-retry',
    })

    const pendingResultNodes = nodes.value.filter(
      (node) => node.id !== 'image-1' && node.data?.kind === 'image',
    )
    expect(pendingResultNodes).toHaveLength(1)
    expect(readImageSnapshot(nodes.value, pendingResultNodes[0].id).pendingTaskId).toBe(
      'task-retried',
    )
    expect(executorMocks.submitTaskRequest).toHaveBeenCalledTimes(2)
    const submitCalls = executorMocks.submitTaskRequest.mock.calls as unknown as Array<
      [{ client_action_id?: string }]
    >
    expect(submitCalls.map(([request]) => request.client_action_id)).toEqual([
      expect.stringMatching(/^canvas:/),
      submitCalls[0][0].client_action_id,
    ])
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
    expect(execution.isNodeExecutionActive('image-1')).toBe(false)
    expect(execution.isNodeExecutionActive('image-2.25')).toBe(false)
  })

  it('restores pending execution state while reattaching running nodes', async () => {
    const taskResult = createDeferred<MockTaskImageResult[]>()
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

    expect(execution.isNodeExecutionActive('image-1')).toBe(true)

    taskResult.resolve([
      {
        url: 'https://example.com/task-1.png',
        width: 400,
        height: 200,
      },
    ])
    await run

    expect(execution.isNodeExecutionActive('image-1')).toBe(false)
    expect(edges.value.find((edge) => edge.id === 'connection-1')?.animated).toBeUndefined()
  })
})
