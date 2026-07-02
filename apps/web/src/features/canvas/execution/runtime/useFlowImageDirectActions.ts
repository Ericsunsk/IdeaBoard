import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { FlowTransactions } from '../../composables/useFlowTransactions'
import { createImageExecutionPlanner } from '../planning/ImageExecutionPlan'
import { useFlowImageWorkflowExecution } from './flowImageWorkflowExecution'
import {
  createImageWorkflowCommands,
  type ImageWorkflowCommands,
} from './imageWorkflowCommandRuntime'

type UseFlowImageDirectActionsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  commitGraphMutation: FlowTransactions['commitGraphMutation']
  commitNodeMutation: FlowTransactions['commitNodeMutation']
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  commitDirty: FlowTransactions['commitDirty']
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
      commitGraphMutation: options.commitGraphMutation,
      commitNodeMutation: options.commitNodeMutation,
      commitEdgeMutation: options.commitEdgeMutation,
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
