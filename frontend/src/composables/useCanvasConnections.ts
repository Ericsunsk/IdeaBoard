import { computed, onUnmounted, ref, type Ref } from 'vue'
import { resolveCanvasConnectionTone, type CanvasConnectionTone } from '../config/canvasConnections'
import {
  isCanvasConnectionNodeType,
  isCanvasConnectionPort,
  isCanvasPortDirection,
} from '../config/canvasConnectionTargets'
import type {
  CanvasConnection,
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasNodeItem,
} from '../types/canvasNodes'
import {
  buildEditNodeIoContext,
  COLOR_NODE_IO_SLOTS,
  EDIT_NODE_IO_SLOTS,
  ENHANCE_NODE_IO_SLOTS,
  FUSION_NODE_IO_SLOTS,
  IMAGE_NODE_IO_SLOTS,
  MASK_NODE_IO_SLOTS,
  MODEL_NODE_IO_SLOTS,
  PROMPT_NODE_IO_SLOTS,
  WEAR_NODE_IO_SLOTS,
  VIEWPOINT_NODE_IO_SLOTS,
  type NodeIoSlotTemplate,
} from '../components/canvas/nodes/nodeIoConfig'

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
  canvasX: Ref<number>
  canvasY: Ref<number>
  zoomLevel: Ref<number>
  isConnectionAllowed: (from: ConnectionEndpoint, to: ConnectionEndpoint) => boolean
  lookupNodeByType: (nodeType: CanvasConnectionNodeType, nodeId: number) => CanvasNodeItem | null
  onConnectionNotAllowed?: () => void
  onInvalidDirection?: () => void
}

type ConnectionRenderableNode = Pick<CanvasNodeItem, 'x' | 'y' | 'width'>

const WORKFLOW_NODE_BORDER_WIDTH = 1.5
// Match the actual visual center of NodePorts relative to the node border box:
// - Port width: 9px, border: 1.5px => port border-box width = 12px
// - Input left: -5.5px, output right: -5.5px
// - Node absolute x/y uses the workflow-node border-box origin, while absolute child positioning
//   is resolved from the padding box. We therefore need to add the node border width back in.
const INPUT_PORT_CENTER_OFFSET_X = WORKFLOW_NODE_BORDER_WIDTH + 0.5
const OUTPUT_PORT_CENTER_OFFSET_X = -(WORKFLOW_NODE_BORDER_WIDTH + 0.5)

