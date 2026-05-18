import { nextTick, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { createImageNodeAt } from '../utils/canvasNodeFactory'
import { clampImageNodeOutputCount, type ImageNodeItem } from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import { resolveImageNodeFrameSize, resolveImageNodeStageHeight } from '../utils/imageNodeSizing'
import {
  resolveImageDirectActionSource,
  type ImageDirectActionSource,
} from '../utils/flowImageDirectActionSource'
import type { FlowTransactions } from './useFlowTransactions'

type UsePendingImageResultNodesOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  commitGraphMutation: FlowTransactions['commitGraphMutation']
  commitNodeMutation: FlowTransactions['commitNodeMutation']
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  refreshNodeInternals?: (nodeIds: string[]) => void
}

type PendingImageNodeItem = ImageNodeItem & {
  pendingTaskId?: string
  pendingSourceNodeId?: string
  pendingPrompt?: string
}

type PendingTaskMetaPatch = Pick<
  PendingImageNodeItem,
  'pendingTaskId' | 'pendingSourceNodeId' | 'pendingPrompt'
>

const IMAGE_RESULT_NODE_X_GAP = 100
const IMAGE_RESULT_NODE_Y_GAP = 14
const IMAGE_NODE_HEADER_HEIGHT = 26

function estimateImageNodeHeight(source: Pick<ImageDirectActionSource, 'width' | 'ratio'>): number {
  const stageHeight = resolveImageNodeStageHeight(source.width, source.ratio, true)
  return IMAGE_NODE_HEADER_HEIGHT + stageHeight
}

