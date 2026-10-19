import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { watch } from 'vue'
import { i18n } from '../../../../../core/i18n'
import { userFacingErrorCopy } from '../../../../../core/errors/errorCatalog'
import type {
  CanvasNodeItem,
  FusionNodeItem,
  PromptNodeInputImage,
  PromptNodeItem,
} from '../../../../board/ports/canvas'
import { getCanvasNodeImageInputLimit } from '../../../../board/ports/canvas'
import { type ImageNodeDisplayPayload } from '../../../session/flow-adapter'
import {
  type CanvasGraphEdgeRecord,
  type CanvasGraphNodeRecord,
  type CanvasGraphReadPort,
  type CanvasGraphStore,
  type CanvasGraphTransactionOptions,
} from '../../../core/graph'
import type { MediaRef } from '../../../../../core/media/mediaRef'
import { createPrefixedId } from '../../../../../core/utils/ids'
import { resolveWritableCanvasGraphStore } from '../../../core/graph/canvasGraphStoreWriter'

export type MediaInputImage = {
  id: string
  sourceType: 'node' | 'external'
  sourceId: string
  sourceNodeId?: string | undefined
  url: string
  ratio: number
  mediaRef?: MediaRef | undefined
}

type UseFlowMediaInputImagesOptions = {
  browser: {
    pickImageFile: () => Promise<File | null>
    readDroppedFile: (event: DragEvent) => File | null
  }
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  uploadImageToBoard: (
    file: File,
  ) => Promise<{ url: string; ratio: number; assetId: string; mediaRef: MediaRef }>
  updateSnapshotNodeBase: (
    nodeId: string,
    mutate: (snapshotNode: CanvasNodeItem) => CanvasNodeItem,
    options?: CanvasGraphTransactionOptions,
  ) => void
  readCanvasGraphStore: () => CanvasGraphReadPort
  writeCanvasGraphStore?: () => CanvasGraphStore
  markFlowExecutionNodeDirty: (nodeId: string, options?: { shouldIncludeSelf?: boolean }) => void
  getImageNodeDisplayPayload: (nodeId: string) => ImageNodeDisplayPayload | null
  openImageUrlPreview: (src: string, ratio?: number, assetId?: string) => Promise<void>
  onWarning: (message: string) => void
}

type LinkedNodePreviewImage = {
  id: string
  sourceType: 'node'
  sourceId: string
  sourceNodeId: string
  url: string
  ratio: number
  mediaRef?: MediaRef
}

type MediaInputSnapshotNode = PromptNodeItem | FusionNodeItem
type MediaInputNodeKind = MediaInputSnapshotNode['type']
function isMediaInputSnapshotNode(
  snapshotNode: CanvasNodeItem,
): snapshotNode is MediaInputSnapshotNode {
  return snapshotNode.type === 'prompt' || snapshotNode.type === 'fusion'
}

function isMediaInputNodeKind(value: unknown): value is MediaInputNodeKind {
  return value === 'prompt' || value === 'fusion'
}

function createNodeById(store: CanvasGraphReadPort) {
  return new Map(
    store.getNodeIds().map((nodeId) => [nodeId, store.getNode(nodeId)] as const),
  ) as Map<string, CanvasGraphNodeRecord | undefined>
}

function createImageInputEdgesByTargetId(store: CanvasGraphReadPort) {
  const result = new Map<string, CanvasGraphEdgeRecord[]>()
  store.getEdgeIds().forEach((edgeId) => {
    const edge = store.getEdge(edgeId)
    if (!edge || edge.connection.toPort !== 'image') return
    const target = `${edge.connection.toNodeType}-${edge.connection.toNodeId}`
    const existing = result.get(target)
    if (existing) {
      existing.push(edge)
      return
    }
    result.set(target, [edge])
  })
  return result
}

