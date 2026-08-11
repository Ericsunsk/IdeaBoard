import type { Edge, Node } from '@vue-flow/core'
import { shallowRef, type Ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SubmitTaskData } from '../../../../shared/api/task'
import { TaskCancelledError } from '../../../../shared/api/taskEvents'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import type { CanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import type { ActivatedImageExecutionPlan } from '../planning/ImageExecutionPlan'
import type { ResultMaterializerResult } from '../result-nodes/resultMaterializer'
import type {
  PendingImageResultExecution,
  PendingImageResultRunSummary,
  MaterializeResolvedAttachedPendingImageResultsParams,
  RunPendingImageResultsParams,
} from '../result-nodes/usePendingImageResultNodes'
import type { CanvasTaskSubmissionAdapter } from './canvasTaskSubmissionAdapter'
import { useCanvasExecutionWorkspace } from './useCanvasExecutionWorkspace'
import { createCanvasExecutionRuntime } from '../runtime/CanvasExecutionRuntime'

function makeRunSummary(
  overrides: Partial<PendingImageResultRunSummary> = {},
): PendingImageResultRunSummary {
  return {
    resultNodeIds: ['image-2'],
    totalCount: 1,
    successCount: 1,
    pendingCount: 0,
    failureCount: 0,
    firstFailure: null,
    failures: [],
    ...overrides,
  }
}

function makeMaterializerResult(
  materializedNodeIds: string[] = ['image-2'],
): ResultMaterializerResult {
  return {
    summary: {} as ResultMaterializerResult['summary'],
    materializedNodeIds: materializedNodeIds as ResultMaterializerResult['materializedNodeIds'],
    skipped: [],
  }
}

function makeTextPromptPlan(
  overrides: Partial<ActivatedImageExecutionPlan & { kind: 'text-prompt' }> = {},
): ActivatedImageExecutionPlan & { kind: 'text-prompt' } {
  return {
    activationId: 'canvas:test-activation',
    kind: 'text-prompt',
    sourceNodeId: 'prompt-1',
    source: {
      url: '',
      ratio: 1,
    },
    prompt: 'draw a precise workspace boundary',
    outputCount: 1,
    aspectRatio: '1:1' as const,
    resultOptions: {
      anchor: {
        x: 0,
        y: 0,
        width: 280,
        outputAnchorNodeId: 'prompt-1',
      },
    },
    ...overrides,
  }
}

function makePendingImageNode(nodeId: string, taskId: string): Node {
  const snapshot: ImageNodeItem = {
    id: Number(nodeId.slice(nodeId.indexOf('-') + 1)) || 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    pendingTaskId: taskId,
    outputStatus: 'running',
    outputImages: [],
  }
  return {
    id: nodeId,
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: 'Image',
      snapshotNode: snapshot,
    },
  }
}

function createGraphAdapter(edges: Ref<Edge[]>): CanvasVueFlowGraphAdapter {
  return {
    replaceEdges: vi.fn((nextEdges: Edge[]) => {
      edges.value = nextEdges
    }),
  } as unknown as CanvasVueFlowGraphAdapter
}

function createPendingResults(
  runPendingResults: (
    params: RunPendingImageResultsParams,
  ) => Promise<PendingImageResultRunSummary>,
): PendingImageResultExecution {
  return {
    runPendingResults: vi.fn(runPendingResults),
    materializeResolvedAttachedPendingResults: vi.fn(
      (params: MaterializeResolvedAttachedPendingImageResultsParams) =>
        makeMaterializerResult(params.results.map((result) => result.resultNodeId)),
    ),
  }
}

function createTaskAdapter(
  submitTaskRequest?: CanvasTaskSubmissionAdapter['submitTaskRequest'],
): CanvasTaskSubmissionAdapter {
  return {
    assertTaskRequestReady: vi.fn(async () => undefined),
    submitTaskRequest: submitTaskRequest ?? vi.fn(async () => ({ taskId: 'task-1' })),
  }
}

