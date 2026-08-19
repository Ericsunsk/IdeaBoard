import type { Edge, Elements, Node } from '@vue-flow/core'
import { computed, ref, type Ref } from 'vue'
import {
  resolveImageNodeFrameSize,
  type CanvasNodeItem,
  type ImageNodeItem,
  type ImageNodeOutputItem,
} from '../../canvas-board/domain'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import {
  isCanvasGraphNodeId,
  toCanvasGraphNodeId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
} from '../graph-runtime'
import { useFlowCanvasInteractionState } from '../composables/useFlowCanvasInteractionState'
import { useFlowNodeUiState } from '../composables/useFlowNodeUiState'
import type { CanvasVueFlowGraphAdapter } from '../composables/useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from '../composables/flowGraphAdapterCommandSideEffects'
import { createImageFlowNode } from '../utils/flowImageNodeFactory'
import { createCanvasNodeAt } from '../utils/canvasNodeFactory'
import { resolveImageNodeDisplayPayload } from '../utils/flowImageNodeSelectors'
import { resolveFlowNodeCanvasRef } from '../utils/flowSnapshotAdapter'
import type { CanvasGraphInteractionBrowserAdapter } from './interactionBrowserAdapter'

type MessageLoadingHandle = { destroy: () => void }

type CanvasGraphInteractionSelectionAdapter = {
  selectedElements: { readonly value: Elements }
  clear: (elements: Elements) => void
}

type UseCanvasGraphInteractionsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  createCurrentCanvasGraphStore: () => CanvasGraphStore
  selection: CanvasGraphInteractionSelectionAdapter
  browser: CanvasGraphInteractionBrowserAdapter
  preview: {
    openNodeImage: (snapshot: ImageNodeItem, mode: 'latest-output') => Promise<void>
  }
  feedback: CanvasFeedback['graph']
  isNodeExecutionActive: (nodeId: string) => boolean
}

type ExportImageEntry = { url: string; mediaAssetId?: number; title: string }

