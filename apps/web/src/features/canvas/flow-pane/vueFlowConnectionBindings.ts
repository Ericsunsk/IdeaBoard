import type { Connection, EdgeUpdateEvent } from '@vue-flow/core'
import type { CanvasConnectionPort } from '../../canvas-board/domain'
import type { CanvasGraphNodeId } from '../graph-runtime'

export type CanvasFlowPaneConnectIntent = {
  sourceNodeId: CanvasGraphNodeId
  targetNodeId: CanvasGraphNodeId
  sourcePort: CanvasConnectionPort
  targetPort: CanvasConnectionPort
}

type CanvasFlowPaneConnectionHandlers = {
  canConnect: (connection: Connection) => boolean
  connect: (connection: Connection) => boolean
  updateEdge: (event: EdgeUpdateEvent) => void
}

export type CanvasFlowPaneConnectionBindings = {
  isValidConnection: (connection: Connection) => boolean
  onConnect: (connection: Connection) => void
  onEdgeUpdate: (event: EdgeUpdateEvent) => void
}

export type CanvasFlowPaneConnectionAdapter = {
  vueFlowBindings: CanvasFlowPaneConnectionBindings
  connectIntent: (intent: CanvasFlowPaneConnectIntent) => boolean
}

export function createCanvasFlowPaneConnectionAdapter(
  handlers: CanvasFlowPaneConnectionHandlers,
): CanvasFlowPaneConnectionAdapter {
  return {
    vueFlowBindings: {
      isValidConnection: handlers.canConnect,
      onConnect: (connection) => {
        void handlers.connect(connection)
      },
      onEdgeUpdate: handlers.updateEdge,
    },
    connectIntent: (intent) =>
      handlers.connect({
        source: intent.sourceNodeId,
        target: intent.targetNodeId,
        sourceHandle: `out:${intent.sourcePort}`,
        targetHandle: `in:${intent.targetPort}`,
      }),
  }
}
