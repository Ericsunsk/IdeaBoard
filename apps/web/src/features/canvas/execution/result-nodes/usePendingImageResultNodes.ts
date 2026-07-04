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
import type {
  CanvasVueFlowGraphAdapter,
  CanvasVueFlowGraphCommandOptions,
} from '../../composables/useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from '../../composables/flowGraphAdapterCommandSideEffects'

type UsePendingImageResultNodesOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  refreshNodeInternals?: (nodeIds: string[]) => void
}

export type PendingImageResultOptions = ImageExecutionResultOptions

export type PendingImageResultReservationParams = {
  source: ImageExecutionInput
  sourceNodeId: string
  outputCount: number
  resultOptions?: PendingImageResultOptions
}

export type PendingImageResultExecution = {
  runPendingResults: (params: RunPendingImageResultsParams) => Promise<PendingImageResultRunSummary>
  resolveAttachedPendingResult: (
    params: ResolveAttachedPendingImageResultParams,
  ) => Promise<boolean>
}

export type PendingImageResultTaskSubmission = {
  taskId: string
}

export type RunPendingImageResultsParams = PendingImageResultReservationParams & {
  submitTask: (resultNodeId: string) => Promise<PendingImageResultTaskSubmission>
  resolveResult: (taskId: string, resultNodeId: string) => Promise<ResolvedTaskImageResult>
  isCurrent: () => boolean
  isPendingError: (error: unknown) => boolean
  persistPendingTaskIds?: boolean
  afterTaskIdAttached?: (params: { resultNodeId: string; taskId: string }) => Promise<void>
  onReserved?: (resultNodeIds: string[]) => void
}

export type ResolveAttachedPendingImageResultParams = {
  resultNodeId: string
  taskId: string
  resolveResult: (taskId: string, resultNodeId: string) => Promise<ResolvedTaskImageResult>
  isCurrent: () => boolean
  isAttached: () => boolean
}

export type PendingImageResultRunSummary = {
  resultNodeIds: string[]
  totalCount: number
  successCount: number
  pendingCount: number
  failureCount: number
  firstFailure: unknown
}

type ImageResultLayout = ImageResultAnchor & {
  ratio: number
}

type PendingImageResultLookup = {
  nodeById: Map<string, Node>
  nodeOrderById: Map<string, number>
  imageChildIdsBySource: Map<string, string[]>
}

type PendingImageResultMutationOptions = {
  recordHistory?: boolean
}

type PendingImageResultPlacementOptions = PendingImageResultMutationOptions & {
  anchor?: ImageResultAnchor
  targetRatio?: number
  targetWidth?: number
}

const IMAGE_RESULT_NODE_X_GAP = 100
const IMAGE_RESULT_NODE_Y_GAP = 14
const IMAGE_NODE_HEADER_HEIGHT = 26

function createPendingResultCommandOptions(
  sideEffects?: PendingImageResultMutationOptions,
): CanvasVueFlowGraphCommandOptions {
  return createFlowGraphCommandOptions({
    recordHistory: sideEffects?.recordHistory,
  })
}

function estimateImageNodeHeight(source: Pick<ImageResultLayout, 'width' | 'ratio'>): number {
  const stageHeight = resolveImageNodeStageHeight(source.width, source.ratio, true)
  return IMAGE_NODE_HEADER_HEIGHT + stageHeight
}

