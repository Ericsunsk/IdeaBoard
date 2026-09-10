import type { Edge, Node } from '@vue-flow/core'
import { onScopeDispose, ref, type Ref } from 'vue'
import { i18n } from '../../../shared/i18n'
import {
  createImageExecutionPlanner,
  createImageWorkflowCommands,
  isCanvasBillingPreflightError,
  type ImageWorkflowCommands,
  useCanvasExecutionWorkspace,
  useImageWorkflowExecution as useRuntimeImageWorkflowExecution,
  type PendingImageExecutionCore,
} from '../execution'
import type { BoardSnapshot } from '../../board/domain'
import type { CanvasGraphStore } from '../graph-runtime'
import type { CanvasRendererMetricsPort } from '../vue-flow-adapter'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import type { TaskImageResult } from '../../../shared/query/taskResult'
import { waitExistingOrLiveTaskImageResults } from '../execution/task-runner/imageDirectActionExecutor'

type CanvasTaskExecutionFeedback = CanvasFeedback['taskExecution']

export type UseImageWorkflowExecutionOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  readCanvasGraphStore: () => CanvasGraphStore
  rendererMetrics: CanvasRendererMetricsPort
  boardBaseSnapshot: Ref<BoardSnapshot>
  flushAutosave: (options?: { silent?: boolean }) => Promise<void>
  refreshNodeInternals: (nodeIds: string[]) => void
  feedback: CanvasTaskExecutionFeedback
  executionCore: PendingImageExecutionCore
  cancelTasks: (taskIds: readonly string[]) => void
  waitTaskImageResults?: (taskId: string) => Promise<TaskImageResult[]>
}

export type ImageWorkflowExecution = ImageWorkflowCommands & {
  hydratePendingImageLeases: () => number
}

function defaultGenerationUnavailableMessage(): string {
  return i18n.global.t('canvas.execution.generationUnavailable')
}
const GENERATION_UNAVAILABLE_RETRY_MS = 15_000

function isProviderUnavailableMessage(message: string): boolean {
  // Degradation warnings originate only from our own single-source copy; no prose matching.
  return message.trim() === i18n.global.t('canvas.execution.generationUnavailable')
}

export function useImageWorkflowExecution(
  options: UseImageWorkflowExecutionOptions,
): ImageWorkflowExecution {
  const generationUnavailableUntilMs = ref(0)
  const generationUnavailableMessage = ref(defaultGenerationUnavailableMessage())
  let generationAvailabilityTimer: ReturnType<typeof setTimeout> | null = null
  // Single task-result waiter shared by the execution workspace and the
  // workflow entrypoint, so completion reconciliation cannot fragment (issue
  // #676). Test doubles inject through the same option on both seams.
  const waitTaskImageResults =
    options.waitTaskImageResults ?? waitExistingOrLiveTaskImageResults

  function isGenerationAvailable(): boolean {
    return Date.now() >= generationUnavailableUntilMs.value
  }

  function clearGenerationAvailabilityTimer() {
    if (!generationAvailabilityTimer) return
    clearTimeout(generationAvailabilityTimer)
    generationAvailabilityTimer = null
  }

  function refreshGenerationAvailabilityAfter(delayMs: number) {
    clearGenerationAvailabilityTimer()
    generationAvailabilityTimer = setTimeout(
      () => {
        generationAvailabilityTimer = null
        if (!isGenerationAvailable()) {
          refreshGenerationAvailabilityAfter(generationUnavailableUntilMs.value - Date.now())
          return
        }
        generationUnavailableUntilMs.value = 0
        generationUnavailableMessage.value = defaultGenerationUnavailableMessage()
      },
      Math.max(0, delayMs),
    )
  }

  function handleWorkflowWarning(message: string) {
    if (isProviderUnavailableMessage(message) && isGenerationAvailable()) {
      generationUnavailableUntilMs.value = Date.now() + GENERATION_UNAVAILABLE_RETRY_MS
      generationUnavailableMessage.value = message.trim() || defaultGenerationUnavailableMessage()
      refreshGenerationAvailabilityAfter(GENERATION_UNAVAILABLE_RETRY_MS)
    }
    options.feedback.warning(message)
  }

  function handleWorkflowError(error: unknown) {
    if (isCanvasBillingPreflightError(error, 'insufficient-credits')) {
      options.feedback.insufficientCredits()
      return true
    }
    return false
  }

  onScopeDispose(clearGenerationAvailabilityTimer)

  const planner = createImageExecutionPlanner(() => ({
    nodes: options.nodes.value,
    edges: options.edges.value,
  }))
  const executionWorkspace = useCanvasExecutionWorkspace({
    nodes: options.nodes,
    edges: options.edges,
    readCanvasGraphStore: options.readCanvasGraphStore,
    rendererMetrics: options.rendererMetrics,
    boardBaseSnapshot: options.boardBaseSnapshot,
    flushAutosave: options.flushAutosave,
    refreshNodeInternals: options.refreshNodeInternals,
    executionCore: options.executionCore,
    cancelTasks: options.cancelTasks,
    waitTaskImageResults,
    onSuccess: options.feedback.succeeded,
    onWarning: handleWorkflowWarning,
    onError: handleWorkflowError,
  })
  const workflow = useRuntimeImageWorkflowExecution({
    nodes: options.nodes,
    edges: options.edges,
    readCanvasGraphStore: options.readCanvasGraphStore,
    rendererMetrics: options.rendererMetrics,
    boardBaseSnapshot: options.boardBaseSnapshot,
    flushAutosave: options.flushAutosave,
    refreshNodeInternals: options.refreshNodeInternals,
    onSuccess: options.feedback.succeeded,
    onWarning: handleWorkflowWarning,
    onError: handleWorkflowError,
    executionWorkspace,
    cancelTasks: options.cancelTasks,
    waitTaskImageResults,
  })

  const commands = createImageWorkflowCommands({
    planner,
    workflow,
    isGenerationAvailable,
    generationUnavailableMessage: () => generationUnavailableMessage.value,
    onWarning: handleWorkflowWarning,
  })
  return {
    ...commands,
    hydratePendingImageLeases: executionWorkspace.commands.hydratePendingImageLeases,
  }
}
