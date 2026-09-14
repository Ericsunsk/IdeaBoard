import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { i18n } from '../../../../../shared/i18n'
import type { TaskImageResult } from '../../../../../shared/query/taskResult'
import type { CanvasGraphStore } from '../../../domain/graph'
import type { CanvasRendererMetricsPort } from '../../../infrastructure/vue-flow-adapter'
import { loadImageSizeFromUrl } from '../media/imageSize'
import type { PendingImageExecutionCore } from '../pending-execution'
import {
  resolveTaskImageResultAtIndexDisplay,
  usePendingImageResultExecution,
  type ImageSizeLoader,
  type PendingImageResultExecution,
} from '../result-nodes'
import { waitExistingOrLiveTaskImageResults } from '../task-runner/imageDirectActionExecutor'
import {
  createCanvasTaskClientActionId,
  type ClientActionIdFactory,
  withTaskClientActionId,
} from '../task-runner/taskClientActionIds'
import {
  defaultCanvasTaskSubmissionAdapter,
  type CanvasTaskSubmissionAdapter,
} from '../workspace/canvasTaskSubmissionAdapter'
import {
  createPendingImageWorkflowRunner,
  type PendingImageWorkflowRunner,
  type PendingImageWorkflowTaskTransport,
} from './pendingImageWorkflowRunner'

export type PendingWorkflowAssemblyOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  readCanvasGraphStore: () => CanvasGraphStore
  rendererMetrics: CanvasRendererMetricsPort
  refreshNodeInternals?: ((nodeIds: string[]) => void) | undefined
  flushAutosave?: ((options?: { silent?: boolean }) => Promise<void>) | undefined
  pendingResults?: PendingImageResultExecution | undefined
  executionCore: PendingImageExecutionCore
  isNodeExecutionActive: (nodeId: string) => boolean
  taskAdapter?: CanvasTaskSubmissionAdapter | undefined
  /** Test seam: replaces the default submission transport. */
  taskTransport?: PendingImageWorkflowTaskTransport | undefined
  waitTaskImageResults?: ((taskId: string) => Promise<TaskImageResult[]>) | undefined
  loadImageSize?: ImageSizeLoader | undefined
  createClientActionId?: ClientActionIdFactory | undefined
  cancelTasks: (taskIds: readonly string[]) => void
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: ((error: unknown) => boolean) | undefined
}

export type PendingWorkflowAssembly = {
  pendingResults: PendingImageResultExecution
  workflowRunner: PendingImageWorkflowRunner
  flushPendingTaskSnapshot: () => Promise<void>
}

/**
 * Shared assembly for the pending-result execution path: result reservation,
 * task transport, and the workflow runner. Both the workspace runtime and the
 * legacy workflow entrypoint build the same seam; diverging copies caused
 * completion reconciliation to fragment (issue #676).
 */
export function assemblePendingImageWorkflow(
  options: PendingWorkflowAssemblyOptions,
): PendingWorkflowAssembly {
  const waitTaskImageResults =
    options.waitTaskImageResults ?? waitExistingOrLiveTaskImageResults
  const loadImageSize = options.loadImageSize ?? loadImageSizeFromUrl
  const taskAdapter = options.taskAdapter ?? defaultCanvasTaskSubmissionAdapter
  const pendingResults =
    options.pendingResults ??
    usePendingImageResultExecution({
      nodes: options.nodes,
      edges: options.edges,
      readCanvasGraphStore: options.readCanvasGraphStore,
      writeCanvasGraphStore: options.readCanvasGraphStore,
      rendererMetrics: options.rendererMetrics,
      readCurrentAttachment: options.executionCore.currentAttachment,
      isAttachmentCurrent: options.executionCore.isAttachmentCurrent,
      refreshNodeInternals: options.refreshNodeInternals,
      isNodeExecutionActive: options.isNodeExecutionActive,
    })

  async function flushPendingTaskSnapshot() {
    try {
      await options.flushAutosave?.({ silent: true })
    } catch (error) {
      console.warn('[canvas] flush pending task snapshot failed', error)
      options.onWarning(i18n.global.t('canvas.execution.canvasSaveFailedProgressLost'))
    }
  }

  const workflowRunner = createPendingImageWorkflowRunner({
    pendingResults,
    executionCore: options.executionCore,
    taskTransport:
      options.taskTransport ??
      ({
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
      } satisfies PendingImageWorkflowTaskTransport),
    cancelTasks: options.cancelTasks,
    afterTaskIdAttached: flushPendingTaskSnapshot,
    createClientActionId: options.createClientActionId,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
    onError: options.onError,
  })

  return { pendingResults, workflowRunner, flushPendingTaskSnapshot }
}