export function usePendingImageResultExecution(
  options: UsePendingImageResultNodesOptions,
): PendingImageResultExecution {
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
    options.graphAdapter.addEdges(
      nextEdge,
      createPendingResultCommandOptions({ recordHistory: false }),
    )
    options.refreshNodeInternals?.([anchorNodeId, resultNodeId])
  }

  function appendPendingImageResultNodeWithLookup(
    source: ImageExecutionInput,
    anchorNodeId: string,
    lookup: PendingImageResultLookup,
    optionsOverride?: PendingImageResultPlacementOptions,
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
      options.graphAdapter.replaceNodes((items) => {
        let didUpdate = false
        const nextItems = items.map((item) => {
          if (item.id !== preferredNodeId || item.data?.kind !== 'image') return item
          const snapshot = item.data.snapshotNode
          if (!snapshot || typeof snapshot !== 'object') return item
          didUpdate = true
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
        })
        return didUpdate ? nextItems : items
      }, createPendingResultCommandOptions(optionsOverride))
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

    options.graphAdapter.addNodes(
      nextNode as Node,
      createPendingResultCommandOptions(optionsOverride),
    )
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

  function reservePendingResults(params: PendingImageResultReservationParams): string[] {
    const count = clampImageNodeOutputCount(params.outputCount)
    const resultOptions = params.resultOptions ?? {}
    const anchorNodeId = resultOptions.anchor?.outputAnchorNodeId ?? params.sourceNodeId
    const lookup = buildPendingResultLookup()
    const reusableNodeId = findReusableOutputImageNodeId(anchorNodeId, lookup)
    return Array.from({ length: count }, (_, index) =>
      appendPendingImageResultNodeWithLookup(
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
    )
  }

  function attachPendingTaskId(params: { resultNodeId: string; taskId: string }) {
    const normalizedTaskId = params.taskId.trim()
    if (!normalizedTaskId) return
    const targetNode = options.nodes.value.find((item) => item.id === params.resultNodeId)
    if (targetNode?.data?.kind !== 'image') return
    const snapshot = targetNode.data.snapshotNode
    if (!snapshot || typeof snapshot !== 'object') return
    options.graphAdapter.updateNode(
      params.resultNodeId,
      {
        data: {
          ...targetNode.data,
          snapshotNode: {
            ...(snapshot as ImageNodeItem),
            pendingTaskId: normalizedTaskId,
          },
        },
      },
      createPendingResultCommandOptions({ recordHistory: false }),
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
    let didUpdateNode = false
    let didUpdateEdge = false
    const nextNodes = options.nodes.value.map((item) => {
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
      didUpdateNode = true
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
    })
    const nextEdges = options.edges.value.map((edge) => {
      if (edge.target !== resultNodeId || edge.targetHandle !== 'in:image') return edge
      if (edge.animated === false) return edge
      didUpdateEdge = true
      return { ...edge, animated: false }
    })
    if (!didUpdateNode && !didUpdateEdge) return
    options.graphAdapter.replaceGraph(
      {
        nodes: didUpdateNode ? nextNodes : options.nodes.value,
        edges: didUpdateEdge ? nextEdges : options.edges.value,
      },
      createPendingResultCommandOptions({ recordHistory: false }),
    )
    if (didUpdateNode) {
      updateResultNodeLayout(resultNodeId, {
        ratio: normalizedRatio,
        ...(resolvedFrameWidth ? { width: resolvedFrameWidth } : {}),
      })
      options.refreshNodeInternals?.([resultNodeId])
    }
  }

  function cancelPendingResult(params: { resultNodeId: string }) {
    cancelledResultNodeIds.add(params.resultNodeId)
    forgetResultNodeLayout(params.resultNodeId)
    options.graphAdapter.removeNodes(params.resultNodeId, {
      ...createPendingResultCommandOptions({ recordHistory: false }),
      removeConnectedEdges: true,
    })
  }

  async function runPendingResults(
    params: RunPendingImageResultsParams,
  ): Promise<PendingImageResultRunSummary> {
    const persistPendingTaskIds = params.persistPendingTaskIds ?? true
    const resultNodeIds = reservePendingResults(params)
    params.onReserved?.(resultNodeIds)

    let successCount = 0
    let pendingCount = 0
    let failureCount = 0
    let firstFailure: unknown = null

    await Promise.allSettled(
      resultNodeIds.map(async (resultNodeId) => {
        try {
          const { taskId } = await params.submitTask(resultNodeId)
          if (persistPendingTaskIds && params.isCurrent()) {
            attachPendingTaskId({ resultNodeId, taskId })
            await params.afterTaskIdAttached?.({ resultNodeId, taskId })
          }
          const resolved = await params.resolveResult(taskId, resultNodeId)
          if (params.isCurrent()) {
            materializeResolvedImage({ resultNodeId, result: resolved })
          }
          successCount += 1
        } catch (error) {
          firstFailure ??= error
          if (params.isPendingError(error)) {
            pendingCount += 1
            if (!persistPendingTaskIds && params.isCurrent()) {
              cancelPendingResult({ resultNodeId })
            }
            return
          }
          failureCount += 1
          if (params.isCurrent()) {
            cancelPendingResult({ resultNodeId })
          }
        }
      }),
    )

    return {
      resultNodeIds,
      totalCount: resultNodeIds.length,
      successCount,
      pendingCount,
      failureCount,
      firstFailure,
    }
  }

  async function resolveAttachedPendingResult(
    params: ResolveAttachedPendingImageResultParams,
  ): Promise<boolean> {
    const resolved = await params.resolveResult(params.taskId, params.resultNodeId)
    if (!params.isCurrent() || !params.isAttached()) return false
    materializeResolvedImage({
      resultNodeId: params.resultNodeId,
      result: resolved,
    })
    return true
  }

  return {
    runPendingResults,
    resolveAttachedPendingResult,
  }
}