export function usePendingImageResultNodes(options: UsePendingImageResultNodesOptions) {
  const cancelledResultNodeIds = new Set<string>()
  const resultLayoutsBySourceNodeId = new Map<string, Map<string, ImageDirectActionSource>>()

  function readImageNodeLayout(node: Node): ImageDirectActionSource | null {
    if (node.data?.kind !== 'image') return null
    const snapshot = node.data.snapshotNode
    if (!snapshot || typeof snapshot !== 'object') return null
    return resolveImageDirectActionSource(node, snapshot as ImageNodeItem)
  }

  function resolvePendingImageResultPosition(
    source: ImageDirectActionSource,
    sourceNodeId: string,
  ) {
    const defaultPosition = {
      x: source.x + source.width + IMAGE_RESULT_NODE_X_GAP,
      y: source.y,
    }
    const childNodeIds = new Set<string>(
      options.edges.value
        .filter(
          (edge) =>
            edge.source === sourceNodeId &&
            edge.sourceHandle === 'out:image' &&
            edge.targetHandle === 'in:image',
        )
        .map((edge) => edge.target),
    )
    const runtimeLayouts = resultLayoutsBySourceNodeId.get(sourceNodeId)
    runtimeLayouts?.forEach((_, nodeId) => childNodeIds.add(nodeId))
    if (childNodeIds.size === 0) return defaultPosition

    let nextY = source.y
    let alignedX = defaultPosition.x
    const measuredNodeIds = new Set<string>()
    const visitChildLayout = (nodeId: string, layout: ImageDirectActionSource) => {
      const bottomY = layout.y + estimateImageNodeHeight(layout) + IMAGE_RESULT_NODE_Y_GAP
      if (bottomY >= nextY) {
        alignedX = layout.x
        nextY = bottomY
      }
      measuredNodeIds.add(nodeId)
    }

    for (const node of options.nodes.value) {
      if (!childNodeIds.has(node.id)) continue
      const layout = readImageNodeLayout(node) ?? runtimeLayouts?.get(node.id)
      if (!layout) continue
      visitChildLayout(node.id, layout)
    }
    runtimeLayouts?.forEach((layout, nodeId) => {
      if (!childNodeIds.has(nodeId)) return
      if (measuredNodeIds.has(nodeId)) return
      visitChildLayout(nodeId, layout)
    })

    return {
      x: alignedX,
      y: nextY,
    }
  }

  function rememberResultNodeLayout(
    sourceNodeId: string,
    resultNodeId: string,
    layout: ImageDirectActionSource,
  ) {
    const current = resultLayoutsBySourceNodeId.get(sourceNodeId) ?? new Map()
    current.set(resultNodeId, layout)
    resultLayoutsBySourceNodeId.set(sourceNodeId, current)
  }

  function forgetResultNodeLayout(resultNodeId: string) {
    resultLayoutsBySourceNodeId.forEach((items, sourceNodeId) => {
      items.delete(resultNodeId)
      if (items.size === 0) resultLayoutsBySourceNodeId.delete(sourceNodeId)
    })
  }

  function updateResultNodeLayout(resultNodeId: string, patch: Partial<ImageDirectActionSource>) {
    resultLayoutsBySourceNodeId.forEach((items) => {
      const current = items.get(resultNodeId)
      if (!current) return
      items.set(resultNodeId, { ...current, ...patch })
    })
  }

  function pruneResultNodeLayouts() {
    const existingNodeIds = new Set(options.nodes.value.map((node) => node.id))
    resultLayoutsBySourceNodeId.forEach((items, sourceNodeId) => {
      items.forEach((_, nodeId) => {
        if (!existingNodeIds.has(nodeId)) items.delete(nodeId)
      })
      if (items.size === 0) resultLayoutsBySourceNodeId.delete(sourceNodeId)
    })
  }

  function waitForFlowRenderFrame(): Promise<void> {
    if (typeof window === 'undefined') return nextTick()
    return nextTick().then(
      () =>
        new Promise((resolve) => {
          window.requestAnimationFrame(() => resolve())
        }),
    )
  }

  async function appendPendingImageResultEdge(sourceNodeId: string, resultNodeId: string) {
    await waitForFlowRenderFrame()
    if (cancelledResultNodeIds.has(resultNodeId)) return
    const hasSource = options.nodes.value.some((node) => node.id === sourceNodeId)
    const targetNode = options.nodes.value.find((node) => node.id === resultNodeId)
    const hasTarget = !!targetNode
    if (!hasSource || !hasTarget) return
    const targetSnapshot =
      targetNode.data?.snapshotNode && typeof targetNode.data.snapshotNode === 'object'
        ? (targetNode.data.snapshotNode as ImageNodeItem)
        : null
    options.refreshNodeInternals?.([sourceNodeId, resultNodeId])
    const nextEdge: Edge = {
      id: `edge-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
      source: sourceNodeId,
      sourceHandle: 'out:image',
      target: resultNodeId,
      targetHandle: 'in:image',
      animated: targetSnapshot?.outputStatus === 'running',
    }
    const hasSameEdge = options.edges.value.some(
      (edge) =>
        edge.source === nextEdge.source &&
        edge.target === nextEdge.target &&
        edge.sourceHandle === nextEdge.sourceHandle &&
        edge.targetHandle === nextEdge.targetHandle,
    )
    if (hasSameEdge) return
    options.commitEdgeMutation((items) => [...items, nextEdge], { recordHistory: false })
    options.refreshNodeInternals?.([sourceNodeId, resultNodeId])
  }

  function appendPendingImageResultNode(
    source: ImageDirectActionSource,
    sourceNodeId: string,
    optionsOverride?: { recordHistory?: boolean },
  ): string {
    pruneResultNodeLayouts()
    const position = resolvePendingImageResultPosition(source, sourceNodeId)
    const imageNode = createImageNodeAt(position)
    imageNode.sourceImageUrl = ''
    imageNode.sourceImageRatio = source.ratio > 0 ? source.ratio : 1
    imageNode.width = resolveImageNodeFrameSize(imageNode.sourceImageRatio, true).width
    imageNode.outputStatus = 'running'
    imageNode.outputImages = []

    const nextNode: Node<FlowNodeData> = {
      id: `image-${imageNode.id}`,
      type: 'canvas',
      dragHandle: resolveFlowNodeDragHandle('image'),
      position: { x: imageNode.x, y: imageNode.y },
      data: {
        kind: 'image',
        nodeType: 'image',
        title: `Image #${imageNode.id}`,
        summary: 'Canvas image node',
        snapshotNode: imageNode,
      },
    }

    options.commitNodeMutation((items) => [...items, nextNode as Node], optionsOverride)
    cancelledResultNodeIds.delete(nextNode.id)
    rememberResultNodeLayout(sourceNodeId, nextNode.id, {
      url: '',
      ratio: imageNode.sourceImageRatio,
      x: imageNode.x,
      y: imageNode.y,
      width: imageNode.width,
    })
    options.refreshNodeInternals?.([sourceNodeId, nextNode.id])
    void appendPendingImageResultEdge(sourceNodeId, nextNode.id)

    return nextNode.id
  }

  function appendPendingImageResultNodes(
    source: ImageDirectActionSource,
    sourceNodeId: string,
    outputCount: number,
  ): string[] {
    const count = clampImageNodeOutputCount(outputCount)
    return Array.from({ length: count }, (_, index) =>
      appendPendingImageResultNode(source, sourceNodeId, {
        recordHistory: index === 0,
      }),
    )
  }

  function updatePendingImageResultNode(nodeId: string, url: string, ratio: number) {
    const frameSize = resolveImageNodeFrameSize(ratio, true)
    options.commitGraphMutation(
      (items, currentEdges) => ({
        nodes: items.map((item) => {
          if (item.id !== nodeId || item.data?.kind !== 'image') return item
          const snapshot = item.data.snapshotNode
          if (!snapshot || typeof snapshot !== 'object') return item
          return {
            ...item,
            data: {
              ...item.data,
              snapshotNode: {
                ...(snapshot as ImageNodeItem),
                width: frameSize.width,
                sourceImageUrl: url,
                sourceImageRatio: ratio > 0 ? ratio : 1,
                outputStatus: 'ready',
                outputImages: [],
                pendingTaskId: undefined,
                pendingSourceNodeId: undefined,
                pendingPrompt: undefined,
              },
            },
          }
        }),
        edges: currentEdges.map((edge) =>
          edge.target === nodeId && edge.targetHandle === 'in:image'
            ? { ...edge, animated: false }
            : edge,
        ),
      }),
      { recordHistory: false },
    )
    updateResultNodeLayout(nodeId, { url, ratio: ratio > 0 ? ratio : 1 })
    options.refreshNodeInternals?.([nodeId])
  }

  function updatePendingImageTaskMeta(nodeId: string, patch: PendingTaskMetaPatch) {
    options.commitGraphMutation(
      (items, currentEdges) => ({
        nodes: items.map((item) => {
          if (item.id !== nodeId || item.data?.kind !== 'image') return item
          const snapshot = item.data.snapshotNode
          if (!snapshot || typeof snapshot !== 'object') return item
          return {
            ...item,
            data: {
              ...item.data,
              snapshotNode: {
                ...(snapshot as ImageNodeItem),
                ...patch,
              },
            },
          }
        }),
        edges: currentEdges,
      }),
      { recordHistory: false },
    )
  }

  function removePendingImageResultNode(nodeId: string) {
    cancelledResultNodeIds.add(nodeId)
    forgetResultNodeLayout(nodeId)
    options.commitGraphMutation(
      (items, currentEdges) => ({
        nodes: items.filter((item) => item.id !== nodeId),
        edges: currentEdges.filter((edge) => edge.source !== nodeId && edge.target !== nodeId),
      }),
      { recordHistory: false },
    )
  }

  return {
    appendPendingImageResultNode,
    appendPendingImageResultNodes,
    updatePendingImageResultNode,
    updatePendingImageTaskMeta,
    removePendingImageResultNode,
  }
}
