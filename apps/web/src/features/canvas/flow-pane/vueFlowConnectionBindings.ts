import type { Connection, EdgeUpdateEvent } from '@vue-flow/core'

export type CanvasFlowPaneConnectIntent = {
  sourceNodeId: string
  targetNodeId: string
  sourceHandle: string | null
  targetHandle: string | null
}

type CanvasFlowPaneConnectionAdapter = {
  canConnect: (connection: Connection) => boolean
  connect: (connection: Connection) => void
  updateEdge: (event: EdgeUpdateEvent) => void
}

export type CanvasFlowPaneConnectionBindings = {
  isValidConnection: (connection: Connection) => boolean
  onConnect: (connection: Connection) => void
  onEdgeUpdate: (event: EdgeUpdateEvent) => void
}

export function createVueFlowConnectionBindings(
  adapter: CanvasFlowPaneConnectionAdapter,
): CanvasFlowPaneConnectionBindings {
  return {
    isValidConnection: adapter.canConnect,
    onConnect: adapter.connect,
    onEdgeUpdate: adapter.updateEdge,
  }
}

export function createVueFlowConnectionFromIntent(
  intent: CanvasFlowPaneConnectIntent,
): Connection {
  return {
    source: intent.sourceNodeId,
    target: intent.targetNodeId,
    sourceHandle: intent.sourceHandle,
    targetHandle: intent.targetHandle,
  }
}
