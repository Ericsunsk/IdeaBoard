import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { watch } from 'vue'
import { IMAGE_UPLOAD_ACCEPT } from '../../../shared/constants/media'
import type {
  CanvasNodeItem,
  FusionNodeItem,
  PromptNodeInputImage,
  PromptNodeItem,
} from '../../canvas-board/domain'
import { getCanvasNodeImageInputLimit } from '../../canvas-board/domain'

export type MediaInputImage = {
  id: string
  sourceType: 'node' | 'external'
  sourceId: string
  sourceNodeId?: string
  url: string
  ratio: number
}

type UseFlowMediaInputImagesOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  uploadImageToBoard: (file: File) => Promise<{ url: string; ratio: number }>
  updateSnapshotNodeBase: (
    nodeId: string,
    mutate: (snapshotNode: CanvasNodeItem) => CanvasNodeItem,
    options?: { history?: boolean; dirty?: boolean },
  ) => void
  commitGraphMutation: (
    mutate: (nodes: Node[], edges: Edge[]) => { nodes: Node[]; edges: Edge[] },
  ) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
  getImageNodeDisplayPayload: (nodeId: string) => { url: string; ratio: number } | null
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

export function useFlowMediaInputImages(options: UseFlowMediaInputImagesOptions) {
  watch(
    [options.nodes, options.edges],
    () => {
      const flowNodes = options.nodes.value
      const flowEdges = options.edges.value
      flowNodes
        .filter((node) => node.data?.kind === 'prompt' || node.data?.kind === 'fusion')
        .forEach((mediaInputNode) => {
          const linkedImages = flowEdges
            .filter((edge) => edge.target === mediaInputNode.id && edge.targetHandle === 'in:image')
            .map((edge) => resolveNodePreviewImageById(edge.source))
            .filter((item): item is LinkedNodePreviewImage => !!item && !!item.url)
          const linkedBySourceId = new Map(linkedImages.map((item) => [item.sourceId, item]))
          options.updateSnapshotNodeBase(
            mediaInputNode.id,
            (snapshotNode) => {
              if (!isMediaInputSnapshotNode(snapshotNode)) return snapshotNode
              const current = snapshotNode.inputImages
              const nextImages = current
                .map((item) => {
                  const sourceType = item.sourceType
                  if (sourceType === 'external') return item
                  const sourceId = item.sourceId
                  return sourceId ? linkedBySourceId.get(sourceId) : null
                })
                .filter((item): item is MediaInputImage => !!item)
              const nextNodeSourceIds = new Set(
                nextImages
                  .filter((item) => item.sourceType === 'node')
                  .map((item) => item.sourceId),
              )
              nextImages.push(
                ...linkedImages.filter((item) => !nextNodeSourceIds.has(item.sourceId)),
              )
              const inputLimit = getCanvasNodeImageInputLimit(snapshotNode.type)
              const limitedNextImages = nextImages.slice(0, inputLimit)
              const same =
                current.length === limitedNextImages.length &&
                current.every(
                  (item, idx) =>
                    item.id === limitedNextImages[idx]?.id &&
                    item.sourceType === limitedNextImages[idx]?.sourceType &&
                    item.sourceId === limitedNextImages[idx]?.sourceId &&
                    item.url === limitedNextImages[idx]?.url &&
                    item.ratio === limitedNextImages[idx]?.ratio,
                )
              return same ? snapshotNode : { ...snapshotNode, inputImages: limitedNextImages }
            },
            { history: false, dirty: true },
          )
        })
    },
    { deep: true },
  )

  function resolveNodePreviewImageById(nodeId: string): LinkedNodePreviewImage | null {
    const node = options.nodes.value.find((item) => item.id === nodeId)
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
      }
    }

    const outputs = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
    const latest = outputs[outputs.length - 1] as { url?: string; ratio?: number } | undefined
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
    }
  }

  function resolveMediaInputNodeKind(nodeId: string): MediaInputNodeKind | null {
    const kind = options.nodes.value.find((node) => node.id === nodeId)?.data?.kind
    return isMediaInputNodeKind(kind) ? kind : null
  }

  function resolveMediaInputNodeLabel(kind: MediaInputNodeKind | null) {
    return kind === 'prompt' ? 'Prompt' : '节点'
  }

  async function onFlowMediaInputImageDrop(nodeId: string, event: DragEvent) {
    const kind = resolveMediaInputNodeKind(nodeId)
    if (!kind) return
    const inputLimit = getCanvasNodeImageInputLimit(kind)
    const currentInputCount = getMediaInputImages(nodeId).length
    const label = resolveMediaInputNodeLabel(kind)
    if (currentInputCount >= inputLimit) {
      options.onWarning(`${label} 最多接入 ${inputLimit} 张图片`)
      return
    }
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
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

  function onFlowMediaInputImageOpen(nodeId: string) {
    const kind = resolveMediaInputNodeKind(nodeId)
    if (kind !== 'prompt') return
    const inputLimit = getCanvasNodeImageInputLimit(kind)
    const currentInputCount = getMediaInputImages(nodeId).length
    const label = resolveMediaInputNodeLabel(kind)
    if (currentInputCount >= inputLimit) {
      options.onWarning(`${label} 最多接入 ${inputLimit} 张图片`)
      return
    }
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = IMAGE_UPLOAD_ACCEPT
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      const syntheticDrop = {
        dataTransfer: { files: [file] },
        clientX: window.innerWidth / 2,
        clientY: window.innerHeight / 2,
      } as unknown as DragEvent
      void onFlowMediaInputImageDrop(nodeId, syntheticDrop)
    }
    input.click()
  }

  function onFlowMediaInputImagePreview(image: { url: string; ratio: number }) {
    void options.openImageUrlPreview(image.url, image.ratio)
  }

  function onFlowMediaInputImageRemove(nodeId: string, imageId: string) {
    const currentInputImages = getMediaInputImages(nodeId)
    const removed = currentInputImages.find((item) => item.id === imageId)
    if (!removed) return
    options.commitGraphMutation((currentNodes, currentEdges) => {
      const nextNodes = currentNodes.map((node) => {
        if (node.id !== nodeId || (node.data?.kind !== 'prompt' && node.data?.kind !== 'fusion'))
          return node
        const snapshotNode = node.data.snapshotNode
        if (!snapshotNode || typeof snapshotNode !== 'object') return node
        return {
          ...node,
          data: {
            ...node.data,
            snapshotNode: {
              ...(snapshotNode as Record<string, unknown>),
              inputImages: currentInputImages.filter((item) => item.id !== imageId),
            },
          },
        }
      })
      const nextEdges =
        removed.sourceType === 'node'
          ? currentEdges.filter(
              (edge) =>
                !(
                  edge.source === removed.sourceId &&
                  edge.target === nodeId &&
                  isImageInputHandle(edge.targetHandle)
                ),
            )
          : currentEdges
      return { nodes: nextNodes, edges: nextEdges }
    })
    options.markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
  }

  function isImageInputHandle(handle: string | null | undefined) {
    return String(handle ?? '') === 'in:image'
  }

  function getMediaInputImages(nodeId: string): MediaInputImage[] {
    const mediaInputNode = options.nodes.value.find((node) => node.id === nodeId)
    const snapshot = mediaInputNode?.data?.snapshotNode as
      | {
          inputImages?: Array<{
            id?: unknown
            sourceType?: unknown
            sourceId?: unknown
            sourceNodeId?: unknown
            url?: unknown
            ratio?: unknown
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
      if (!id || !sourceType || !sourceId || !url || !Number.isFinite(ratio) || ratio <= 0) return
      normalized.push({ id, sourceType, sourceId, sourceNodeId, url, ratio })
    })
    return normalized
  }

  function resolvePromptInputImageNodeIds(promptNodeId: string): string[] {
    const ordered = getMediaInputImages(promptNodeId)
      .filter((item) => item.sourceType === 'node')
      .map((item) => item.sourceId)
    const linked = new Set(
      options.edges.value
        .filter((edge) => edge.target === promptNodeId && isImageInputHandle(edge.targetHandle))
        .map((edge) => edge.source),
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
