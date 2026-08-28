import type { Edge, Elements, Node } from '@vue-flow/core'
import { computed, ref, type Ref } from 'vue'
import { i18n } from '../../../shared/i18n'
import {
  resolveImageNodeFrameSize,
  type CanvasNodeItem,
  type ImageNodeItem,
  type ImageNodeOutputItem,
} from '../../canvas-board/domain'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import {
  isCanvasGraphNodeId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
  type CanvasGraphReadPort,
} from '../graph-runtime'
import { useFlowCanvasInteractionState } from '../workspace-ui/useFlowCanvasInteractionState'
import { useFlowNodeUiState } from '../node-presentation/useFlowNodeUiState'
import type { CanvasVueFlowGraphAdapter } from '../vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import type { CanvasRendererMetricsPort } from '../vue-flow-adapter'
import { createFlowGraphCommandOptions } from '../vue-flow-adapter/flowGraphAdapterCommandSideEffects'
import { createImageFlowNode } from '../utils/flowImageNodeFactory'
import { createCanvasNodeAt } from '../utils/canvasNodeFactory'
import { resolveImageNodeDisplayPayload } from '../utils/flowImageNodeSelectors'
import type { CanvasGraphInteractionBrowserAdapter } from './interactionBrowserAdapter'
import {
  renderImageAdjustmentRecipeFile,
  type ImageAdjustmentRenderSource,
} from '../image-editor/services/imageAdjustmentRenderer'
import { resolveMediaAssetOriginalURL } from '../../../shared/api/mediaAssetDelivery'

type MessageLoadingHandle = {
  destroy: () => void
  update?: (content: unknown, options?: { description?: string; duration?: number }) => void
}

type CanvasGraphInteractionSelectionAdapter = {
  selectedElements: { readonly value: Elements }
  clear: (elements: Elements) => void
}

type UseCanvasGraphInteractionsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  readCanvasGraphStore: () => CanvasGraphReadPort
  isGraphSessionAttached: () => boolean
  writeCanvasGraphStore?: () => CanvasGraphStore
  rendererMetrics?: CanvasRendererMetricsPort
  selection: CanvasGraphInteractionSelectionAdapter
  browser: CanvasGraphInteractionBrowserAdapter
  preview: {
    openNodeImage: (snapshot: ImageNodeItem, mode: 'latest-output') => Promise<void>
  }
  feedback: CanvasFeedback['graph']
  isNodeExecutionActive: (nodeId: string) => boolean
}