function createWorkspaceHarness(options: {
  nodes?: Node[]
  edges?: Edge[]
  runPendingResults: (params: RunPendingImageResultsParams) => Promise<PendingImageResultRunSummary>
  submitTaskRequest?: CanvasTaskSubmissionAdapter['submitTaskRequest']
  createClientActionId?: () => string
  currentBoardId?: Ref<string>
  materializerResult?: ResultMaterializerResult
  waitTaskImageResults?: (
    taskId: string,
  ) => Promise<Array<{ url: string; width?: number; height?: number }>>
}): {
  workspace: ReturnType<typeof useCanvasExecutionWorkspace>
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  pendingResults: PendingImageResultExecution
  taskAdapter: CanvasTaskSubmissionAdapter
  waitTaskImageResults: ReturnType<typeof vi.fn>
  flushAutosave: ReturnType<typeof vi.fn>
  onSuccess: ReturnType<typeof vi.fn>
  onWarning: ReturnType<typeof vi.fn>
} {
  const nodes = shallowRef<Node[]>(options.nodes ?? [])
  const edges = shallowRef<Edge[]>(options.edges ?? [])
  const currentBoardId = options.currentBoardId ?? shallowRef('board-a')
  const graphAdapter = createGraphAdapter(edges)
  const pendingResults = createPendingResults(options.runPendingResults)
  if (options.materializerResult) {
    vi.mocked(pendingResults.materializeResolvedAttachedPendingResults).mockReturnValue(
      options.materializerResult,
    )
  }
  const taskAdapter = createTaskAdapter(options.submitTaskRequest)
  const waitTaskImageResults = vi.fn(
    options.waitTaskImageResults ??
      (async () => [
        {
          url: 'https://example.com/result.png',
          width: 400,
          height: 200,
        },
      ]),
  )
  const flushAutosave = vi.fn(async () => undefined)
  const onSuccess = vi.fn()
  const onWarning = vi.fn()
  const executionRuntime = createCanvasExecutionRuntime()
  const attachment = executionRuntime.attachBoard(currentBoardId.value)
  nodes.value.forEach((node) => {
    const snapshot = node.data?.snapshotNode as ImageNodeItem | undefined
    const taskId = snapshot?.pendingTaskId?.trim()
    if (
      /^image-\d+(?:\.\d+)?$/.test(node.id) &&
      snapshot?.type === 'image-node' &&
      snapshot.outputStatus === 'running' &&
      taskId
    ) {
      executionRuntime.startNode(attachment, node.id, { status: 'running', taskId })
    }
  })
  const workspace = useCanvasExecutionWorkspace({
    nodes,
    edges,
    graphAdapter,
    createCurrentCanvasGraphStore: vi.fn(() => ({})) as never,
    flushAutosave,
    executionRuntime,
    pendingResults,
    taskAdapter,
    waitTaskImageResults,
    createClientActionId: options.createClientActionId,
    onSuccess,
    onWarning,
  })

  return {
    workspace,
    nodes,
    edges,
    pendingResults,
    taskAdapter,
    waitTaskImageResults,
    flushAutosave,
    onSuccess,
    onWarning,
  }
}

