import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { createCanvasNodeAt } from '../../utils/canvasNodeFactory'
import {
  clampImageNodeOutputCount,
  type CanvasBoardSnapshot,
  type CanvasConnection,
  type CanvasNodeItem,
  type ImageNodeItem,
} from '../../../canvas-board/domain'
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
} from '../planning/imageExecutionResultTypes'
import { resolveImageNodeLayoutAnchor } from '../../utils/flowImageNodeSelectors'
import type { ResolvedTaskImageResult } from './resolvedTaskImageResult'
import type {
  CanvasVueFlowGraphAdapter,
  CanvasVueFlowGraphCommandOptions,
} from '../../composables/useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from '../../composables/flowGraphAdapterCommandSideEffects'
import {
  canvasConnectionToFlowEdge,
  flowToCanvasBoardSnapshot,
  parseFlowNodeId,
  resolveNextCanvasConnectionId,
} from '../../utils/flowSnapshotAdapter'
import {
  createCanvasGraphStore,
  toCanvasGraphNodeId,
  type CanvasGraphNodeId,
  type CanvasGraphTransaction,
  type CanvasGraphTransactionSummary,
} from '../../graph-runtime'

type UsePendingImageResultNodesOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  boardBaseSnapshot?: Ref<CanvasBoardSnapshot>
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
  resolveAttachedPendingResults: (
    params: ResolveAttachedPendingImageResultsParams,
  ) => Promise<ResolveAttachedPendingImageResultsSummary>
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
  afterTaskIdsAttached?: (params: {
    attachments: Array<{ resultNodeId: string; taskId: string }>
  }) => Promise<void>
  onReserved?: (resultNodeIds: string[]) => void
}

export type ResolveAttachedPendingImageResultParams = {
  resultNodeId: string
  taskId: string
  resolveResult: (taskId: string, resultNodeId: string) => Promise<ResolvedTaskImageResult>
  isCurrent: () => boolean
  isAttached: () => boolean
}

export type AttachedPendingImageResultTask = {
  resultNodeId: string
  taskId: string
}

export type ResolveAttachedPendingImageResultsParams = {
  tasks: AttachedPendingImageResultTask[]
  resolveResult: (taskId: string, resultNodeId: string) => Promise<ResolvedTaskImageResult>
  isCurrent: () => boolean
  isAttached: (task: AttachedPendingImageResultTask) => boolean
  onResolveError?: (task: AttachedPendingImageResultTask, error: unknown) => void
}

export type PendingImageResultRunSummary = {
  resultNodeIds: string[]
  totalCount: number
  successCount: number
  pendingCount: number
  failureCount: number
  firstFailure: unknown
}