type ExportImageEntry = {
  url: string
  mediaAssetId?: number
  title: string
  adjustmentRecipe?: ImageNodeItem['adjustmentRecipe']
}
type ExportImageCandidate = Omit<ExportImageEntry, 'title'>

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
  const entries: ExportImageCandidate[] =
    snapshot.type === 'image-node'
      ? [
          outputs.at(-1)
            ? {
                url: outputs.at(-1)!.url,
                mediaAssetId: outputs.at(-1)!.mediaAssetId,
                adjustmentRecipe: snapshot.adjustmentRecipe,
              }
            : {
                url: snapshot.sourceImageUrl,
                mediaAssetId: snapshot.sourceMediaAssetId,
                adjustmentRecipe: snapshot.adjustmentRecipe,
              },
        ]
      : outputs.map((item) => ({ url: item.url, mediaAssetId: item.mediaAssetId }))
  return entries
    .filter((entry) => !!entry.url?.trim() || Number.isInteger(entry.mediaAssetId))
    .map((entry) => ({
      url: entry.url.trim(),
      ...(Number.isInteger(entry.mediaAssetId) ? { mediaAssetId: entry.mediaAssetId } : {}),
      ...(entry.adjustmentRecipe ? { adjustmentRecipe: entry.adjustmentRecipe } : {}),
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
  const writeGraphStore = () =>
    options.writeCanvasGraphStore?.() ?? (options.readCanvasGraphStore() as CanvasGraphStore)
  const rendererMetrics: CanvasRendererMetricsPort = options.rendererMetrics ?? {
    readNodeMetrics: (nodeId) => {
      const node = options.nodes.value.find((candidate) => candidate.id === nodeId)
      if (!node) return null
      return {
        nodeId,
        position: { x: node.position.x, y: node.position.y },
        dimensions: { width: Number(node.width ?? 0), height: Number(node.height ?? 0) },
        computedPosition: { x: node.position.x, y: node.position.y },
        measured: Number(node.width ?? 0) > 0 && Number(node.height ?? 0) > 0,
      }
    },
  }
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

  function resolveGraphNodeId(flowNodeId: string): CanvasGraphNodeId | null {
    return isCanvasGraphNodeId(flowNodeId) ? flowNodeId : null
  }

  function readSelectedGraphNodeIds(): CanvasGraphNodeId[] {
    const selectedElements = options.selection.selectedElements.value
    if (!options.isGraphSessionAttached()) return []
    const selectedFlowIds = new Set(selectedElements.map((item) => item.id))
    const store = options.readCanvasGraphStore()
    const graphNodeIds = Array.from(selectedFlowIds)
      .map(resolveGraphNodeId)
      .filter((nodeId): nodeId is CanvasGraphNodeId => nodeId !== null)
      .filter((nodeId) => !!store.getNode(nodeId))
    return Array.from(new Set(graphNodeIds))
  }

  function readImageNode(nodeId: string): ImageNodeItem | null {
    const graphNodeId = resolveGraphNodeId(nodeId)
    if (!graphNodeId) return null
    const record = options.readCanvasGraphStore().getNode(graphNodeId)
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
    image.adjustmentRecipe = source.adjustmentRecipe
      ? structuredClone(source.adjustmentRecipe)
      : undefined
    image.sourceImageRatio = payload.ratio
    image.outputCount = source.outputCount ?? image.outputCount
    image.width = resolveImageNodeFrameSize(payload.ratio, true).width
    image.outputStatus = 'ready'
    image.outputImages = []
    const transaction = writeGraphStore().beginTransaction('node-update')
    try {
      transaction.addNode(image).commit()
    } catch (error) {
      transaction.cancel()
      console.error('[Canvas] Failed to append image node', error)
      options.feedback.operationWarning(i18n.global.t('canvas.session.addImageFailed'))
      return
    }
    options.graphAdapter.addNodes(createImageFlowNode(image), createFlowGraphCommandOptions())
  }

  async function exportSelectedImages() {
    const store = options.readCanvasGraphStore()
    const entries = readSelectedGraphNodeIds().flatMap((nodeId) => {
      const record = store.getNode(nodeId)
      return record ? resolveExportEntries(record.snapshot) : []
    })
    if (entries.length === 0) {
      options.feedback.operationWarning(i18n.global.t('canvas.session.noExportableImages'))
      return
    }
    exportLoading?.destroy()
    const loading = options.feedback.operationLoading(
      i18n.global.t('canvas.session.exportingImages', { count: entries.length }),
      { duration: Number.POSITIVE_INFINITY },
    )
    exportLoading = loading
    const counts = new Map<string, number>()
    let failed = 0
    try {
      for (const [index, entry] of entries.entries()) {
        loading.update?.(
          i18n.global.t('canvas.session.exportingImageProgress', {
            current: index + 1,
            total: entries.length,
          }),
        )
        let renderedUrl = ''
        let exportFileName = ''
        try {
          let downloadUrl = entry.url
          if (entry.adjustmentRecipe) {
            const source: ImageAdjustmentRenderSource = {
              url: resolveMediaAssetOriginalURL(entry.adjustmentRecipe.sourceMediaAssetId),
              title: entry.title,
            }
            const rendered = await renderImageAdjustmentRecipeFile(source, entry.adjustmentRecipe)
            renderedUrl = URL.createObjectURL(rendered)
            downloadUrl = renderedUrl
            exportFileName = buildUniqueExportFileName(
              { ...entry, url: rendered.name, adjustmentRecipe: undefined },
              counts,
            )
          } else {
            exportFileName = buildUniqueExportFileName(entry, counts)
          }
          await options.browser.downloadImage({
            url: downloadUrl,
            ...(!entry.adjustmentRecipe && entry.mediaAssetId
              ? { mediaAssetId: entry.mediaAssetId }
              : {}),
            fileName: exportFileName,
          })
        } catch {
          failed += 1
        } finally {
          if (renderedUrl) URL.revokeObjectURL(renderedUrl)
        }
      }
    } finally {
      loading.destroy()
      if (exportLoading === loading) exportLoading = null
    }
    if (failed === 0) {
      options.feedback.operationSucceeded(
        i18n.global.t('canvas.session.exportQueued', { count: entries.length }),
      )
    } else {
      options.feedback.operationWarning(
        i18n.global.t('canvas.session.exportPartiallyQueued', {
          completed: entries.length - failed,
          total: entries.length,
          failed,
        }),
      )
    }
  }

  function beginNodeDragDuplicateTracking(isDuplicateMode: boolean) {
    if (!isDuplicateMode) {
      duplicateDrag.value = undefined
      return
    }
    const store = options.readCanvasGraphStore()
    const selectedIds = new Set(options.selection.selectedElements.value.map((item) => item.id))
    const candidateIds = selectedIds.size > 0 ? Array.from(selectedIds) : store.getNodeIds()
    duplicateDrag.value = new Map(
      candidateIds.flatMap((flowNodeId) => {
        const graphNodeId = resolveGraphNodeId(flowNodeId)
        const record = graphNodeId ? store.getNode(graphNodeId) : undefined
        const metrics = graphNodeId ? rendererMetrics.readNodeMetrics(flowNodeId) : null
        const position =
          metrics?.position ?? (record ? { x: record.snapshot.x, y: record.snapshot.y } : null)
        return graphNodeId && record?.kind === 'image' && position
          ? [[flowNodeId, { graphNodeId, x: position.x, y: position.y }] as const]
          : []
      }),
    )
  }

  function finishNodeDragDuplicateTracking(isDuplicateMode: boolean) {
    const initial = duplicateDrag.value
    duplicateDrag.value = undefined
    if (!initial || !isDuplicateMode || initial.size === 0) return
    const store = options.readCanvasGraphStore()
    const reset = new Map<string, { x: number; y: number }>()
    const appended: Node[] = []
    const appendedSnapshots: ImageNodeItem[] = []
    initial.forEach((start, flowNodeId) => {
      const record = store.getNode(start.graphNodeId)
      const metrics = rendererMetrics.readNodeMetrics(flowNodeId)
      if (!metrics || record?.kind !== 'image') return
      const dx = metrics.position.x - start.x
      const dy = metrics.position.y - start.y
      if (Math.abs(dx) < 0.5 && Math.abs(dy) < 0.5) return
      const source = record.snapshot as ImageNodeItem
      // The drag transaction is committed before duplicate finalization. Use
      // the captured drag origin so the copy follows the pointer once while
      // the original is reset to its pre-drag position.
      const copied = createCanvasNodeAt('image', { x: start.x + dx, y: start.y + dy })
      copied.sourceImageUrl = source.sourceImageUrl.trim()
      copied.sourceMediaAssetId = source.sourceMediaAssetId
      copied.adjustmentRecipe = source.adjustmentRecipe
        ? structuredClone(source.adjustmentRecipe)
        : undefined
      copied.sourceImageRatio = source.sourceImageRatio > 0 ? source.sourceImageRatio : 1
      copied.outputCount = source.outputCount ?? copied.outputCount
      copied.outputImages = cloneImageOutputs(source)
      const payload = resolveImageNodeDisplayPayload(copied)
      copied.width = resolveImageNodeFrameSize(payload?.ratio ?? 1, !!payload).width
      copied.outputStatus = source.outputStatus
      reset.set(flowNodeId, { x: start.x, y: start.y })
      appended.push(createImageFlowNode(copied))
      appendedSnapshots.push(copied)
    })
    if (appended.length === 0) return
    const transaction = writeGraphStore().beginTransaction('node-update')
    try {
      initial.forEach((start, flowNodeId) => {
        if (reset.has(flowNodeId)) {
          transaction.updateNode(start.graphNodeId, { x: start.x, y: start.y })
        }
      })
      appendedSnapshots.forEach((snapshot) => transaction.addNode(snapshot))
      transaction.commit()
    } catch (error) {
      transaction.cancel()
      console.error('[Canvas] Failed to duplicate image nodes', error)
      options.feedback.operationWarning(i18n.global.t('canvas.session.duplicateImagesFailed'))
      return
    }
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
