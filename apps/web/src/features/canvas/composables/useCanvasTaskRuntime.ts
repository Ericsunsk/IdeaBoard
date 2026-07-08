import type { Edge, Node } from '@vue-flow/core'
import { onScopeDispose, ref, type Ref } from 'vue'
import {
  createImageExecutionPlanner,
  createImageWorkflowCommands,
  type ImageWorkflowCommands,
  useFlowImageWorkflowExecution,
} from '../execution'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import type { CanvasGraphStore } from '../graph-runtime'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import type { FlowToCanvasBoardSnapshotOptions } from '../utils/flowSnapshotAdapter'

type ValueRef<T> = {
  readonly value: T
}

type CanvasTaskExecutionFeedback = CanvasFeedback['taskExecution']

type UseCanvasTaskRuntimeOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  createCurrentCanvasGraphStore: (
    snapshotOptions?: FlowToCanvasBoardSnapshotOptions,
  ) => CanvasGraphStore
  boardBaseSnapshot?: Ref<CanvasBoardSnapshot>
  flushAutosave: (options?: { silent?: boolean }) => Promise<void>
  currentBoardId: ValueRef<string | null | undefined>
  refreshNodeInternals: (nodeIds: string[]) => void
  feedback: CanvasTaskExecutionFeedback
}

export type CanvasTaskRuntime = ImageWorkflowCommands

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

export function useCanvasTaskRuntime(options: UseCanvasTaskRuntimeOptions): CanvasTaskRuntime {
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
    generationAvailabilityTimer = setTimeout(() => {
      generationAvailabilityTimer = null
      if (!isGenerationAvailable()) {
        refreshGenerationAvailabilityAfter(generationUnavailableUntilMs.value - Date.now())
        return
      }
      generationUnavailableUntilMs.value = 0
      generationUnavailableMessage.value = DEFAULT_GENERATION_UNAVAILABLE_MESSAGE
    }, Math.max(0, delayMs))
  }

  function handleWorkflowWarning(message: string) {
    if (isProviderUnavailableMessage(message) && isGenerationAvailable()) {
      generationUnavailableUntilMs.value = Date.now() + GENERATION_UNAVAILABLE_RETRY_MS
      generationUnavailableMessage.value = message.trim() || DEFAULT_GENERATION_UNAVAILABLE_MESSAGE
      refreshGenerationAvailabilityAfter(GENERATION_UNAVAILABLE_RETRY_MS)
    }
    options.feedback.warning(message)
  }

  onScopeDispose(clearGenerationAvailabilityTimer)

  const planner = createImageExecutionPlanner(() => ({
    nodes: options.nodes.value,
    edges: options.edges.value,
  }))
  const workflow = useFlowImageWorkflowExecution({
    nodes: options.nodes,
    edges: options.edges,
    graphAdapter: options.graphAdapter,
    createCurrentCanvasGraphStore: options.createCurrentCanvasGraphStore,
    boardBaseSnapshot: options.boardBaseSnapshot,
    flushAutosave: options.flushAutosave,
    refreshNodeInternals: options.refreshNodeInternals,
    getCurrentBoardId: () => options.currentBoardId.value ?? '',
    onSuccess: options.feedback.succeeded,
    onWarning: handleWorkflowWarning,
  })

  return createImageWorkflowCommands({
    planner,
    workflow,
    isGenerationAvailable,
    generationUnavailableMessage: () => generationUnavailableMessage.value,
    onWarning: handleWorkflowWarning,
  })
}
