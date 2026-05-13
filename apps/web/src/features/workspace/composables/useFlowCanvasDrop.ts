import type { FlowCreatableNodeKind } from './useFlowNodeMutations'
import type { ImageNodeItem, MaskNodeItem } from '../types/canvasNodes'
import { loadImageSizeFromUrl } from './useFlowGenerationHandlers'

const FLOW_NODE_KIND_DRAG_MIME = 'application/x-ideaboard-flow-node-kind'

type UploadImageResult = { url: string; ratio: number }

type UseFlowCanvasDropOptions = {
  currentBoardId: { value: string | null | undefined }
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  createNodeAtPosition: (
    kind: FlowCreatableNodeKind,
    x: number,
    y: number,
    initialSnapshotPatch?: Record<string, unknown>,
  ) => string | null
  updateSnapshotNode: (
    nodeId: string,
    patcher: (prev: Record<string, unknown>) => Record<string, unknown>,
  ) => void
  getImageNodeSnapshot: (nodeId: string) => ImageNodeItem | null
  getMaskNodeSnapshot: (nodeId: string) => MaskNodeItem | null
  uploadApi: {
    upload: (
      file: File,
      boardId: string,
    ) => Promise<{ code: number; msg?: string; data?: { url?: string } }>
  }
  sanitizeApiErrorMessage: (msg: string | null | undefined, fallback: string) => string
  renderMaskNodeMask: (nodeId: string) => void
  clearMaskCanvas: (nodeId: string) => void
  onWarning: (msg: string) => void
  onSuccess: (msg: string) => void
  onNodeMutated?: (nodeId: string) => void
  createNodeOptions: ReadonlyArray<{ value?: unknown }>
}

export function useFlowCanvasDrop(options: UseFlowCanvasDropOptions) {
  const uploadCache = new Map<string, Promise<UploadImageResult>>()

  function buildUploadCacheKey(boardId: string, file: File): string {
    return `${boardId}::${file.name}::${file.size}::${file.lastModified}`
  }

  async function loadImageRatioFromFile(file: File): Promise<number | null> {
    const localUrl = URL.createObjectURL(file)
    try {
      const size = await loadImageSizeFromUrl(localUrl)
      if (!size || !size.width || !size.height) return null
      return size.height / size.width
    } finally {
      URL.revokeObjectURL(localUrl)
    }
  }

  async function uploadImageToBoard(file: File): Promise<UploadImageResult> {
    if (!file.type.startsWith('image/')) {
      throw new Error('请上传图片文件')
    }
    const boardId = `${options.currentBoardId.value ?? ''}`.trim()
    if (!boardId) {
      throw new Error('缺少 boardId，无法上传图片')
    }
    const cacheKey = buildUploadCacheKey(boardId, file)
    const cached = uploadCache.get(cacheKey)
    if (cached) return cached
    const uploadTask = (async () => {
      try {
        const ratio = await loadImageRatioFromFile(file)
        const uploadRes = await options.uploadApi.upload(file, boardId)
        if (uploadRes.code !== 0 || !uploadRes.data?.url) {
          throw new Error(options.sanitizeApiErrorMessage(uploadRes.msg, '上传失败'))
        }
        return { url: uploadRes.data.url, ratio: ratio && ratio > 0 ? ratio : 1 }
      } catch (error) {
        uploadCache.delete(cacheKey)
        throw error
      }
    })()
    uploadCache.set(cacheKey, uploadTask)
    return uploadTask
  }

  async function applyImageFileToImageNode(nodeId: string, file: File): Promise<void> {
    const snapshotNode = options.getImageNodeSnapshot(nodeId)
    if (!snapshotNode) {
      options.onWarning('Image 节点不存在')
      return
    }
    try {
      const uploaded = await uploadImageToBoard(file)
      options.updateSnapshotNode(nodeId, (prev) => ({
        ...prev,
        sourceImageUrl: uploaded.url,
        sourceImageRatio: uploaded.ratio,
        outputImages: [],
      }))
      options.onNodeMutated?.(nodeId)
      options.onSuccess('Image 节点输入已更新')
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
    }
  }

  async function applyImageFileToMaskNode(nodeId: string, file: File): Promise<void> {
    const snapshotNode = options.getMaskNodeSnapshot(nodeId)
    if (!snapshotNode) {
      options.onWarning('Mask 节点不存在')
      return
    }
    try {
      const uploaded = await uploadImageToBoard(file)
      options.updateSnapshotNode(nodeId, (prev) => ({
        ...prev,
        sourceImageUrl: uploaded.url,
        sourceImageRatio: uploaded.ratio,
        executionMaskDataUrl: '',
        maskDataUrl: '',
      }))
      options.onNodeMutated?.(nodeId)
      void nextTick(() => options.renderMaskNodeMask(nodeId))
      options.onSuccess('Mask 节点输入已更新')
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
    }
  }

  function onCanvasDragOver(event: DragEvent) {
    if (!event.dataTransfer) return
    event.dataTransfer.dropEffect = 'copy'
  }

  function createImageNodeFromPayload(point: { x: number; y: number }, payload: UploadImageResult) {
    const nodeId = options.createNodeAtPosition('image', point.x, point.y, {
      sourceImageUrl: payload.url,
      sourceImageRatio: payload.ratio,
      outputImages: [],
    })
    if (!nodeId) return
    options.onNodeMutated?.(nodeId)
  }

  async function onCanvasDrop(event: DragEvent) {
    const point = options.screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
    const nodeKindRaw = event.dataTransfer?.getData(FLOW_NODE_KIND_DRAG_MIME) || ''
    const nodeKind = nodeKindRaw.trim() as FlowCreatableNodeKind
    if (nodeKind && options.createNodeOptions.some((item) => item.value === nodeKind)) {
      options.createNodeAtPosition(nodeKind, point.x, point.y)
      return
    }

    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    if (!file.type.startsWith('image/')) {
      options.onWarning('请上传图片文件')
      return
    }
    try {
      const uploaded = await uploadImageToBoard(file)
      createImageNodeFromPayload(point, uploaded)
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
    }
  }

  return {
    onCanvasDragOver,
    onCanvasDrop,
    uploadImageToBoard,
    loadImageRatioFromFile,
    applyImageFileToImageNode,
    applyImageFileToMaskNode,
  }
}

export { FLOW_NODE_KIND_DRAG_MIME }
