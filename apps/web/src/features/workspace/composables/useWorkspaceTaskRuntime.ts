import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { useFlowImageDirectActions } from '../execution'
import type { FlowTransactions } from './useFlowTransactions'

type ValueRef<T> = {
  readonly value: T
}

type WorkspaceTaskRuntimeMessage = {
  success: (message: string) => void
  warning: (message: string) => void
}

type UseWorkspaceTaskRuntimeOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  flowTransactions: Pick<
    FlowTransactions,
    'commitGraphMutation' | 'commitNodeMutation' | 'commitEdgeMutation' | 'commitDirty'
  >
  flushAutosave: (options?: { silent?: boolean }) => Promise<void>
  currentBoardId: ValueRef<string | null | undefined>
  refreshNodeInternals: (nodeIds: string[]) => void
  message: WorkspaceTaskRuntimeMessage
}

export function useWorkspaceTaskRuntime(options: UseWorkspaceTaskRuntimeOptions) {
  return useFlowImageDirectActions({
    nodes: options.nodes,
    edges: options.edges,
    commitGraphMutation: options.flowTransactions.commitGraphMutation,
    commitNodeMutation: options.flowTransactions.commitNodeMutation,
    commitEdgeMutation: options.flowTransactions.commitEdgeMutation,
    commitDirty: options.flowTransactions.commitDirty,
    flushAutosave: options.flushAutosave,
    refreshNodeInternals: options.refreshNodeInternals,
    getCurrentBoardId: () => options.currentBoardId.value ?? '',
    onSuccess: options.message.success,
    onWarning: options.message.warning,
  })
}
