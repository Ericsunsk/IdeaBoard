import type { Edge, Node } from '@vue-flow/core'
import { ref, type Ref } from 'vue'
import { useTimeoutFn } from '@vueuse/core'
import { i18n } from '../../../../core/i18n'
import {
  createImageExecutionPlanner,
  createImageWorkflowCommands,
  isCanvasBillingPreflightError,
  type ImageWorkflowCommands,
  useCanvasExecutionWorkspace,
  useImageWorkflowExecution as useRuntimeImageWorkflowExecution,
  type PendingImageExecutionCore,
} from '../execution'
import type { BoardSnapshot } from '../../../board/ports/canvas'
import type { CanvasGraphStore } from '../../core/graph'
import type { CanvasRendererMetricsPort } from '../../session/flow-adapter'
import type { CanvasFeedback } from '../workspace/useCanvasFeedback.ts'
import type { TaskImageResult } from '../../../../core/query/taskResult'
import { waitExistingOrLiveTaskImageResults } from '../execution/task-runner/imageDirectActionExecutor'
import { CANVAS_WORKSPACE_REQUIRED_MESSAGE } from '../execution/workspace/canvasWorkspaceTaskAdapter'
import { getWorkspaceSelectionStore } from '../../../workspace'

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
  /** Workspace gate: canvas task actions stay disabled without a selection. */
  isWorkspaceAvailable?: (() => boolean) | undefined
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
  const cooldownDelayMs = ref(0)
  // Cooldown timer owned by `useTimeoutFn` (auto-cancelled on scope dispose).
  // `interval` stays a ref because each schedule carries its own delay while
  // `start` only restarts with the configured interval.
  const { start: startCooldownTimer, stop: stopCooldownTimer } = useTimeoutFn(
    () => {
      if (!isGenerationAvailable()) {
        refreshGenerationAvailabilityAfter(generationUnavailableUntilMs.value - Date.now())
        return
      }
      generationUnavailableUntilMs.value = 0
      generationUnavailableMessage.value = defaultGenerationUnavailableMessage()
    },
    cooldownDelayMs,
    { immediate: false },
  )
  // Single task-result waiter shared by the execution workspace and the
  // workflow entrypoint, so completion reconciliation cannot fragment (issue
  // #676). Test doubles inject through the same option on both seams.
  const waitTaskImageResults = options.waitTaskImageResults ?? waitExistingOrLiveTaskImageResults

  // Workspace gate (fail-closed): without a selected workspace every canvas
  // task action stays disabled with guidance; the adapter throws the same
  // guidance when execution is attempted directly. Defaults to the shared
  // selection store; tests inject a stub.
  const isWorkspaceAvailable =
    options.isWorkspaceAvailable ?? (() => getWorkspaceSelectionStore().getSelectedId() != null)

  function isGenerationAvailable(): boolean {
    if (!isWorkspaceAvailable()) return false
    return Date.now() >= generationUnavailableUntilMs.value
  }

  function resolveUnavailableMessage(): string {
    if (!isWorkspaceAvailable()) return CANVAS_WORKSPACE_REQUIRED_MESSAGE
    return generationUnavailableMessage.value
  }

  function refreshGenerationAvailabilityAfter(delayMs: number) {
    stopCooldownTimer()
    cooldownDelayMs.value = Math.max(0, delayMs)
    startCooldownTimer()
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
    generationUnavailableMessage: resolveUnavailableMessage,
    onWarning: handleWorkflowWarning,
  })
  return {
    ...commands,
    hydratePendingImageLeases: executionWorkspace.commands.hydratePendingImageLeases,
  }
}
