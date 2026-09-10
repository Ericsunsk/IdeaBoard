import type { Connection } from '@vue-flow/core'
import type { CanvasConnectionPort } from '../../board/domain'
import type { CanvasGraphNodeId } from '../graph-runtime'

export type CanvasVueFlowConnectIntent = {
  sourceNodeId: CanvasGraphNodeId
  targetNodeId: CanvasGraphNodeId
  sourcePort: CanvasConnectionPort
  targetPort: CanvasConnectionPort
}

type CanvasVueFlowConnectionHandlers = {
  canConnect: (connection: Connection) => boolean
  connect: (connection: Connection) => boolean
}

export type CanvasVueFlowConnectionBindings = {
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
