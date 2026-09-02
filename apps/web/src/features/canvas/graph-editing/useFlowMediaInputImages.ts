import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { watch } from 'vue'
import { i18n } from '../../../shared/i18n'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import type {
  CanvasNodeItem,
  FusionNodeItem,
  PromptNodeInputImage,
  PromptNodeItem,
} from '../../canvas-board/domain'
import { getCanvasNodeImageInputLimit } from '../../canvas-board/domain'
import { type ImageNodeDisplayPayload } from '../utils/flowImageNodeSelectors'
import type { CanvasVueFlowGraphAdapter } from '../vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from '../vue-flow-adapter/flowGraphAdapterCommandSideEffects'
import {
  type CanvasGraphEdgeRecord,
  type CanvasGraphNodeRecord,
  type CanvasGraphReadPort,
  type CanvasGraphStore,
} from '../graph-runtime'
import { canvasBoardSnapshotToFlow } from '../utils/flowSnapshotAdapter'
import type { MediaRef } from '../../../shared/media/MediaRef'

export type MediaInputImage = {
  id: string
  sourceType: 'node' | 'external'
  sourceId: string
  sourceNodeId?: string
  url: string
  ratio: number
  mediaRef?: MediaRef
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
    options?: { history?: boolean; dirty?: boolean },
  ) => void
  readCanvasGraphStore: () => CanvasGraphReadPort
  writeCanvasGraphStore?: () => CanvasGraphStore
  graphAdapter: CanvasVueFlowGraphAdapter
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
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
    options.writeCanvasGraphStore?.() ?? (options.readCanvasGraphStore() as CanvasGraphStore)
  watch(createProjectionFactSignature, syncLinkedMediaInputs)

  function createProjectionFactSignature() {
    const nodeParts = options.nodes.value.map((node) => {
      const snapshot = node.data?.snapshotNode
      if (!snapshot || typeof snapshot !== 'object') return node.id
      const { x: _x, y: _y, ...domain } = snapshot as Record<string, unknown>
      return `${node.id}:${JSON.stringify(domain)}`
    })
    const edgeParts = options.edges.value.map(
      (edge) =>
        `${edge.id}:${edge.source}:${edge.sourceHandle ?? ''}:${edge.target}:${edge.targetHandle ?? ''}`,
    )
    return [...nodeParts, ...edgeParts].sort().join('|')
  }

  function syncLinkedMediaInputs() {
    const store = options.readCanvasGraphStore()
    store.getNodeIds().forEach((nodeId) => {
      const record = store.getNode(nodeId)
      if (!record || (record.kind !== 'prompt' && record.kind !== 'fusion')) return
      if (!isMediaInputSnapshotNode(record.snapshot)) return
      const linkedImages = resolveLinkedImagesForTarget(nodeId)
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
        { history: false, dirty: true },
      )
    })
  }

  function resolveLinkedImagesForTarget(targetNodeId: string): LinkedNodePreviewImage[] {
    const imageInputEdgesByTargetId = createImageInputEdgesByTargetId(
      options.readCanvasGraphStore(),
    )
    return (imageInputEdgesByTargetId.get(targetNodeId) ?? [])
      .map((edge) =>
        resolveNodePreviewImageById(
          `${edge.connection.fromNodeType}-${edge.connection.fromNodeId}`,
        ),
      )
      .filter((item): item is LinkedNodePreviewImage => !!item && !!item.url)
  }

  function resolveNodePreviewImageById(nodeId: string): LinkedNodePreviewImage | null {
    const record = createNodeById(options.readCanvasGraphStore()).get(nodeId)
    const kind = record?.kind
    const snapshot = record?.snapshot as Record<string, unknown> | undefined
    if (!record || !kind || !snapshot) return null

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
        ...(display.mediaRef ? { mediaRef: display.mediaRef } : {}),
      }
    }

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
          id: `external:${Date.now()}:${Math.random().toString(16).slice(2, 8)}`,
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
      options.markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
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
    const projection = canvasBoardSnapshotToFlow(writeGraphStore().exportSnapshot())
    options.graphAdapter.replaceGraph(
      { nodes: projection.nodes as Node[], edges: projection.edges as Edge[] },
      createFlowGraphCommandOptions(),
    )
    options.markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
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
