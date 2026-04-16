import { onUnmounted, ref, type Ref } from 'vue'
import { isCanvasConnectionAllowed, resolveCanvasConnectionTone, type CanvasConnectionTone } from '../config/canvasConnections'
import {
  isCanvasConnectionNodeType,
  isCanvasConnectionPort,
  isCanvasPortDirection,
} from '../config/canvasConnectionTargets'
import type {
  CanvasConnection,
  CanvasConnectionNodeType,
  CanvasConnectionPort,
} from '../types/canvasNodes'

export interface ConnectionEndpoint {
  nodeType: CanvasConnectionNodeType
  nodeId: number
  port: CanvasConnectionPort
}

export interface ParsedPortTarget {
  endpoint: ConnectionEndpoint
  direction: 'input' | 'output'
  element: HTMLElement
}

export interface RenderedConnection {
  id: number
  path: string
  startX: number
  startY: number
  endX: number
  endY: number
  tone: CanvasConnectionTone
}

export interface UseCanvasConnectionsOptions {
  connections: Ref<CanvasConnection[]>
  canvasLayoutRef: Ref<HTMLElement | null>
  onConnectionNotAllowed?: () => void
  onInvalidDirection?: () => void
}

function createConnectionId(): number {
  return Date.now() + Math.random()
}

function getNodeConnectionErrorKey(nodeType: CanvasConnectionNodeType, nodeId: number) {
  return `${nodeType}:${nodeId}`
}

