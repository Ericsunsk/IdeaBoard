import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { CanvasBoardSnapshot } from '../../../canvas-board/domain'
import type { SubmitTaskData } from '../../../../shared/api/task'
import type { TaskImageResult } from '../../../../shared/query/taskResult'
import { TaskCancelledError } from '../../../../shared/api/taskEvents'
import type { CanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import type { CanvasGraphNodeId, CanvasGraphStore } from '../../graph-runtime'
import { loadImageSizeFromUrl } from '../media/imageSize'
import type { ActivatedImageExecutionPlan } from '../planning/ImageExecutionPlan'
import {
  resolveFirstTaskImageResultDisplay,
  resolveTaskImageResultAtIndexDisplay,
} from '../result-nodes/flowImageDirectActionTaskResult'
import {
  collectPendingImageTasks,
  isPendingImageTaskSnapshot,
  readPendingImageSnapshot,
} from '../result-nodes/flowPendingImageTaskSnapshot'
import type { ResultMaterializerImageResult } from '../result-nodes/resultMaterializer'
import {
  type PendingImageResultExecution,
  usePendingImageResultExecution,
} from '../result-nodes/usePendingImageResultNodes'
import { waitExistingOrLiveTaskImageResults } from '../task-runner/flowImageDirectActionExecutor'
import type { ClientActionIdFactory } from '../task-runner/taskClientActionIds'
import { buildImageExecutionTaskRequest } from '../commands/flowImageTaskRequests'
import type { FlowToCanvasBoardSnapshotOptions } from '../../utils/flowSnapshotAdapter'
import { createPendingImageWorkflowRunner } from '../runtime/pendingImageWorkflowRunner'
import type { ImageSizeLoader } from '../result-nodes/flowImageDirectActionTaskResult'
import {
  createCanvasTaskSubmissionCallbacks,
  type CanvasTaskSubmissionAdapter,
} from './canvasTaskSubmissionAdapter'
import {
  type CanvasExecutionAttachment,
  type CanvasExecutionLease,
  type CanvasExecutionRuntime,
} from '../runtime/CanvasExecutionRuntime'

export type CanvasExecutionWorkspaceState = {
  readonly executionRuntime: CanvasExecutionRuntime
  isNodeRunning: (nodeId: string) => boolean
}

export type CanvasExecutionWorkspaceCommands = {
  runPromptGeneration: (
    plan: ActivatedImageExecutionPlan & { kind: 'prompt-generation' },
  ) => Promise<void>
  reattachPendingImageResults: () => Promise<void>
}

export type CanvasExecutionWorkspace = {
  state: CanvasExecutionWorkspaceState
  commands: CanvasExecutionWorkspaceCommands
}

export type UseCanvasExecutionWorkspaceOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  createCurrentCanvasGraphStore: (
    snapshotOptions?: FlowToCanvasBoardSnapshotOptions,
  ) => CanvasGraphStore
  boardBaseSnapshot?: Ref<CanvasBoardSnapshot>
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  pendingResults?: PendingImageResultExecution
  taskAdapter?: CanvasTaskSubmissionAdapter
  waitTaskImageResults?: (taskId: string) => Promise<TaskImageResult[]>
  loadImageSize?: ImageSizeLoader
  createClientActionId?: ClientActionIdFactory
  executionRuntime: CanvasExecutionRuntime
  cancelTasks: (taskIds: readonly string[]) => void
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: (error: unknown) => boolean
}

function isCanvasGraphNodeId(nodeId: string): nodeId is CanvasGraphNodeId {
  return /^(?:image|prompt|fusion)-\d+(?:\.\d+)?$/.test(nodeId)
}

type ReattachResolvedAttempt = {
  reattachKey: string
  nodeId: CanvasGraphNodeId
  taskId: string
  lease: CanvasExecutionLease
  resolved: ResultMaterializerImageResult
}

