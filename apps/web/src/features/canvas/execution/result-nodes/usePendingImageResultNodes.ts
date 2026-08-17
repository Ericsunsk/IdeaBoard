import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { createCanvasNodeAt } from '../../utils/canvasNodeFactory'
import { requireImageNodeOutputCount, type ImageNodeItem } from '../../../canvas-board/domain'
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
import { waitForNextRenderFrame } from '../../utils/animationFrame'
import type {
  CanvasVueFlowGraph,
  CanvasVueFlowGraphCommandOptions,
  CanvasVueFlowGraphNodeUpdateOptions,
  CanvasVueFlowGraphRemoveNodesOptions,
} from '../../composables/useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from '../../composables/flowGraphAdapterCommandSideEffects'
import {
  canvasBoardSnapshotToFlow,
  type FlowToCanvasBoardSnapshotOptions,
} from '../../utils/flowSnapshotAdapter'
import type { CanvasGraphStore } from '../../graph-runtime'
import { CanvasBoardGraphSession } from '../pending-execution'

export type PendingImageResultGraphAdapter = {
  replaceGraph: (
    nextGraph: CanvasVueFlowGraph,
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ) => void
  addEdges: (
    edges: Edge | Edge[],
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ) => void
  updateNode: (
    nodeId: string,
    update: Partial<Node> | ((node: Node) => Partial<Node>),
    commandOptions?: CanvasVueFlowGraphNodeUpdateOptions,
  ) => void
  removeNodes: (
    nodeId: string,
    commandOptions?: CanvasVueFlowGraphRemoveNodesOptions,
  ) => void
}

type UsePendingImageResultNodesOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: PendingImageResultGraphAdapter
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
  attachPendingTaskIds: (
    slots: readonly Readonly<{
      resultNodeId: string
      taskId: string
      resultIndex: number
      clientActionId: string
    }>[],
  ) => void
  removePendingResults: (resultNodeIds: readonly string[]) => void
  createBoardGraphSession: (boardId: string) => CanvasBoardGraphSession
}

export type PendingImageResultReservation = {
  resultNodeIds: string[]
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

function estimateImageNodeHeight(source: Pick<ImageResultLayout, 'width' | 'ratio'>): number {
  const stageHeight = resolveImageNodeStageHeight(source.width, source.ratio, true)
  return IMAGE_NODE_HEADER_HEIGHT + stageHeight
}

export function usePendingImageResultExecution(
  options: UsePendingImageResultNodesOptions,
): PendingImageResultExecution {
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
      pendingResultIndex: undefined,
      pendingClientActionId: undefined,
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
    const activeResultNodeIds = resultNodeIds.filter((resultNodeId) =>
      options.nodes.value.some((node) => node.id === resultNodeId),
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
    const count = requireImageNodeOutputCount(params.outputCount)
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

  function attachPendingTaskIds(
    slots: readonly Readonly<{
      resultNodeId: string
      taskId: string
      resultIndex: number
      clientActionId: string
    }>[],
  ) {
    for (const slot of slots) {
      const taskId = slot.taskId.trim()
      if (!taskId) continue
      const targetNode = options.nodes.value.find((item) => item.id === slot.resultNodeId)
      const snapshot = targetNode?.data?.snapshotNode
      if (targetNode?.data?.kind !== 'image' || !snapshot || typeof snapshot !== 'object') continue
      options.graphAdapter.updateNode(
        slot.resultNodeId,
        {
          data: {
            ...targetNode.data,
            snapshotNode: {
              ...(snapshot as ImageNodeItem),
              pendingTaskId: taskId,
              pendingResultIndex: slot.resultIndex,
              pendingClientActionId: slot.clientActionId,
            },
          },
        },
        createPendingResultCommandOptions({ recordHistory: false }),
      )
    }
  }

  function removePendingResults(resultNodeIds: readonly string[]) {
    for (const resultNodeId of resultNodeIds) {
      options.graphAdapter.removeNodes(resultNodeId, {
        ...createPendingResultCommandOptions({ recordHistory: false }),
        removeConnectedEdges: true,
      })
    }
  }

  function createBoardGraphSession(boardId: string): CanvasBoardGraphSession {
    return CanvasBoardGraphSession.create(boardId, {
      createGraphStore: () => options.createCurrentCanvasGraphStore({ projection: 'runtime' }),
      onMaterialized: (store, result) => {
        const snapshot = store.exportSnapshot(options.boardBaseSnapshot?.value)
        const nextFlow = canvasBoardSnapshotToFlow(snapshot)
        options.graphAdapter.replaceGraph(
          { nodes: nextFlow.nodes as Node[], edges: nextFlow.edges as Edge[] },
          createPendingResultCommandOptions({ recordHistory: false }),
        )
        options.refreshNodeInternals?.([...result.changedNodeIds])
      },
    })
  }

  return {
    reservePendingResults,
    attachPendingTaskIds,
    removePendingResults,
    createBoardGraphSession,
  }
}
