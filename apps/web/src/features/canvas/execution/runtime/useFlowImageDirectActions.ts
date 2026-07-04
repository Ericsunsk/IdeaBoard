import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { CanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import { createImageExecutionPlanner } from '../planning/ImageExecutionPlan'
import { useFlowImageWorkflowExecution } from './flowImageWorkflowExecution'
import {
  createImageWorkflowCommands,
  type ImageWorkflowCommands,
} from './imageWorkflowCommandRuntime'

type UseFlowImageDirectActionsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  refreshNodeInternals?: (nodeIds: string[]) => void
  flushAutosave?: (options?: { silent?: boolean }) => Promise<void>
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
  getCurrentBoardId?: () => string
}

export type FlowImageDirectActions = ImageWorkflowCommands

export function useFlowImageDirectActions(
  options: UseFlowImageDirectActionsOptions,
): FlowImageDirectActions {
  const planner = createImageExecutionPlanner(() => ({
    nodes: options.nodes.value,
    edges: options.edges.value,
  }))
  const { executeImageWorkflow, isNodeDirectActionRunning, reattachPendingImageResults } =
    useFlowImageWorkflowExecution({
      nodes: options.nodes,
      edges: options.edges,
      graphAdapter: options.graphAdapter,
      refreshNodeInternals: options.refreshNodeInternals,
      flushAutosave: options.flushAutosave,
      getCurrentBoardId: options.getCurrentBoardId,
      onSuccess: options.onSuccess,
      onWarning: options.onWarning,
    })

  return createImageWorkflowCommands({
    planner,
    workflow: {
      executeImageWorkflow,
      isNodeDirectActionRunning,
      reattachPendingImageResults,
    },
    onWarning: options.onWarning,
  })
}