export function useCanvasExecutionWorkspace(
  options: UseCanvasExecutionWorkspaceOptions,
): CanvasExecutionWorkspace {
  const executionRuntime = options.executionRuntime
  const activeReattachKeys = new Set<string>()
  const waitTaskImageResults = options.waitTaskImageResults ?? waitExistingOrLiveTaskImageResults
  const loadImageSize = options.loadImageSize ?? loadImageSizeFromUrl
  const ownedPendingResults = usePendingImageResultExecution({
    nodes: options.nodes,
    edges: options.edges,
    graphAdapter: options.graphAdapter,
    createCurrentCanvasGraphStore: options.createCurrentCanvasGraphStore,
    boardBaseSnapshot: options.boardBaseSnapshot,
    refreshNodeInternals: options.refreshNodeInternals,
    isNodeExecutionActive: executionRuntime.isNodeActive,
  })
  const pendingResults = options.pendingResults ?? ownedPendingResults

  function isNodeRunning(nodeId: string): boolean {
    return executionRuntime.isNodeActive(nodeId)
  }

  function transitionOwnedReattachExecution(
    lease: CanvasExecutionLease,
    taskId: string,
    status: 'succeeded' | 'failed' | 'cancelled',
  ): boolean {
    const current = executionRuntime.getNodeState(lease.nodeId)
    if (current?.taskId !== taskId || !executionRuntime.isNodeActive(lease.nodeId)) return false
    return executionRuntime.transitionNode(lease, { status, taskId })
  }

  function requireCurrentAttachment(): CanvasExecutionAttachment {
    const attachment = executionRuntime.attachment.value
    if (!attachment) throw new Error('Canvas execution requires an active Board attachment')
    return attachment
  }

  async function flushPendingTaskSnapshot() {
    try {
      await options.flushAutosave?.({ silent: true })
    } catch (error) {
      console.warn('[canvas] flush pending task snapshot failed', error)
      options.onWarning('画布保存失败，刷新页面后任务进度可能丢失')
    }
  }

  function isPendingImageTaskStillCurrent(nodeId: string, taskId: string): boolean {
    const snapshot = readPendingImageSnapshot(options.nodes.value, nodeId)
    return !!snapshot && isPendingImageTaskSnapshot(snapshot, taskId)
  }

  function isReattachExecutionCurrent(lease: CanvasExecutionLease, taskId: string): boolean {
    const current = executionRuntime.getNodeState(lease.nodeId)
    return (
      executionRuntime.getNodeLease(lease.nodeId) === lease &&
      executionRuntime.isAttachmentCurrent(lease.attachment) &&
      executionRuntime.isNodeActive(lease.nodeId) &&
      current?.taskId === taskId
    )
  }

  const workflowRunner = createPendingImageWorkflowRunner({
    pendingResults,
    executionRuntime,
    cancelTasks: options.cancelTasks,
    afterTaskIdAttached: flushPendingTaskSnapshot,
    createClientActionId: options.createClientActionId,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
    onError: options.onError,
  })

  async function runPromptGeneration(
    plan: ActivatedImageExecutionPlan & { kind: 'prompt-generation' },
  ): Promise<void> {
    const attachment = requireCurrentAttachment()
    let taskResultsPromise: Promise<TaskImageResult[]> | null = null
    let taskRequest: SubmitTaskData | null = null
    const buildTaskRequest = () => {
      taskRequest ??= buildImageExecutionTaskRequest(plan)
      return taskRequest
    }
    const taskSubmission = createCanvasTaskSubmissionCallbacks({
      readinessRequest: buildTaskRequest,
      buildRequest: buildTaskRequest,
      taskAdapter: options.taskAdapter,
    })
    await workflowRunner.run({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      attachment,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      preflight: async () => {
        buildTaskRequest()
      },
      assertReady: taskSubmission.assertReady,
      submitTask: (_resultNodeId, clientActionId) => taskSubmission.submitTask(clientActionId),
      resolveResult: async (taskId, _resultNodeId, resultIndex) => {
        taskResultsPromise ??= waitTaskImageResults(taskId)
        return resolveTaskImageResultAtIndexDisplay(
          await taskResultsPromise,
          resultIndex,
          plan.source.ratio,
          loadImageSize,
        )
      },
      fallbackErrorMessage: '生成失败',
    })
  }

  async function reattachPendingImageResults(): Promise<void> {
    const attachment = requireCurrentAttachment()
    const pendingTasks = collectPendingImageTasks(options.nodes.value)
    if (pendingTasks.length === 0) return
    let failureCount = 0
    const resolvedAttempts: ReattachResolvedAttempt[] = []
    await Promise.all(
      pendingTasks.map(async ({ nodeId, taskId, resultIndex, fallbackRatio }) => {
        const reattachKey = `${attachment.epoch}:${nodeId}:${taskId}`
        if (activeReattachKeys.has(reattachKey)) return
        if (!isCanvasGraphNodeId(nodeId)) return
        activeReattachKeys.add(reattachKey)
        const hydratedState = executionRuntime.getNodeState(nodeId)
        const hydratedLease = executionRuntime.getNodeLease(nodeId)
        if (
          hydratedState?.status !== 'running' ||
          hydratedState.taskId !== taskId ||
          hydratedLease?.attachment !== attachment
        ) {
          activeReattachKeys.delete(reattachKey)
          return
        }
        const lease = hydratedLease
        try {
          const results = await waitTaskImageResults(taskId)
          const resolved =
            resultIndex === undefined
              ? await resolveFirstTaskImageResultDisplay(results, fallbackRatio, loadImageSize)
              : await resolveTaskImageResultAtIndexDisplay(
                  results,
                  resultIndex,
                  fallbackRatio,
                  loadImageSize,
                )
          const isCurrentBoard = executionRuntime.isAttachmentCurrent(attachment)
          const isAttached =
            isReattachExecutionCurrent(lease, taskId) &&
            isPendingImageTaskStillCurrent(nodeId, taskId)
          if (!isCurrentBoard || !isAttached) {
            transitionOwnedReattachExecution(lease, taskId, 'cancelled')
            activeReattachKeys.delete(reattachKey)
            return
          }
          resolvedAttempts.push({
            reattachKey,
            nodeId,
            taskId,
            lease,
            resolved: { ...resolved, resultNodeId: nodeId, taskId },
          })
        } catch (error) {
          const isCurrentBoard = executionRuntime.isAttachmentCurrent(attachment)
          const isAttached =
            isReattachExecutionCurrent(lease, taskId) &&
            isPendingImageTaskStillCurrent(nodeId, taskId)
          const status =
            !isCurrentBoard || !isAttached || error instanceof TaskCancelledError
              ? 'cancelled'
              : 'failed'
          transitionOwnedReattachExecution(lease, taskId, status)
          if (status === 'failed') {
            console.warn('[canvas] reattach pending image task failed', error)
            failureCount += 1
          }
          activeReattachKeys.delete(reattachKey)
        }
      }),
    )
    if (!executionRuntime.isAttachmentCurrent(attachment)) {
      resolvedAttempts.forEach((attempt) => {
        transitionOwnedReattachExecution(attempt.lease, attempt.taskId, 'cancelled')
        activeReattachKeys.delete(attempt.reattachKey)
      })
      return
    }

    let materialized: ReturnType<
      PendingImageResultExecution['materializeResolvedAttachedPendingResults']
    > | null = null
    try {
      materialized =
        resolvedAttempts.length > 0
          ? pendingResults.materializeResolvedAttachedPendingResults({
              results: resolvedAttempts.map((attempt) => attempt.resolved),
              isAttached: (result) =>
                !!result.taskId &&
                isPendingImageTaskStillCurrent(result.resultNodeId, result.taskId) &&
                resolvedAttempts.some(
                  (attempt) =>
                    attempt.nodeId === result.resultNodeId &&
                    attempt.taskId === result.taskId &&
                    isReattachExecutionCurrent(attempt.lease, attempt.taskId),
                ),
            })
          : null
    } catch (error) {
      console.warn('[canvas] materialize reattached image task failed', error)
      failureCount += resolvedAttempts.length
      resolvedAttempts.forEach((attempt) => {
        transitionOwnedReattachExecution(attempt.lease, attempt.taskId, 'failed')
        activeReattachKeys.delete(attempt.reattachKey)
      })
    }

    const materializedNodeIds = new Set(materialized?.materializedNodeIds ?? [])
    resolvedAttempts.forEach((attempt) => {
      if (materializedNodeIds.has(attempt.nodeId)) {
        transitionOwnedReattachExecution(attempt.lease, attempt.taskId, 'succeeded')
      } else if (!materialized) {
        // The materializer failure path already assigned failed above.
        activeReattachKeys.delete(attempt.reattachKey)
        return
      } else {
        transitionOwnedReattachExecution(attempt.lease, attempt.taskId, 'cancelled')
      }
      activeReattachKeys.delete(attempt.reattachKey)
    })

    const successCount = materialized?.materializedNodeIds.length ?? 0
    if (successCount > 0) {
      options.onSuccess(successCount > 1 ? `已恢复 ${successCount} 个生成结果` : '已恢复生成结果')
      await flushPendingTaskSnapshot()
    }
    if (failureCount > 0) {
      options.onWarning(
        failureCount > 1 ? `${failureCount} 个生成任务恢复失败` : '生成任务恢复失败',
      )
    }
  }

  return {
    state: {
      executionRuntime,
      isNodeRunning,
    },
    commands: {
      runPromptGeneration,
      reattachPendingImageResults,
    },
  }
}
