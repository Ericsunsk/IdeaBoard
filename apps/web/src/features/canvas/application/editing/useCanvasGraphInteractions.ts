import type { Edge, Elements, Node } from '@vue-flow/core'
import { computed, ref, type Ref } from 'vue'
import { i18n } from '../../../../shared/i18n'
import {
  resolveImageNodeFrameSize,
  resolveImageNodeLatestOutput,
  type CanvasNodeItem,
  type ImageNodeItem,
  type ImageNodeOutputItem,
} from '../../../board'
import type { CanvasFeedback } from '../workspace/useCanvasFeedback.ts'
import {
  isCanvasGraphNodeId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
  type CanvasGraphReadPort,
} from '../../domain/graph'
import { useFlowCanvasInteractionState } from '../../presentation/screens/useFlowCanvasInteractionState.ts'
import { useFlowNodeUiState } from '../../presentation/nodes/useFlowNodeUiState.ts'
import type { CanvasRendererMetricsPort } from '../../infrastructure/vue-flow-adapter'
import { createCanvasNodeAt } from '../../domain/nodes/canvasNodeFactory.ts'
import { resolveImageNodeDisplayPayload } from '../../infrastructure/vue-flow-adapter/flowImageNodeSelectors.ts'
import type { CanvasGraphInteractionBrowserAdapter } from './interactionBrowserAdapter'
import {
  renderImageRetouchRecipeFile,
  type ImageRetouchRenderSource,
} from '../../tools/retouch/rendering/renderer.ts'
import { resolveMediaAssetOriginalURL } from '../../../../shared/api/mediaAssetDelivery'

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
  executionEpoch?: Ref<unknown>
}

type ExportImageEntry = {
  url: string
  assetId?: string | undefined
  title: string
  retouchRecipe?: ImageNodeItem['retouchRecipe'] | undefined
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
                assetId: outputs.at(-1)!.mediaRef?.assetId?.trim() || undefined,
                retouchRecipe: snapshot.retouchRecipe,
              }
            : {
                url: snapshot.sourceImageUrl,
                assetId: snapshot.sourceMediaRef?.assetId?.trim() || undefined,
                retouchRecipe: snapshot.retouchRecipe,
              },
        ]
      : outputs.map((item) => ({
          url: item.url,
          assetId: item.mediaRef?.assetId?.trim() || undefined,
        }))
  return entries
    .filter((entry) => !!entry.url?.trim() || !!entry.assetId)
    .map((entry) => ({
      url: entry.url.trim(),
      ...(entry.assetId ? { assetId: entry.assetId } : {}),
      ...(entry.retouchRecipe ? { retouchRecipe: entry.retouchRecipe } : {}),
      title: snapshot.title?.trim() || 'image',
    }))
}

type ExportEntryOutcome = 'downloaded' | 'failed' | 'superseded'

