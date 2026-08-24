import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { computed, watch } from 'vue'
import type {
  CanvasNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  PromptNodeInputImage,
  PromptNodeItem,
} from '../../canvas-board/domain'
import { getCanvasNodeImageInputLimit } from '../../canvas-board/domain'
import {
  resolveImageNodeDisplayPayload,
  type ImageNodeDisplayPayload,
} from '../utils/flowImageNodeSelectors'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from './flowGraphAdapterCommandSideEffects'
import type { FlowHistorySnapshot } from './useFlowHistory'
import {
  type CanvasGraphStore,
} from '../graph-runtime'
import { canvasBoardSnapshotToFlow } from '../utils/flowSnapshotAdapter'

export type MediaInputImage = {
  id: string
  sourceType: 'node' | 'external'
  sourceId: string
  sourceNodeId?: string
  url: string
  ratio: number
  mediaAssetId?: number
}

type UseFlowMediaInputImagesOptions = {
  browser: {
    pickImageFile: () => Promise<File | null>
    readDroppedFile: (event: DragEvent) => File | null
  }
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  uploadImageToBoard: (file: File) => Promise<{ url: string; ratio: number; mediaAssetId: number }>
  updateSnapshotNodeBase: (
    nodeId: string,
    mutate: (snapshotNode: CanvasNodeItem) => CanvasNodeItem,
    options?: { history?: boolean; dirty?: boolean },
  ) => void
  readCanvasGraphStore: () => CanvasGraphStore
  captureHistory: () => FlowHistorySnapshot
  graphAdapter: CanvasVueFlowGraphAdapter
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
  getImageNodeDisplayPayload: (nodeId: string) => ImageNodeDisplayPayload | null
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  onWarning: (message: string) => void
}

type LinkedNodePreviewImage = {
  id: string
  sourceType: 'node'
  sourceId: string
  sourceNodeId: string
  url: string
  ratio: number
  mediaAssetId?: number
}

type MediaInputSnapshotNode = PromptNodeItem | FusionNodeItem
type MediaInputNodeKind = MediaInputSnapshotNode['type']
type FlowMediaInputNode = Node & {
  data?: {
    kind?: string
    snapshotNode?: unknown
  }
}

function isMediaInputSnapshotNode(
  snapshotNode: CanvasNodeItem,
): snapshotNode is MediaInputSnapshotNode {
  return snapshotNode.type === 'prompt' || snapshotNode.type === 'fusion'
}

function isMediaInputNodeKind(value: unknown): value is MediaInputNodeKind {
  return value === 'prompt' || value === 'fusion'
}

function isMediaInputFlowNode(node: FlowMediaInputNode): boolean {
  return node.data?.kind === 'prompt' || node.data?.kind === 'fusion'
}

function isImageInputHandle(handle: string | null | undefined) {
  return String(handle ?? '') === 'in:image'
}

function createNodeById(nodes: FlowMediaInputNode[]) {
  return new Map(nodes.map((node) => [node.id, node]))
}

function createImageInputEdgesByTargetId(edges: Edge[]) {
  const result = new Map<string, Edge[]>()
  edges.forEach((edge) => {
    if (!isImageInputHandle(edge.targetHandle)) return
    const existing = result.get(edge.target)
    if (existing) {
      existing.push(edge)
      return
    }
    result.set(edge.target, [edge])
  })
  return result
}

function createMediaInputSnapshotSignature(snapshotNode: unknown) {
  if (!snapshotNode || typeof snapshotNode !== 'object') return ''
  const record = snapshotNode as { inputImages?: unknown }
  if (!Array.isArray(record.inputImages)) return ''
  return record.inputImages
    .map((item) => {
      const input = item as {
        id?: unknown
        sourceType?: unknown
        sourceId?: unknown
        url?: unknown
        ratio?: unknown
        mediaAssetId?: unknown
      }
      return [
        input.id,
        input.sourceType,
        input.sourceId,
        input.url,
        input.ratio,
        input.mediaAssetId,
      ].join(',')
    })
    .join(';')
}

