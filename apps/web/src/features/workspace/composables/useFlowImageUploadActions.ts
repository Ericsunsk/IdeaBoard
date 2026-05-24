import type { FlowCreatableNodeKind } from './useFlowNodeMutations'
import type { ImageNodeItem } from '../types/canvasNodes'
import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../shared/utils/imageUploadSupport'
import { prepareImageAssetForUpload } from '../../../shared/media/imageAssetPipeline'
import { resolveImageNodeFrameSize } from '../utils/imageNodeSizing'

type FlowImageUploadResult = { url: string; ratio: number }

type UploadImageResponseData = {
  url?: string
  width?: number
  height?: number
  mime?: string
  ext?: string
}

type UseFlowImageUploadActionsOptions = {
  currentBoardId: { value: string | null | undefined }
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
  uploadApi: {
    upload: (
      file: File,
      boardId: string,
    ) => Promise<{ code: number; msg?: string; data?: UploadImageResponseData }>
  }
  sanitizeApiErrorMessage: (msg: string | null | undefined, fallback: string) => string
  onWarning: (msg: string) => void
  onSuccess: (msg: string) => void
  onNodeMutated?: (nodeId: string) => void
}

export function useFlowImageUploadActions(options: UseFlowImageUploadActionsOptions) {
  const uploadCache = new Map<string, Promise<FlowImageUploadResult>>()

  function buildUploadCacheKey(boardId: string, file: File): string {
    return `${boardId}::${file.name}::${file.size}::${file.lastModified}`
  }

  function resolveImageFileTitle(file: File): string {
    return file.name.trim()
  }

  async function uploadImageToBoard(file: File): Promise<FlowImageUploadResult> {
    if (!isSupportedUploadImageFile(file)) {
      throw new Error(unsupportedUploadImageMessage())
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
        const prepared = await prepareImageAssetForUpload(file)
        prepared.warnings.forEach((message) => options.onWarning(message))
        const uploadRes = await options.uploadApi.upload(prepared.uploadFile, boardId)
        if (uploadRes.code !== 0 || !uploadRes.data?.url) {
          throw new Error(options.sanitizeApiErrorMessage(uploadRes.msg, '上传失败'))
        }
        const width = Number(uploadRes.data.width)
        const height = Number(uploadRes.data.height)
        const serverRatio =
          Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0
            ? height / width
            : null
        const preparedRatio = prepared.dimensionsKnown && prepared.ratio > 0 ? prepared.ratio : null
        const resolvedRatio =
          preparedRatio ?? (serverRatio && serverRatio > 0 ? serverRatio : null) ?? 1
        return { url: uploadRes.data.url, ratio: resolvedRatio }
      } catch (error) {
        uploadCache.delete(cacheKey)
        throw error
      }
    })()
    uploadCache.set(cacheKey, uploadTask)
    return uploadTask
  }

  async function applyImageFileToImageNode(nodeId: string, file: File): Promise<boolean> {
    const snapshotNode = options.getImageNodeSnapshot(nodeId)
    if (!snapshotNode) {
      options.onWarning('Image 节点不存在')
      return false
    }
    try {
      const uploaded = await uploadImageToBoard(file)
      options.updateSnapshotNode(nodeId, (prev) => ({
        ...prev,
        title: resolveImageFileTitle(file) || prev.title,
        width: resolveImageNodeFrameSize(uploaded.ratio, true).width,
        sourceImageUrl: uploaded.url,
        sourceImageRatio: uploaded.ratio,
        outputStatus: 'ready',
        outputImages: [],
      }))
      options.onNodeMutated?.(nodeId)
      options.onSuccess('Image 节点输入已更新')
      return true
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
      return false
    }
  }

  function createImageNodeFromPayload(
    point: { x: number; y: number },
    payload: FlowImageUploadResult,
    title?: string,
  ): string | null {
    const frameSize = resolveImageNodeFrameSize(payload.ratio, true)
    const nodeId = options.createNodeAtPosition('image', point.x, point.y, {
      ...(title ? { title } : {}),
      width: frameSize.width,
      sourceImageUrl: payload.url,
      sourceImageRatio: payload.ratio,
      outputStatus: 'ready',
      outputImages: [],
    })
    if (!nodeId) return null
    options.onNodeMutated?.(nodeId)
    return nodeId
  }

  async function createImageNodeFromFile(
    point: { x: number; y: number },
    file: File,
  ): Promise<string | null> {
    if (!isSupportedUploadImageFile(file)) {
      options.onWarning(unsupportedUploadImageMessage())
      return null
    }
    const uploaded = await uploadImageToBoard(file)
    return createImageNodeFromPayload(point, uploaded, resolveImageFileTitle(file))
  }

  return {
    uploadImageToBoard,
    applyImageFileToImageNode,
    createImageNodeFromFile,
  }
}