function excerptMediaSyncSnapshot(snapshot: unknown): string {
  if (!snapshot || typeof snapshot !== 'object') return ''
  const record = snapshot as Record<string, unknown> & { type?: unknown }
  if (record.type === 'image-node') {
    const typed = record as unknown as {
      sourceImageUrl?: unknown
      sourceMediaRef?: unknown
      sourceImageRatio?: unknown
      outputImages?: unknown
    }
    return JSON.stringify({
      type: 'image-node',
      sourceImageUrl: typeof typed.sourceImageUrl === 'string' ? typed.sourceImageUrl : '',
      sourceMediaRef: typed.sourceMediaRef ?? null,
      sourceImageRatio: typed.sourceImageRatio ?? 1,
      outputImages: Array.isArray(typed.outputImages) ? typed.outputImages : [],
    })
  }
  if (record.type === 'prompt' || record.type === 'fusion') {
    const typed = record as unknown as { inputImages?: unknown; outputImages?: unknown }
    return JSON.stringify({
      type: record.type,
      inputImages: Array.isArray(typed.inputImages) ? typed.inputImages : [],
      outputImages: Array.isArray(typed.outputImages) ? typed.outputImages : [],
    })
  }
  return JSON.stringify({ type: typeof record.type === 'string' ? record.type : '' })
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
        item.mediaRef?.assetId === next[idx]?.mediaRef?.assetId,
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
  const writeGraphStore = () =>
    resolveWritableCanvasGraphStore(options.readCanvasGraphStore, options.writeCanvasGraphStore)
  watch(createProjectionFactSignature, syncLinkedMediaInputs)

  function createProjectionFactSignature() {
    const nodeParts = options.nodes.value.map((node) => {
      const snapshot = node.data?.snapshotNode
      if (!snapshot || typeof snapshot !== 'object') return node.id
      return `${node.id}:${excerptMediaSyncSnapshot(snapshot)}`
    })
    const edgeParts = options.edges.value
      .filter(
        (edge) =>
          (edge.targetHandle ?? '').includes('image') ||
          (edge.sourceHandle ?? '').includes('image'),
      )
      .map(
        (edge) =>
          `${edge.id}:${edge.source}:${edge.sourceHandle ?? ''}:${edge.target}:${edge.targetHandle ?? ''}`,
      )
    return [...nodeParts, ...edgeParts].sort().join('|')
  }

  function syncLinkedMediaInputs() {
    const store = options.readCanvasGraphStore()
    const nodeById = createNodeById(store)
    const imageInputEdgesByTargetId = createImageInputEdgesByTargetId(store)
    store.getNodeIds().forEach((nodeId) => {
      const record = nodeById.get(nodeId) ?? store.getNode(nodeId)
      if (!record || (record.kind !== 'prompt' && record.kind !== 'fusion')) return
      if (!isMediaInputSnapshotNode(record.snapshot)) return
      const linkedImages = resolveLinkedImagesForTargetWithMaps(
        nodeId,
        nodeById,
        imageInputEdgesByTargetId,
      )
      const current = record.snapshot.inputImages
      const nextImages = createSyncedMediaInputImages(current, linkedImages, record.snapshot.type)
      if (areMediaInputImagesEqual(current, nextImages)) return

      options.updateSnapshotNodeBase(
        nodeId,
        (snapshotNode) => {
          if (!isMediaInputSnapshotNode(snapshotNode)) return snapshotNode
          const current = snapshotNode.inputImages
          const nextImages = createSyncedMediaInputImages(current, linkedImages, snapshotNode.type)
          return areMediaInputImagesEqual(current, nextImages)
            ? snapshotNode
            : { ...snapshotNode, inputImages: nextImages }
        },
        { origin: 'system', shouldRecordHistory: false, shouldMarkDirty: true },
      )
    })
  }

  function resolveLinkedImagesForTargetWithMaps(
    targetNodeId: string,
    nodeById: Map<string, CanvasGraphNodeRecord | undefined>,
    imageInputEdgesByTargetId: Map<string, CanvasGraphEdgeRecord[]>,
  ): LinkedNodePreviewImage[] {
    return (imageInputEdgesByTargetId.get(targetNodeId) ?? [])
      .map((edge) =>
        resolveNodePreviewImageWithMaps(
          `${edge.connection.fromNodeType}-${edge.connection.fromNodeId}`,
          nodeById,
        ),
      )
      .filter((item): item is LinkedNodePreviewImage => !!item && !!item.url)
  }

  function resolveImageNodePreview(nodeId: string): LinkedNodePreviewImage | null {
    const display = options.getImageNodeDisplayPayload(nodeId)
    if (!display?.url) return null
    return {
      id: `node:${nodeId}`,
      sourceType: 'node',
      sourceId: nodeId,
      sourceNodeId: nodeId,
      url: display.url,
      ratio: Number.isFinite(display.ratio) && display.ratio > 0 ? display.ratio : 1,
      ...(display.mediaRef ? { mediaRef: display.mediaRef } : {}),
    }
  }

  function resolveOutputNodePreview(
    nodeId: string,
    snapshot: Record<string, unknown>,
  ): LinkedNodePreviewImage | null {
    const outputs = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
    const latest = outputs[outputs.length - 1] as
      { url?: string; ratio?: number; mediaRef?: MediaRef } | undefined
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
      ...(latest?.mediaRef ? { mediaRef: latest.mediaRef } : {}),
    }
  }

  function resolveNodePreviewImageWithMaps(
    nodeId: string,
    nodeById: Map<string, CanvasGraphNodeRecord | undefined>,
  ): LinkedNodePreviewImage | null {
    const record = nodeById.get(nodeId)
    const kind = record?.kind
    const snapshot = record?.snapshot as Record<string, unknown> | undefined
    if (!record || !kind || !snapshot) return null

    if (kind === 'image') {
      return resolveImageNodePreview(nodeId)
    }

    return resolveOutputNodePreview(nodeId, snapshot)
  }

  function resolveMediaInputNodeKind(nodeId: string): MediaInputNodeKind | null {
    const kind = createNodeById(options.readCanvasGraphStore()).get(nodeId)?.kind
    return isMediaInputNodeKind(kind) ? kind : null
  }

  function resolveMediaInputNodeLabel(kind: MediaInputNodeKind | null) {
    return kind === 'prompt' ? 'Prompt' : i18n.global.t('canvas.session.mediaInputNodeLabel')
  }

  async function addMediaInputImageFile(nodeId: string, file: File) {
    const kind = resolveMediaInputNodeKind(nodeId)
    if (!kind) return
    const inputLimit = getCanvasNodeImageInputLimit(kind)
    const currentInputCount = getMediaInputImages(nodeId).length
    const label = resolveMediaInputNodeLabel(kind)
    if (currentInputCount >= inputLimit) {
      options.onWarning(
        i18n.global.t('canvas.session.inputLimitExceeded', { label, count: inputLimit }),
      )
      return
    }
    try {
      const uploaded = await options.uploadImageToBoard(file)
      options.updateSnapshotNodeBase(nodeId, (snapshotNode) => {
        if (!isMediaInputSnapshotNode(snapshotNode)) return snapshotNode
        const current = snapshotNode.inputImages
        const snapshotInputLimit = getCanvasNodeImageInputLimit(snapshotNode.type)
        const externalImage: PromptNodeInputImage = {
          id: createPrefixedId('external', ':'),
          sourceType: 'external',
          sourceId: uploaded.assetId,
          url: uploaded.url,
          ratio: uploaded.ratio,
          mediaRef: uploaded.mediaRef,
        }
        return {
          ...snapshotNode,
          inputImages: [...current, externalImage].slice(0, snapshotInputLimit),
        }
      })
      options.markFlowExecutionNodeDirty(nodeId, { shouldIncludeSelf: true })
    } catch (error) {
      const msg = userFacingErrorCopy(error, i18n.global.t('canvas.session.uploadFailed'))
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
      options.onWarning(
        i18n.global.t('canvas.session.inputLimitExceeded', { label, count: inputLimit }),
      )
      return
    }
    const file = await options.browser.pickImageFile()
    if (file) await addMediaInputImageFile(nodeId, file)
  }

  function onFlowMediaInputImagePreview(image: {
    url: string
    ratio: number
    mediaRef?: MediaRef
  }) {
    void options.openImageUrlPreview(image.url, image.ratio, image.mediaRef?.assetId)
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
    const transaction = writeGraphStore().beginTransaction('node-update')
    try {
      transaction.updateNode(target.id, { inputImages: nextInputImages })
      edgeIds.forEach((edgeId) => transaction.removeEdge(edgeId))
      transaction.commit()
    } catch (error) {
      transaction.cancel()
      console.error('[Canvas] Failed to remove media input', error)
      options.onWarning(i18n.global.t('canvas.session.removeInputImageFailed'))
      return
    }
    if (
      !isFlowProjectionInSync(
        target.id,
        nextInputImages.map((item) => item.id),
        edgeIds,
      )
    ) {
      console.warn('[Canvas] Graph projection did not reflect removed media input after commit', {
        targetNodeId: target.id,
        removedEdgeIds: edgeIds,
      })
    }
    options.markFlowExecutionNodeDirty(nodeId, { shouldIncludeSelf: true })
  }

  function isFlowProjectionInSync(
    targetNodeId: string,
    nextInputIds: readonly string[],
    removedEdgeIds: readonly string[],
  ): boolean {
    const store = options.readCanvasGraphStore()
    const flowNodeIds = new Set(options.nodes.value.map((node) => node.id))
    if (store.getNodeIds().some((nodeId) => !flowNodeIds.has(nodeId))) return false
    const target = options.nodes.value.find((node) => node.id === targetNodeId)
    const snapshotNode = (
      target?.data as { snapshotNode?: { inputImages?: Array<{ id?: unknown }> } } | undefined
    )?.snapshotNode
    const flowInputIds = Array.isArray(snapshotNode?.inputImages)
      ? snapshotNode.inputImages.map((item) => item.id)
      : null
    if (
      !flowInputIds ||
      flowInputIds.length !== nextInputIds.length ||
      flowInputIds.some((id, index) => id !== nextInputIds[index])
    ) {
      return false
    }
    const flowEdgeIds = new Set(options.edges.value.map((edge) => edge.id))
    return removedEdgeIds.every((edgeId) => !flowEdgeIds.has(edgeId))
  }

  function getMediaInputImages(nodeId: string): MediaInputImage[] {
    const mediaInputNode = createNodeById(options.readCanvasGraphStore()).get(nodeId)
    const snapshot = mediaInputNode?.snapshot as
      | {
          inputImages?: Array<{
            id?: unknown
            sourceType?: unknown
            sourceId?: unknown
            sourceNodeId?: unknown
            url?: unknown
            ratio?: unknown
            mediaRef?: unknown
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
      const mediaRef = item.mediaRef as MediaRef | undefined
      if (!id || !sourceType || !sourceId || !url || !Number.isFinite(ratio) || ratio <= 0) return
      normalized.push({
        id,
        sourceType,
        sourceId,
        sourceNodeId,
        url,
        ratio,
        ...(mediaRef?.assetId ? { mediaRef } : {}),
      })
    })
    return normalized
  }

  function resolvePromptInputImageNodeIds(promptNodeId: string): string[] {
    const ordered = getMediaInputImages(promptNodeId)
      .filter((item) => item.sourceType === 'node')
      .map((item) => item.sourceId)
    const imageInputEdgesByTargetId = createImageInputEdgesByTargetId(
      options.readCanvasGraphStore(),
    )
    const linked = new Set(
      (imageInputEdgesByTargetId.get(promptNodeId) ?? []).map(
        (edge) => `${edge.connection.fromNodeType}-${edge.connection.fromNodeId}`,
      ),
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