function createSourceImageSignature(node: FlowMediaInputNode) {
  const kind = node.data?.kind
  const snapshot = node.data?.snapshotNode
  if (!snapshot || typeof snapshot !== 'object') return ''
  if (kind === 'image') {
    const display = resolveImageNodeDisplayPayload(snapshot as ImageNodeItem)
    return display ? `${display.url},${display.ratio},${display.mediaAssetId ?? ''}` : ''
  }
  const outputImages = Array.isArray((snapshot as { outputImages?: unknown }).outputImages)
    ? (
        snapshot as {
          outputImages: Array<{ url?: unknown; ratio?: unknown; mediaAssetId?: unknown }>
        }
      ).outputImages
    : []
  const latest = outputImages[outputImages.length - 1]
  return latest ? `${latest.url ?? ''},${latest.ratio ?? ''},${latest.mediaAssetId ?? ''}` : ''
}

function areMediaInputImagesEqual(current: MediaInputImage[], next: MediaInputImage[]) {
  return (
    current.length === next.length &&
    current.every(
      (item, idx) =>
        item.id === next[idx]?.id &&
        item.sourceType === next[idx]?.sourceType &&
        item.sourceId === next[idx]?.sourceId &&
        item.url === next[idx]?.url &&
        item.ratio === next[idx]?.ratio &&
        item.mediaAssetId === next[idx]?.mediaAssetId,
    )
  )
}

function createSyncedMediaInputImages(
  current: MediaInputImage[],
  linkedImages: LinkedNodePreviewImage[],
  nodeType: MediaInputNodeKind,
) {
  const linkedBySourceId = new Map(linkedImages.map((item) => [item.sourceId, item]))
  const nextImages = current
    .map((item) => {
      const sourceType = item.sourceType
      if (sourceType === 'external') return item
      const sourceId = item.sourceId
      return sourceId ? linkedBySourceId.get(sourceId) : null
    })
    .filter((item): item is MediaInputImage => !!item)
  const nextNodeSourceIds = new Set(
    nextImages.filter((item) => item.sourceType === 'node').map((item) => item.sourceId),
  )
  nextImages.push(...linkedImages.filter((item) => !nextNodeSourceIds.has(item.sourceId)))
  const inputLimit = getCanvasNodeImageInputLimit(nodeType)
  return nextImages.slice(0, inputLimit)
}

