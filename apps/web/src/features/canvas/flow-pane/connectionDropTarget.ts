import type { Connection, HandleType } from '@vue-flow/core'

export type ConnectionEndPoint = {
  clientX: number
  clientY: number
}

export type ConnectionStartHandle = {
  id?: string | null
  nodeId: string
  type: HandleType | null
}

type CanvasConnectionDropLifecycleOptions = {
  document: Document
  connect: (connection: Connection) => void
}

export function getConnectionEndPoint(
  event: MouseEvent | TouchEvent | undefined,
): ConnectionEndPoint | null {
  if (!event) return null

  if ('changedTouches' in event) {
    const touch = event.changedTouches[0]
    return touch ? { clientX: touch.clientX, clientY: touch.clientY } : null
  }

  return { clientX: event.clientX, clientY: event.clientY }
}

export function resolveCanvasNodeIdAtPoint(
  document: Document,
  point: ConnectionEndPoint,
): string | null {
  const element = document.elementFromPoint(point.clientX, point.clientY)
  const node = element?.closest<HTMLElement>('[data-flow-node-id]')
  if (!node) return null

  const bounds = node.getBoundingClientRect()
  const isInsideNode =
    point.clientX >= bounds.left &&
    point.clientX <= bounds.right &&
    point.clientY >= bounds.top &&
    point.clientY <= bounds.bottom

  return isInsideNode ? (node.dataset.flowNodeId ?? null) : null
}

export function createCanvasConnectionDropLifecycle(options: CanvasConnectionDropLifecycleOptions) {
  return {
    onConnectEnd(
      event: MouseEvent | TouchEvent | undefined,
      sourceHandle: ConnectionStartHandle | null,
    ) {
      if (!sourceHandle || sourceHandle.type !== 'source') return false

      const point = getConnectionEndPoint(event)
      if (!point) return false

      const targetNodeId = resolveCanvasNodeIdAtPoint(options.document, point)
      if (!targetNodeId) return false

      options.connect({
        source: sourceHandle.nodeId,
        sourceHandle: sourceHandle.id ?? null,
        target: targetNodeId,
        targetHandle: 'in:__auto',
      })
      return true
    },
  }
}