async function downloadExportEntry(
  entry: ExportImageEntry,
  counts: Map<string, number>,
  ports: {
    downloadImage: (args: { url: string; assetId?: string; fileName: string }) => unknown
    isSuperseded: () => boolean
  },
): Promise<ExportEntryOutcome> {
  let renderedUrl = ''
  try {
    let downloadUrl = entry.url
    let exportFileName = ''
    if (entry.retouchRecipe) {
      const source: ImageRetouchRenderSource = {
        url: resolveMediaAssetOriginalURL(entry.retouchRecipe.sourceAssetId),
        title: entry.title,
      }
      const rendered = await renderImageRetouchRecipeFile(source, entry.retouchRecipe)
      if (ports.isSuperseded()) return 'superseded'
      renderedUrl = URL.createObjectURL(rendered)
      downloadUrl = renderedUrl
      exportFileName = buildUniqueExportFileName(
        { ...entry, url: rendered.name, retouchRecipe: undefined },
        counts,
      )
    } else {
      exportFileName = buildUniqueExportFileName(entry, counts)
    }
    await ports.downloadImage({
      url: downloadUrl,
      ...(!entry.retouchRecipe && entry.assetId ? { assetId: entry.assetId } : {}),
      fileName: exportFileName,
    })
    return ports.isSuperseded() ? 'superseded' : 'downloaded'
  } catch {
    return 'failed'
  } finally {
    if (renderedUrl) URL.revokeObjectURL(renderedUrl)
  }
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
    ...(options.executionEpoch ? { executionEpoch: options.executionEpoch } : {}),
  })
  let exportLoading: MessageLoadingHandle | null = null
  let exportRunSequence = 0
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
    image.sourceMediaAssetId = source.sourceMediaAssetId
    image.sourceMediaRef = payload.mediaRef ? { ...payload.mediaRef } : undefined
    // The duplicated node keeps the durable persisted URL; the delivery URL
    // in the payload is a runtime detail and is never persisted.
    const latestOutput = resolveImageNodeLatestOutput(source)
    if (latestOutput?.url?.trim()) image.sourceImageUrl = latestOutput.url.trim()
    image.retouchRecipe = source.retouchRecipe ? structuredClone(source.retouchRecipe) : undefined
    image.retouchStack = source.retouchStack ? structuredClone(source.retouchStack) : undefined
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
    const flowNodeId = `image-${image.id}`
    if (!options.nodes.value.some((node) => node.id === flowNodeId)) {
      console.warn('[Canvas] Graph projection did not reflect appended image node after commit', {
        flowNodeId,
      })
    }
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
    exportRunSequence += 1
    const exportRunId = exportRunSequence
    const loading = options.feedback.operationLoading(
      i18n.global.t('canvas.session.exportingImages', { count: entries.length }),
      { duration: Number.POSITIVE_INFINITY },
    )
    exportLoading = loading
    const counts = new Map<string, number>()
    const isSuperseded = () => exportRunId !== exportRunSequence
    let failed = 0
    try {
      for (const [index, entry] of entries.entries()) {
        // A second export supersedes the first: bail instead of continuing
        // to download into a destroyed loading handle.
        if (isSuperseded()) return
        loading.update?.(
          i18n.global.t('canvas.session.exportingImageProgress', {
            current: index + 1,
            total: entries.length,
          }),
        )
        const outcome = await downloadExportEntry(entry, counts, {
          downloadImage: (args) => options.browser.downloadImage(args),
          isSuperseded,
        })
        if (outcome === 'superseded') return
        if (outcome === 'failed') failed += 1
      }
    } finally {
      // Only the latest run owns the loading handle; a superseded run must
      // not destroy its successor's handle.
      if (!isSuperseded()) {
        loading.destroy()
        if (exportLoading === loading) exportLoading = null
      }
    }
    if (isSuperseded()) return
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
      copied.retouchRecipe = source.retouchRecipe
        ? structuredClone(source.retouchRecipe)
        : undefined
      copied.sourceImageRatio = source.sourceImageRatio > 0 ? source.sourceImageRatio : 1
      copied.outputCount = source.outputCount ?? copied.outputCount
      copied.outputImages = cloneImageOutputs(source)
      const payload = resolveImageNodeDisplayPayload(copied)
      copied.width = resolveImageNodeFrameSize(payload?.ratio ?? 1, !!payload).width
      copied.outputStatus = source.outputStatus
      reset.set(flowNodeId, { x: start.x, y: start.y })
      appendedSnapshots.push(copied)
    })
    if (appendedSnapshots.length === 0) return
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
    const flowNodesById = new Map(options.nodes.value.map((node) => [node.id, node] as const))
    let isProjected = appendedSnapshots.every((snapshot) =>
      flowNodesById.has(`image-${snapshot.id}`),
    )
    if (isProjected) {
      for (const [flowNodeId, position] of reset) {
        const node = flowNodesById.get(flowNodeId)
        if (!node || node.position.x !== position.x || node.position.y !== position.y) {
          isProjected = false
          break
        }
      }
    }
    if (!isProjected) {
      console.warn(
        '[Canvas] Graph projection did not reflect duplicated image nodes after commit',
        {
          nodeIds: appendedSnapshots.map((snapshot) => `image-${snapshot.id}`),
        },
      )
    }
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
