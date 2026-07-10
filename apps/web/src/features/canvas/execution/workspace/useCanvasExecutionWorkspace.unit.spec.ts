import type { Edge, Node } from '@vue-flow/core'
import { shallowRef, type Ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SubmitTaskData } from '../../../../shared/api/task'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import type { CanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import type { TextPromptExecutionPlan } from '../planning/ImageExecutionPlan'
import type { ResultMaterializerResult } from '../result-nodes/resultMaterializer'
import type {
  PendingImageResultExecution,
  PendingImageResultRunSummary,
  RunPendingImageResultsParams,
} from '../result-nodes/usePendingImageResultNodes'
import type { CanvasTaskSubmissionAdapter } from './canvasTaskSubmissionAdapter'
import { useCanvasExecutionWorkspace } from './useCanvasExecutionWorkspace'

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
    projection: {
      updatedEdges: [],
    },
  }
}

function makeTextPromptPlan(
  overrides: Partial<TextPromptExecutionPlan> = {},
): TextPromptExecutionPlan {
  return {
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
    materializeResolvedAttachedPendingResults: vi.fn(() => makeMaterializerResult()),
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
  runPendingResults: (params: RunPendingImageResultsParams) => Promise<PendingImageResultRunSummary>
  submitTaskRequest?: CanvasTaskSubmissionAdapter['submitTaskRequest']
  createClientActionId?: () => string
}): {
  workspace: ReturnType<typeof useCanvasExecutionWorkspace>
  pendingResults: PendingImageResultExecution
  taskAdapter: CanvasTaskSubmissionAdapter
  waitTaskImageResults: ReturnType<typeof vi.fn>
  flushAutosave: ReturnType<typeof vi.fn>
  onSuccess: ReturnType<typeof vi.fn>
  onWarning: ReturnType<typeof vi.fn>
} {
  const nodes = shallowRef<Node[]>(options.nodes ?? [])
  const edges = shallowRef<Edge[]>([])
  const graphAdapter = createGraphAdapter(edges)
  const pendingResults = createPendingResults(options.runPendingResults)
  const taskAdapter = createTaskAdapter(options.submitTaskRequest)
  const waitTaskImageResults = vi.fn(async () => [
    {
      url: 'https://example.com/result.png',
      width: 400,
      height: 200,
    },
  ])
  const flushAutosave = vi.fn(async () => undefined)
  const onSuccess = vi.fn()
  const onWarning = vi.fn()
  const workspace = useCanvasExecutionWorkspace({
    nodes,
    edges,
    graphAdapter,
    createCurrentCanvasGraphStore: vi.fn(() => ({})) as never,
    flushAutosave,
    getCurrentBoardId: () => 'board-a',
    pendingResults,
    taskAdapter,
    waitTaskImageResults,
    createClientActionId: options.createClientActionId,
    onSuccess,
    onWarning,
  })

  return {
    workspace,
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
          await params.resolveResult(taskId, 'image-2')
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

  it('reuses the reserved pending result client action id across retries', async () => {
    const createClientActionId = vi.fn(() => 'client-action-1')
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

    expect(createClientActionId).toHaveBeenCalledTimes(1)
    expect(submittedClientActionIds).toEqual(['client-action-1', 'client-action-1'])
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
    const { workspace, pendingResults, waitTaskImageResults, flushAutosave, onSuccess } =
      createWorkspaceHarness({
        nodes: [makePendingImageNode('image-2', 'task-1')],
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
