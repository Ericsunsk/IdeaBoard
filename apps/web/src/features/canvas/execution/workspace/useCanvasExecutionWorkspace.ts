import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { CanvasBoardSnapshot } from '../../../canvas-board/domain'
import type { TaskImageResult } from '../../../../shared/query/taskResult'
import { TaskCancelledError } from '../../../../shared/api/taskEvents'
import type { CanvasGraphStore } from '../../graph-runtime'
import { loadImageSizeFromUrl } from '../media/imageSize'
import type { ActivatedImageExecutionPlan } from '../planning/ImageExecutionPlan'
import {
  resolveFirstTaskImageResultDisplay,
  resolveTaskImageResultAtIndexDisplay,
  type ImageSizeLoader,
} from '../result-nodes/flowImageDirectActionTaskResult'
import {
  type PendingImageResultExecution,
  usePendingImageResultExecution,
} from '../result-nodes/usePendingImageResultNodes'
import { waitExistingOrLiveTaskImageResults } from '../task-runner/flowImageDirectActionExecutor'
import {
  createCanvasTaskClientActionId,
  type ClientActionIdFactory,
  withTaskClientActionId,
} from '../task-runner/taskClientActionIds'
import { buildImageExecutionTaskRequest } from '../commands/flowImageTaskRequests'
import { createPendingImageWorkflowRunner } from '../runtime/pendingImageWorkflowRunner'
import { type PendingImageExecutionCore, collectPendingImageTasks } from '../pending-execution'
import {
  defaultCanvasTaskSubmissionAdapter,
  type CanvasTaskSubmissionAdapter,
} from './canvasTaskSubmissionAdapter'

export type CanvasExecutionWorkspaceState = {
  readonly executionCore: PendingImageExecutionCore
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
  readCanvasGraphStore: () => CanvasGraphStore
  boardBaseSnapshot: Ref<CanvasBoardSnapshot>
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  pendingResults?: PendingImageResultExecution
  taskAdapter?: CanvasTaskSubmissionAdapter
  waitTaskImageResults?: (taskId: string) => Promise<TaskImageResult[]>
  loadImageSize?: ImageSizeLoader
  createClientActionId?: ClientActionIdFactory
  executionCore: PendingImageExecutionCore
  cancelTasks: (taskIds: readonly string[]) => void
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: (error: unknown) => boolean
}

export function useCanvasExecutionWorkspace(
  options: UseCanvasExecutionWorkspaceOptions,
): CanvasExecutionWorkspace {
  const waitTaskImageResults = options.waitTaskImageResults ?? waitExistingOrLiveTaskImageResults
  const loadImageSize = options.loadImageSize ?? loadImageSizeFromUrl
  const taskAdapter = options.taskAdapter ?? defaultCanvasTaskSubmissionAdapter
  const pendingResults =
    options.pendingResults ??
    usePendingImageResultExecution({
      nodes: options.nodes,
      edges: options.edges,
      readCanvasGraphStore: options.readCanvasGraphStore,
      readCurrentAttachment: options.executionCore.currentAttachment,
      isAttachmentCurrent: options.executionCore.isAttachmentCurrent,
      refreshNodeInternals: options.refreshNodeInternals,
      isNodeExecutionActive: (nodeId) =>
        options.executionCore.captureActiveLeases([nodeId as `image-${number}`]).length > 0,
    })

  function isNodeRunning(nodeId: string): boolean {
    return options.executionCore.captureActiveLeases([nodeId as `image-${number}`]).length > 0
  }

  function requireAttachment() {
    const attachment = options.executionCore.currentAttachment()
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

  const workflowRunner = createPendingImageWorkflowRunner({
    pendingResults,
    executionCore: options.executionCore,
    taskTransport: {
      assertReady: (request, fallbackMessage) =>
        taskAdapter.assertTaskRequestReady(
          withTaskClientActionId(request, createCanvasTaskClientActionId()),
          fallbackMessage,
        ),
      submit: (request, clientActionId, fallbackMessage) =>
        taskAdapter.submitTaskRequest(
          withTaskClientActionId(request, clientActionId),
          fallbackMessage,
        ),
      resolve: async (taskId, resultIndex, fallbackRatio) =>
        resolveTaskImageResultAtIndexDisplay(
          await waitTaskImageResults(taskId),
          resultIndex,
          fallbackRatio,
          loadImageSize,
        ),
    },
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
    const attachment = requireAttachment()
    const taskRequest = buildImageExecutionTaskRequest(plan)
    await workflowRunner.execute({
      source: plan.source,
      sourceNodeId: plan.sourceNodeId,
      attachment,
      outputCount: plan.outputCount,
      resultOptions: plan.resultOptions,
      taskRequest,
      readinessRequest: taskRequest,
      fallbackRatio: plan.source.ratio,
    })
  }

  async function reattachPendingImageResults(): Promise<void> {
    const attachment = requireAttachment()
    const pending = collectPendingImageTasks(options.boardBaseSnapshot.value)
    await Promise.all(
      pending.map(async ({ resultNodeId, taskId, resultIndex, fallbackRatio, clientActionId }) => {
        const lease = options.executionCore.claimHydratedLease(attachment, {
          resultNodeId,
          taskId,
          clientActionId,
        })
        if (!lease) return
        try {
          const results = await waitTaskImageResults(taskId)
          if (!options.executionCore.isCurrent(lease, taskId)) return
          const result =
            resultIndex === undefined
              ? await resolveFirstTaskImageResultDisplay(results, fallbackRatio, loadImageSize)
              : await resolveTaskImageResultAtIndexDisplay(
                  results,
                  resultIndex,
                  fallbackRatio,
                  loadImageSize,
                )
          options.executionCore.materializeBatch([{ lease, result }])
          await flushPendingTaskSnapshot()
        } catch (error) {
          if (error instanceof TaskCancelledError) options.executionCore.cancel([lease])
          else options.executionCore.fail(lease)
        }
      }),
    )
  }

  return {
    state: { executionCore: options.executionCore, isNodeRunning },
    commands: { runPromptGeneration, reattachPendingImageResults },
  }
}