function safeDecodeURIComponent(value: string): string {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

function sanitizeFileNameSegment(rawName: string): string {
  return rawName
    .replace(/[\\/:*?"<>|]+/g, '_')
    .split('')
    .filter((char) => char.charCodeAt(0) >= 32)
    .join('')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/[. ]+$/g, '')
}

function inferExtensionFromUrl(rawUrl: string): string {
  const rawName = rawUrl.split('/').pop()?.split('?')[0] || ''
  const extension = safeDecodeURIComponent(rawName).match(/\.([a-z0-9]{2,5})$/i)?.[1]
  return extension ? `.${extension.toLowerCase()}` : '.png'
}

function buildUniqueExportFileName(entry: ExportImageEntry, counts: Map<string, number>) {
  const safeTitle = sanitizeFileNameSegment(entry.title)
  const titleExtension = safeTitle.match(/\.([a-z0-9]{2,5})$/i)?.[0] ?? ''
  const stem =
    sanitizeFileNameSegment(
      titleExtension ? safeTitle.slice(0, -titleExtension.length) : safeTitle,
    ) || 'image'
  const extension = titleExtension.toLowerCase() || inferExtensionFromUrl(entry.url)
  const key = `${stem}${extension}`.toLowerCase()
  const count = counts.get(key) ?? 0
  counts.set(key, count + 1)
  return count === 0 ? `${stem}${extension}` : `${stem}(${count})${extension}`
}

function readOutputImages(snapshot: CanvasNodeItem): ImageNodeOutputItem[] {
  if (!('outputImages' in snapshot) || !Array.isArray(snapshot.outputImages)) return []
  return snapshot.outputImages.filter((item) => !!item.url?.trim())
}

function resolveExportEntries(snapshot: CanvasNodeItem): ExportImageEntry[] {
  if (snapshot.type !== 'image-node' && snapshot.type !== 'fusion') return []
  const outputs = readOutputImages(snapshot)
  const entries =
    snapshot.type === 'image-node'
      ? [
          outputs.at(-1)
            ? { url: outputs.at(-1)!.url, mediaAssetId: outputs.at(-1)!.mediaAssetId }
            : { url: snapshot.sourceImageUrl, mediaAssetId: snapshot.sourceMediaAssetId },
        ]
      : outputs.map((item) => ({ url: item.url, mediaAssetId: item.mediaAssetId }))
  return entries
    .filter((entry) => !!entry.url?.trim() || Number.isInteger(entry.mediaAssetId))
    .map((entry) => ({
      url: entry.url.trim(),
      ...(Number.isInteger(entry.mediaAssetId) ? { mediaAssetId: entry.mediaAssetId } : {}),
      title: snapshot.title?.trim() || 'image',
    }))
}

function cloneImageOutputs(source: ImageNodeItem): ImageNodeItem['outputImages'] {
  return source.outputImages
    .map((item) => {
      const mediaAssetId = Number(item.mediaAssetId)
      return {
        url: item.url.trim(),
        ratio: Number.isFinite(item.ratio) && item.ratio > 0 ? item.ratio : 1,
        ...(Number.isInteger(mediaAssetId) && mediaAssetId > 0 ? { mediaAssetId } : {}),
      }
    })
    .filter((item) => item.url)
}

export function useCanvasGraphInteractions(options: UseCanvasGraphInteractionsOptions) {
  const interaction = useFlowCanvasInteractionState()
  const nodeUiState = useFlowNodeUiState({
    nodes: options.nodes,
    edges: options.edges,
    isNodeExecutionActive: options.isNodeExecutionActive,
  })
  let exportLoading: MessageLoadingHandle | null = null
  const duplicateDrag = ref<
    Map<string, { graphNodeId: CanvasGraphNodeId; x: number; y: number }> | undefined
  >()

  function resolveGraphNodeId(flowNode: Pick<Node, 'id' | 'data'>): CanvasGraphNodeId | null {
    const ref = resolveFlowNodeCanvasRef(flowNode)
    if (ref) return toCanvasGraphNodeId(ref.nodeType, ref.id)
    return isCanvasGraphNodeId(flowNode.id) ? flowNode.id : null
  }

  function readSelectedGraphNodeIds(): CanvasGraphNodeId[] {
    const selectedFlowIds = new Set(options.selection.selectedElements.value.map((item) => item.id))
    const graphNodeIds = options.nodes.value
      .filter((node) => selectedFlowIds.has(node.id))
      .map(resolveGraphNodeId)
      .filter((nodeId): nodeId is CanvasGraphNodeId => nodeId !== null)
    return Array.from(new Set(graphNodeIds))
  }

  function readImageNode(nodeId: string): ImageNodeItem | null {
    const flowNode = options.nodes.value.find((node) => node.id === nodeId)
    const graphNodeId = flowNode
      ? resolveGraphNodeId(flowNode)
      : isCanvasGraphNodeId(nodeId)
        ? nodeId
        : null
    if (!graphNodeId) return null
    const record = options.createCurrentCanvasGraphStore().getNode(graphNodeId)
    return record?.kind === 'image' ? (record.snapshot as ImageNodeItem) : null
  }

  function clearSelection() {
    const elements = [...options.selection.selectedElements.value]
    if (elements.length > 0) options.selection.clear(elements)
  }

  function openImagePreview(nodeId: string) {
    const snapshot = readImageNode(nodeId)
    if (snapshot) void options.preview.openNodeImage(snapshot, 'latest-output')
  }

  function appendImageToCanvas(nodeId: string) {
    const source = readImageNode(nodeId)
    const payload = source ? resolveImageNodeDisplayPayload(source) : null
    if (!source || !payload) return
    const image = createCanvasNodeAt('image', {
      x: source.x + Math.max(240, source.width + 64),
      y: source.y + 18,
    })
    image.sourceImageUrl = payload.url
    image.sourceMediaAssetId = payload.mediaAssetId
    image.sourceImageRatio = payload.ratio
    image.outputCount = source.outputCount ?? image.outputCount
    image.width = resolveImageNodeFrameSize(payload.ratio, true).width
    image.outputStatus = 'ready'
    image.outputImages = []
    options.graphAdapter.addNodes(createImageFlowNode(image), createFlowGraphCommandOptions())
  }

  async function exportSelectedImages() {
    const store = options.createCurrentCanvasGraphStore()
    const entries = readSelectedGraphNodeIds().flatMap((nodeId) => {
      const record = store.getNode(nodeId)
      return record ? resolveExportEntries(record.snapshot) : []
    })
    if (entries.length === 0) {
      options.feedback.operationWarning('当前选中节点没有可导出的图片')
      return
    }
    exportLoading?.destroy()
    const loading = options.feedback.operationLoading(`Exporting ${entries.length} images...`, {
      duration: 1500,
    })
    exportLoading = loading
    const counts = new Map<string, number>()
    let failed = 0
    try {
      entries.forEach((entry) => {
        try {
          options.browser.downloadImage({
            url: entry.url,
            ...(entry.mediaAssetId ? { mediaAssetId: entry.mediaAssetId } : {}),
            fileName: buildUniqueExportFileName(entry, counts),
          })
        } catch {
          failed += 1
        }
      })
    } finally {
      loading.destroy()
      if (exportLoading === loading) exportLoading = null
    }
    if (failed === 0) {
      options.feedback.operationSucceeded(
        `Export queued: ${entries.length} image${entries.length > 1 ? 's' : ''}`,
      )
    } else {
      options.feedback.operationWarning(
        `Export queued ${entries.length - failed}/${entries.length}, failed ${failed}`,
      )
    }
  }

  function beginNodeDragDuplicateTracking(isDuplicateMode: boolean) {
    if (!isDuplicateMode) {
      duplicateDrag.value = undefined
      return
    }
    const selectedIds = new Set(options.selection.selectedElements.value.map((item) => item.id))
    const candidates =
      selectedIds.size > 0
        ? options.nodes.value.filter((node) => selectedIds.has(node.id))
        : options.nodes.value
    duplicateDrag.value = new Map(
      candidates.flatMap((node) => {
        const graphNodeId = resolveGraphNodeId(node)
        return graphNodeId
          ? [[node.id, { graphNodeId, x: node.position.x, y: node.position.y }] as const]
          : []
      }),
    )
  }

  function finishNodeDragDuplicateTracking(isDuplicateMode: boolean) {
    const initial = duplicateDrag.value
    duplicateDrag.value = undefined
    if (!initial || !isDuplicateMode || initial.size === 0) return
    const store = options.createCurrentCanvasGraphStore()
    const currentById = new Map(options.nodes.value.map((node) => [node.id, node]))
    const reset = new Map<string, { x: number; y: number }>()
    const appended: Node[] = []
    initial.forEach((start, flowNodeId) => {
      const current = currentById.get(flowNodeId)
      const record = store.getNode(start.graphNodeId)
      if (!current || record?.kind !== 'image') return
      const dx = current.position.x - start.x
      const dy = current.position.y - start.y
      if (Math.abs(dx) < 0.5 && Math.abs(dy) < 0.5) return
      const source = record.snapshot as ImageNodeItem
      const copied = createCanvasNodeAt('image', { x: source.x + dx, y: source.y + dy })
      copied.sourceImageUrl = source.sourceImageUrl.trim()
      copied.sourceMediaAssetId = source.sourceMediaAssetId
      copied.sourceImageRatio = source.sourceImageRatio > 0 ? source.sourceImageRatio : 1
      copied.outputCount = source.outputCount ?? copied.outputCount
      copied.outputImages = cloneImageOutputs(source)
      const payload = resolveImageNodeDisplayPayload(copied)
      copied.width = resolveImageNodeFrameSize(payload?.ratio ?? 1, !!payload).width
      copied.outputStatus = source.outputStatus
      reset.set(flowNodeId, { x: start.x, y: start.y })
      appended.push(createImageFlowNode(copied))
    })
    if (appended.length === 0) return
    options.graphAdapter.replaceNodes(
      (nodes) => [
        ...nodes.map((node) => {
          const position = reset.get(node.id)
          return position ? { ...node, position } : node
        }),
        ...appended,
      ],
      createFlowGraphCommandOptions({ recordHistory: false }),
    )
  }

  return {
    viewModel: {
      hasSelectedNodes: computed(() => readSelectedGraphNodeIds().length > 0),
      isInteractionActive: interaction.isCanvasInteractionActive,
      isViewportPanInteractionActive: interaction.isViewportPanInteractionActive,
      isInteractionLocked: interaction.isCanvasInteractionLocked,
    },
    commands: {
      clearSelection,
      exportSelectedImages,
      openImagePreview,
      appendImageToCanvas,
      beginNodeDragDuplicateTracking,
      finishNodeDragDuplicateTracking,
      getNodeUiState: nodeUiState.getFlowNodeUiStateCached,
    },
    runtimePort: {
      startInteraction: interaction.startCanvasInteraction,
      stopInteraction: interaction.stopCanvasInteraction,
    },
  }
}
