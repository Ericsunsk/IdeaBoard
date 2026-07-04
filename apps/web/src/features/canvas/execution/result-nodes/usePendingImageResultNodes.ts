import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { createCanvasNodeAt } from '../../utils/canvasNodeFactory'
import { clampImageNodeOutputCount, type ImageNodeItem } from '../../../canvas-board/domain'
import type { FlowNodeData } from '../../types/flowTypes'
import { resolveFlowNodeDragHandle } from '../../utils/flowNodeDragHandle'
import {
  resolveImageNodeFrameSize,
  resolveImageNodeStageHeight,
} from '../../../canvas-board/domain'
import type {
  ImageExecutionInput,
  ImageExecutionResultOptions,
  ImageResultAnchor,
} from '../planning/ImageExecutionPlan'
import { resolveImageNodeLayoutAnchor } from '../../utils/flowImageNodeSelectors'
import type { ResolvedTaskImageResult } from './flowImageDirectActionTaskResult'
import { waitForNextRenderFrame } from '../../utils/animationFrame'
import type { FlowTransactions } from '../../composables/useFlowTransactions'

type UsePendingImageResultNodesOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  commitGraphMutation: FlowTransactions['commitGraphMutation']
  commitNodeMutation: FlowTransactions['commitNodeMutation']
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  refreshNodeInternals?: (nodeIds: string[]) => void
}

export type PendingImageResultOptions = ImageExecutionResultOptions

export type PendingImageResultReservation = {
  resultNodeId: string
}

export type ReservePendingImageResultParams = {
  source: ImageExecutionInput
  sourceNodeId: string
  resultOptions?: PendingImageResultOptions
}

export type ReservePendingImageResultsParams = ReservePendingImageResultParams & {
  outputCount: number
}

export type PendingImageResultLifecycle = {
  reservePendingResult: (
    params: ReservePendingImageResultParams,
  ) => PendingImageResultReservation
  reservePendingResults: (
    params: ReservePendingImageResultsParams,
  ) => PendingImageResultReservation[]
  attachPendingTaskId: (params: { resultNodeId: string; taskId: string }) => void
  materializeResolvedImage: (params: {
    resultNodeId: string
    result: ResolvedTaskImageResult
  }) => void
  cancelPendingResult: (params: { resultNodeId: string }) => void
}

type ImageResultLayout = ImageResultAnchor & {
  ratio: number
}

type PendingImageResultLookup = {
  nodeById: Map<string, Node>
  nodeOrderById: Map<string, number>
  imageChildIdsBySource: Map<string, string[]>
}

const IMAGE_RESULT_NODE_X_GAP = 100
const IMAGE_RESULT_NODE_Y_GAP = 14
const IMAGE_NODE_HEADER_HEIGHT = 26

function estimateImageNodeHeight(source: Pick<ImageResultLayout, 'width' | 'ratio'>): number {
  const stageHeight = resolveImageNodeStageHeight(source.width, source.ratio, true)
  return IMAGE_NODE_HEADER_HEIGHT + stageHeight
}