export function useFlowMediaInputImages(options: UseFlowMediaInputImagesOptions) {
  const nodeById = computed(() => createNodeById(options.nodes.value as FlowMediaInputNode[]))
  const imageInputEdgesByTargetId = computed(() =>
    createImageInputEdgesByTargetId(options.edges.value),
  )

  watch(createMediaInputSyncSignature, syncLinkedMediaInputs)

  function createMediaInputSyncSignature() {
    const flowNodes = options.nodes.value as FlowMediaInputNode[]
    const sourceIds = new Set<string>()
    const parts: string[] = []

    options.edges.value.forEach((edge) => {
      if (!isImageInputHandle(edge.targetHandle)) return
      sourceIds.add(edge.source)
      parts.push(`edge:${edge.source}>${edge.target}`)
    })

    flowNodes.forEach((node) => {
      if (isMediaInputFlowNode(node)) {
        parts.push(
          `target:${node.id}:${createMediaInputSnapshotSignature(node.data?.snapshotNode)}`,
        )
        return
      }
      if (sourceIds.has(node.id)) {
        parts.push(`source:${node.id}:${createSourceImageSignature(node)}`)
      }
    })

    return parts.sort().join('|')
  }

  function syncLinkedMediaInputs() {
    const flowNodes = options.nodes.value as FlowMediaInputNode[]
    flowNodes.filter(isMediaInputFlowNode).forEach((mediaInputNode) => {
      const currentSnapshotNode = mediaInputNode.data?.snapshotNode
      if (!currentSnapshotNode || typeof currentSnapshotNode !== 'object') return
      if (!isMediaInputSnapshotNode(currentSnapshotNode as CanvasNodeItem)) return
      const linkedImages = resolveLinkedImagesForTarget(mediaInputNode.id)
      const current = currentSnapshotNode.inputImages
      const nextImages = createSyncedMediaInputImages(
        current,
        linkedImages,
        currentSnapshotNode.type,
      )
      if (areMediaInputImagesEqual(current, nextImages)) return

      options.updateSnapshotNodeBase(
        mediaInputNode.id,
        (snapshotNode) => {
          if (!isMediaInputSnapshotNode(snapshotNode)) return snapshotNode
          const current = snapshotNode.inputImages
          const nextImages = createSyncedMediaInputImages(current, linkedImages, snapshotNode.type)
          return areMediaInputImagesEqual(current, nextImages)
            ? snapshotNode
            : { ...snapshotNode, inputImages: nextImages }
        },
        { history: false, dirty: true },
      )
    })
  }

  function resolveLinkedImagesForTarget(targetNodeId: string): LinkedNodePreviewImage[] {
    return (imageInputEdgesByTargetId.value.get(targetNodeId) ?? [])
      .map((edge) => resolveNodePreviewImageById(edge.source))
      .filter((item): item is LinkedNodePreviewImage => !!item && !!item.url)
  }

  function resolveNodePreviewImageById(nodeId: string): LinkedNodePreviewImage | null {
    const node = nodeById.value.get(nodeId)
    const kind = node?.data?.kind
    const snapshot = node?.data?.snapshotNode as Record<string, unknown> | undefined
    if (!kind || !snapshot) return null

    if (kind === 'image') {
      const display = options.getImageNodeDisplayPayload(nodeId)
      if (!display?.url) return null
      return {
        id: `node:${nodeId}`,
        sourceType: 'node',
        sourceId: nodeId,
        sourceNodeId: nodeId,
        url: display.url,
        ratio: Number.isFinite(display.ratio) && display.ratio > 0 ? display.ratio : 1,
        mediaAssetId: display.mediaAssetId,
      }
    }

    const outputs = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
    const latest = outputs[outputs.length - 1] as
      { url?: string; ratio?: number; mediaAssetId?: number } | undefined
    const url = typeof latest?.url === 'string' ? latest.url.trim() : ''
    if (!url) return null
    const ratio = Number(latest?.ratio)
    return {
      id: `node:${nodeId}`,
      sourceType: 'node',
      sourceId: nodeId,
      sourceNodeId: nodeId,
      url,
      ratio: Number.isFinite(ratio) && ratio > 0 ? ratio : 1,
      mediaAssetId: latest?.mediaAssetId,
    }
  }

  function resolveMediaInputNodeKind(nodeId: string): MediaInputNodeKind | null {
    const kind = nodeById.value.get(nodeId)?.data?.kind
    return isMediaInputNodeKind(kind) ? kind : null
  }

  function resolveMediaInputNodeLabel(kind: MediaInputNodeKind | null) {
    return kind === 'prompt' ? 'Prompt' : '节点'
  }

  async function addMediaInputImageFile(nodeId: string, file: File) {
    const kind = resolveMediaInputNodeKind(nodeId)
    if (!kind) return
    const inputLimit = getCanvasNodeImageInputLimit(kind)
    const currentInputCount = getMediaInputImages(nodeId).length
    const label = resolveMediaInputNodeLabel(kind)
    if (currentInputCount >= inputLimit) {
      options.onWarning(`${label} 最多接入 ${inputLimit} 张图片`)
      return
    }
    try {
      const uploaded = await options.uploadImageToBoard(file)
      options.updateSnapshotNodeBase(nodeId, (snapshotNode) => {
        if (!isMediaInputSnapshotNode(snapshotNode)) return snapshotNode
        const current = snapshotNode.inputImages
        const snapshotInputLimit = getCanvasNodeImageInputLimit(snapshotNode.type)
        const externalImage: PromptNodeInputImage = {
          id: `external:${Date.now()}:${Math.random().toString(16).slice(2, 8)}`,
          sourceType: 'external',
          sourceId: uploaded.url,
          url: uploaded.url,
          ratio: uploaded.ratio,
          mediaAssetId: uploaded.mediaAssetId,
        }
        return {
          ...snapshotNode,
          inputImages: [...current, externalImage].slice(0, snapshotInputLimit),
        }
      })
      options.markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
    }
  }

  async function onFlowMediaInputImageDrop(nodeId: string, event: DragEvent) {
    const file = options.browser.readDroppedFile(event)
    if (file) await addMediaInputImageFile(nodeId, file)
  }

  async function onFlowMediaInputImageOpen(nodeId: string) {
    const kind = resolveMediaInputNodeKind(nodeId)
    if (kind !== 'prompt') return
    const inputLimit = getCanvasNodeImageInputLimit(kind)
    const currentInputCount = getMediaInputImages(nodeId).length
    const label = resolveMediaInputNodeLabel(kind)
    if (currentInputCount >= inputLimit) {
      options.onWarning(`${label} 最多接入 ${inputLimit} 张图片`)
      return
    }
    const file = await options.browser.pickImageFile()
    if (file) await addMediaInputImageFile(nodeId, file)
  }

  function onFlowMediaInputImagePreview(image: { url: string; ratio: number }) {
    void options.openImageUrlPreview(image.url, image.ratio)
  }

  function onFlowMediaInputImageRemove(nodeId: string, imageId: string) {
    const store = options.readCanvasGraphStore()
    const target = store.getNode(nodeId as `prompt-${number}` | `fusion-${number}`)
    if (!target || !isMediaInputSnapshotNode(target.snapshot)) return
    const currentInputImages = target.snapshot.inputImages
    const removed = currentInputImages.find((item) => item.id === imageId)
    if (!removed) return
    const nextInputImages = currentInputImages.filter((item) => item.id !== imageId)
    const edgeIds =
      removed.sourceType === 'node'
        ? store.getIncomingEdgeIds(target.id).filter((edgeId) => {
            const connection = store.getEdge(edgeId)?.connection
            return (
              connection?.toPort === 'image' &&
              `${connection.fromNodeType}-${connection.fromNodeId}` === removed.sourceId
            )
          })
        : []
    const historyBefore = options.captureHistory()
    const transaction = store.beginTransaction('node-update')
    try {
      transaction.updateNode(target.id, { inputImages: nextInputImages })
      edgeIds.forEach((edgeId) => transaction.removeEdge(edgeId))
      transaction.commit()
    } catch (error) {
      transaction.cancel()
      console.error('[Canvas] Failed to remove media input', error)
      options.onWarning('移除输入图片失败，请刷新画布后重试')
      return
    }
    const projection = canvasBoardSnapshotToFlow(store.exportSnapshot())
    options.graphAdapter.replaceGraph(
      { nodes: projection.nodes as Node[], edges: projection.edges as Edge[] },
      createFlowGraphCommandOptions({ historyBefore }),
    )
    options.markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
  }

  function getMediaInputImages(nodeId: string): MediaInputImage[] {
    const mediaInputNode = nodeById.value.get(nodeId)
    const snapshot = mediaInputNode?.data?.snapshotNode as
      | {
          inputImages?: Array<{
            id?: unknown
            sourceType?: unknown
            sourceId?: unknown
            sourceNodeId?: unknown
            url?: unknown
            ratio?: unknown
            mediaAssetId?: unknown
          }>
        }
      | undefined
    if (!Array.isArray(snapshot?.inputImages)) return []
    const normalized: MediaInputImage[] = []
    snapshot.inputImages.forEach((item) => {
      const id = typeof item.id === 'string' ? item.id : ''
      const sourceType =
        item.sourceType === 'node' || item.sourceType === 'external' ? item.sourceType : null
      const sourceId = typeof item.sourceId === 'string' ? item.sourceId : ''
      const sourceNodeId = typeof item.sourceNodeId === 'string' ? item.sourceNodeId : undefined
      const url = typeof item.url === 'string' ? item.url.trim() : ''
      const ratio = Number(item.ratio)
      const mediaAssetId = Number(item.mediaAssetId)
      if (!id || !sourceType || !sourceId || !url || !Number.isFinite(ratio) || ratio <= 0) return
      normalized.push({
        id,
        sourceType,
        sourceId,
        sourceNodeId,
        url,
        ratio,
        ...(Number.isInteger(mediaAssetId) && mediaAssetId > 0 ? { mediaAssetId } : {}),
      })
    })
    return normalized
  }

  function resolvePromptInputImageNodeIds(promptNodeId: string): string[] {
    const ordered = getMediaInputImages(promptNodeId)
      .filter((item) => item.sourceType === 'node')
      .map((item) => item.sourceId)
    const linked = new Set(
      (imageInputEdgesByTargetId.value.get(promptNodeId) ?? []).map((edge) => edge.source),
    )
    return ordered.length
      ? ordered.filter((sourceNodeId) => linked.has(sourceNodeId))
      : Array.from(linked)
  }

  return {
    onFlowMediaInputImageDrop,
    onFlowMediaInputImageOpen,
    onFlowMediaInputImagePreview,
    onFlowMediaInputImageRemove,
    getMediaInputImages,
    resolvePromptInputImageNodeIds,
  }
}
