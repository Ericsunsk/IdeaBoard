import type { Edge, Node } from '@vue-flow/core'
import { getCurrentScope, onScopeDispose, type Ref } from 'vue'
import { i18n } from '../../../../../core/i18n'
import type { CanvasTaskRequestData } from '../../../core/execution/canvasTaskRequest'
import type { TaskImageOutcomeWaiter } from '../task-runner/taskImageOutcome'
import type { CanvasGraphStore } from '../../../core/graph'
import type { CanvasRendererMetricsPort } from '../../../session/flow-adapter'
import { readImageNaturalSize } from '../../../../../core/media/imageDimensions'
import type { PendingImageExecutionCore } from '../pending-execution/core'
import { type ImageSizeLoader } from '../result-nodes/resultDisplay'
import {
  usePendingImageResultExecution,
  type PendingImageResultExecution,
} from '../result-nodes/usePendingImageResultExecution.ts'
import { waitCanvasWorkspaceTaskImageOutcome } from '../workspace/canvasTaskWaiting'
import { createTaskImageObserver } from '../task-runner/taskImageOutcome'
import {
  type ClientActionIdFactory,
  withTaskClientActionId,
} from '../task-runner/taskClientActionIds'
import { submitTaskRequest } from '../workspace/canvasTaskSubmissionAdapter'
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
  flushAutosave?: ((options?: { isSilent?: boolean }) => Promise<void>) | undefined
  pendingResults?: PendingImageResultExecution | undefined
  executionCore: PendingImageExecutionCore
  isNodeExecutionActive: (nodeId: string) => boolean
  /** Test seam: replaces the default submission transport. */
  taskTransport?: PendingImageWorkflowTaskTransport | undefined
  waitTaskImageOutcome?: TaskImageOutcomeWaiter | undefined
  loadImageSize?: ImageSizeLoader | undefined
  createClientActionId?: ClientActionIdFactory | undefined
  cancelTasks: (taskIds: readonly string[]) => void
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  onError?: ((error: unknown) => boolean) | undefined
  /** Test seam: background continuation scheduler (defaults to setTimeout). */
  scheduleContinuation?: ((callback: () => void, delayMs: number) => () => void) | undefined
}

export type PendingWorkflowAssembly = {
  pendingResults: PendingImageResultExecution
  workflowRunner: PendingImageWorkflowRunner
  flushPendingTaskSnapshot: () => Promise<void>
}

/**
 * Board attribution (#937): canvas submits ride the Board under
 * execution. The Board attachment is ground truth and wins over any
 * request-carried value; absent attachments stay absent (the server
 * governs unbound tasks as submitter-only, never workspace-visible).
 * Malformed attachment ids fail closed downstream in the workspace
 * adapter mapping, never silently.
 */
function withExecutionBoardId(
  executionCore: PendingImageExecutionCore,
  request: CanvasTaskRequestData,
): CanvasTaskRequestData {
  const boardId = executionCore.currentAttachment()?.boardId?.trim()
  if (!boardId) return request
  return { ...request, board_id: boardId }
}

/**
 * Shared assembly for the pending-result execution path: result reservation,
 * task transport, and the workflow runner. Both the workspace runtime and the
 * image workflow entrypoint build the same seam; diverging copies caused
 * completion reconciliation to fragment (issue #676).
 */
export function assemblePendingImageWorkflow(
  options: PendingWorkflowAssemblyOptions,
): PendingWorkflowAssembly {
  const scope = getCurrentScope()
  const waitTaskImageOutcome = options.waitTaskImageOutcome ?? waitCanvasWorkspaceTaskImageOutcome
  const loadImageSize = options.loadImageSize ?? readImageNaturalSize
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
      await options.flushAutosave?.({ isSilent: true })
    } catch (error) {
      console.warn('[canvas] flush pending task snapshot failed', error)
      if (!scope || scope.active)
        options.onWarning(i18n.global.t('canvas.execution.canvasSaveFailedProgressLost'))
    }
  }

  const workflowRunner = createPendingImageWorkflowRunner({
    pendingResults,
    executionCore: options.executionCore,
    taskTransport:
      options.taskTransport ??
      ({
        submit: (request, clientActionId) =>
          submitTaskRequest(
            withTaskClientActionId(
              withExecutionBoardId(options.executionCore, request),
              clientActionId,
            ),
          ),
        observe: createTaskImageObserver(waitTaskImageOutcome, loadImageSize),
      } satisfies PendingImageWorkflowTaskTransport),
    cancelTasks: options.cancelTasks,
    afterTaskIdAttached: flushPendingTaskSnapshot,
    createClientActionId: options.createClientActionId,
    scheduleContinuation: options.scheduleContinuation,
    onSuccess: options.onSuccess,
    onWarning: options.onWarning,
    onError: options.onError,
  })

  if (scope) onScopeDispose(workflowRunner.dispose)
  return { pendingResults, workflowRunner, flushPendingTaskSnapshot }
}
