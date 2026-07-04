import type { Ref } from 'vue'
import type { Connection, Edge, EdgeUpdateEvent } from '@vue-flow/core'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from './flowGraphAdapterCommandSideEffects'

type UseFlowConnectionMutationsOptions = {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- vue-flow edge data is an open Record; narrowing breaks vue-flow's internal type expectations
  edges: Ref<Array<Record<string, any>>>
  isValidConnection: (connection: Connection) => boolean
  normalizeConnection?: (
    connection: Connection,
    existingEdges: readonly Edge[],
  ) => Connection | null
  allowsMultipleTargetConnections?: (connection: Connection) => boolean
  validateConnectionMutation?: (
    connection: Connection,
    existingEdges: readonly Edge[],
  ) => string | null
  graphAdapter: CanvasVueFlowGraphAdapter
  onWarning: (message: string) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
}

export function useFlowConnectionMutations(options: UseFlowConnectionMutationsOptions) {
  function resolveValidatedConnection(
    connection: Connection,
    existingEdges: readonly Edge[],
  ): Connection | null {
    const resolvedConnection = options.normalizeConnection
      ? options.normalizeConnection(connection, existingEdges)
      : connection
    if (!resolvedConnection || !options.isValidConnection(resolvedConnection)) {
      options.onWarning('该连线不符合当前连接语义')
      return null
    }
    if (!resolvedConnection.source || !resolvedConnection.target) return null
    const validationMessage =
      options.validateConnectionMutation?.(resolvedConnection, existingEdges) ?? null
    if (validationMessage) {
      options.onWarning(validationMessage)
      return null
    }
    return resolvedConnection
  }

  function onConnect(connection: Connection) {
    const existingEdges = options.edges.value as unknown as Edge[]
    const resolvedConnection = resolveValidatedConnection(connection, existingEdges)
    if (!resolvedConnection) return
    const nextEdge = {
      ...resolvedConnection,
      id: `edge-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
      source: resolvedConnection.source,
      target: resolvedConnection.target,
      animated: false,
    } as Edge
    const allowsMultipleTargetConnections =
      options.allowsMultipleTargetConnections?.(resolvedConnection) ?? false
    const filteredEdges = existingEdges.filter((edge) => {
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
    const commandOptions = createFlowGraphCommandOptions()
    if (filteredEdges.length === existingEdges.length) {
      options.graphAdapter.addEdges(nextEdge, commandOptions)
    } else {
      options.graphAdapter.replaceEdges([...filteredEdges, nextEdge], commandOptions)
    }
    options.markFlowExecutionNodeDirty(resolvedConnection.target, { includeSelf: true })
  }

  function onEdgeUpdate(event: EdgeUpdateEvent) {
    const { edge, connection } = event
    const existingEdges = (options.edges.value as unknown as Edge[]).filter(
      (item) => item.id !== edge.id,
    )
    const resolvedConnection = resolveValidatedConnection(connection, existingEdges)
    if (!resolvedConnection) return
    const allowsMultipleTargetConnections =
      options.allowsMultipleTargetConnections?.(resolvedConnection) ?? false
    const retainedEdges = (options.edges.value as unknown as Edge[]).filter((item) => {
      if (item.id === edge.id) return true
      const isSameTargetHandle =
        item.target === resolvedConnection.target &&
        (item.targetHandle ?? null) === (resolvedConnection.targetHandle ?? null)
      if (isSameTargetHandle && !allowsMultipleTargetConnections) {
        return false
      }
      const isSameConnection =
        item.source === resolvedConnection.source &&
        item.target === resolvedConnection.target &&
        (item.sourceHandle ?? null) === (resolvedConnection.sourceHandle ?? null) &&
        (item.targetHandle ?? null) === (resolvedConnection.targetHandle ?? null)
      return !isSameConnection
    })
    const nextPatch = {
      source: resolvedConnection.source!,
      target: resolvedConnection.target!,
      sourceHandle: resolvedConnection.sourceHandle ?? null,
      targetHandle: resolvedConnection.targetHandle ?? null,
    }
    const commandOptions = createFlowGraphCommandOptions()
    if (retainedEdges.length === options.edges.value.length) {
      options.graphAdapter.updateEdge(edge.id, nextPatch, commandOptions)
    } else {
      options.graphAdapter.replaceEdges(
        retainedEdges.map((item) =>
          item.id === edge.id ? ({ ...item, ...nextPatch } as Edge) : item,
        ),
        commandOptions,
      )
    }
    options.markFlowExecutionNodeDirty(resolvedConnection.target, { includeSelf: true })
  }

  return {
    onConnect,
    onEdgeUpdate,
  }
}
