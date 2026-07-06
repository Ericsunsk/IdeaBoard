import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { createImageExecutionPlanner } from '../execution'
import { useFlowImageWorkflowExecution } from '../execution/runtime/flowImageWorkflowExecution'
import {
  createImageWorkflowCommands,
  type ImageWorkflowCommands,
} from '../execution/runtime/imageWorkflowCommandRuntime'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import type { CanvasGraphStore } from '../graph-runtime'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'

type ValueRef<T> = {
  readonly value: T
}

type CanvasTaskExecutionFeedback = CanvasFeedback['taskExecution']

type UseCanvasTaskRuntimeOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  createCurrentCanvasGraphStore?: () => CanvasGraphStore
  boardBaseSnapshot?: Ref<CanvasBoardSnapshot>
  flushAutosave: (options?: { silent?: boolean }) => Promise<void>
  currentBoardId: ValueRef<string | null | undefined>
  refreshNodeInternals: (nodeIds: string[]) => void
  feedback: CanvasTaskExecutionFeedback
}

export type CanvasTaskRuntime = ImageWorkflowCommands

export function useCanvasTaskRuntime(options: UseCanvasTaskRuntimeOptions): CanvasTaskRuntime {
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
    onWarning: options.feedback.warning,
  })

  return createImageWorkflowCommands({
    planner,
    workflow,
    onWarning: options.feedback.warning,
  })
}
