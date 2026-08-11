import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { createCanvasNodeAt } from '../../utils/canvasNodeFactory'
import { clampImageNodeOutputCount, type ImageNodeItem } from '../../../canvas-board/domain'
import type { FlowNodeData } from '../../types/flowTypes'
import { resolveFlowNodeDragHandle } from '../../utils/flowNodeDragHandle'
import {
  resolveImageNodeFrameSize,
  resolveImageNodeStageHeight,
  type CanvasBoardSnapshot,
} from '../../../canvas-board/domain'
import type {
  ImageExecutionInput,
  ImageExecutionResultOptions,
  ImageResultAnchor,
} from '../planning/imageExecutionResultTypes'
import { resolveImageNodeLayoutAnchor } from '../../utils/flowImageNodeSelectors'
import type { ResolvedTaskImageResult } from './resolvedTaskImageResult'
import { waitForNextRenderFrame } from '../../utils/animationFrame'
import type {
  CanvasVueFlowGraphAdapter,
  CanvasVueFlowGraphCommandOptions,
} from '../../composables/useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from '../../composables/flowGraphAdapterCommandSideEffects'
import {
  canvasBoardSnapshotToFlow,
  type FlowToCanvasBoardSnapshotOptions,
} from '../../utils/flowSnapshotAdapter'
import { type CanvasGraphNodeId, type CanvasGraphStore } from '../../graph-runtime'
import {
  materializeResolvedImageResults,
  type ResultMaterializerImageResult,
  type ResultMaterializerResult,
} from './resultMaterializer'
import { TaskCancelledError } from '../../../../shared/api/taskEvents'

type UsePendingImageResultNodesOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  createCurrentCanvasGraphStore: (
    snapshotOptions?: FlowToCanvasBoardSnapshotOptions,
  ) => CanvasGraphStore
  boardBaseSnapshot?: Ref<CanvasBoardSnapshot>
  refreshNodeInternals?: (nodeIds: string[]) => void
  isNodeExecutionActive: (nodeId: string) => boolean
}

export type PendingImageResultOptions = ImageExecutionResultOptions

export type PendingImageResultReservationParams = {
  source: ImageExecutionInput
  sourceNodeId: string
  outputCount: number
  resultOptions?: PendingImageResultOptions
}

export type PendingImageResultExecution = {
  reservePendingResults: (
    params: PendingImageResultReservationParams,
  ) => PendingImageResultReservation
  runReservedPendingResults: (
    params: RunReservedPendingImageResultsParams,
  ) => Promise<PendingImageResultRunSummary>
  cancelPendingResults: (resultNodeIds: readonly string[]) => void
  materializeResolvedAttachedPendingResults: (
    params: MaterializeResolvedAttachedPendingImageResultsParams,
  ) => ResultMaterializerResult
}

export type PendingImageResultTaskSubmission = {
  taskId: string
}

export type PendingImageResultReservation = {
  resultNodeIds: string[]
}

export type RunReservedPendingImageResultsParams = {
  resultNodeIds: readonly string[]
  submitTask: (resultNodeId: string) => Promise<PendingImageResultTaskSubmission>
  resolveResult: (taskId: string, resultNodeId: string) => Promise<ResolvedTaskImageResult>
  isCurrent: () => boolean
  isPendingError: (error: unknown) => boolean
  persistPendingTaskIds?: boolean
  afterTaskIdAttached?: (params: { resultNodeId: string; taskId: string }) => Promise<void>
}

export type MaterializeResolvedAttachedPendingImageResultsParams = {
  results: ResultMaterializerImageResult[]
  isAttached: (result: ResultMaterializerImageResult) => boolean
}

export type PendingImageResultRunSummary = {
  resultNodeIds: string[]
  totalCount: number
  successCount: number
  pendingCount: number
  failureCount: number
  firstFailure: unknown
  failures: unknown[]
  outcomes: PendingImageResultOutcome[]
}

export type PendingImageResultOutcome = {
  resultNodeId: string
  taskId?: string
  status: 'pending' | 'succeeded' | 'failed' | 'cancelled'
}

type ImageResultLayout = ImageResultAnchor & {
  ratio: number
}

type PendingImageResultLookup = {
  nodeById: Map<string, Node>
  imageChildIdsBySource: Map<string, string[]>
}

type PendingImageResultMutationOptions = {
  recordHistory?: boolean
}

type PendingImageResultPlacementOptions = {
  anchor?: ImageResultAnchor
  targetRatio?: number
  targetWidth?: number
}

