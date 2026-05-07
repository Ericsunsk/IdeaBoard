import { ref, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import { createCanvasImageItemAt } from '../utils/canvasNodeFactory'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import type { CanvasInteractionKey } from './useFlowCanvasInteractionState'
import type { CanvasImageItem } from '../types/canvasNodes'
import type { FlowAdapterNode } from '../utils/flowSnapshotAdapter'
import type { FlowNodeData } from '../types/flowTypes'

type ResizeDirection = 'n' | 's' | 'e' | 'w' | 'nw' | 'ne' | 'sw' | 'se'
type FlowHistorySnapshot = { nodes: unknown[]; edges: unknown[] }

type UseFlowMessageImageInteractionsOptions = {
  nodes: Ref<Node[]>
  FLOW_NODE_IMAGE_DRAG_MIME: string
  MIN_MESSAGE_IMAGE_WIDTH: number
  MIN_MESSAGE_IMAGE_HEIGHT: number
  getImageNodeDisplayPayload: (nodeId: string) => { url: string; ratio: number } | null
  getImageNodeSnapshot: (nodeId: string) => { x: number; y: number; width: number } | null
  getViewport: () => { zoom: number }
  updateSnapshotNode: (
    nodeId: string,
    patcher: (snapshotNode: Record<string, any>) => Record<string, any>,
    mutationOptions?: { history?: boolean; dirty?: boolean; historyBefore?: FlowHistorySnapshot },
  ) => void
  captureFlowHistorySnapshot: () => FlowHistorySnapshot
  pushFlowHistorySnapshot: (snapshot: FlowHistorySnapshot) => void
  markDirty: () => void
  refreshSnapshot: () => void
  appendFlowNodes: (nextNodes: Node[]) => void
  startCanvasInteraction: (key: CanvasInteractionKey) => void
  stopCanvasInteraction: (key: CanvasInteractionKey) => void
}

export function useFlowMessageImageInteractions(options: UseFlowMessageImageInteractionsOptions) {
  const flowAltDragDuplicateState = ref<{
    initialPositions: Map<string, { x: number; y: number }>
  } | null>(null)

  function appendMessageFlowNode(item: CanvasImageItem) {
    const historyBefore = options.captureFlowHistorySnapshot()
    const data: FlowNodeData = {
      kind: 'message',
      title: `Message #${item.id}`,
      summary: 'Canvas message image',
      inputPorts: [],
      outputPorts: [],
      snapshotNode: item,
    }
    const nextNode: FlowAdapterNode = {
      id: `message-${item.id}`,
      type: 'canvas',
      dragHandle: resolveFlowNodeDragHandle('message'),
      position: { x: item.x, y: item.y },
      data,
    }
    options.appendFlowNodes([nextNode as Node])
    options.pushFlowHistorySnapshot(historyBefore)
    options.markDirty()
    options.refreshSnapshot()
  }

  function onFlowImageAppendToCanvas(nodeId: string) {
    const payload = options.getImageNodeDisplayPayload(nodeId)
    const sourceNode = options.getImageNodeSnapshot(nodeId)
    if (!payload || !sourceNode) return
    const messageNode = createCanvasImageItemAt(
      { x: sourceNode.x + Math.max(240, sourceNode.width + 64), y: sourceNode.y + 18 },
      payload.url,
      payload.ratio,
    )
    messageNode.width = sourceNode.width
    appendMessageFlowNode(messageNode)
  }

  function onFlowImageDisplayDragStart(nodeId: string, event: DragEvent) {
    const payload = options.getImageNodeDisplayPayload(nodeId)
    if (!payload || !event.dataTransfer) return
    event.dataTransfer.effectAllowed = 'copy'
    event.dataTransfer.setData(options.FLOW_NODE_IMAGE_DRAG_MIME, payload.url)
    event.dataTransfer.setData(`${options.FLOW_NODE_IMAGE_DRAG_MIME}-ratio`, String(payload.ratio))
  }

  function onFlowMessageImageDragStart(nodeId: string, event: DragEvent) {
    const target = (options.nodes.value as Array<{ id: string; data?: FlowNodeData }>).find(node => node.id === nodeId)
    const snapshot = target?.data?.snapshotNode as CanvasImageItem | undefined
    const url = `${snapshot?.url ?? ''}`.trim()
    if (!url || !event.dataTransfer) return
    const ratio = Number.isFinite(snapshot?.height_ratio) && Number(snapshot?.height_ratio) > 0
      ? Number(snapshot?.height_ratio)
      : 1
    event.dataTransfer.effectAllowed = 'copy'
    event.dataTransfer.setData(options.FLOW_NODE_IMAGE_DRAG_MIME, url)
    event.dataTransfer.setData(`${options.FLOW_NODE_IMAGE_DRAG_MIME}-ratio`, String(ratio))
  }

  function onFlowMessageResizeMouseDown(nodeId: string, payload: { event: MouseEvent; direction: ResizeDirection }) {
    const { event, direction } = payload
    const target = (options.nodes.value as Array<{ id: string; data?: FlowNodeData }>).find(node => node.id === nodeId)
    const snapshot = target?.data?.snapshotNode as CanvasImageItem | undefined
    const flowNode = target as { position?: { x?: number; y?: number } } | undefined
    if (!snapshot) return
    const startClientX = event.clientX
    const startClientY = event.clientY
    const startWidthRaw = Number(snapshot.width)
    const startWidth = Number.isFinite(startWidthRaw) && startWidthRaw > 0 ? startWidthRaw : 300
    const ratioRaw = Number(snapshot.height_ratio)
    const ratio = Number.isFinite(ratioRaw) && ratioRaw > 0 ? ratioRaw : 1
    const startHeight = startWidth * ratio
    const startX = Number(snapshot.x)
    const startY = Number(snapshot.y)
    const startNodeX = Number(flowNode?.position?.x ?? startX)
    const startNodeY = Number(flowNode?.position?.y ?? startY)
    const historyBefore = options.captureFlowHistorySnapshot()
    let hasResized = false
    options.startCanvasInteraction('message-resize')

    const onPointerMove = (moveEvent: MouseEvent) => {
      const viewport = options.getViewport()
      const zoom = Number.isFinite(viewport.zoom) && viewport.zoom > 0 ? viewport.zoom : 1
      const dx = (moveEvent.clientX - startClientX) / zoom
      const dy = (moveEvent.clientY - startClientY) / zoom
      let nextWidth = startWidth
      let nextX = startX
      let nextY = startY
      let nextNodeX = startNodeX
      let nextNodeY = startNodeY

      if (direction.includes('e')) {
        nextWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, startWidth + dx)
      } else if (direction.includes('w')) {
        nextWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, startWidth - dx)
        const shiftX = startWidth - nextWidth
        nextX = startX + shiftX
        nextNodeX = startNodeX + shiftX
      } else if (direction.includes('n')) {
        const nextHeight = Math.max(options.MIN_MESSAGE_IMAGE_HEIGHT, startHeight - dy)
        nextWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, nextHeight / ratio)
        const deltaHeight = (nextWidth - startWidth) * ratio
        nextY = startY - deltaHeight
        nextNodeY = startNodeY - deltaHeight
      } else if (direction.includes('s')) {
        const nextHeight = Math.max(options.MIN_MESSAGE_IMAGE_HEIGHT, startHeight + dy)
        nextWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, nextHeight / ratio)
      }

      if (direction.includes('n') && direction.includes('w')) {
        const westWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, startWidth - dx)
        const northHeight = Math.max(options.MIN_MESSAGE_IMAGE_HEIGHT, startHeight - dy)
        const northWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, northHeight / ratio)
        nextWidth = Math.max(westWidth, northWidth)
        const shiftX = startWidth - nextWidth
        nextX = startX + shiftX
        nextNodeX = startNodeX + shiftX
        const deltaHeight = (nextWidth - startWidth) * ratio
        nextY = startY - deltaHeight
        nextNodeY = startNodeY - deltaHeight
      } else if (direction.includes('n') && direction.includes('e')) {
        const eastWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, startWidth + dx)
        const northHeight = Math.max(options.MIN_MESSAGE_IMAGE_HEIGHT, startHeight - dy)
        const northWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, northHeight / ratio)
        nextWidth = Math.max(eastWidth, northWidth)
        const deltaHeight = (nextWidth - startWidth) * ratio
        nextY = startY - deltaHeight
        nextNodeY = startNodeY - deltaHeight
      } else if (direction.includes('s') && direction.includes('w')) {
        const westWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, startWidth - dx)
        const southHeight = Math.max(options.MIN_MESSAGE_IMAGE_HEIGHT, startHeight + dy)
        const southWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, southHeight / ratio)
        nextWidth = Math.max(westWidth, southWidth)
        const shiftX = startWidth - nextWidth
        nextX = startX + shiftX
        nextNodeX = startNodeX + shiftX
      } else if (direction.includes('s') && direction.includes('e')) {
        const eastWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, startWidth + dx)
        const southHeight = Math.max(options.MIN_MESSAGE_IMAGE_HEIGHT, startHeight + dy)
        const southWidth = Math.max(options.MIN_MESSAGE_IMAGE_WIDTH, southHeight / ratio)
        nextWidth = Math.max(eastWidth, southWidth)
      }

      nextWidth = Math.round(nextWidth)
      options.updateSnapshotNode(nodeId, (prev) => {
        const prevWidth = Number(prev.width)
        const prevRatio = Number(prev.height_ratio)
        const prevX = Number(prev.x)
        const prevY = Number(prev.y)
        if (prevWidth === nextWidth && prevRatio === ratio && prevX === nextX && prevY === nextY) return prev
        hasResized = true
        return {
          ...prev,
          width: nextWidth,
          height_ratio: ratio,
          x: nextX,
          y: nextY,
        }
      }, { history: false, dirty: false })
      const flowNodes = options.nodes.value as Array<Node & { position?: { x: number; y: number } }>
      const node = flowNodes.find(n => n.id === nodeId)
      if (node) {
        const currentX = Number(node.position?.x ?? 0)
        const currentY = Number(node.position?.y ?? 0)
        if (currentX !== nextNodeX || currentY !== nextNodeY) {
          node.position = { x: nextNodeX, y: nextNodeY }
        }
      }
    }

    const onPointerUp = () => {
      window.removeEventListener('mousemove', onPointerMove, true)
      window.removeEventListener('mouseup', onPointerUp, true)
      if (hasResized) {
        options.pushFlowHistorySnapshot(historyBefore)
        options.markDirty()
        options.refreshSnapshot()
      }
      options.stopCanvasInteraction('message-resize')
    }

    window.addEventListener('mousemove', onPointerMove, true)
    window.addEventListener('mouseup', onPointerUp, true)
  }

  function beginFlowNodeDragDuplicateTracking(isDuplicateMode: boolean) {
    if (!isDuplicateMode) {
      flowAltDragDuplicateState.value = null
      return
    }
    const selectedNodeIds = new Set(
      (options.nodes.value as Array<{ id: string; selected?: boolean }>)
        .filter(node => !!node.selected)
        .map(node => node.id),
    )
    const draggingSelectedIds = selectedNodeIds.size > 0
      ? selectedNodeIds
      : new Set<string>()
    flowAltDragDuplicateState.value = {
      initialPositions: new Map(
        (options.nodes.value as Array<{ id: string; position?: { x?: number; y?: number } }>)
          .filter(node => draggingSelectedIds.size === 0 || draggingSelectedIds.has(node.id))
          .map(node => [node.id, { x: Number(node.position?.x ?? 0), y: Number(node.position?.y ?? 0) }]),
      ),
    }
  }

  function finishFlowNodeDragDuplicateTracking(isDuplicateMode: boolean) {
    const altState = flowAltDragDuplicateState.value
    flowAltDragDuplicateState.value = null
    if (!altState || !isDuplicateMode || altState.initialPositions.size === 0) return
    const flowNodes = options.nodes.value as Array<{ id: string; data?: FlowNodeData; position: { x: number; y: number } }>
    const idToNode = new Map(flowNodes.map(node => [node.id, node]))
    const deltaByNode = new Map<string, { x: number; y: number }>()
    altState.initialPositions.forEach((startPos, nodeId) => {
      const current = idToNode.get(nodeId)
      if (!current) return
      const dx = Number(current.position?.x ?? 0) - startPos.x
      const dy = Number(current.position?.y ?? 0) - startPos.y
      if (Math.abs(dx) < 0.5 && Math.abs(dy) < 0.5) return
      deltaByNode.set(nodeId, { x: dx, y: dy })
    })
    if (deltaByNode.size === 0) return
    flowNodes.forEach((node) => {
      const delta = deltaByNode.get(node.id)
      if (!delta) return
      const startPos = altState.initialPositions.get(node.id)
      if (!startPos) return
      node.position = { x: startPos.x, y: startPos.y }
    })
    const appended: Node[] = []
    deltaByNode.forEach((delta, nodeId) => {
      const sourceNode = idToNode.get(nodeId)
      if (!sourceNode?.data || sourceNode.data.kind !== 'message') return
      const source = sourceNode.data.snapshotNode as CanvasImageItem | undefined
      if (!source || typeof source !== 'object' || !source.url) return
      const copied = createCanvasImageItemAt(
        {
          x: source.x + delta.x,
          y: source.y + delta.y,
        },
        source.url,
        source.height_ratio,
      )
      const sourceWidth = Number(source.width)
      copied.width = Number.isFinite(sourceWidth) && sourceWidth > 0 ? sourceWidth : copied.width
      copied.renderState = source.renderState
      const nextNode: FlowAdapterNode = {
        id: `message-${copied.id}`,
        type: 'canvas',
        dragHandle: resolveFlowNodeDragHandle('message'),
        position: { x: copied.x, y: copied.y },
        data: {
          kind: 'message',
          title: `Message #${copied.id}`,
          summary: 'Canvas message image',
          inputPorts: [],
          outputPorts: [],
          snapshotNode: copied,
        },
      }
      appended.push(nextNode as Node)
    })
    if (appended.length === 0) return
    options.appendFlowNodes(appended)
    options.markDirty()
    options.refreshSnapshot()
  }

  return {
    appendMessageFlowNode,
    flowAltDragDuplicateState,
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    onFlowImageAppendToCanvas,
    onFlowImageDisplayDragStart,
    onFlowMessageImageDragStart,
    onFlowMessageResizeMouseDown,
  }
}
