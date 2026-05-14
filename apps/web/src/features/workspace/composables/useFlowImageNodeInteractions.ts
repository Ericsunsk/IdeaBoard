import { ref, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import { createImageNodeAt } from '../utils/canvasNodeFactory'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import { resolveImageNodeFrameSize } from '../utils/imageNodeSizing'
import type { ImageNodeItem } from '../types/canvasNodes'
import type { FlowAdapterNode } from '../utils/flowSnapshotAdapter'
import type { FlowNodeData } from '../types/flowTypes'

type FlowHistorySnapshot = { nodes: unknown[]; edges: unknown[] }

type UseFlowImageNodeInteractionsOptions = {
  nodes: Ref<Node[]>
  getImageNodeDisplayPayload: (nodeId: string) => { url: string; ratio: number } | null
  getImageNodeSnapshot: (
    nodeId: string,
  ) => { x: number; y: number; width: number; outputCount?: ImageNodeItem['outputCount'] } | null
  captureFlowHistorySnapshot: () => FlowHistorySnapshot
  pushFlowHistorySnapshot: (snapshot: FlowHistorySnapshot) => void
  markDirty: () => void
  refreshSnapshot: () => void
  appendFlowNodes: (nextNodes: Node[]) => void
}

function createImageFlowNode(item: ImageNodeItem): Node {
  const data: FlowNodeData = {
    kind: 'image',
    nodeType: 'image',
    title: `Image #${item.id}`,
    summary: 'Canvas image node',
    snapshotNode: item,
  }
  const nextNode: FlowAdapterNode = {
    id: `image-${item.id}`,
    type: 'canvas',
    dragHandle: resolveFlowNodeDragHandle('image'),
    position: { x: item.x, y: item.y },
    data,
  }
  return nextNode as Node
}

function cloneImageOutputs(source: ImageNodeItem): ImageNodeItem['outputImages'] {
  return Array.isArray(source.outputImages)
    ? source.outputImages
        .map((item) => ({
          url: `${item?.url ?? ''}`.trim(),
          ratio: Number.isFinite(item?.ratio) && Number(item.ratio) > 0 ? Number(item.ratio) : 1,
        }))
        .filter((item) => item.url)
    : []
}

export function useFlowImageNodeInteractions(options: UseFlowImageNodeInteractionsOptions) {
  const flowAltDragDuplicateState = ref<{
    initialPositions: Map<string, { x: number; y: number }>
  } | null>(null)

  function appendImageFlowNode(item: ImageNodeItem) {
    const historyBefore = options.captureFlowHistorySnapshot()
    options.appendFlowNodes([createImageFlowNode(item)])
    options.pushFlowHistorySnapshot(historyBefore)
    options.markDirty()
    options.refreshSnapshot()
  }

  function onFlowImageAppendToCanvas(nodeId: string) {
    const payload = options.getImageNodeDisplayPayload(nodeId)
    const sourceNode = options.getImageNodeSnapshot(nodeId)
    if (!payload || !sourceNode) return
    const imageNode = createImageNodeAt({
      x: sourceNode.x + Math.max(240, sourceNode.width + 64),
      y: sourceNode.y + 18,
    })
    imageNode.sourceImageUrl = payload.url
    imageNode.sourceImageRatio = payload.ratio
    imageNode.outputCount = sourceNode.outputCount ?? imageNode.outputCount
    imageNode.width = resolveImageNodeFrameSize(payload.ratio, true).width
    imageNode.outputStatus = 'ready'
    imageNode.outputImages = []
    appendImageFlowNode(imageNode)
  }

  function beginFlowNodeDragDuplicateTracking(isDuplicateMode: boolean) {
    if (!isDuplicateMode) {
      flowAltDragDuplicateState.value = null
      return
    }
    const selectedNodeIds = new Set(
      (options.nodes.value as Array<{ id: string; selected?: boolean }>)
        .filter((node) => !!node.selected)
        .map((node) => node.id),
    )
    const draggingSelectedIds = selectedNodeIds.size > 0 ? selectedNodeIds : new Set<string>()
    flowAltDragDuplicateState.value = {
      initialPositions: new Map(
        (options.nodes.value as Array<{ id: string; position?: { x?: number; y?: number } }>)
          .filter((node) => draggingSelectedIds.size === 0 || draggingSelectedIds.has(node.id))
          .map((node) => [
            node.id,
            { x: Number(node.position?.x ?? 0), y: Number(node.position?.y ?? 0) },
          ]),
      ),
    }
  }

  function finishFlowNodeDragDuplicateTracking(isDuplicateMode: boolean) {
    const altState = flowAltDragDuplicateState.value
    flowAltDragDuplicateState.value = null
    if (!altState || !isDuplicateMode || altState.initialPositions.size === 0) return
    const flowNodes = options.nodes.value as Array<{
      id: string
      data?: FlowNodeData
      position: { x: number; y: number }
    }>
    const idToNode = new Map(flowNodes.map((node) => [node.id, node]))
    const duplicateCandidates: Array<{ nodeId: string; delta: { x: number; y: number } }> = []
    altState.initialPositions.forEach((startPos, nodeId) => {
      const current = idToNode.get(nodeId)
      if (!current || current.data?.kind !== 'image') return
      const dx = Number(current.position?.x ?? 0) - startPos.x
      const dy = Number(current.position?.y ?? 0) - startPos.y
      if (Math.abs(dx) < 0.5 && Math.abs(dy) < 0.5) return
      duplicateCandidates.push({ nodeId, delta: { x: dx, y: dy } })
    })
    if (duplicateCandidates.length === 0) return
    duplicateCandidates.forEach(({ nodeId }) => {
      const node = idToNode.get(nodeId)
      const startPos = altState.initialPositions.get(nodeId)
      if (!node || !startPos) return
      node.position = { x: startPos.x, y: startPos.y }
    })
    const appended: Node[] = []
    duplicateCandidates.forEach(({ nodeId, delta }) => {
      const sourceNode = idToNode.get(nodeId)
      const source = sourceNode?.data?.snapshotNode as ImageNodeItem | undefined
      if (!source || typeof source !== 'object') return
      const copied = createImageNodeAt({
        x: Number(source.x ?? 0) + delta.x,
        y: Number(source.y ?? 0) + delta.y,
      })
      copied.sourceImageUrl = `${source.sourceImageUrl ?? ''}`.trim()
      copied.sourceImageRatio =
        Number.isFinite(source.sourceImageRatio) && Number(source.sourceImageRatio) > 0
          ? Number(source.sourceImageRatio)
          : 1
      copied.outputCount = source.outputCount ?? copied.outputCount
      copied.outputImages = cloneImageOutputs(source)
      const latestOutput =
        copied.outputImages.length > 0 ? copied.outputImages[copied.outputImages.length - 1] : null
      copied.width = resolveImageNodeFrameSize(
        latestOutput?.ratio ?? copied.sourceImageRatio,
        !!(latestOutput?.url || copied.sourceImageUrl),
      ).width
      copied.outputStatus = source.outputStatus
      appended.push(createImageFlowNode(copied))
    })
    if (appended.length === 0) return
    options.appendFlowNodes(appended)
    options.markDirty()
    options.refreshSnapshot()
  }

  return {
    appendImageFlowNode,
    flowAltDragDuplicateState,
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    onFlowImageAppendToCanvas,
  }
}