describe('useCanvasExecutionWorkspace', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('submits text prompt generation through the workspace API adapter', async () => {
    const submittedRequests: SubmitTaskData[] = []
    const { workspace, taskAdapter, waitTaskImageResults, onSuccess, onWarning } =
      createWorkspaceHarness({
        createClientActionId: vi.fn(() => 'client-action-1'),
        submitTaskRequest: vi.fn(async (request) => {
          submittedRequests.push(request)
          return { taskId: 'task-1' }
        }),
        runPendingResults: async (params) => {
          params.onReserved?.(['image-2'])
          const { taskId } = await params.submitTask('image-2')
          params.onExecutionStateChange?.({
            nodeId: 'image-2',
            state: { status: 'running', taskId },
          })
          await params.resolveResult(taskId, 'image-2')
          params.onExecutionStateChange?.({
            nodeId: 'image-2',
            state: { status: 'succeeded', taskId },
          })
          return makeRunSummary()
        },
      })

    await workspace.commands.runTextPromptGeneration(makeTextPromptPlan())

    expect(taskAdapter.submitTaskRequest).toHaveBeenCalledTimes(1)
    expect(submittedRequests).toHaveLength(1)
    expect(submittedRequests[0]).toEqual(
      expect.objectContaining({
        prompt: 'draw a precise workspace boundary',
        feature: 'pattern-gen',
        params: { ratio: '1:1' },
        client_action_id: 'client-action-1',
      }),
    )
    expect(waitTaskImageResults).toHaveBeenCalledWith('task-1')
    expect(onSuccess).toHaveBeenCalledWith('生成完成')
    expect(onWarning).not.toHaveBeenCalled()
    expect(workspace.state.executionRuntime.getNodeState('image-2')).toEqual({
      status: 'succeeded',
      taskId: 'task-1',
    })
  })

  it('reports pattern-gen preflight errors without submitting a task', async () => {
    const { workspace, pendingResults, taskAdapter, onSuccess, onWarning } = createWorkspaceHarness(
      {
        runPendingResults: async () => makeRunSummary(),
      },
    )

    await workspace.commands.runTextPromptGeneration(
      makeTextPromptPlan({ aspectRatio: '2:1' as never }),
    )

    expect(pendingResults.runPendingResults).not.toHaveBeenCalled()
    expect(taskAdapter.submitTaskRequest).not.toHaveBeenCalled()
    expect(onWarning).toHaveBeenCalledWith(
      '图案生成参数错误：图案生成尺寸比例仅支持 auto、1:1、3:4、4:3、9:16、16:9',
    )
    expect(onSuccess).not.toHaveBeenCalled()
  })

  it('creates a distinct idempotency id for each deliberate activation', async () => {
    const createClientActionId = vi
      .fn()
      .mockReturnValueOnce('client-action-1')
      .mockReturnValueOnce('client-action-2')
    const submittedClientActionIds: Array<unknown> = []
    const { workspace } = createWorkspaceHarness({
      createClientActionId,
      submitTaskRequest: vi.fn(async (request) => {
        submittedClientActionIds.push(request.client_action_id)
        return { taskId: 'task-1' }
      }),
      runPendingResults: async (params) => {
        params.onReserved?.(['image-2'])
        await params.submitTask('image-2')
        return makeRunSummary()
      },
    })

    await workspace.commands.runTextPromptGeneration(makeTextPromptPlan())
    await workspace.commands.runTextPromptGeneration(makeTextPromptPlan())

    expect(createClientActionId).toHaveBeenCalledTimes(2)
    expect(submittedClientActionIds).toEqual(['client-action-1', 'client-action-2'])
  })

  it('flushes the board snapshot after a pending task id attaches', async () => {
    const { workspace, flushAutosave, onWarning } = createWorkspaceHarness({
      createClientActionId: vi.fn(() => 'client-action-1'),
      runPendingResults: async (params) => {
        params.onReserved?.(['image-2'])
        const { taskId } = await params.submitTask('image-2')
        await params.afterTaskIdAttached?.({ resultNodeId: 'image-2', taskId })
        return makeRunSummary({
          successCount: 0,
          pendingCount: 1,
          firstFailure: new Error('任务进度推送超时，请稍后查看结果'),
        })
      },
    })

    await workspace.commands.runTextPromptGeneration(makeTextPromptPlan())

    expect(flushAutosave).toHaveBeenCalledWith({ silent: true })
    expect(onWarning).toHaveBeenCalledWith('任务进度推送超时，请稍后查看结果')
  })

  it('reattaches saved pending task results through the workspace command', async () => {
    const { workspace, edges, pendingResults, waitTaskImageResults, flushAutosave, onSuccess } =
      createWorkspaceHarness({
        nodes: [makePendingImageNode('image-2', 'task-1')],
        edges: [
          {
            id: 'edge-1',
            source: 'prompt-1',
            target: 'image-2',
            targetHandle: 'in:image',
            animated: false,
          },
        ],
        runPendingResults: async () => makeRunSummary(),
      })

    await workspace.commands.reattachPendingImageResults()

    expect(waitTaskImageResults).toHaveBeenCalledWith('task-1')
    expect(pendingResults.materializeResolvedAttachedPendingResults).toHaveBeenCalledWith({
      results: [
        {
          url: 'https://example.com/result.png',
          ratio: 0.5,
          width: 400,
          height: 200,
          resultNodeId: 'image-2',
          taskId: 'task-1',
        },
      ],
      isAttached: expect.any(Function),
    })
    expect(onSuccess).toHaveBeenCalledWith('已恢复生成结果')
    expect(flushAutosave).toHaveBeenCalledWith({ silent: true })
    expect(workspace.state.isNodeRunning('image-2')).toBe(false)
    expect(workspace.state.executionRuntime.getNodeState('image-2')).toEqual({
      status: 'succeeded',
      taskId: 'task-1',
    })
    expect(edges.value[0]?.animated).toBe(false)
  })

  it('restores a saved pending task as running while reattaching', async () => {
    let resolveTask!: (results: Array<{ url: string; width: number; height: number }>) => void
    const taskResults = new Promise<Array<{ url: string; width: number; height: number }>>(
      (resolve) => {
        resolveTask = resolve
      },
    )
    const { workspace, waitTaskImageResults } = createWorkspaceHarness({
      nodes: [makePendingImageNode('image-2', 'task-1')],
      edges: [
        {
          id: 'edge-1',
          source: 'prompt-1',
          target: 'image-2',
          targetHandle: 'in:image',
          animated: false,
        },
      ],
      waitTaskImageResults: () => taskResults,
      runPendingResults: async () => makeRunSummary(),
    })

    const reattach = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(waitTaskImageResults).toHaveBeenCalledWith('task-1'))

    expect(workspace.state.executionRuntime.getNodeState('image-2')).toEqual({
      status: 'running',
      taskId: 'task-1',
    })

    resolveTask([{ url: 'https://example.com/result.png', width: 400, height: 200 }])
    await reattach
  })

  it('marks a saved pending task as failed without reviving it on a second reattach', async () => {
    const { workspace, onWarning, waitTaskImageResults } = createWorkspaceHarness({
      nodes: [makePendingImageNode('image-2', 'task-1')],
      waitTaskImageResults: async () => {
        throw new Error('provider unavailable')
      },
      runPendingResults: async () => makeRunSummary(),
    })

    await workspace.commands.reattachPendingImageResults()

    expect(workspace.state.executionRuntime.getNodeState('image-2')).toEqual({
      status: 'failed',
      taskId: 'task-1',
    })
    expect(workspace.state.isNodeRunning('image-2')).toBe(false)
    expect(onWarning).toHaveBeenCalledWith('生成任务恢复失败')

    await workspace.commands.reattachPendingImageResults()

    expect(waitTaskImageResults).toHaveBeenCalledTimes(1)
    expect(workspace.state.executionRuntime.getNodeState('image-2')).toEqual({
      status: 'failed',
      taskId: 'task-1',
    })
  })

  it('marks a saved pending task as cancelled when the task is cancelled', async () => {
    const { workspace, onWarning } = createWorkspaceHarness({
      nodes: [makePendingImageNode('image-2', 'task-1')],
      waitTaskImageResults: async () => {
        throw new TaskCancelledError('用户取消')
      },
      runPendingResults: async () => makeRunSummary(),
    })

    await workspace.commands.reattachPendingImageResults()

    expect(workspace.state.executionRuntime.getNodeState('image-2')).toEqual({
      status: 'cancelled',
      taskId: 'task-1',
    })
    expect(workspace.state.isNodeRunning('image-2')).toBe(false)
    expect(onWarning).not.toHaveBeenCalled()
  })

  it('cancels an old Board reattach without projecting onto the new Board', async () => {
    let resolveTask!: (results: Array<{ url: string; width: number; height: number }>) => void
    const currentBoardId = shallowRef('board-a')
    const taskResults = new Promise<Array<{ url: string; width: number; height: number }>>(
      (resolve) => {
        resolveTask = resolve
      },
    )
    const { workspace, edges, waitTaskImageResults } = createWorkspaceHarness({
      nodes: [makePendingImageNode('image-2', 'task-1')],
      edges: [
        {
          id: 'edge-board-a',
          source: 'prompt-1',
          target: 'image-2',
          targetHandle: 'in:image',
          animated: false,
        },
      ],
      currentBoardId,
      waitTaskImageResults: () => taskResults,
      runPendingResults: async () => makeRunSummary(),
    })

    const reattach = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(waitTaskImageResults).toHaveBeenCalledWith('task-1'))
    currentBoardId.value = 'board-b'
    workspace.state.executionRuntime.attachBoard('board-b')
    edges.value = [
      {
        id: 'edge-board-b',
        source: 'prompt-1',
        target: 'image-2',
        targetHandle: 'in:image',
        animated: true,
      },
    ]
    resolveTask([{ url: 'https://example.com/result.png', width: 400, height: 200 }])
    await reattach

    expect(workspace.state.executionRuntime.getNodeState('image-2')).toBeNull()
    expect(edges.value[0]?.animated).toBe(true)
  })

  it('cancels a reattach when its pending node is detached', async () => {
    let resolveTask!: (results: Array<{ url: string; width: number; height: number }>) => void
    const taskResults = new Promise<Array<{ url: string; width: number; height: number }>>(
      (resolve) => {
        resolveTask = resolve
      },
    )
    const { workspace, nodes, pendingResults, waitTaskImageResults } = createWorkspaceHarness({
      nodes: [makePendingImageNode('image-2', 'task-1')],
      waitTaskImageResults: () => taskResults,
      runPendingResults: async () => makeRunSummary(),
    })

    const reattach = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(waitTaskImageResults).toHaveBeenCalledWith('task-1'))
    nodes.value = []
    resolveTask([{ url: 'https://example.com/result.png', width: 400, height: 200 }])
    await reattach

    expect(workspace.state.executionRuntime.getNodeState('image-2')).toEqual({
      status: 'cancelled',
      taskId: 'task-1',
    })
    expect(workspace.state.isNodeRunning('image-2')).toBe(false)
    expect(pendingResults.materializeResolvedAttachedPendingResults).not.toHaveBeenCalled()
  })

  it('reattaches the same saved task only once while it is active', async () => {
    let resolveTask!: (results: Array<{ url: string; width: number; height: number }>) => void
    const taskResults = new Promise<Array<{ url: string; width: number; height: number }>>(
      (resolve) => {
        resolveTask = resolve
      },
    )
    const { workspace, waitTaskImageResults, onSuccess } = createWorkspaceHarness({
      nodes: [makePendingImageNode('image-2', 'task-1')],
      waitTaskImageResults: () => taskResults,
      runPendingResults: async () => makeRunSummary(),
    })

    const firstReattach = workspace.commands.reattachPendingImageResults()
    const duplicateReattach = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(waitTaskImageResults).toHaveBeenCalledTimes(1))
    expect(workspace.state.executionRuntime.getNodeState('image-2')).toEqual({
      status: 'running',
      taskId: 'task-1',
    })

    resolveTask([{ url: 'https://example.com/result.png', width: 400, height: 200 }])
    await Promise.all([firstReattach, duplicateReattach])

    expect(waitTaskImageResults).toHaveBeenCalledTimes(1)
    expect(workspace.state.executionRuntime.getNodeState('image-2')).toEqual({
      status: 'succeeded',
      taskId: 'task-1',
    })
    expect(onSuccess).toHaveBeenCalledTimes(1)
  })

  it('keeps concurrent saved task reattachments isolated', async () => {
    let resolveSecondTask: (results: Array<{ url: string; width: number; height: number }>) => void
    let rejectFirstTask: (error: Error) => void
    const firstTaskResults = new Promise<Array<{ url: string; width: number; height: number }>>(
      (_resolve, reject) => {
        rejectFirstTask = reject
      },
    )
    const secondTaskResults = new Promise<Array<{ url: string; width: number; height: number }>>(
      (resolve) => {
        resolveSecondTask = resolve
      },
    )
    const { workspace, waitTaskImageResults } = createWorkspaceHarness({
      nodes: [makePendingImageNode('image-2', 'task-1'), makePendingImageNode('image-3', 'task-2')],
      edges: [
        {
          id: 'edge-1',
          source: 'prompt-1',
          target: 'image-2',
          targetHandle: 'in:image',
          animated: false,
        },
        {
          id: 'edge-2',
          source: 'prompt-1',
          target: 'image-3',
          targetHandle: 'in:image',
          animated: false,
        },
      ],
      waitTaskImageResults: (taskId) =>
        taskId === 'task-1' ? firstTaskResults : secondTaskResults,
      runPendingResults: async () => makeRunSummary(),
    })

    const reattach = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(waitTaskImageResults).toHaveBeenCalledTimes(2))

    rejectFirstTask!(new Error('provider unavailable'))
    await vi.waitFor(() =>
      expect(workspace.state.executionRuntime.getNodeState('image-2')).toEqual({
        status: 'failed',
        taskId: 'task-1',
      }),
    )
    expect(workspace.state.executionRuntime.getNodeState('image-3')).toEqual({
      status: 'running',
      taskId: 'task-2',
    })

    resolveSecondTask!([{ url: 'https://example.com/result-2.png', width: 400, height: 200 }])
    await reattach

    expect(workspace.state.executionRuntime.getNodeState('image-3')).toEqual({
      status: 'succeeded',
      taskId: 'task-2',
    })
  })

  it('cancels a reattach when the resolved result is not materialized', async () => {
    const skippedResult = makeMaterializerResult([])
    skippedResult.skipped = [{ resultNodeId: 'image-2', reason: 'detached-node' }]
    const { workspace } = createWorkspaceHarness({
      nodes: [makePendingImageNode('image-2', 'task-1')],
      materializerResult: skippedResult,
      runPendingResults: async () => makeRunSummary(),
    })

    await workspace.commands.reattachPendingImageResults()

    expect(workspace.state.executionRuntime.getNodeState('image-2')).toEqual({
      status: 'cancelled',
      taskId: 'task-1',
    })
    expect(workspace.state.isNodeRunning('image-2')).toBe(false)
  })

  it('does not start a reattach for an invalid Canvas node id', async () => {
    const { workspace, waitTaskImageResults, pendingResults } = createWorkspaceHarness({
      nodes: [makePendingImageNode('image-invalid', 'task-1')],
      runPendingResults: async () => makeRunSummary(),
    })

    await workspace.commands.reattachPendingImageResults()

    expect(waitTaskImageResults).not.toHaveBeenCalled()
    expect(pendingResults.materializeResolvedAttachedPendingResults).not.toHaveBeenCalled()
    expect(workspace.state.executionRuntime.getNodeState('image-invalid')).toBeNull()
  })

  it('reports task failures through workspace feedback', async () => {
    const { workspace, onWarning, onSuccess } = createWorkspaceHarness({
      runPendingResults: async (params) => {
        params.onReserved?.(['image-2'])
        return makeRunSummary({
          successCount: 0,
          failureCount: 1,
          firstFailure: new Error('生成服务暂不可用'),
        })
      },
    })

    await workspace.commands.runTextPromptGeneration(makeTextPromptPlan())

    expect(onWarning).toHaveBeenCalledWith('生成服务暂不可用')
    expect(onSuccess).not.toHaveBeenCalled()
  })
})
