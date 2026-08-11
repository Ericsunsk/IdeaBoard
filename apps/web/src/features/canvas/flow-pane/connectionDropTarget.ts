import type { Connection, OnConnectStartParams } from '@vue-flow/core'

export type ConnectionEndPoint = {
  clientX: number
  clientY: number
}

export type CanvasConnectionStartEvent = OnConnectStartParams & {
  event?: MouseEvent | TouchEvent
}

type CanvasConnectionDropLifecycleOptions = {
  document: Document
  connect: (connection: Connection) => void
  onTargetChange: (nodeId: string | null) => void
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
  let sourceHandle: {
    nodeId: string
    handleId: string | null
  } | null = null
  let targetNodeId: string | null = null

  function setTargetNodeId(nodeId: string | null) {
    if (targetNodeId === nodeId) return
    targetNodeId = nodeId
    options.onTargetChange(nodeId)
  }

  function resolveTargetNodeId(point: ConnectionEndPoint) {
    const nodeId = resolveCanvasNodeIdAtPoint(options.document, point)
    return nodeId && nodeId !== sourceHandle?.nodeId ? nodeId : null
  }

  function updateTarget(event: MouseEvent | TouchEvent) {
    if (!sourceHandle) return
    const point = getConnectionEndPoint(event)
    setTargetNodeId(point ? resolveTargetNodeId(point) : null)
  }

  function onMouseMove(event: MouseEvent) {
    updateTarget(event)
  }

  function onTouchMove(event: TouchEvent) {
    updateTarget(event)
  }

  function stopTracking() {
    options.document.removeEventListener('mousemove', onMouseMove)
    options.document.removeEventListener('touchmove', onTouchMove)
    sourceHandle = null
    setTargetNodeId(null)
  }

  function onConnectStart(event: CanvasConnectionStartEvent) {
    stopTracking()
    if (!event.nodeId || event.handleType !== 'source') return false

    sourceHandle = {
      nodeId: event.nodeId,
      handleId: event.handleId,
    }
    options.document.addEventListener('mousemove', onMouseMove)
    options.document.addEventListener('touchmove', onTouchMove, { passive: true })
    return true
  }

  function onConnectEnd(event: MouseEvent | TouchEvent | undefined) {
    const activeSourceHandle = sourceHandle
    const point = getConnectionEndPoint(event)
    const releasedTargetNodeId = point ? resolveTargetNodeId(point) : null
    stopTracking()

    if (!activeSourceHandle || !releasedTargetNodeId) return false

    options.connect({
      source: activeSourceHandle.nodeId,
      sourceHandle: activeSourceHandle.handleId,
      target: releasedTargetNodeId,
      targetHandle: 'in:__auto',
    })
    return true
  }

  return {
    onConnectStart,
    onConnectEnd,
    cancel: stopTracking,
    dispose: stopTracking,
  }
}
