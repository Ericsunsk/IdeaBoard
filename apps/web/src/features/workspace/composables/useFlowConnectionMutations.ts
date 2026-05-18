import type { Ref } from 'vue'
import type { Connection, Edge, EdgeUpdateEvent } from '@vue-flow/core'
import type { FlowTransactions } from './useFlowTransactions'

type UseFlowConnectionMutationsOptions = {
  edges: Ref<Array<Record<string, any>>>
  isValidConnection: (connection: Connection) => boolean
  normalizeConnection?: (connection: Connection, existingEdges: readonly Edge[]) => Connection | null
  allowsMultipleTargetConnections?: (connection: Connection) => boolean
  validateConnectionMutation?: (
    connection: Connection,
    existingEdges: readonly Edge[],
  ) => string | null
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  onWarning: (message: string) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
}

export function useFlowConnectionMutations(options: UseFlowConnectionMutationsOptions) {
  function onConnect(connection: Connection) {
    const existingEdges = options.edges.value as unknown as Edge[]
    const resolvedConnection = options.normalizeConnection
      ? options.normalizeConnection(connection, existingEdges)
      : connection
    if (!resolvedConnection || !options.isValidConnection(resolvedConnection)) {
      options.onWarning('该连线不符合当前连接语义')
      return
    }
    if (!resolvedConnection.source || !resolvedConnection.target) return
    const validationMessage =
      options.validateConnectionMutation?.(resolvedConnection, existingEdges) ?? null
    if (validationMessage) {
      options.onWarning(validationMessage)
      return
    }
    const nextEdge = {
      ...resolvedConnection,
      id: `edge-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
      source: resolvedConnection.source,
      target: resolvedConnection.target,
      animated: false,
    } as Edge
    options.commitEdgeMutation((currentEdges) => {
      const allowsMultipleTargetConnections =
        options.allowsMultipleTargetConnections?.(resolvedConnection) ?? false
      const filteredEdges = currentEdges.filter((edge) => {
        const isSameTargetHandle =
          edge.target === resolvedConnection.target &&
          edge.targetHandle === resolvedConnection.targetHandle
        if (isSameTargetHandle && !allowsMultipleTargetConnections) {
          return false
        }
        if (
          edge.source === resolvedConnection.source &&
          edge.target === resolvedConnection.target &&
          edge.sourceHandle === resolvedConnection.sourceHandle &&
          edge.targetHandle === resolvedConnection.targetHandle
        )
          return false
        return true
      })
      return [...filteredEdges, nextEdge]
    })
    options.markFlowExecutionNodeDirty(resolvedConnection.target, { includeSelf: true })
  }

  function onEdgeUpdate(event: EdgeUpdateEvent) {
    const { edge, connection } = event
    const existingEdges = (options.edges.value as unknown as Edge[]).filter(
      (item) => item.id !== edge.id,
    )
    const resolvedConnection = options.normalizeConnection
      ? options.normalizeConnection(connection, existingEdges)
      : connection
    if (!resolvedConnection || !options.isValidConnection(resolvedConnection)) {
      options.onWarning('该连线不符合当前连接语义')
      return
    }
    if (!resolvedConnection.source || !resolvedConnection.target) return
    const validationMessage =
      options.validateConnectionMutation?.(resolvedConnection, existingEdges) ?? null
    if (validationMessage) {
      options.onWarning(validationMessage)
      return
    }
    options.commitEdgeMutation((currentEdges) =>
      currentEdges
        .filter((item) => {
          if (item.id === edge.id) return true
          const isSameTargetHandle =
            item.target === resolvedConnection.target &&
            (item.targetHandle ?? null) === (resolvedConnection.targetHandle ?? null)
          if (isSameTargetHandle && !options.allowsMultipleTargetConnections?.(resolvedConnection)) {
            return false
          }
          const isSameConnection =
            item.source === resolvedConnection.source &&
            item.target === resolvedConnection.target &&
            (item.sourceHandle ?? null) === (resolvedConnection.sourceHandle ?? null) &&
            (item.targetHandle ?? null) === (resolvedConnection.targetHandle ?? null)
          return !isSameConnection
        })
        .map((item) => {
          if (item.id !== edge.id) return item
          return {
            ...item,
            source: resolvedConnection.source!,
            target: resolvedConnection.target!,
            sourceHandle: resolvedConnection.sourceHandle ?? null,
            targetHandle: resolvedConnection.targetHandle ?? null,
          } as Edge
        }),
    )
    options.markFlowExecutionNodeDirty(resolvedConnection.target, { includeSelf: true })
  }

  return {
    onConnect,
    onEdgeUpdate,
  }
}