export function useCanvasConnections(options: UseCanvasConnectionsOptions) {
  const selectedConnectionId = ref<number | null>(null)
  const renderedConnections = ref<RenderedConnection[]>([])
  const draftConnectionPath = ref('')
  const draftConnectionTone = ref<'default' | 'valid' | 'invalid'>('default')
  const draftConnectionKind = ref<CanvasConnectionTone>('image')
  const nodeConnectionErrorPorts = ref<Record<string, string[]>>({})

  const connectionDragState = ref<{
    from: ConnectionEndpoint
    startX: number
    startY: number
    currentX: number
    currentY: number
  } | null>(null)

  let connectionRenderRaf: number | null = null
  const nodeConnectionErrorTimers = new Map<string, number>()
  const nodeConnectionErrorFrames = new Map<string, number>()

  function clearNodeConnectionError(nodeType: CanvasConnectionNodeType, nodeId: number) {
    const key = getNodeConnectionErrorKey(nodeType, nodeId)
    const timerId = nodeConnectionErrorTimers.get(key)
    if (timerId !== undefined) {
      window.clearTimeout(timerId)
      nodeConnectionErrorTimers.delete(key)
    }
    const frameId = nodeConnectionErrorFrames.get(key)
    if (frameId !== undefined) {
      window.cancelAnimationFrame(frameId)
      nodeConnectionErrorFrames.delete(key)
    }
    if (!(key in nodeConnectionErrorPorts.value)) return
    const nextErrors = { ...nodeConnectionErrorPorts.value }
    delete nextErrors[key]
    nodeConnectionErrorPorts.value = nextErrors
  }

  function clearAllNodeConnectionErrors() {
    nodeConnectionErrorTimers.forEach(timerId => window.clearTimeout(timerId))
    nodeConnectionErrorFrames.forEach(frameId => window.cancelAnimationFrame(frameId))
    nodeConnectionErrorTimers.clear()
    nodeConnectionErrorFrames.clear()
    nodeConnectionErrorPorts.value = {}
  }

  function getNodeConnectionErrorPorts(nodeType: CanvasConnectionNodeType, nodeId: number): string[] {
    return nodeConnectionErrorPorts.value[getNodeConnectionErrorKey(nodeType, nodeId)] ?? []
  }

  function triggerNodeConnectionError(
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
    portKeys: string[],
  ) {
    const nextPortKeys = [...new Set(portKeys.filter(Boolean))]
    if (nextPortKeys.length === 0) {
      clearNodeConnectionError(nodeType, nodeId)
      return
    }

    const key = getNodeConnectionErrorKey(nodeType, nodeId)
    clearNodeConnectionError(nodeType, nodeId)
    nodeConnectionErrorPorts.value = {
      ...nodeConnectionErrorPorts.value,
      [key]: [],
    }
    const frameId = window.requestAnimationFrame(() => {
      nodeConnectionErrorFrames.delete(key)
      nodeConnectionErrorPorts.value = {
        ...nodeConnectionErrorPorts.value,
        [key]: nextPortKeys,
      }
    })
    nodeConnectionErrorFrames.set(key, frameId)
    const timerId = window.setTimeout(() => {
      clearNodeConnectionError(nodeType, nodeId)
    }, 820)
    nodeConnectionErrorTimers.set(key, timerId)
  }

  function clearSelectedConnection() {
    selectedConnectionId.value = null
  }

  function hasSelectedConnection() {
    return selectedConnectionId.value !== null
  }

  function findIncomingConnection(
    toNodeType: CanvasConnectionNodeType,
    toNodeId: number,
    toPort: CanvasConnectionPort,
  ): CanvasConnection | null {
    return (
      options.connections.value.find(connection =>
        connection.toNodeType === toNodeType &&
        connection.toNodeId === toNodeId &&
        connection.toPort === toPort,
      ) ?? null
    )
  }

  function getIncomingConnections(toNodeType: CanvasConnectionNodeType, toNodeId: number): CanvasConnection[] {
    return options.connections.value.filter(connection => connection.toNodeType === toNodeType && connection.toNodeId === toNodeId)
  }

  function hasIncomingPortConnection(
    toNodeType: CanvasConnectionNodeType,
    toNodeId: number,
    toPort: CanvasConnectionPort,
  ): boolean {
    return !!findIncomingConnection(toNodeType, toNodeId, toPort)
  }

  function findOutgoingConnection(
    fromNodeType: CanvasConnectionNodeType,
    fromNodeId: number,
    fromPort: CanvasConnectionPort,
  ): CanvasConnection | null {
    return (
      options.connections.value.find(connection =>
        connection.fromNodeType === fromNodeType &&
        connection.fromNodeId === fromNodeId &&
        connection.fromPort === fromPort,
      ) ?? null
    )
  }

  function hasOutgoingPortConnection(
    fromNodeType: CanvasConnectionNodeType,
    fromNodeId: number,
    fromPort: CanvasConnectionPort,
  ): boolean {
    return !!findOutgoingConnection(fromNodeType, fromNodeId, fromPort)
  }

  function removeConnectionsForNode(nodeType: CanvasConnectionNodeType, nodeId: number) {
    const removedSelected = options.connections.value.some(connection =>
      connection.id === selectedConnectionId.value &&
      (
        (connection.fromNodeType === nodeType && connection.fromNodeId === nodeId) ||
        (connection.toNodeType === nodeType && connection.toNodeId === nodeId)
      ),
    )
    options.connections.value = options.connections.value.filter(connection =>
      !(
        (connection.fromNodeType === nodeType && connection.fromNodeId === nodeId) ||
        (connection.toNodeType === nodeType && connection.toNodeId === nodeId)
      ),
    )
    if (removedSelected) {
      clearSelectedConnection()
    }
  }

  function deleteSelectedConnection(): boolean {
    if (selectedConnectionId.value === null) return false
    const nextConnections = options.connections.value.filter(connection => connection.id !== selectedConnectionId.value)
    if (nextConnections.length === options.connections.value.length) return false
    options.connections.value = nextConnections
    clearSelectedConnection()
    return true
  }

  function getPortSelector(
    direction: 'input' | 'output',
    endpoint: ConnectionEndpoint,
  ): string {
    return [
      '.node-port',
      `[data-port-direction="${direction}"]`,
      `[data-node-kind="${endpoint.nodeType}"]`,
      `[data-node-id="${endpoint.nodeId}"]`,
      `[data-port-name="${endpoint.port}"]`,
    ].join('')
  }

  function parsePortTarget(target: HTMLElement | null, direction?: 'input' | 'output'): ParsedPortTarget | null {
    const selector = direction
      ? `.node-port[data-port-direction="${direction}"][data-port-name][data-node-kind][data-node-id]`
      : '.node-port[data-port-direction][data-port-name][data-node-kind][data-node-id]'
    const portEl = target?.closest<HTMLElement>(selector) ?? null
    if (!portEl) return null
    const nodeType = portEl.dataset.nodeKind
    const port = portEl.dataset.portName
    const rawNodeId = portEl.dataset.nodeId
    const portDirection = portEl.dataset.portDirection
    const nodeId = Number(rawNodeId)
    if (
      !isCanvasConnectionNodeType(nodeType) ||
      !isCanvasConnectionPort(port) ||
      !isCanvasPortDirection(portDirection) ||
      !Number.isFinite(nodeId)
    ) {
      return null
    }
    return {
      endpoint: {
        nodeType,
        nodeId,
        port,
      },
      direction: portDirection,
      element: portEl,
    }
  }

  function clearConnectionPortHighlights() {
    const layout = options.canvasLayoutRef.value
    if (!layout) return
    layout.querySelectorAll<HTMLElement>('.node-port').forEach((portEl) => {
      portEl.classList.remove(
        'is-connection-source',
        'is-connection-target',
        'is-connection-hover-target',
        'is-connection-invalid-target',
      )
    })
  }

  function refreshConnectionPortHighlights(hoveredPort: ParsedPortTarget | null = null) {
    clearConnectionPortHighlights()
    if (!connectionDragState.value) return
    const layout = options.canvasLayoutRef.value
    if (!layout) return

    const sourcePort = layout.querySelector<HTMLElement>(
      getPortSelector('output', connectionDragState.value.from),
    )
    sourcePort?.classList.add('is-connection-source')

    layout.querySelectorAll<HTMLElement>('.node-port[data-port-direction="input"]').forEach((portEl) => {
      const parsed = parsePortTarget(portEl, 'input')
      if (!parsed) return
      if (isCanvasConnectionAllowed(connectionDragState.value!.from, parsed.endpoint)) {
        portEl.classList.add('is-connection-target')
      }
    })

    if (!hoveredPort) return

    const isValidHover =
      hoveredPort.direction === 'input' &&
      isCanvasConnectionAllowed(connectionDragState.value.from, hoveredPort.endpoint)

    hoveredPort.element.classList.add(
      isValidHover ? 'is-connection-hover-target' : 'is-connection-invalid-target',
    )
  }

  function getPortCenter(
    direction: 'input' | 'output',
    endpoint: ConnectionEndpoint,
  ): { x: number; y: number } | null {
    const layout = options.canvasLayoutRef.value
    if (!layout) return null
    const port = layout.querySelector<HTMLElement>(getPortSelector(direction, endpoint))
    if (!port) return null
    const layoutRect = layout.getBoundingClientRect()
    const portRect = port.getBoundingClientRect()
    return {
      x: portRect.left - layoutRect.left + portRect.width / 2,
      y: portRect.top - layoutRect.top + portRect.height / 2,
    }
  }

  function getPortCenterFromElement(
    portEl: HTMLElement,
    layoutRect: DOMRect,
  ): { x: number; y: number } {
    const portRect = portEl.getBoundingClientRect()
    return {
      x: portRect.left - layoutRect.left + portRect.width / 2,
      y: portRect.top - layoutRect.top + portRect.height / 2,
    }
  }

  function buildConnectionPath(start: { x: number; y: number }, end: { x: number; y: number }): string {
    const curve = Math.max(56, Math.abs(end.x - start.x) * 0.45)
    return `M ${start.x} ${start.y} C ${start.x + curve} ${start.y}, ${end.x - curve} ${end.y}, ${end.x} ${end.y}`
  }

  function refreshRenderedConnections() {
    const nextRendered: RenderedConnection[] = []
    options.connections.value.forEach((connection) => {
      const start = getPortCenter('output', {
        nodeType: connection.fromNodeType,
        nodeId: connection.fromNodeId,
        port: connection.fromPort,
      })
      const end = getPortCenter('input', {
        nodeType: connection.toNodeType,
        nodeId: connection.toNodeId,
        port: connection.toPort,
      })
      if (!start || !end) return
      nextRendered.push({
        id: connection.id,
        path: buildConnectionPath(start, end),
        startX: start.x,
        startY: start.y,
        endX: end.x,
        endY: end.y,
        tone: resolveCanvasConnectionTone(connection.fromPort),
      })
    })
    renderedConnections.value = nextRendered
    if (connectionDragState.value) {
      draftConnectionPath.value = buildConnectionPath(
        { x: connectionDragState.value.startX, y: connectionDragState.value.startY },
        { x: connectionDragState.value.currentX, y: connectionDragState.value.currentY },
      )
      return
    }
    draftConnectionPath.value = ''
  }

  function scheduleRenderedConnectionsRefresh() {
    if (connectionRenderRaf !== null) return
    connectionRenderRaf = window.requestAnimationFrame(() => {
      connectionRenderRaf = null
      refreshRenderedConnections()
    })
  }

  function clearConnectionDraft() {
    connectionDragState.value = null
    draftConnectionPath.value = ''
    draftConnectionTone.value = 'default'
    draftConnectionKind.value = 'image'
    clearConnectionPortHighlights()
  }

  function upsertConnection(from: ConnectionEndpoint, to: ConnectionEndpoint) {
    if (!isCanvasConnectionAllowed(from, to)) return false
    const filtered = options.connections.value.filter(connection =>
      !(
        connection.toNodeType === to.nodeType &&
        connection.toNodeId === to.nodeId &&
        connection.toPort === to.port
      ),
    )
    const existingExact = filtered.find(connection =>
      connection.fromNodeType === from.nodeType &&
      connection.fromNodeId === from.nodeId &&
      connection.fromPort === from.port &&
      connection.toNodeType === to.nodeType &&
      connection.toNodeId === to.nodeId &&
      connection.toPort === to.port,
    )
    options.connections.value = existingExact
      ? filtered
      : [
          ...filtered,
          {
            id: createConnectionId(),
            fromNodeType: from.nodeType,
            fromNodeId: from.nodeId,
            fromPort: from.port,
            toNodeType: to.nodeType,
            toNodeId: to.nodeId,
            toPort: to.port,
          },
        ]
    clearSelectedConnection()
    return true
  }

  function beginConnectionDrag(from: ConnectionEndpoint, target: HTMLElement) {
    const layout = options.canvasLayoutRef.value
    if (!layout) return
    const layoutRect = layout.getBoundingClientRect()
    const targetRect = target.getBoundingClientRect()
    const startX = targetRect.left - layoutRect.left + targetRect.width / 2
    const startY = targetRect.top - layoutRect.top + targetRect.height / 2
    connectionDragState.value = {
      from,
      startX,
      startY,
      currentX: startX,
      currentY: startY,
    }
    draftConnectionPath.value = buildConnectionPath(
      { x: startX, y: startY },
      { x: startX, y: startY },
    )
    draftConnectionKind.value = resolveCanvasConnectionTone(from.port)
    draftConnectionTone.value = 'default'
    refreshConnectionPortHighlights()
  }

  function updateConnectionDrag(clientX: number, clientY: number) {
    if (!connectionDragState.value || !options.canvasLayoutRef.value) return
    const layoutRect = options.canvasLayoutRef.value.getBoundingClientRect()
    const target = document.elementFromPoint(clientX, clientY) as HTMLElement | null
    const hoveredPort = parsePortTarget(target)
    const isValidHover =
      !!hoveredPort &&
      hoveredPort.direction === 'input' &&
      isCanvasConnectionAllowed(connectionDragState.value.from, hoveredPort.endpoint)
    const previewPoint = isValidHover && hoveredPort
      ? getPortCenterFromElement(hoveredPort.element, layoutRect)
      : {
          x: clientX - layoutRect.left,
          y: clientY - layoutRect.top,
        }
    connectionDragState.value = {
      ...connectionDragState.value,
      currentX: previewPoint.x,
      currentY: previewPoint.y,
    }
    draftConnectionPath.value = buildConnectionPath(
      { x: connectionDragState.value.startX, y: connectionDragState.value.startY },
      { x: connectionDragState.value.currentX, y: connectionDragState.value.currentY },
    )
    if (!hoveredPort) {
      draftConnectionTone.value = 'default'
      refreshConnectionPortHighlights(null)
      return
    }
    draftConnectionTone.value = isValidHover ? 'valid' : 'invalid'
    refreshConnectionPortHighlights(hoveredPort)
  }

  function finishConnectionDrag(clientX: number, clientY: number): 'linked' | 'invalid-pair' | 'invalid-direction' | 'noop' {
    if (!connectionDragState.value) return 'noop'
    const target = document.elementFromPoint(clientX, clientY) as HTMLElement | null
    const parsedPort = parsePortTarget(target)
    let result: 'linked' | 'invalid-pair' | 'invalid-direction' | 'noop' = 'noop'
    if (parsedPort?.direction === 'input') {
      const linked = upsertConnection(connectionDragState.value.from, parsedPort.endpoint)
      result = linked ? 'linked' : 'invalid-pair'
    } else if (parsedPort) {
      result = 'invalid-direction'
    }
    clearConnectionDraft()
    return result
  }

  function handleConnectionMouseMove(event: MouseEvent) {
    updateConnectionDrag(event.clientX, event.clientY)
  }

  function handleConnectionMouseUp(event: MouseEvent) {
    const result = finishConnectionDrag(event.clientX, event.clientY)
    if (result === 'invalid-pair') {
      options.onConnectionNotAllowed?.()
      return
    }
    if (result === 'invalid-direction') {
      options.onInvalidDirection?.()
    }
  }

  function addGlobalConnectionListeners() {
    window.addEventListener('mousemove', handleConnectionMouseMove, true)
    window.addEventListener('mouseup', handleConnectionMouseUp, true)
  }

  function removeGlobalConnectionListeners() {
    window.removeEventListener('mousemove', handleConnectionMouseMove, true)
    window.removeEventListener('mouseup', handleConnectionMouseUp, true)
  }

  function startConnectionDrag(
    event: MouseEvent,
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
    port: CanvasConnectionPort,
  ) {
    const target = event.currentTarget
    if (!(target instanceof HTMLElement)) return
    beginConnectionDrag({ nodeType, nodeId, port }, target)
  }

  function selectConnection(connectionId: number) {
    clearSelectedConnection()
    selectedConnectionId.value = connectionId
  }

  function disposeConnectionRuntime() {
    removeGlobalConnectionListeners()
    if (connectionRenderRaf !== null) {
      window.cancelAnimationFrame(connectionRenderRaf)
      connectionRenderRaf = null
    }
    clearConnectionDraft()
    clearAllNodeConnectionErrors()
  }

  onUnmounted(() => {
    disposeConnectionRuntime()
  })

  return {
    selectedConnectionId,
    renderedConnections,
    draftConnectionPath,
    draftConnectionTone,
    draftConnectionKind,
    clearSelectedConnection,
    hasSelectedConnection,
    findIncomingConnection,
    getIncomingConnections,
    hasIncomingPortConnection,
    findOutgoingConnection,
    hasOutgoingPortConnection,
    removeConnectionsForNode,
    deleteSelectedConnection,
    clearNodeConnectionError,
    clearAllNodeConnectionErrors,
    getNodeConnectionErrorPorts,
    triggerNodeConnectionError,
    scheduleRenderedConnectionsRefresh,
    clearConnectionDraft,
    startConnectionDrag,
    beginConnectionDrag,
    updateConnectionDrag,
    finishConnectionDrag,
    addGlobalConnectionListeners,
    removeGlobalConnectionListeners,
    disposeConnectionRuntime,
    selectConnection,
  }
}
