import type { Connection } from '@vue-flow/core'
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
}

export type CanvasFlowPaneConnectionBindings = {
  isValidConnection: (connection: Connection) => boolean
  onConnect: (connection: Connection) => void
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