type PendingImageResultReservationPlan = {
  resultNodeId: string
  nextNode: Node
  edgeToAppend?: Edge
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

function isCanvasGraphNodeId(nodeId: string): nodeId is CanvasGraphNodeId {
  return /^(?:image|prompt|fusion)-\d+(?:\.\d+)?$/.test(nodeId)
}

function estimateImageNodeHeight(source: Pick<ImageResultLayout, 'width' | 'ratio'>): number {
  const stageHeight = resolveImageNodeStageHeight(source.width, source.ratio, true)
  return IMAGE_NODE_HEADER_HEIGHT + stageHeight
}

export function usePendingImageResultExecution(
  options: UsePendingImageResultNodesOptions,
): PendingImageResultExecution {
  const cancelledResultNodeIds = new Set<string>()

  function buildPendingResultLookup(): PendingImageResultLookup {
    const nodeById = new Map<string, Node>()
    const imageChildIdsBySource = new Map<string, string[]>()

    options.nodes.value.forEach((node) => {
      if (nodeById.has(node.id)) return
      nodeById.set(node.id, node)
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
      imageChildIdsBySource,
    }
  }

  function readImageNodeLayout(node: Node): ImageResultLayout | null {
    if (node.data?.kind !== 'image') return null
    const snapshot = node.data.snapshotNode
    if (!snapshot || typeof snapshot !== 'object') return null
    const anchor = resolveImageNodeLayoutAnchor(node, snapshot as ImageNodeItem)
    const measuredWidth = readMeasuredNodeWidth(node)
    return anchor
      ? {
          x: anchor.x,
          y: anchor.y,
          width: measuredWidth ?? anchor.width,
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
    const node = lookup.nodeById.get(nodeId)
    if (node) {
      if (node.data?.kind === 'image') {
        return readImageNodeLayout(node) ?? fallback
      }
      const snapshot = node.data?.snapshotNode as { width?: unknown } | undefined
      const width = readMeasuredNodeWidth(node) ?? Number(snapshot?.width)
      return {
        ratio: 1,
        x: Number(node.position?.x ?? 0),
        y: Number(node.position?.y ?? 0),
        width: Number.isFinite(width) && width > 0 ? width : fallback.width,
      }
    }
    if (anchorHint) {
      return {
        ...anchorHint,
        ratio: fallback.ratio,
      }
    }
    return fallback
  }

  function readMeasuredNodeWidth(node: Node): number | null {
    const dimensions = (node as { dimensions?: { width?: unknown } }).dimensions
    const measuredWidth = Number(dimensions?.width)
    if (Number.isFinite(measuredWidth) && measuredWidth > 0) return measuredWidth
    const directWidth = Number((node as { width?: unknown }).width)
    return Number.isFinite(directWidth) && directWidth > 0 ? directWidth : null
  }

  function readNodeBounds(
    node: Node,
  ): { x: number; y: number; width: number; height: number } | null {
    const imageLayout = readImageNodeLayout(node)
    if (imageLayout) {
      const dimensions = (node as { dimensions?: { height?: unknown } }).dimensions
      const measuredHeight = Number(dimensions?.height)
      return {
        x: imageLayout.x,
        y: imageLayout.y,
        width: imageLayout.width,
        height:
          Number.isFinite(measuredHeight) && measuredHeight > 0
            ? measuredHeight
            : estimateImageNodeHeight(imageLayout),
      }
    }

    const snapshot = node.data?.snapshotNode as { width?: unknown; height?: unknown } | undefined
    const width = readMeasuredNodeWidth(node) ?? Number(snapshot?.width)
    if (!Number.isFinite(width) || width <= 0) return null
    const dimensions = (node as { dimensions?: { height?: unknown } }).dimensions
    const measuredHeight = Number(dimensions?.height)
    const snapshotHeight = Number(snapshot?.height)
    return {
      x: Number(node.position?.x ?? 0),
      y: Number(node.position?.y ?? 0),
      width,
      height:
        Number.isFinite(measuredHeight) && measuredHeight > 0
          ? measuredHeight
          : Number.isFinite(snapshotHeight) && snapshotHeight > 0
            ? snapshotHeight
            : width,
    }
  }

  function isEmptyReusableImageNode(node: Node): boolean {
    if (node.data?.kind !== 'image') return false
    const snapshot = node.data.snapshotNode as ImageNodeItem | undefined
    if (!snapshot || typeof snapshot !== 'object') return false
    const sourceUrl = `${snapshot.sourceImageUrl ?? ''}`.trim()
    const outputImages = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
    return !sourceUrl && outputImages.length === 0 && !options.isNodeExecutionActive(node.id)
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
    lookup: PendingImageResultLookup,
    target: Pick<ImageResultLayout, 'width' | 'ratio'>,
  ) {
    const preferredPosition = {
      x: anchor.x + anchor.width + IMAGE_RESULT_NODE_X_GAP,
      y: anchor.y,
    }
    const occupiedBounds = Array.from(lookup.nodeById.values())
      .map(readNodeBounds)
      .filter(
        (bounds): bounds is { x: number; y: number; width: number; height: number } => !!bounds,
      )
    const targetHeight = estimateImageNodeHeight(target)

    const isCollisionFree = (position: { x: number; y: number }) =>
      occupiedBounds.every(
        (bounds) =>
          position.x + target.width <= bounds.x ||
          position.x >= bounds.x + bounds.width ||
          position.y + targetHeight <= bounds.y ||
          position.y >= bounds.y + bounds.height,
      )
    if (isCollisionFree(preferredPosition)) return preferredPosition

    let preferredColumnBottom = preferredPosition.y
    for (const bounds of occupiedBounds) {
      const overlapsPreferredColumn =
        preferredPosition.x < bounds.x + bounds.width &&
        preferredPosition.x + target.width > bounds.x
      if (!overlapsPreferredColumn || bounds.y + bounds.height <= preferredPosition.y) continue
      preferredColumnBottom = Math.max(
        preferredColumnBottom,
        bounds.y + bounds.height + IMAGE_RESULT_NODE_Y_GAP,
      )
    }
    const candidate = { x: preferredPosition.x, y: preferredColumnBottom }
    if (isCollisionFree(candidate)) return candidate

    const escapePosition = {
      x: preferredPosition.x,
      y:
        occupiedBounds.reduce(
          (bottom, bounds) => Math.max(bottom, bounds.y + bounds.height),
          preferredPosition.y,
        ) + IMAGE_RESULT_NODE_Y_GAP,
    }
    if (isCollisionFree(escapePosition)) return escapePosition
    throw new Error('Unable to find a collision-free position for derived image node')
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

  async function appendPendingImageResultEdges(
    anchorNodeId: string,
    resultNodeIds: string[],
    edgesToAppend: Edge[],
  ) {
    if (edgesToAppend.length === 0) return
    await waitForNextRenderFrame()
    const activeResultNodeIds = resultNodeIds.filter(
      (resultNodeId) => !cancelledResultNodeIds.has(resultNodeId),
    )
    if (activeResultNodeIds.length === 0) return
    const hasSource = options.nodes.value.some((node) => node.id === anchorNodeId)
    if (!hasSource) return
    const activeResultNodeIdSet = new Set(activeResultNodeIds)
    const existingEdgeKeys = new Set(
      options.edges.value.map(
        (edge) =>
          `${edge.source}:${edge.sourceHandle ?? ''}:${edge.target}:${edge.targetHandle ?? ''}`,
      ),
    )
    const nodesById = new Map(options.nodes.value.map((node) => [node.id, node]))
    const nextEdges = edgesToAppend
      .filter((edge) => {
        if (!activeResultNodeIdSet.has(edge.target)) return false
        const targetNode = nodesById.get(edge.target)
        if (!targetNode) return false
        const edgeKey = `${edge.source}:${edge.sourceHandle ?? ''}:${edge.target}:${edge.targetHandle ?? ''}`
        if (existingEdgeKeys.has(edgeKey)) return false
        existingEdgeKeys.add(edgeKey)
        return true
      })
      .map((edge) => ({ ...edge }))
    if (nextEdges.length === 0) return
    options.refreshNodeInternals?.([anchorNodeId, ...activeResultNodeIds])
    options.graphAdapter.addEdges(
      nextEdges,
      createPendingResultCommandOptions({ recordHistory: false }),
    )
    options.refreshNodeInternals?.([anchorNodeId, ...activeResultNodeIds])
  }

  function planPendingImageResultNodeWithLookup(
    source: ImageExecutionInput,
    anchorNodeId: string,
    lookup: PendingImageResultLookup,
    optionsOverride?: PendingImageResultPlacementOptions,
    preferredNodeId?: string | null,
  ): PendingImageResultReservationPlan {
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
    const position = resolvePendingImageResultPosition(anchor, lookup, {
      width: resolvedWidth,
      ratio: resolvedRatio,
    })
    const imageNode = createCanvasNodeAt('image', position)
    const pendingImageNode = buildPendingImageNodeSnapshot(imageNode, {
      sourceImageRatio: resolvedRatio,
      width: resolvedWidth,
    })

    if (preferredNodeId) {
      let updatedNode: Node | null = null
      const currentNode = lookup.nodeById.get(preferredNodeId)
      if (currentNode?.data?.kind === 'image') {
        const snapshot = currentNode.data.snapshotNode
        if (snapshot && typeof snapshot === 'object') {
          const currentSnapshot = snapshot as ImageNodeItem
          updatedNode = {
            ...currentNode,
            data: {
              ...currentNode.data,
              snapshotNode: buildPendingImageNodeSnapshot(currentSnapshot, {
                sourceImageRatio: pendingImageNode.sourceImageRatio,
                width: pendingImageNode.width,
              }),
            },
          }
        }
      }
      if (!updatedNode) {
        throw new Error(`Reusable pending result node is no longer available: ${preferredNodeId}`)
      }
      lookup.nodeById.set(preferredNodeId, updatedNode)
      cancelledResultNodeIds.delete(preferredNodeId)
      return {
        resultNodeId: preferredNodeId,
        nextNode: updatedNode,
      }
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

    lookup.nodeById.set(nextNode.id, nextNode as Node)
    cancelledResultNodeIds.delete(nextNode.id)
    return {
      resultNodeId: nextNode.id,
      nextNode: nextNode as Node,
      edgeToAppend: {
        id: `edge-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
        source: anchorNodeId,
        sourceHandle: 'out:image',
        target: nextNode.id,
        targetHandle: 'in:image',
      },
    }
  }

  function reservePendingResults(
    params: PendingImageResultReservationParams,
  ): PendingImageResultReservation {
    const count = clampImageNodeOutputCount(params.outputCount)
    const resultOptions = params.resultOptions ?? {}
    const anchorNodeId = resultOptions.anchor?.outputAnchorNodeId ?? params.sourceNodeId
    const lookup = buildPendingResultLookup()
    const reusableNodeId = findReusableOutputImageNodeId(anchorNodeId, lookup)
    const plans = Array.from({ length: count }, (_, index) =>
      planPendingImageResultNodeWithLookup(
        params.source,
        anchorNodeId,
        lookup,
        {
          anchor: resultOptions.anchor,
          targetRatio: resultOptions.targetRatio,
          targetWidth: resultOptions.targetWidth,
        },
        index === 0 ? reusableNodeId : null,
      ),
    )
    const plannedNodesById = new Map(plans.map((plan) => [plan.resultNodeId, plan.nextNode]))
    const existingNodeIds = new Set(options.nodes.value.map((node) => node.id))
    const nextNodes = [
      ...options.nodes.value.map((node) => plannedNodesById.get(node.id) ?? node),
      ...plans
        .filter((plan) => !existingNodeIds.has(plan.resultNodeId))
        .map((plan) => plan.nextNode),
    ]
    const resultNodeIds = plans.map((plan) => plan.resultNodeId)
    options.graphAdapter.replaceGraph(
      {
        nodes: nextNodes,
        edges: options.edges.value,
      },
      createPendingResultCommandOptions({ recordHistory: false }),
    )
    options.refreshNodeInternals?.([anchorNodeId, ...resultNodeIds])
    void appendPendingImageResultEdges(
      anchorNodeId,
      resultNodeIds,
      plans.flatMap((plan) => (plan.edgeToAppend ? [plan.edgeToAppend] : [])),
    )
    return { resultNodeIds }
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

  function applyResultMaterializerGraph(store: CanvasGraphStore, result: ResultMaterializerResult) {
    if (!result.summary.didChange) return
    const snapshot = store.exportSnapshot(options.boardBaseSnapshot?.value)
    const nextFlow = canvasBoardSnapshotToFlow(snapshot)

    options.graphAdapter.replaceGraph(
      {
        nodes: nextFlow.nodes as Node[],
        edges: nextFlow.edges as Edge[],
      },
      createPendingResultCommandOptions({ recordHistory: false }),
    )
  }

  function materializeResolvedAttachedPendingResults(
    params: MaterializeResolvedAttachedPendingImageResultsParams,
  ): ResultMaterializerResult {
    const store = options.createCurrentCanvasGraphStore({
      projection: 'runtime',
    })
    const result = materializeResolvedImageResults({
      store,
      results: params.results,
      isAttached: params.isAttached,
    })
    applyResultMaterializerGraph(store, result)
    result.materializedNodeIds.forEach((nodeId) => {
      options.refreshNodeInternals?.([nodeId])
    })
    return result
  }

  function cancelPendingResult(params: { resultNodeId: string }) {
    cancelledResultNodeIds.add(params.resultNodeId)
    options.graphAdapter.removeNodes(params.resultNodeId, {
      ...createPendingResultCommandOptions({ recordHistory: false }),
      removeConnectedEdges: true,
    })
  }

  function cancelPendingResults(resultNodeIds: readonly string[]) {
    resultNodeIds.forEach((resultNodeId) => cancelPendingResult({ resultNodeId }))
  }

  async function runReservedPendingResults(
    params: RunReservedPendingImageResultsParams,
  ): Promise<PendingImageResultRunSummary> {
    const persistPendingTaskIds = params.persistPendingTaskIds ?? true
    const resultNodeIds = [...params.resultNodeIds]
    const resolvedResults = new Array<ResultMaterializerImageResult | null>(
      resultNodeIds.length,
    ).fill(null)
    const outcomes = new Array<PendingImageResultOutcome | null>(resultNodeIds.length).fill(null)

    let pendingCount = 0
    let failureCount = 0
    let firstFailure: unknown = null
    const failures: unknown[] = []

    await Promise.allSettled(
      resultNodeIds.map(async (resultNodeId, index) => {
        let taskId: string | undefined
        try {
          const submission = await params.submitTask(resultNodeId)
          taskId = submission.taskId
          if (persistPendingTaskIds && params.isCurrent()) {
            attachPendingTaskId({ resultNodeId, taskId })
            await params.afterTaskIdAttached?.({ resultNodeId, taskId })
          }
          const resolved = await params.resolveResult(taskId, resultNodeId)
          if (params.isCurrent() && isCanvasGraphNodeId(resultNodeId)) {
            resolvedResults[index] = {
              ...resolved,
              resultNodeId,
              taskId: persistPendingTaskIds ? taskId : undefined,
            }
          }
          outcomes[index] = {
            resultNodeId,
            status: 'cancelled',
            taskId,
          }
        } catch (error) {
          firstFailure ??= error
          failures.push(error)
          if (params.isPendingError(error)) {
            pendingCount += 1
            if (!persistPendingTaskIds && params.isCurrent()) {
              cancelPendingResult({ resultNodeId })
            }
            outcomes[index] = {
              resultNodeId,
              status: persistPendingTaskIds && params.isCurrent() ? 'pending' : 'cancelled',
              taskId,
            }
            return
          }
          failureCount += 1
          outcomes[index] = {
            resultNodeId,
            status:
              !params.isCurrent() || error instanceof TaskCancelledError ? 'cancelled' : 'failed',
            taskId,
          }
          if (params.isCurrent()) {
            cancelPendingResult({ resultNodeId })
          }
        }
      }),
    )

    const materializableResults = resolvedResults.filter(
      (result): result is ResultMaterializerImageResult => !!result,
    )
    if (materializableResults.length > 0 && params.isCurrent()) {
      const materialized = materializeResolvedAttachedPendingResults({
        results: materializableResults,
        isAttached: () => params.isCurrent(),
      })
      const materializedNodeIds = new Set<string>(materialized.materializedNodeIds)
      materializableResults.forEach((result) => {
        const index = resultNodeIds.indexOf(result.resultNodeId)
        if (index < 0) return
        outcomes[index] = {
          resultNodeId: result.resultNodeId,
          status: materializedNodeIds.has(result.resultNodeId) ? 'succeeded' : 'cancelled',
          taskId: outcomes[index]?.taskId,
        }
      })
    }

    const completedOutcomes = outcomes.filter(
      (outcome): outcome is PendingImageResultOutcome => !!outcome,
    )

    return {
      resultNodeIds,
      totalCount: resultNodeIds.length,
      successCount: completedOutcomes.filter((outcome) => outcome.status === 'succeeded').length,
      pendingCount,
      failureCount,
      firstFailure,
      failures,
      outcomes: completedOutcomes,
    }
  }

  return {
    reservePendingResults,
    runReservedPendingResults,
    cancelPendingResults,
    materializeResolvedAttachedPendingResults,
  }
}
