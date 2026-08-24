import type { Edge, Node } from '@vue-flow/core'
import { onScopeDispose, ref, type Ref } from 'vue'
import {
  createImageExecutionPlanner,
  createImageWorkflowCommands,
  isCanvasBillingPreflightError,
  type ImageWorkflowCommands,
  useCanvasExecutionWorkspace,
  useFlowImageWorkflowExecution,
  type PendingImageExecutionCore,
} from '../execution'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import type { CanvasGraphStore } from '../graph-runtime'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'

type CanvasTaskExecutionFeedback = CanvasFeedback['taskExecution']

export type UseImageWorkflowExecutionOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  readCanvasGraphStore: () => CanvasGraphStore
  boardBaseSnapshot: Ref<CanvasBoardSnapshot>
  flushAutosave: (options?: { silent?: boolean }) => Promise<void>
  refreshNodeInternals: (nodeIds: string[]) => void
  feedback: CanvasTaskExecutionFeedback
  executionCore: PendingImageExecutionCore
  cancelTasks: (taskIds: readonly string[]) => void
}

export type ImageWorkflowExecution = ImageWorkflowCommands & {
  hydratePendingImageLeases: () => number
}

const DEFAULT_GENERATION_UNAVAILABLE_MESSAGE = '生成服务暂不可用，请稍后重试'
const GENERATION_UNAVAILABLE_RETRY_MS = 15_000

function isProviderUnavailableMessage(message: string): boolean {
  const normalized = message.trim().toLowerCase()
  if (!normalized) return false
  return (
    /provider.*unavailable/.test(normalized) ||
    /no available/.test(normalized) ||
    /暂无可用|没有可用|无可用|服务暂不可用|服务暂时不可用/.test(message)
  )
}

export function useImageWorkflowExecution(
  options: UseImageWorkflowExecutionOptions,
): ImageWorkflowExecution {
  const generationUnavailableUntilMs = ref(0)
  const generationUnavailableMessage = ref(DEFAULT_GENERATION_UNAVAILABLE_MESSAGE)
  let generationAvailabilityTimer: ReturnType<typeof setTimeout> | null = null

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
        generationUnavailableMessage.value = DEFAULT_GENERATION_UNAVAILABLE_MESSAGE
      },
      Math.max(0, delayMs),
    )
  }

  function handleWorkflowWarning(message: string) {
    if (isProviderUnavailableMessage(message) && isGenerationAvailable()) {
      generationUnavailableUntilMs.value = Date.now() + GENERATION_UNAVAILABLE_RETRY_MS
      generationUnavailableMessage.value = message.trim() || DEFAULT_GENERATION_UNAVAILABLE_MESSAGE
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
    boardBaseSnapshot: options.boardBaseSnapshot,
    flushAutosave: options.flushAutosave,
    refreshNodeInternals: options.refreshNodeInternals,
    executionCore: options.executionCore,
    cancelTasks: options.cancelTasks,
    onSuccess: options.feedback.succeeded,
    onWarning: handleWorkflowWarning,
    onError: handleWorkflowError,
  })
  const workflow = useFlowImageWorkflowExecution({
    nodes: options.nodes,
    edges: options.edges,
    readCanvasGraphStore: options.readCanvasGraphStore,
    boardBaseSnapshot: options.boardBaseSnapshot,
    flushAutosave: options.flushAutosave,
    refreshNodeInternals: options.refreshNodeInternals,
    onSuccess: options.feedback.succeeded,
    onWarning: handleWorkflowWarning,
    onError: handleWorkflowError,
    executionWorkspace,
    cancelTasks: options.cancelTasks,
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