export type ResolveAttachedPendingImageResultsSummary = {
  successCount: number
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
  markDirty?: boolean
  transactionSummary?: CanvasGraphTransactionSummary
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
  const transactionSummary = sideEffects?.transactionSummary
  const shouldUseTransactionSideEffects = transactionSummary?.didChange === true
  return createFlowGraphCommandOptions({
    recordHistory: shouldUseTransactionSideEffects
      ? transactionSummary.shouldRecordHistory
      : sideEffects?.recordHistory,
    markDirty: shouldUseTransactionSideEffects
      ? transactionSummary.shouldMarkDirty
      : sideEffects?.markDirty,
    transactionSummary,
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

  function createCurrentCanvasGraphSnapshot(): CanvasBoardSnapshot {
    return flowToCanvasBoardSnapshot(
      {
        nodes: options.nodes.value,
        edges: options.edges.value as Parameters<typeof flowToCanvasBoardSnapshot>[0]['edges'],
      },
      options.boardBaseSnapshot?.value,
    )
  }

  function commitResultMaterializeTransaction(params: {
    recordHistory: boolean
    markDirty: boolean
    apply: (transaction: CanvasGraphTransaction, snapshot: CanvasBoardSnapshot) => void
  }): CanvasGraphTransactionSummary {
    const snapshot = createCurrentCanvasGraphSnapshot()
    const transaction = createCanvasGraphStore(snapshot).beginTransaction('result-materialize', {
      recordHistory: params.recordHistory,
      markDirty: params.markDirty,
    })
    params.apply(transaction, snapshot)
    return transaction.commit()
  }

  function readCanvasGraphNodeId(nodeId: string): CanvasGraphNodeId | null {
    const ref = parseFlowNodeId(nodeId)
    return ref ? toCanvasGraphNodeId(ref.nodeType, ref.id) : null
  }

  function readCanvasNodeSnapshot(node: Node): CanvasNodeItem | null {
    const snapshot = node.data?.snapshotNode
    if (!snapshot || typeof snapshot !== 'object') return null
    return snapshot as CanvasNodeItem
  }

  function hasCanvasGraphNodeSnapshot(snapshot: CanvasBoardSnapshot, nodeId: string): boolean {
    const ref = parseFlowNodeId(nodeId)
    if (!ref) return false
    if (ref.nodeType === 'image') return snapshot.imageNodes.some((node) => node.id === ref.id)
    if (ref.nodeType === 'prompt') return snapshot.promptNodes.some((node) => node.id === ref.id)
    return snapshot.fusionNodes.some((node) => node.id === ref.id)
  }

  function resolveConnectionIdAllocator(snapshot: CanvasBoardSnapshot): () => number {
    const usedIds = new Set(snapshot.connections.map((connection) => connection.id))
    let nextConnectionId = resolveNextCanvasConnectionId(
      snapshot,
      options.edges.value as Parameters<typeof resolveNextCanvasConnectionId>[1],
    )
    return () => {
      while (usedIds.has(nextConnectionId)) nextConnectionId += 1
      const connectionId = nextConnectionId
      usedIds.add(connectionId)
      nextConnectionId += 1
      return connectionId
    }
  }

  function createPendingImageResultConnection(
    connectionId: number,
    anchorNodeId: string,
    resultNodeId: string,
  ): CanvasConnection | null {
    const source = parseFlowNodeId(anchorNodeId)
    const target = parseFlowNodeId(resultNodeId)
    if (!source || !target) return null
    return {
      id: connectionId,
      fromNodeType: source.nodeType,
      fromNodeId: source.id,
      fromPort: 'image',
      toNodeType: target.nodeType,
      toNodeId: target.id,
      toPort: 'image',
    }
  }

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

  function createPendingImageResultEdge(
    connection: CanvasConnection,
    targetSnapshot: ImageNodeItem,
  ): Edge {
    const edge = canvasConnectionToFlowEdge(connection)
    return {
      ...edge,
      animated: targetSnapshot.outputStatus === 'running',
    }
  }

  function hasSamePendingImageResultEdge(
    edges: readonly Edge[],
    anchorNodeId: string,
    resultNodeId: string,
  ): boolean {
    return edges.some(
      (edge) =>
        edge.source === anchorNodeId &&
        edge.target === resultNodeId &&
        edge.sourceHandle === 'out:image' &&
        edge.targetHandle === 'in:image',
    )
  }

  type PendingImageResultReservationPlan = {
    resultNodeId: string
    node: Node
    mode: 'add' | 'update'
    anchorNodeId: string
    layout: ImageResultLayout
    refreshNodeIds: string[]
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
      const targetNode = lookup.nodeById.get(preferredNodeId)
      const snapshot = targetNode?.data?.snapshotNode
      if (targetNode?.data?.kind === 'image' && snapshot && typeof snapshot === 'object') {
        const updatedNode: Node = {
          ...targetNode,
          data: {
            ...targetNode.data,
            snapshotNode: buildPendingImageNodeSnapshot(snapshot as ImageNodeItem, {
              sourceImageRatio: pendingImageNode.sourceImageRatio,
              width: pendingImageNode.width,
            }),
          },
        }
        lookup.nodeById.set(preferredNodeId, updatedNode)
        cancelledResultNodeIds.delete(preferredNodeId)
        const layout = {
          ratio: pendingImageNode.sourceImageRatio,
          x: position.x,
          y: position.y,
          width: pendingImageNode.width,
        }
        rememberResultNodeLayout(anchorNodeId, preferredNodeId, layout)
        return {
          resultNodeId: preferredNodeId,
          node: updatedNode,
          mode: 'update',
          anchorNodeId,
          layout,
          refreshNodeIds: [anchorNodeId, preferredNodeId],
        }
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
    lookup.nodeOrderById.set(nextNode.id, lookup.nodeOrderById.size)
    cancelledResultNodeIds.delete(nextNode.id)
    const layout = {
      ratio: pendingImageNode.sourceImageRatio,
      x: pendingImageNode.x,
      y: pendingImageNode.y,
      width: pendingImageNode.width,
    }
    rememberResultNodeLayout(anchorNodeId, nextNode.id, layout)

    return {
      resultNodeId: nextNode.id,
      node: nextNode as Node,
      mode: 'add',
      anchorNodeId,
      layout,
      refreshNodeIds: [anchorNodeId, nextNode.id],
    }
  }

  function commitPendingResultReservationBatch(plans: PendingImageResultReservationPlan[]) {
    if (plans.length === 0) return
    const nodeUpdates = new Map<string, Node>()
    const nodesToAdd: Node[] = []
    plans.forEach((plan) => {
      if (plan.mode === 'update') {
        nodeUpdates.set(plan.resultNodeId, plan.node)
      } else {
        nodesToAdd.push(plan.node)
      }
    })

    const nextNodes =
      nodeUpdates.size > 0 || nodesToAdd.length > 0
        ? [...options.nodes.value.map((node) => nodeUpdates.get(node.id) ?? node), ...nodesToAdd]
        : options.nodes.value

    const graphTransaction = commitResultMaterializeTransaction({
      recordHistory: true,
      markDirty: false,
      apply: (transaction, snapshot) => {
        plans.forEach((plan) => {
          const snapshotNode = readCanvasNodeSnapshot(plan.node)
          if (!snapshotNode) return
          if (plan.mode === 'add') {
            transaction.addNode(snapshotNode)
            return
          }
          const nodeId = readCanvasGraphNodeId(plan.resultNodeId)
          if (nodeId) transaction.updateNode(nodeId, () => snapshotNode)
        })

        const allocateConnectionId = resolveConnectionIdAllocator(snapshot)
        plans.forEach((plan) => {
          const existingEdges = options.edges.value
          if (hasSamePendingImageResultEdge(existingEdges, plan.anchorNodeId, plan.resultNodeId)) {
            return
          }
          const connection = createPendingImageResultConnection(
            allocateConnectionId(),
            plan.anchorNodeId,
            plan.resultNodeId,
          )
          if (connection) transaction.addEdge(connection)
        })
      },
    })

    const connectionIdsByPlanKey = new Map<string, number>()
    graphTransaction.projectionPatch.edges.added.forEach((record) => {
      connectionIdsByPlanKey.set(
        `${record.connection.fromNodeType}-${record.connection.fromNodeId}:${record.connection.toNodeType}-${record.connection.toNodeId}`,
        record.connection.id,
      )
    })

    const edgesToAdd: Edge[] = []
    plans.forEach((plan) => {
      const existingEdges = [...options.edges.value, ...edgesToAdd]
      if (hasSamePendingImageResultEdge(existingEdges, plan.anchorNodeId, plan.resultNodeId)) return
      const snapshot = plan.node.data?.snapshotNode
      if (!snapshot || typeof snapshot !== 'object') return
      const source = parseFlowNodeId(plan.anchorNodeId)
      const target = parseFlowNodeId(plan.resultNodeId)
      const connectionId =
        source && target
          ? connectionIdsByPlanKey.get(
              `${source.nodeType}-${source.id}:${target.nodeType}-${target.id}`,
            )
          : undefined
      if (!connectionId) return
      const connection = createPendingImageResultConnection(
        connectionId,
        plan.anchorNodeId,
        plan.resultNodeId,
      )
      if (!connection) return
      edgesToAdd.push(createPendingImageResultEdge(connection, snapshot as ImageNodeItem))
    })
    const nextEdges =
      edgesToAdd.length > 0 ? [...options.edges.value, ...edgesToAdd] : options.edges.value

    options.graphAdapter.replaceGraph(
      {
        nodes: nextNodes,
        edges: nextEdges,
      },
      createPendingResultCommandOptions({
        recordHistory: true,
        markDirty: false,
        transactionSummary: graphTransaction,
      }),
    )

    const refreshNodeIds = Array.from(new Set(plans.flatMap((plan) => plan.refreshNodeIds)))
    if (refreshNodeIds.length > 0) options.refreshNodeInternals?.(refreshNodeIds)
  }

  function reservePendingResults(params: PendingImageResultReservationParams): string[] {
    const count = clampImageNodeOutputCount(params.outputCount)
    const resultOptions = params.resultOptions ?? {}
    const anchorNodeId = resultOptions.anchor?.outputAnchorNodeId ?? params.sourceNodeId
    pruneResultNodeLayouts()
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
    commitPendingResultReservationBatch(plans)
    return plans.map((plan) => plan.resultNodeId)
  }

  function attachPendingTaskIds(params: {
    attachments: Array<{ resultNodeId: string; taskId: string }>
    markDirty: boolean
  }): Promise<void> {
    const attachments = new Map<string, string>()
    params.attachments.forEach((attachment) => {
      const normalizedTaskId = attachment.taskId.trim()
      if (normalizedTaskId) attachments.set(attachment.resultNodeId, normalizedTaskId)
    })
    if (attachments.size === 0) return Promise.resolve()
    let didUpdate = false
    const updatedSnapshotsByNodeId = new Map<string, ImageNodeItem>()
    const nextNodes = options.nodes.value.map((item) => {
      const taskId = attachments.get(item.id)
      if (!taskId || item.data?.kind !== 'image') return item
      const snapshot = item.data.snapshotNode
      if (!snapshot || typeof snapshot !== 'object') return item
      const nextSnapshot = {
        ...(snapshot as ImageNodeItem),
        pendingTaskId: taskId,
      }
      didUpdate = true
      updatedSnapshotsByNodeId.set(item.id, nextSnapshot)
      return {
        ...item,
        data: {
          ...item.data,
          snapshotNode: nextSnapshot,
        },
      }
    })
    if (!didUpdate) return Promise.resolve()
    const graphTransaction = commitResultMaterializeTransaction({
      recordHistory: false,
      markDirty: params.markDirty,
      apply: (transaction, currentSnapshot) => {
        updatedSnapshotsByNodeId.forEach((updatedSnapshot, nodeId) => {
          const graphNodeId = readCanvasGraphNodeId(nodeId)
          if (!graphNodeId) return
          if (hasCanvasGraphNodeSnapshot(currentSnapshot, nodeId)) {
            transaction.updateNode(graphNodeId, () => updatedSnapshot)
            return
          }
          transaction.addNode(updatedSnapshot)
        })
      },
    })
    options.graphAdapter.replaceNodes(
      nextNodes,
      createPendingResultCommandOptions({
        recordHistory: false,
        markDirty: params.markDirty,
        transactionSummary: graphTransaction,
      }),
    )
    return Promise.resolve()
  }

  type ResolvedImageProjection = {
    snapshot: ImageNodeItem
    normalizedRatio: number
    resolvedFrameWidth: number | null
  }

  function createResolvedImageProjection(
    currentSnapshot: ImageNodeItem,
    result: ResolvedTaskImageResult,
  ): ResolvedImageProjection {
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
    const currentWidth = Number(currentSnapshot.width)
    const width =
      resolvedFrameWidth ??
      (Number.isFinite(currentWidth) && currentWidth > 0
        ? Math.max(1, Math.round(currentWidth))
        : resolveImageNodeFrameSize(normalizedRatio, true).width)
    return {
      snapshot: {
        ...currentSnapshot,
        width,
        sourceImageUrl: result.url,
        sourceImageRatio: normalizedRatio,
        pendingTaskId: undefined,
        outputStatus: 'ready',
        outputImages: [],
      },
      normalizedRatio,
      resolvedFrameWidth,
    }
  }

  function materializeResolvedImages(
    materializations: Array<{ resultNodeId: string; result: ResolvedTaskImageResult }>,
  ): Promise<void> {
    const materializationsByNodeId = new Map<string, ResolvedTaskImageResult>()
    materializations.forEach((materialization) => {
      materializationsByNodeId.set(materialization.resultNodeId, materialization.result)
    })
    if (materializationsByNodeId.size === 0) return Promise.resolve()

    const appliedLayouts = new Map<
      string,
      { normalizedRatio: number; resolvedFrameWidth: number | null }
    >()
    const updatedSnapshotsByNodeId = new Map<string, ImageNodeItem>()
    let didUpdateNode = false
    const nextNodes = options.nodes.value.map((item) => {
      const result = materializationsByNodeId.get(item.id)
      if (!result || item.data?.kind !== 'image') return item
      const snapshot = item.data.snapshotNode
      if (!snapshot || typeof snapshot !== 'object') return item
      const projection = createResolvedImageProjection(snapshot as ImageNodeItem, result)
      didUpdateNode = true
      appliedLayouts.set(item.id, {
        normalizedRatio: projection.normalizedRatio,
        resolvedFrameWidth: projection.resolvedFrameWidth,
      })
      updatedSnapshotsByNodeId.set(item.id, projection.snapshot)
      return {
        ...item,
        data: {
          ...item.data,
          snapshotNode: projection.snapshot,
        },
      }
    })

    const updatedNodeIds = new Set(appliedLayouts.keys())
    let didUpdateEdge = false
    const nextEdges = options.edges.value.map((edge) => {
      if (!updatedNodeIds.has(edge.target) || edge.targetHandle !== 'in:image') return edge
      if (edge.animated === false) return edge
      didUpdateEdge = true
      return { ...edge, animated: false }
    })
    if (!didUpdateNode && !didUpdateEdge) return Promise.resolve()
    const graphTransaction = commitResultMaterializeTransaction({
      recordHistory: false,
      markDirty: true,
      apply: (transaction) => {
        updatedSnapshotsByNodeId.forEach((snapshot, nodeId) => {
          const graphNodeId = readCanvasGraphNodeId(nodeId)
          if (graphNodeId) transaction.updateNode(graphNodeId, () => snapshot)
        })
      },
    })
    options.graphAdapter.replaceGraph(
      {
        nodes: didUpdateNode ? nextNodes : options.nodes.value,
        edges: didUpdateEdge ? nextEdges : options.edges.value,
      },
      createPendingResultCommandOptions({
        recordHistory: false,
        markDirty: true,
        transactionSummary: graphTransaction,
      }),
    )
    appliedLayouts.forEach((layout, resultNodeId) => {
      updateResultNodeLayout(resultNodeId, {
        ratio: layout.normalizedRatio,
        ...(layout.resolvedFrameWidth ? { width: layout.resolvedFrameWidth } : {}),
      })
    })
    const refreshNodeIds = Array.from(updatedNodeIds)
    if (refreshNodeIds.length > 0) options.refreshNodeInternals?.(refreshNodeIds)
    return Promise.resolve()
  }

  function cancelPendingResults(resultNodeIds: string[]): Promise<void> {
    const uniqueResultNodeIds = Array.from(new Set(resultNodeIds))
    if (uniqueResultNodeIds.length === 0) return Promise.resolve()
    uniqueResultNodeIds.forEach((resultNodeId) => {
      cancelledResultNodeIds.add(resultNodeId)
      forgetResultNodeLayout(resultNodeId)
    })
    const graphTransaction = commitResultMaterializeTransaction({
      recordHistory: false,
      markDirty: true,
      apply: (transaction) => {
        uniqueResultNodeIds.forEach((resultNodeId) => {
          const graphNodeId = readCanvasGraphNodeId(resultNodeId)
          if (graphNodeId) transaction.removeNode(graphNodeId, { removeConnectedEdges: true })
        })
      },
    })
    options.graphAdapter.removeNodes(uniqueResultNodeIds, {
      ...createPendingResultCommandOptions({
        recordHistory: false,
        markDirty: true,
        transactionSummary: graphTransaction,
      }),
      removeConnectedEdges: true,
    })
    return Promise.resolve()
  }

  async function runTaskIdAttachmentCallbacks(
    params: RunPendingImageResultsParams,
    attachments: Array<{ resultNodeId: string; taskId: string }>,
  ): Promise<void> {
    if (!params.afterTaskIdsAttached && !params.afterTaskIdAttached) return Promise.resolve()
    if (attachments.length === 0) return Promise.resolve()
    if (params.afterTaskIdsAttached) {
      await params.afterTaskIdsAttached({ attachments })
      return
    }
    await Promise.all(attachments.map((attachment) => params.afterTaskIdAttached?.(attachment)))
  }

  async function runPendingResults(
    params: RunPendingImageResultsParams,
  ): Promise<PendingImageResultRunSummary> {
    const persistPendingTaskIds = params.persistPendingTaskIds ?? true
    const resultNodeIds = reservePendingResults(params)
    params.onReserved?.(resultNodeIds)
    const resultNodeOrder = new Map(
      resultNodeIds.map((resultNodeId, index) => [resultNodeId, index]),
    )

    let successCount = 0
    let pendingCount = 0
    let failureCount = 0
    let firstFailure: unknown = null
    const submittedResults: Array<{ resultNodeId: string; taskId: string }> = []
    const materializations: Array<{ resultNodeId: string; result: ResolvedTaskImageResult }> = []
    const resultNodeIdsToCancel: string[] = []

    await Promise.allSettled(
      resultNodeIds.map(async (resultNodeId) => {
        try {
          const { taskId } = await params.submitTask(resultNodeId)
          submittedResults.push({ resultNodeId, taskId })
        } catch (error) {
          firstFailure ??= error
          if (params.isPendingError(error)) {
            pendingCount += 1
            if (!persistPendingTaskIds && params.isCurrent()) {
              resultNodeIdsToCancel.push(resultNodeId)
            }
            return
          }
          failureCount += 1
          if (params.isCurrent()) {
            resultNodeIdsToCancel.push(resultNodeId)
          }
        }
      }),
    )

    const orderedSubmittedResults = submittedResults.sort(
      (left, right) =>
        (resultNodeOrder.get(left.resultNodeId) ?? Number.MAX_SAFE_INTEGER) -
        (resultNodeOrder.get(right.resultNodeId) ?? Number.MAX_SAFE_INTEGER),
    )

    if (persistPendingTaskIds && params.isCurrent()) {
      const shouldPersistTaskIdAttachment =
        !!params.afterTaskIdsAttached || !!params.afterTaskIdAttached
      await attachPendingTaskIds({
        attachments: orderedSubmittedResults,
        markDirty: shouldPersistTaskIdAttachment,
      })
      await runTaskIdAttachmentCallbacks(params, orderedSubmittedResults)
    }

    await Promise.allSettled(
      orderedSubmittedResults.map(async ({ resultNodeId, taskId }) => {
        try {
          const resolved = await params.resolveResult(taskId, resultNodeId)
          if (params.isCurrent()) {
            materializations.push({ resultNodeId, result: resolved })
          }
          successCount += 1
        } catch (error) {
          firstFailure ??= error
          if (params.isPendingError(error)) {
            pendingCount += 1
            if (!persistPendingTaskIds && params.isCurrent()) {
              resultNodeIdsToCancel.push(resultNodeId)
            }
            return
          }
          failureCount += 1
          if (params.isCurrent()) {
            resultNodeIdsToCancel.push(resultNodeId)
          }
        }
      }),
    )

    if (params.isCurrent()) {
      const orderedMaterializations = materializations.sort(
        (left, right) =>
          (resultNodeOrder.get(left.resultNodeId) ?? Number.MAX_SAFE_INTEGER) -
          (resultNodeOrder.get(right.resultNodeId) ?? Number.MAX_SAFE_INTEGER),
      )
      await materializeResolvedImages(orderedMaterializations)
      await cancelPendingResults(resultNodeIdsToCancel)
    }

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
    const summary = await resolveAttachedPendingResults({
      tasks: [{ resultNodeId: params.resultNodeId, taskId: params.taskId }],
      resolveResult: params.resolveResult,
      isCurrent: params.isCurrent,
      isAttached: () => params.isAttached(),
    })
    return summary.successCount > 0
  }

  async function resolveAttachedPendingResults(
    params: ResolveAttachedPendingImageResultsParams,
  ): Promise<ResolveAttachedPendingImageResultsSummary> {
    const taskOrder = new Map(
      params.tasks.map((task, index) => [`${task.resultNodeId}:${task.taskId}`, index]),
    )
    let successCount = 0
    let failureCount = 0
    let firstFailure: unknown = null
    const materializations: Array<{
      resultNodeId: string
      result: ResolvedTaskImageResult
      taskId: string
    }> = []

    await Promise.allSettled(
      params.tasks.map(async (task) => {
        try {
          const resolved = await params.resolveResult(task.taskId, task.resultNodeId)
          if (!params.isCurrent() || !params.isAttached(task)) return
          materializations.push({
            resultNodeId: task.resultNodeId,
            result: resolved,
            taskId: task.taskId,
          })
          successCount += 1
        } catch (error) {
          if (!params.isCurrent() || !params.isAttached(task)) return
          firstFailure ??= error
          failureCount += 1
          params.onResolveError?.(task, error)
        }
      }),
    )

    if (params.isCurrent()) {
      const orderedMaterializations = materializations
        .sort(
          (left, right) =>
            (taskOrder.get(`${left.resultNodeId}:${left.taskId}`) ?? Number.MAX_SAFE_INTEGER) -
            (taskOrder.get(`${right.resultNodeId}:${right.taskId}`) ?? Number.MAX_SAFE_INTEGER),
        )
        .map(({ resultNodeId, result }) => ({ resultNodeId, result }))
      await materializeResolvedImages(orderedMaterializations)
    }

    return {
      successCount,
      failureCount,
      firstFailure,
    }
  }

  return {
    runPendingResults,
    resolveAttachedPendingResult,
    resolveAttachedPendingResults,
  }
}