const NODE_IO_SLOTS_BY_TYPE: Record<CanvasConnectionNodeType, readonly NodeIoSlotTemplate<any>[]> = {
  image: IMAGE_NODE_IO_SLOTS,
  prompt: PROMPT_NODE_IO_SLOTS,
  mask: MASK_NODE_IO_SLOTS,
  color: COLOR_NODE_IO_SLOTS,
  model: MODEL_NODE_IO_SLOTS,
  viewpoint: VIEWPOINT_NODE_IO_SLOTS,
  edit: EDIT_NODE_IO_SLOTS,
  fusion: FUSION_NODE_IO_SLOTS,
  enhance: ENHANCE_NODE_IO_SLOTS,
  wear: WEAR_NODE_IO_SLOTS,
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
  const reconnectingConnectionId = computed(() => connectionDragState.value?.reconnectConnectionId ?? null)

  const connectionDragState = ref<{
    from: ConnectionEndpoint
    startX: number
    startY: number
    currentX: number
    currentY: number
    reconnectConnectionId?: number
    reconnectConnectionSnapshot?: CanvasConnection
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

  function getOutgoingConnections(
    fromNodeType: CanvasConnectionNodeType,
    fromNodeId: number,
    fromPort: CanvasConnectionPort,
  ): CanvasConnection[] {
    return options.connections.value.filter(connection =>
      connection.fromNodeType === fromNodeType &&
      connection.fromNodeId === fromNodeId &&
      connection.fromPort === fromPort,
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

  function getNodeSelector(nodeType: CanvasConnectionNodeType, nodeId: number): string {
    return [
      '.workflow-node',
      `[data-node-kind="${nodeType}"]`,
      `[data-node-id="${nodeId}"]`,
    ].join('')
  }

  function getCanvasCenter(layout: HTMLElement) {
    return {
      x: layout.clientWidth / 2,
      y: layout.clientHeight / 2,
    }
  }

  function getCanvasLayerPointFromClient(clientX: number, clientY: number): { x: number; y: number } | null {
    const layout = options.canvasLayoutRef.value
    if (!layout) return null
    const layoutRect = layout.getBoundingClientRect()
    const center = getCanvasCenter(layout)
    const zoom = options.zoomLevel.value / 100
    return {
      x: center.x + (clientX - layoutRect.left - options.canvasX.value - center.x) / zoom,
      y: center.y + (clientY - layoutRect.top - options.canvasY.value - center.y) / zoom,
    }
  }

  function getNodeRenderHeight(nodeType: CanvasConnectionNodeType, nodeId: number): number | null {
    const layout = options.canvasLayoutRef.value
    if (!layout) return null
    const nodeEl = layout.querySelector<HTMLElement>(getNodeSelector(nodeType, nodeId))
    if (!nodeEl) return null
    return nodeEl.offsetHeight || null
  }

  function resolvePortTop(
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
    direction: 'input' | 'output',
    portName: CanvasConnectionPort,
    nodeHeight: number,
  ): number {
    const slot = NODE_IO_SLOTS_BY_TYPE[nodeType].find(item =>
      item.direction === direction && item.portName === portName,
    )
    if (!slot || slot.top == null) return nodeHeight / 2
    if (typeof slot.top === 'function') {
      const node = options.lookupNodeByType(nodeType, nodeId)
      if (!node) return nodeHeight / 2
      if (nodeType === 'edit' && node.type === 'edit') {
        return slot.top(buildEditNodeIoContext(node) as never)
      }
      return slot.top(node as never)
    }
    return slot.top
  }

  function getNodePortCenter(
    direction: 'input' | 'output',
    node: ConnectionRenderableNode,
    endpoint: ConnectionEndpoint,
  ): { x: number; y: number } | null {
    const layout = options.canvasLayoutRef.value
    if (!layout) return null
    const nodeHeight = getNodeRenderHeight(endpoint.nodeType, endpoint.nodeId)
    if (!nodeHeight) return null
    const center = getCanvasCenter(layout)
    const x =
      center.x +
      node.x +
      (direction === 'input' ? INPUT_PORT_CENTER_OFFSET_X : node.width + OUTPUT_PORT_CENTER_OFFSET_X)
    const y =
      center.y +
      node.y +
      WORKFLOW_NODE_BORDER_WIDTH +
      resolvePortTop(endpoint.nodeType, endpoint.nodeId, direction, endpoint.port, nodeHeight)
    return { x, y }
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
      if (options.isConnectionAllowed(connectionDragState.value!.from, parsed.endpoint)) {
        portEl.classList.add('is-connection-target')
      }
    })

    if (!hoveredPort) return

    const isValidHover =
      hoveredPort.direction === 'input' &&
      options.isConnectionAllowed(connectionDragState.value.from, hoveredPort.endpoint)

    hoveredPort.element.classList.add(
      isValidHover ? 'is-connection-hover-target' : 'is-connection-invalid-target',
    )
  }

  function getPortCenter(
    direction: 'input' | 'output',
    endpoint: ConnectionEndpoint,
  ): { x: number; y: number } | null {
    const node = options.lookupNodeByType(endpoint.nodeType, endpoint.nodeId)
    if (!node) return null
    return getNodePortCenter(direction, node, endpoint)
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
    if (!options.isConnectionAllowed(from, to)) return false
    const existingExact = options.connections.value.find(connection =>
      connection.fromNodeType === from.nodeType &&
      connection.fromNodeId === from.nodeId &&
      connection.fromPort === from.port &&
      connection.toNodeType === to.nodeType &&
      connection.toNodeId === to.nodeId &&
      connection.toPort === to.port,
    )
    if (existingExact) {
      options.connections.value = options.connections.value.filter(connection => connection.id !== existingExact.id)
      if (selectedConnectionId.value === existingExact.id) {
        clearSelectedConnection()
      }
      return true
    }

    const filtered = options.connections.value.filter(connection =>
      !(
        connection.toNodeType === to.nodeType &&
        connection.toNodeId === to.nodeId &&
        connection.toPort === to.port
      ),
    )
    options.connections.value = [
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

  function removeIncomingConnectionsForPort(
    toNodeType: CanvasConnectionNodeType,
    toNodeId: number,
    toPort: CanvasConnectionPort,
  ) {
    const removedSelected = options.connections.value.some(connection =>
      connection.id === selectedConnectionId.value &&
      connection.toNodeType === toNodeType &&
      connection.toNodeId === toNodeId &&
      connection.toPort === toPort,
    )
    options.connections.value = options.connections.value.filter(connection =>
      !(
        connection.toNodeType === toNodeType &&
        connection.toNodeId === toNodeId &&
        connection.toPort === toPort
      ),
    )
    if (removedSelected) {
      clearSelectedConnection()
    }
  }

  function beginConnectionDrag(from: ConnectionEndpoint) {
    const startPoint = getPortCenter('output', from)
    if (!startPoint) return
    connectionDragState.value = {
      from,
      startX: startPoint.x,
      startY: startPoint.y,
      currentX: startPoint.x,
      currentY: startPoint.y,
    }
    draftConnectionPath.value = buildConnectionPath(
      { x: startPoint.x, y: startPoint.y },
      { x: startPoint.x, y: startPoint.y },
    )
    draftConnectionKind.value = resolveCanvasConnectionTone(from.port)
    draftConnectionTone.value = 'default'
    refreshConnectionPortHighlights()
  }

  function beginConnectionReconnect(connectionId: number) {
    const connection = options.connections.value.find(item => item.id === connectionId)
    if (!connection) return
    const startPoint = getPortCenter('output', {
      nodeType: connection.fromNodeType,
      nodeId: connection.fromNodeId,
      port: connection.fromPort,
    })
    const endPoint = getPortCenter('input', {
      nodeType: connection.toNodeType,
      nodeId: connection.toNodeId,
      port: connection.toPort,
    })
    if (!startPoint || !endPoint) return
    connectionDragState.value = {
      from: {
        nodeType: connection.fromNodeType,
        nodeId: connection.fromNodeId,
        port: connection.fromPort,
      },
      startX: startPoint.x,
      startY: startPoint.y,
      currentX: endPoint.x,
      currentY: endPoint.y,
      reconnectConnectionId: connectionId,
      reconnectConnectionSnapshot: { ...connection },
    }
    draftConnectionPath.value = buildConnectionPath(startPoint, endPoint)
    draftConnectionKind.value = resolveCanvasConnectionTone(connection.fromPort)
    draftConnectionTone.value = 'default'
    refreshConnectionPortHighlights()
  }

  function updateConnectionDrag(clientX: number, clientY: number) {
    if (!connectionDragState.value) return
    const target = document.elementFromPoint(clientX, clientY) as HTMLElement | null
    const hoveredPort = parsePortTarget(target)
    const isValidHover =
      !!hoveredPort &&
      hoveredPort.direction === 'input' &&
      options.isConnectionAllowed(connectionDragState.value.from, hoveredPort.endpoint)
    const previewPoint = isValidHover && hoveredPort
      ? getPortCenter('input', hoveredPort.endpoint)
      : getCanvasLayerPointFromClient(clientX, clientY)
    if (!previewPoint) return
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
    const reconnectConnectionId = connectionDragState.value.reconnectConnectionId ?? null
    const reconnectConnectionSnapshot = connectionDragState.value.reconnectConnectionSnapshot
    let result: 'linked' | 'invalid-pair' | 'invalid-direction' | 'noop' = 'noop'
    if (parsedPort?.direction === 'input') {
      if (reconnectConnectionId !== null) {
        options.connections.value = options.connections.value.filter(connection => connection.id !== reconnectConnectionId)
      }
      const linked = upsertConnection(connectionDragState.value.from, parsedPort.endpoint)
      result = linked ? 'linked' : 'invalid-pair'
    } else if (parsedPort) {
      result = 'invalid-direction'
    }
    if (result !== 'linked' && reconnectConnectionSnapshot) {
      options.connections.value = [
        ...options.connections.value.filter(connection => connection.id !== reconnectConnectionSnapshot.id),
        reconnectConnectionSnapshot,
      ]
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
    beginConnectionDrag({ nodeType, nodeId, port })
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
    reconnectingConnectionId,
    clearSelectedConnection,
    hasSelectedConnection,
    findIncomingConnection,
    getIncomingConnections,
    hasIncomingPortConnection,
    findOutgoingConnection,
    getOutgoingConnections,
    hasOutgoingPortConnection,
    removeConnectionsForNode,
    removeIncomingConnectionsForPort,
    deleteSelectedConnection,
    clearNodeConnectionError,
    clearAllNodeConnectionErrors,
    getNodeConnectionErrorPorts,
    triggerNodeConnectionError,
    refreshRenderedConnections,
    scheduleRenderedConnectionsRefresh,
    clearConnectionDraft,
    startConnectionDrag,
    beginConnectionReconnect,
    beginConnectionDrag,
    updateConnectionDrag,
    finishConnectionDrag,
    addGlobalConnectionListeners,
    removeGlobalConnectionListeners,
    disposeConnectionRuntime,
    selectConnection,
  }
}