export function usePendingImageResultNodeLifecycle(
  options: UsePendingImageResultNodesOptions,
): PendingImageResultLifecycle {
  const cancelledResultNodeIds = new Set<string>()
  const resultLayoutsBySourceNodeId = new Map<string, Map<string, ImageResultLayout>>()

  function buildPendingResultLookup(): PendingImageResultLookup {
    const nodeById = new Map<string, Node>()
    const nodeOrderById = new Map<string, number>()
    const imageChildIdsBySource = new Map<string, string[]>()

    options.nodes.value.forEach((node, index) => {
      if (nodeById.has(node.id)) return
      nodeById.set(node.id, node)
      nodeOrderById.set(node.id, index)
    })

    for (const edge of options.edges.value) {
      if (edge.sourceHandle !== 'out:image' || edge.targetHandle !== 'in:image') continue
      const current = imageChildIdsBySource.get(edge.source)
      if (current) {
        current.push(edge.target)
      } else {
        imageChildIdsBySource.set(edge.source, [edge.target])
      }
    }

    return {
      nodeById,
      nodeOrderById,
      imageChildIdsBySource,
    }
  }

  function readImageNodeLayout(node: Node): ImageResultLayout | null {
    if (node.data?.kind !== 'image') return null
    const snapshot = node.data.snapshotNode
    if (!snapshot || typeof snapshot !== 'object') return null
    const anchor = resolveImageNodeLayoutAnchor(node, snapshot as ImageNodeItem)
    return anchor
      ? {
          x: anchor.x,
          y: anchor.y,
          width: anchor.width,
          ratio: anchor.ratio,
        }
      : null
  }

  function readOutputAnchorLayout(
    nodeId: string,
    fallback: ImageResultLayout,
    lookup: PendingImageResultLookup,
    anchorHint?: ImageResultAnchor,
  ): ImageResultLayout {
    if (anchorHint) {
      return {
        ...anchorHint,
        ratio: fallback.ratio,
      }
    }
    const node = lookup.nodeById.get(nodeId)
    if (!node) return fallback
    if (node.data?.kind === 'image') {
      return readImageNodeLayout(node) ?? fallback
    }
    const snapshot = node.data?.snapshotNode as { width?: unknown } | undefined
    const width = Number(snapshot?.width)
    return {
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

  function findReusableOutputImageNodeId(
    anchorNodeId: string,
    lookup: PendingImageResultLookup,
  ): string | null {
    const candidateIds = lookup.imageChildIdsBySource.get(anchorNodeId) ?? []
    for (const candidateId of candidateIds) {
      const node = lookup.nodeById.get(candidateId)
      if (node && isEmptyReusableImageNode(node)) return candidateId
    }
    return null
  }

  function resolvePendingImageResultPosition(
    anchor: ImageResultLayout,
    anchorNodeId: string,
    lookup: PendingImageResultLookup,
  ) {
    const defaultPosition = {
      x: anchor.x + anchor.width + IMAGE_RESULT_NODE_X_GAP,
      y: anchor.y,
    }
    const childNodeIds = new Set<string>(lookup.imageChildIdsBySource.get(anchorNodeId) ?? [])
    const runtimeLayouts = resultLayoutsBySourceNodeId.get(anchorNodeId)
    runtimeLayouts?.forEach((_, nodeId) => childNodeIds.add(nodeId))
    if (childNodeIds.size === 0) return defaultPosition

    let nextY = anchor.y
    let alignedX = defaultPosition.x
    const measuredNodeIds = new Set<string>()
    const visitChildLayout = (nodeId: string, layout: ImageResultLayout) => {
      const bottomY = layout.y + estimateImageNodeHeight(layout) + IMAGE_RESULT_NODE_Y_GAP
      if (bottomY >= nextY) {
        alignedX = layout.x
        nextY = bottomY
      }
      measuredNodeIds.add(nodeId)
    }

    const existingChildNodeIds = Array.from(childNodeIds)
      .filter((nodeId) => lookup.nodeById.has(nodeId))
      .sort(
        (left, right) =>
          (lookup.nodeOrderById.get(left) ?? Number.MAX_SAFE_INTEGER) -
          (lookup.nodeOrderById.get(right) ?? Number.MAX_SAFE_INTEGER),
      )

    for (const nodeId of existingChildNodeIds) {
      const node = lookup.nodeById.get(nodeId)
      if (!node) continue
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
    layout: ImageResultLayout,
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

  function updateResultNodeLayout(resultNodeId: string, patch: Partial<ImageResultLayout>) {
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

  function appendPendingImageResultNodeWithLookup(
    source: ImageExecutionInput,
    anchorNodeId: string,
    lookup: PendingImageResultLookup,
    optionsOverride?: {
      recordHistory?: boolean
      anchor?: ImageResultAnchor
      targetRatio?: number
      targetWidth?: number
    },
    preferredNodeId?: string | null,
  ): string {
    pruneResultNodeLayouts()
    const sourceRatio = source.ratio > 0 ? source.ratio : 1
    const fallbackAnchor: ImageResultLayout = {
      x: 0,
      y: 0,
      width: resolveImageNodeFrameSize(sourceRatio, true).width,
      ratio: sourceRatio,
    }
    const anchor = readOutputAnchorLayout(
      anchorNodeId,
      fallbackAnchor,
      lookup,
      optionsOverride?.anchor,
    )
    const position = resolvePendingImageResultPosition(anchor, anchorNodeId, lookup)
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
      let updatedNode: Node | null = null
      options.commitNodeMutation(
        (items) =>
          items.map((item) => {
            if (item.id !== preferredNodeId || item.data?.kind !== 'image') return item
            const snapshot = item.data.snapshotNode
            if (!snapshot || typeof snapshot !== 'object') return item
            updatedNode = {
              ...item,
              data: {
                ...item.data,
                snapshotNode: buildPendingImageNodeSnapshot(snapshot as ImageNodeItem, {
                  sourceImageRatio: pendingImageNode.sourceImageRatio,
                  width: pendingImageNode.width,
                }),
              },
            }
            return updatedNode
          }),
        optionsOverride,
      )
      if (updatedNode) lookup.nodeById.set(preferredNodeId, updatedNode)
      cancelledResultNodeIds.delete(preferredNodeId)
      rememberResultNodeLayout(anchorNodeId, preferredNodeId, {
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
    lookup.nodeById.set(nextNode.id, nextNode as Node)
    lookup.nodeOrderById.set(nextNode.id, lookup.nodeOrderById.size)
    cancelledResultNodeIds.delete(nextNode.id)
    rememberResultNodeLayout(anchorNodeId, nextNode.id, {
      ratio: pendingImageNode.sourceImageRatio,
      x: pendingImageNode.x,
      y: pendingImageNode.y,
      width: pendingImageNode.width,
    })
    options.refreshNodeInternals?.([anchorNodeId, nextNode.id])
    void appendPendingImageResultEdge(anchorNodeId, nextNode.id)

    return nextNode.id
  }

  function reservePendingResult(
    params: ReservePendingImageResultParams,
  ): PendingImageResultReservation {
    return {
      resultNodeId: appendPendingImageResultNodeWithLookup(
        params.source,
        params.sourceNodeId,
        buildPendingResultLookup(),
        {
          recordHistory: true,
          anchor: params.resultOptions?.anchor,
          targetRatio: params.resultOptions?.targetRatio,
          targetWidth: params.resultOptions?.targetWidth,
        },
        null,
      ),
    }
  }

  function reservePendingResults(
    params: ReservePendingImageResultsParams,
  ): PendingImageResultReservation[] {
    const count = clampImageNodeOutputCount(params.outputCount)
    const resultOptions = params.resultOptions ?? {}
    const anchorNodeId = resultOptions.anchor?.outputAnchorNodeId ?? params.sourceNodeId
    const lookup = buildPendingResultLookup()
    const reusableNodeId = findReusableOutputImageNodeId(anchorNodeId, lookup)
    return Array.from({ length: count }, (_, index) => ({
      resultNodeId: appendPendingImageResultNodeWithLookup(
        params.source,
        anchorNodeId,
        lookup,
        {
          recordHistory: index === 0,
          anchor: resultOptions.anchor,
          targetRatio: resultOptions.targetRatio,
          targetWidth: resultOptions.targetWidth,
        },
        index === 0 ? reusableNodeId : null,
      ),
    }))
  }

  function attachPendingTaskId(params: { resultNodeId: string; taskId: string }) {
    const normalizedTaskId = params.taskId.trim()
    if (!normalizedTaskId) return
    options.commitNodeMutation(
      (items) => {
        let didUpdate = false
        const nextItems = items.map((item) => {
          if (item.id !== params.resultNodeId || item.data?.kind !== 'image') return item
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

  function materializeResolvedImage(params: {
    resultNodeId: string
    result: ResolvedTaskImageResult
  }) {
    const { resultNodeId, result } = params
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
          if (item.id !== resultNodeId || item.data?.kind !== 'image') return item
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
          edge.target === resultNodeId && edge.targetHandle === 'in:image'
            ? { ...edge, animated: false }
            : edge,
        ),
      }),
      { recordHistory: false },
    )
    updateResultNodeLayout(resultNodeId, {
      ratio: normalizedRatio,
      ...(resolvedFrameWidth ? { width: resolvedFrameWidth } : {}),
    })
    options.refreshNodeInternals?.([resultNodeId])
  }

  function cancelPendingResult(params: { resultNodeId: string }) {
    cancelledResultNodeIds.add(params.resultNodeId)
    forgetResultNodeLayout(params.resultNodeId)
    options.commitGraphMutation(
      (items, currentEdges) => ({
        nodes: items.filter((item) => item.id !== params.resultNodeId),
        edges: currentEdges.filter(
          (edge) => edge.source !== params.resultNodeId && edge.target !== params.resultNodeId,
        ),
      }),
      { recordHistory: false },
    )
  }

  return {
    reservePendingResult,
    reservePendingResults,
    attachPendingTaskId,
    materializeResolvedImage,
    cancelPendingResult,
  }
}

export function usePendingImageResultNodes(options: UsePendingImageResultNodesOptions) {
  const lifecycle = usePendingImageResultNodeLifecycle(options)

  return {
    appendPendingImageResultNode: (
      source: ImageExecutionInput,
      sourceNodeId: string,
      resultOptions?: PendingImageResultOptions,
    ) =>
      lifecycle.reservePendingResult({ source, sourceNodeId, resultOptions }).resultNodeId,
    appendPendingImageResultNodes: (
      source: ImageExecutionInput,
      sourceNodeId: string,
      outputCount: number,
      resultOptions?: PendingImageResultOptions,
    ) =>
      lifecycle
        .reservePendingResults({ source, sourceNodeId, outputCount, resultOptions })
        .map((reservation) => reservation.resultNodeId),
    markPendingImageResultTask: (resultNodeId: string, taskId: string) =>
      lifecycle.attachPendingTaskId({ resultNodeId, taskId }),
    updatePendingImageResultNode: (resultNodeId: string, result: ResolvedTaskImageResult) =>
      lifecycle.materializeResolvedImage({ resultNodeId, result }),
    removePendingImageResultNode: (resultNodeId: string) =>
      lifecycle.cancelPendingResult({ resultNodeId }),
  }
}
