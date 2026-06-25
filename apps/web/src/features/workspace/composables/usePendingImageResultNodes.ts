import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { createCanvasNodeAt } from '../utils/canvasNodeFactory'
import { clampImageNodeOutputCount, type ImageNodeItem } from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'
import { resolveFlowNodeDragHandle } from '../utils/flowNodeDragHandle'
import { resolveImageNodeFrameSize, resolveImageNodeStageHeight } from '../utils/imageNodeSizing'
import {
  resolveImageDirectActionSource,
  type ImageDirectActionSource,
} from '../utils/flowImageDirectActionSource'
import type { ResolvedTaskImageResult } from '../utils/flowImageDirectActionTaskResult'
import { waitForNextRenderFrame } from '../utils/animationFrame'
import type { FlowTransactions } from './useFlowTransactions'

type UsePendingImageResultNodesOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  commitGraphMutation: FlowTransactions['commitGraphMutation']
  commitNodeMutation: FlowTransactions['commitNodeMutation']
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  refreshNodeInternals?: (nodeIds: string[]) => void
}

type PendingImageResultOptions = {
  outputAnchorNodeId?: string
  targetRatio?: number
  targetWidth?: number
}

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

  function readOutputAnchorLayout(nodeId: string, fallback: ImageDirectActionSource) {
    const node = options.nodes.value.find((item) => item.id === nodeId)
    if (!node) return fallback
    if (node.data?.kind === 'image') {
      return readImageNodeLayout(node) ?? fallback
    }
    const snapshot = node.data?.snapshotNode as { width?: unknown } | undefined
    const width = Number(snapshot?.width)
    return {
      url: '',
      ratio: 1,
      x: Number(node.position?.x ?? 0),
      y: Number(node.position?.y ?? 0),
      width: Number.isFinite(width) && width > 0 ? width : fallback.width,
    }
  }

  function isEmptyReusableImageNode(node: Node): boolean {
    if (node.data?.kind !== 'image') return false
    const snapshot = node.data.snapshotNode as ImageNodeItem | undefined
    if (!snapshot || typeof snapshot !== 'object') return false
    const sourceUrl = `${snapshot.sourceImageUrl ?? ''}`.trim()
    const outputImages = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
    return !sourceUrl && outputImages.length === 0 && snapshot.outputStatus !== 'running'
  }

  function findReusableOutputImageNodeId(anchorNodeId: string): string | null {
    const candidateIds = options.edges.value
      .filter(
        (edge) =>
          edge.source === anchorNodeId &&
          edge.sourceHandle === 'out:image' &&
          edge.targetHandle === 'in:image',
      )
      .map((edge) => edge.target)
    for (const candidateId of candidateIds) {
      const node = options.nodes.value.find((item) => item.id === candidateId)
      if (node && isEmptyReusableImageNode(node)) return candidateId
    }
    return null
  }

  function resolvePendingImageResultPosition(
    anchor: ImageDirectActionSource,
    anchorNodeId: string,
  ) {
    const defaultPosition = {
      x: anchor.x + anchor.width + IMAGE_RESULT_NODE_X_GAP,
      y: anchor.y,
    }
    const childNodeIds = new Set<string>(
      options.edges.value
        .filter(
          (edge) =>
            edge.source === anchorNodeId &&
            edge.sourceHandle === 'out:image' &&
            edge.targetHandle === 'in:image',
        )
        .map((edge) => edge.target),
    )
    const runtimeLayouts = resultLayoutsBySourceNodeId.get(anchorNodeId)
    runtimeLayouts?.forEach((_, nodeId) => childNodeIds.add(nodeId))
    if (childNodeIds.size === 0) return defaultPosition

    let nextY = anchor.y
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
    anchorNodeId: string,
    resultNodeId: string,
    layout: ImageDirectActionSource,
  ) {
    const current = resultLayoutsBySourceNodeId.get(anchorNodeId) ?? new Map()
    current.set(resultNodeId, layout)
    resultLayoutsBySourceNodeId.set(anchorNodeId, current)
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

  function buildPendingImageNodeSnapshot(
    base: ImageNodeItem,
    params: { sourceImageRatio: number; width: number },
  ): ImageNodeItem {
    return {
      ...base,
      sourceImageUrl: '',
      sourceImageRatio: params.sourceImageRatio,
      width: params.width,
      pendingTaskId: undefined,
      outputStatus: 'running',
      outputImages: [],
    }
  }

  async function appendPendingImageResultEdge(anchorNodeId: string, resultNodeId: string) {
    await waitForNextRenderFrame()
    if (cancelledResultNodeIds.has(resultNodeId)) return
    const hasSource = options.nodes.value.some((node) => node.id === anchorNodeId)
    const targetNode = options.nodes.value.find((node) => node.id === resultNodeId)
    const hasTarget = !!targetNode
    if (!hasSource || !hasTarget) return
    const targetSnapshot =
      targetNode.data?.snapshotNode && typeof targetNode.data.snapshotNode === 'object'
        ? (targetNode.data.snapshotNode as ImageNodeItem)
        : null
    options.refreshNodeInternals?.([anchorNodeId, resultNodeId])
    const nextEdge: Edge = {
      id: `edge-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
      source: anchorNodeId,
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
    options.refreshNodeInternals?.([anchorNodeId, resultNodeId])
  }

  function appendPendingImageResultNode(
    source: ImageDirectActionSource,
    anchorNodeId: string,
    optionsOverride?: { recordHistory?: boolean; targetRatio?: number; targetWidth?: number },
    preferredNodeId?: string | null,
  ): string {
    pruneResultNodeLayouts()
    const anchor = readOutputAnchorLayout(anchorNodeId, source)
    const position = resolvePendingImageResultPosition(anchor, anchorNodeId)
    const imageNode = createCanvasNodeAt('image', position)
    const resolvedRatio =
      Number.isFinite(optionsOverride?.targetRatio) && (optionsOverride?.targetRatio as number) > 0
        ? (optionsOverride?.targetRatio as number)
        : source.ratio > 0
          ? source.ratio
          : 1
    const resolvedWidth =
      Number.isFinite(optionsOverride?.targetWidth) && (optionsOverride?.targetWidth as number) > 0
        ? Math.max(1, Math.round(optionsOverride?.targetWidth as number))
        : resolveImageNodeFrameSize(resolvedRatio, true).width
    const pendingImageNode = buildPendingImageNodeSnapshot(imageNode, {
      sourceImageRatio: resolvedRatio,
      width: resolvedWidth,
    })

    if (preferredNodeId) {
      options.commitNodeMutation(
        (items) =>
          items.map((item) => {
            if (item.id !== preferredNodeId || item.data?.kind !== 'image') return item
            const snapshot = item.data.snapshotNode
            if (!snapshot || typeof snapshot !== 'object') return item
            return {
              ...item,
              data: {
                ...item.data,
                snapshotNode: buildPendingImageNodeSnapshot(snapshot as ImageNodeItem, {
                  sourceImageRatio: pendingImageNode.sourceImageRatio,
                  width: pendingImageNode.width,
                }),
              },
            }
          }),
        optionsOverride,
      )
      cancelledResultNodeIds.delete(preferredNodeId)
      rememberResultNodeLayout(anchorNodeId, preferredNodeId, {
        url: '',
        ratio: pendingImageNode.sourceImageRatio,
        x: position.x,
        y: position.y,
        width: pendingImageNode.width,
      })
      options.refreshNodeInternals?.([anchorNodeId, preferredNodeId])
      return preferredNodeId
    }

    const nextNode: Node<FlowNodeData> = {
      id: `image-${pendingImageNode.id}`,
      type: 'canvas',
      dragHandle: resolveFlowNodeDragHandle('image'),
      position: { x: pendingImageNode.x, y: pendingImageNode.y },
      data: {
        kind: 'image',
        nodeType: 'image',
        title: `Image #${pendingImageNode.id}`,
        summary: 'Canvas image node',
        snapshotNode: pendingImageNode,
      },
    }

    options.commitNodeMutation((items) => [...items, nextNode as Node], optionsOverride)
    cancelledResultNodeIds.delete(nextNode.id)
    rememberResultNodeLayout(anchorNodeId, nextNode.id, {
      url: '',
      ratio: pendingImageNode.sourceImageRatio,
      x: pendingImageNode.x,
      y: pendingImageNode.y,
      width: pendingImageNode.width,
    })
    options.refreshNodeInternals?.([anchorNodeId, nextNode.id])
    void appendPendingImageResultEdge(anchorNodeId, nextNode.id)

    return nextNode.id
  }

  function appendPendingImageResultNodes(
    source: ImageDirectActionSource,
    sourceNodeId: string,
    outputCount: number,
    resultOptions: PendingImageResultOptions = {},
  ): string[] {
    const count = clampImageNodeOutputCount(outputCount)
    const anchorNodeId = resultOptions.outputAnchorNodeId ?? sourceNodeId
    const reusableNodeId = findReusableOutputImageNodeId(anchorNodeId)
    return Array.from({ length: count }, (_, index) =>
      appendPendingImageResultNode(
        source,
        anchorNodeId,
        {
          recordHistory: index === 0,
          targetRatio: resultOptions.targetRatio,
          targetWidth: resultOptions.targetWidth,
        },
        index === 0 ? reusableNodeId : null,
      ),
    )
  }

  function markPendingImageResultTask(nodeId: string, taskId: string) {
    const normalizedTaskId = taskId.trim()
    if (!normalizedTaskId) return
    options.commitNodeMutation(
      (items) => {
        let didUpdate = false
        const nextItems = items.map((item) => {
          if (item.id !== nodeId || item.data?.kind !== 'image') return item
          const snapshot = item.data.snapshotNode
          if (!snapshot || typeof snapshot !== 'object') return item
          didUpdate = true
          return {
            ...item,
            data: {
              ...item.data,
              snapshotNode: {
                ...(snapshot as ImageNodeItem),
                pendingTaskId: normalizedTaskId,
              },
            },
          }
        })
        return didUpdate ? nextItems : items
      },
      { recordHistory: false },
    )
  }

  function updatePendingImageResultNode(nodeId: string, result: ResolvedTaskImageResult) {
    const resolvedImageWidth = Number(result.width)
    const resolvedImageHeight = Number(result.height)
    const hasResolvedSize =
      Number.isFinite(resolvedImageWidth) &&
      resolvedImageWidth > 0 &&
      Number.isFinite(resolvedImageHeight) &&
      resolvedImageHeight > 0
    const normalizedRatio = hasResolvedSize
      ? resolvedImageHeight / resolvedImageWidth
      : result.ratio > 0
        ? result.ratio
        : 1
    const resolvedFrameWidth = hasResolvedSize
      ? resolveImageNodeFrameSize(normalizedRatio, true).width
      : null
    options.commitGraphMutation(
      (items, currentEdges) => ({
        nodes: items.map((item) => {
          if (item.id !== nodeId || item.data?.kind !== 'image') return item
          const snapshot = item.data.snapshotNode
          if (!snapshot || typeof snapshot !== 'object') return item
          const currentSnapshot = snapshot as ImageNodeItem
          const currentWidth = Number(currentSnapshot.width)
          const width =
            resolvedFrameWidth ??
            (Number.isFinite(currentWidth) && currentWidth > 0
              ? Math.max(1, Math.round(currentWidth))
              : resolveImageNodeFrameSize(normalizedRatio, true).width)
          return {
            ...item,
            data: {
              ...item.data,
              snapshotNode: {
                ...currentSnapshot,
                width,
                sourceImageUrl: result.url,
                sourceImageRatio: normalizedRatio,
                pendingTaskId: undefined,
                outputStatus: 'ready',
                outputImages: [],
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
    updateResultNodeLayout(nodeId, {
      url: result.url,
      ratio: normalizedRatio,
      ...(resolvedFrameWidth ? { width: resolvedFrameWidth } : {}),
    })
    options.refreshNodeInternals?.([nodeId])
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
    markPendingImageResultTask,
    updatePendingImageResultNode,
    removePendingImageResultNode,
  }
}
