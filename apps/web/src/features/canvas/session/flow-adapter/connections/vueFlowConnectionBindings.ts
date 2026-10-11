import type { Connection } from '@vue-flow/core'
import type { CanvasConnectionPort } from '../../../../board/ports/canvas'
import type { CanvasGraphNodeId } from '../../../core/graph'

type CanvasVueFlowConnectIntent = {
  sourceNodeId: CanvasGraphNodeId
  targetNodeId: CanvasGraphNodeId
  sourcePort: CanvasConnectionPort
  targetPort: CanvasConnectionPort
}

type CanvasVueFlowConnectionHandlers = {
  canConnect: (connection: Connection) => boolean
  connect: (connection: Connection) => boolean
}

type CanvasVueFlowConnectionBindings = {
  isValidConnection: (connection: Connection) => boolean
  onConnect: (connection: Connection) => void
}

export type CanvasVueFlowConnectionAdapter = {
  vueFlowBindings: CanvasVueFlowConnectionBindings
  connectIntent: (intent: CanvasVueFlowConnectIntent) => boolean
}

export function createCanvasVueFlowConnectionAdapter(
  handlers: CanvasVueFlowConnectionHandlers,
): CanvasVueFlowConnectionAdapter {
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
