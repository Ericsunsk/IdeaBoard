import type {
  CanvasCreatableNodeKind,
  CanvasNodeItem,
  ImageNodeItem,
} from '../../canvas-board/domain'
import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../shared/utils/imageUploadSupport'
import { prepareImageAssetForUpload } from '../../../shared/media/imageAssetPipeline'
import { resolveImageNodeFrameSize } from '../../canvas-board/domain'
import { readCurrentBoardIdRef } from '../utils/currentBoardId'
import type { CanvasGraphNodeIdForKind } from '../graph-runtime'

type FlowImageUploadResult = { url: string; ratio: number }

type UploadedImageData = {
  url: string
  width?: number
  height?: number
  mime?: string
  ext?: string
}

type UseFlowImageUploadActionsOptions = {
  currentBoardId: { value: string | null | undefined }
  createNodeAtPosition: (
    kind: Extract<CanvasCreatableNodeKind, 'image'>,
    x: number,
    y: number,
    initialSnapshotPatch?: Partial<ImageNodeItem>,
  ) => CanvasGraphNodeIdForKind<'image'> | null
  updateSnapshotNode: (nodeId: string, patcher: (prev: CanvasNodeItem) => CanvasNodeItem) => void
  getImageNodeSnapshot: (nodeId: string) => ImageNodeItem | null
  uploadApi: {
    upload: (file: File, boardId: string) => Promise<UploadedImageData>
  }
  onWarning: (msg: string) => void
  onSuccess: (msg: string) => void
  onNodeMutated?: (nodeId: string) => void
}

export function useFlowImageUploadActions(options: UseFlowImageUploadActionsOptions) {
  const inFlightUploads = new Map<string, Promise<FlowImageUploadResult>>()
  const nodeUploadRequests = new Map<string, number>()
  let uploadRequestSequence = 0

  function buildInFlightUploadKey(boardId: string, file: File): string {
    return `${boardId}::${file.name}::${file.size}::${file.lastModified}`
  }

  function isBoardStillCurrent(boardId: string): boolean {
    return readCurrentBoardIdRef(options.currentBoardId) === boardId
  }

  function startNodeUploadRequest(nodeId: string): number {
    const requestId = ++uploadRequestSequence
    nodeUploadRequests.set(nodeId, requestId)
    return requestId
  }

  function isNodeUploadRequestCurrent(nodeId: string, requestId: number): boolean {
    return nodeUploadRequests.get(nodeId) === requestId
  }

  function resolveImageFileTitle(file: File): string {
    return file.name.trim()
  }

  async function uploadImageToBoardForBoard(
    file: File,
    boardId: string,
  ): Promise<FlowImageUploadResult> {
    if (!isSupportedUploadImageFile(file)) {
      throw new Error(unsupportedUploadImageMessage())
    }
    if (!boardId) {
      throw new Error('缺少 boardId，无法上传图片')
    }
    const cacheKey = buildInFlightUploadKey(boardId, file)
    const cached = inFlightUploads.get(cacheKey)
    if (cached) return cached
    const uploadTask = (async () => {
      try {
        const prepared = await prepareImageAssetForUpload(file)
        prepared.warnings.forEach((message) => options.onWarning(message))
        const uploaded = await options.uploadApi.upload(prepared.uploadFile, boardId)
        const width = Number(uploaded.width)
        const height = Number(uploaded.height)
        const serverRatio =
          Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0
            ? height / width
            : null
        const preparedRatio = prepared.dimensionsKnown && prepared.ratio > 0 ? prepared.ratio : null
        const resolvedRatio =
          preparedRatio ?? (serverRatio && serverRatio > 0 ? serverRatio : null) ?? 1
        return { url: uploaded.url, ratio: resolvedRatio }
      } finally {
        inFlightUploads.delete(cacheKey)
      }
    })()
    inFlightUploads.set(cacheKey, uploadTask)
    return uploadTask
  }

  async function uploadImageToBoard(file: File): Promise<FlowImageUploadResult> {
    const boardId = readCurrentBoardIdRef(options.currentBoardId)
    const uploaded = await uploadImageToBoardForBoard(file, boardId)
    if (!isBoardStillCurrent(boardId)) {
      throw new Error('上传结果已过期，请重新选择图片')
    }
    return uploaded
  }

  async function applyImageFileToImageNode(nodeId: string, file: File): Promise<boolean> {
    const snapshotNode = options.getImageNodeSnapshot(nodeId)
    if (!snapshotNode) {
      options.onWarning('Image 节点不存在')
      return false
    }
    const boardId = readCurrentBoardIdRef(options.currentBoardId)
    const requestId = startNodeUploadRequest(nodeId)
    try {
      const uploaded = await uploadImageToBoardForBoard(file, boardId)
      if (
        !isBoardStillCurrent(boardId) ||
        !isNodeUploadRequestCurrent(nodeId, requestId) ||
        !options.getImageNodeSnapshot(nodeId)
      ) {
        return false
      }
      options.updateSnapshotNode(nodeId, (prev) => {
        if (prev.type !== 'image-node') return prev
        return {
          ...prev,
          title: resolveImageFileTitle(file) || prev.title,
          width: resolveImageNodeFrameSize(uploaded.ratio, true).width,
          sourceImageUrl: uploaded.url,
          sourceMediaAssetId: undefined,
          sourceImageRatio: uploaded.ratio,
          outputStatus: 'ready',
          outputImages: [],
        }
      })
      options.onNodeMutated?.(nodeId)
      options.onSuccess('Image 节点输入已更新')
      return true
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
      return false
    } finally {
      if (isNodeUploadRequestCurrent(nodeId, requestId)) {
        nodeUploadRequests.delete(nodeId)
      }
    }
  }

  function createImageNodeFromPayload(
    point: { x: number; y: number },
    payload: FlowImageUploadResult,
    title?: string,
  ): CanvasGraphNodeIdForKind<'image'> | null {
    const frameSize = resolveImageNodeFrameSize(payload.ratio, true)
    const nodeId = options.createNodeAtPosition('image', point.x, point.y, {
      ...(title ? { title } : {}),
      width: frameSize.width,
      sourceImageUrl: payload.url,
      sourceMediaAssetId: undefined,
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
  ): Promise<CanvasGraphNodeIdForKind<'image'> | null> {
    if (!isSupportedUploadImageFile(file)) {
      options.onWarning(unsupportedUploadImageMessage())
      return null
    }
    const boardId = readCurrentBoardIdRef(options.currentBoardId)
    const uploaded = await uploadImageToBoardForBoard(file, boardId)
    if (!isBoardStillCurrent(boardId)) return null
    return createImageNodeFromPayload(point, uploaded, resolveImageFileTitle(file))
  }

  return {
    uploadImageToBoard,
    applyImageFileToImageNode,
    createImageNodeFromFile,
  }
}
