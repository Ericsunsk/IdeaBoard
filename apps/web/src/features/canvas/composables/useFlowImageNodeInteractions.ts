import { ref, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import { createCanvasNodeAt } from '../utils/canvasNodeFactory'
import { resolveImageNodeFrameSize } from '../../canvas-board/domain'
import {
  resolveImageNodeDisplayPayload,
  type ImageNodeDisplayPayload,
} from '../utils/flowImageNodeSelectors'
import type { ImageNodeItem } from '../../canvas-board/domain'
import type { FlowNodeData } from '../types/flowTypes'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from './flowGraphAdapterCommandSideEffects'
import { createImageFlowNode } from '../utils/flowImageNodeFactory'

type UseFlowImageNodeInteractionsOptions = {
  nodes: Ref<Node[]>
  getImageNodeDisplayPayload: (nodeId: string) => ImageNodeDisplayPayload | null
  graphAdapter: CanvasVueFlowGraphAdapter
}

function cloneImageOutputs(source: ImageNodeItem): ImageNodeItem['outputImages'] {
  return Array.isArray(source.outputImages)
    ? source.outputImages
        .map((item) => {
          const mediaAssetId = Number(item?.mediaAssetId)
          return {
            url: `${item?.url ?? ''}`.trim(),
            ratio: Number.isFinite(item?.ratio) && Number(item.ratio) > 0 ? Number(item.ratio) : 1,
            ...(Number.isInteger(mediaAssetId) && mediaAssetId > 0 ? { mediaAssetId } : {}),
          }
        })
        .filter((item) => item.url)
    : []
}

export function useFlowImageNodeInteractions(options: UseFlowImageNodeInteractionsOptions) {
  const flowAltDragDuplicateState = ref<{
    initialPositions: Map<string, { x: number; y: number }>
  } | null>(null)

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
    const appended: Node[] = []
    duplicateCandidates.forEach(({ nodeId, delta }) => {
      const sourceNode = idToNode.get(nodeId)
      const source = sourceNode?.data?.snapshotNode as ImageNodeItem | undefined
      if (!source || typeof source !== 'object') return
      const copied = createCanvasNodeAt('image', {
        x: Number(source.x ?? 0) + delta.x,
        y: Number(source.y ?? 0) + delta.y,
      })
      copied.sourceImageUrl = `${source.sourceImageUrl ?? ''}`.trim()
      copied.sourceMediaAssetId = source.sourceMediaAssetId
      copied.sourceImageRatio =
        Number.isFinite(source.sourceImageRatio) && Number(source.sourceImageRatio) > 0
          ? Number(source.sourceImageRatio)
          : 1
      copied.outputCount = source.outputCount ?? copied.outputCount
      copied.outputImages = cloneImageOutputs(source)
      const displayPayload = resolveImageNodeDisplayPayload(copied)
      copied.width = resolveImageNodeFrameSize(displayPayload?.ratio ?? 1, !!displayPayload).width
      copied.outputStatus = source.outputStatus
      appended.push(createImageFlowNode(copied))
    })
    if (appended.length === 0) return
    const resetPositionByNodeId = new Map<string, { x: number; y: number }>()
    duplicateCandidates.forEach(({ nodeId }) => {
      const startPos = altState.initialPositions.get(nodeId)
      if (startPos) resetPositionByNodeId.set(nodeId, startPos)
    })
    options.graphAdapter.replaceNodes(
      (items) => [
        ...items.map((item) => {
          const resetPosition = resetPositionByNodeId.get(item.id)
          return resetPosition ? { ...item, position: resetPosition } : item
        }),
        ...appended,
      ],
      createFlowGraphCommandOptions({ recordHistory: false }),
    )
  }

  return {
    flowAltDragDuplicateState,
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
  }
}
