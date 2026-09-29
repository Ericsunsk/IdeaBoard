import type { CanvasCreatableNodeKind, CanvasNodeItem, ImageNodeItem } from '../../../board/ports/canvas'
import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../../shared/utils/imageUploadSupport'
import { prepareImageAssetForUpload } from '../../../../shared/media/imageAssetPipeline'
import { preloadImage } from '../../../../shared/media/preloadImage'
import { buildMediaBrowserURL, type MediaRef } from '../../../../shared/media/mediaRef'
import { userFacingErrorCopy } from '../../../../shared/errors/errorCatalog'
import { i18n } from '../../../../shared/i18n'
import { resolveImageNodeFrameSize } from '../../../board/ports/canvas'
import { readCurrentBoardIdRef } from '../board-session/currentBoardId.ts'
import type { CanvasGraphNodeIdForKind } from '../../domain/graph'
import type { FlowLocalUploadState } from '../../domain/nodes/flowTypes.ts'

export type FlowImageUploadResult = {
  url: string
  ratio: number
  assetId: string
  mediaRef: MediaRef
}

export type CanvasImageNodeCreationResult =
  | Readonly<{ kind: 'ready'; nodeId: CanvasGraphNodeIdForKind<'image'> }>
  | Readonly<{
      kind: 'awaiting-retry'
      nodeId: CanvasGraphNodeIdForKind<'image'>
      failureMessage: string
    }>
  | Readonly<{ kind: 'rejected'; failureMessage: string }>

type LocalUploadMaterializationResult =
  | Readonly<{ kind: 'ready' }>
  | Readonly<{ kind: 'failed'; message: string }>
  | Readonly<{ kind: 'stale' }>

type UploadedImageData = {
  url: string
  asset_id: string
  width?: number
  height?: number
  mime?: string
  ext?: string
}

type UseFlowImageUploadActionsOptions = {
  currentBoardId: { value: string | null | undefined }
  workspaceId?: (() => string) | undefined
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
  onNodeMutated?: (nodeId: string) => void
  setLocalUploadState: (nodeId: string, state: FlowLocalUploadState | null) => void
  createObjectUrl?: (file: File) => string
  revokeObjectUrl?: (url: string) => void
  preloadImage?: (url: string) => Promise<boolean>
}

type LocalUpload = {
  boardId: string
  file: File
  previewUrl: string
}

export function useFlowImageUploadActions(options: UseFlowImageUploadActionsOptions) {
  const inFlightUploads = new Map<string, Promise<FlowImageUploadResult>>()
  const nodeUploadRequests = new Map<string, number>()
  const localUploads = new Map<string, LocalUpload>()
  let uploadRequestSequence = 0
  let disposed = false

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

  async function resolveLocalPreviewRatio(file: File): Promise<number> {
    if (typeof createImageBitmap !== 'function') return 1
    try {
      const bitmap = await createImageBitmap(file)
      try {
        return bitmap.width > 0 && bitmap.height > 0 ? bitmap.height / bitmap.width : 1
      } finally {
        bitmap.close()
      }
    } catch {
      return 1
    }
  }

  function createObjectUrl(file: File): string {
    if (options.createObjectUrl) return options.createObjectUrl(file)
    if (typeof URL === 'undefined' || typeof URL.createObjectURL !== 'function') return ''
    return URL.createObjectURL(file)
  }

  async function preloadLocalPreview(previewUrl: string): Promise<boolean> {
    return (options.preloadImage ?? preloadImage)(previewUrl).catch(() => false)
  }

  function revokeObjectUrl(url: string) {
    if (!url) return
    if (options.revokeObjectUrl) {
      options.revokeObjectUrl(url)
      return
    }
    if (typeof URL !== 'undefined' && typeof URL.revokeObjectURL === 'function') {
      URL.revokeObjectURL(url)
    }
  }

  function updateLocalUploadState(nodeId: string, state: FlowLocalUploadState | null) {
    options.setLocalUploadState(nodeId, state)
  }

  function releaseLocalUpload(nodeId: string) {
    const localUpload = localUploads.get(nodeId)
    if (!localUpload) return
    localUploads.delete(nodeId)
    nodeUploadRequests.delete(nodeId)
    updateLocalUploadState(nodeId, null)
    revokeObjectUrl(localUpload.previewUrl)
  }

  function retainFailedLocalUpload(nodeId: string, message: string) {
    const localUpload = localUploads.get(nodeId)
    if (!localUpload) return
    updateLocalUploadState(nodeId, {
      previewUrl: localUpload.previewUrl,
      failureMessage: message,
    })
  }

  function beginLocalUpload(nodeId: string, localUpload: LocalUpload): number {
    releaseLocalUpload(nodeId)
    localUploads.set(nodeId, localUpload)
    updateLocalUploadState(nodeId, { previewUrl: localUpload.previewUrl })
    return startNodeUploadRequest(nodeId)
  }

  async function uploadImageToBoardForBoard(
    file: File,
    boardId: string,
  ): Promise<FlowImageUploadResult> {
    if (!isSupportedUploadImageFile(file)) {
      throw new Error(unsupportedUploadImageMessage())
    }
    if (!boardId) {
      throw new Error(i18n.global.t('canvas.session.missingBoardIdUpload'))
    }
    const cacheKey = buildInFlightUploadKey(boardId, file)
    const cached = inFlightUploads.get(cacheKey)
    if (cached) return cached
    const uploadTask = (async () => {
      try {
        const prepared = await prepareImageAssetForUpload(file, {
          messages: {
            unknownError: i18n.global.t('canvas.session.unknownError'),
            readImageSizeSkipCompression: i18n.global.t(
              'canvas.session.readImageSizeSkipCompression',
            ),
            compressFallbackOriginal: i18n.global.t('canvas.session.compressFallbackOriginal'),
          },
        })
        prepared.warnings.forEach((message) => options.onWarning(message))
        const workspaceId = options.workspaceId?.().trim()
        if (!workspaceId) throw new Error(i18n.global.t('canvas.session.uploadFailed'))
        const uploaded = await options.uploadApi.upload(prepared.uploadFile, workspaceId)
        const assetId = `${uploaded.asset_id ?? ''}`.trim()
        if (!assetId) throw new Error(i18n.global.t('canvas.session.uploadMissingMediaAsset'))
        const width = Number(uploaded.width)
        const height = Number(uploaded.height)
        const serverRatio =
          Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0
            ? height / width
            : null
        const preparedRatio = prepared.dimensionsKnown && prepared.ratio > 0 ? prepared.ratio : null
        const resolvedRatio =
          preparedRatio ?? (serverRatio && serverRatio > 0 ? serverRatio : null) ?? 1
        const mediaRef: MediaRef = {
          assetId,
          derivativeTier: 'preview',
        }
        return {
          url: buildMediaBrowserURL(mediaRef),
          ratio: resolvedRatio,
          assetId,
          mediaRef,
        }
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
      throw new Error(i18n.global.t('canvas.session.uploadStale'))
    }
    return uploaded
  }

  async function applyImageFileToImageNode(nodeId: string, file: File): Promise<boolean> {
    const snapshotNode = options.getImageNodeSnapshot(nodeId)
    if (!snapshotNode) {
      options.onWarning(i18n.global.t('canvas.session.imageNodeMissing'))
      return false
    }
    const boardId = readCurrentBoardIdRef(options.currentBoardId)
    if (!isSupportedUploadImageFile(file)) {
      options.onWarning(unsupportedUploadImageMessage())
      return false
    }
    if (!boardId) {
      options.onWarning(i18n.global.t('canvas.session.missingBoardIdUpload'))
      return false
    }
    const previewUrl = createObjectUrl(file)
    const previewReady = await preloadLocalPreview(previewUrl)
    if (!previewReady) {
      revokeObjectUrl(previewUrl)
      options.onWarning(i18n.global.t('canvas.session.uploadFailed'))
      return false
    }
    const requestId = beginLocalUpload(nodeId, { boardId, file, previewUrl })
    const result = await materializeLocalUpload(nodeId, requestId)
    if (result.kind === 'failed') options.onWarning(result.message)
    return result.kind === 'ready'
  }

  async function createImageNodeFromFile(
    point: { x: number; y: number },
    file: File,
  ): Promise<CanvasImageNodeCreationResult | null> {
    if (!isSupportedUploadImageFile(file)) {
      return { kind: 'rejected', failureMessage: unsupportedUploadImageMessage() }
    }
    const boardId = readCurrentBoardIdRef(options.currentBoardId)
    const previewRatio = await resolveLocalPreviewRatio(file)
    if (disposed || !isBoardStillCurrent(boardId)) return null
    const previewUrl = createObjectUrl(file)
    const previewReady = await preloadLocalPreview(previewUrl)
    if (!previewReady) {
      revokeObjectUrl(previewUrl)
      return {
        kind: 'rejected',
        failureMessage: i18n.global.t('canvas.session.uploadFailed'),
      }
    }
    if (disposed || !isBoardStillCurrent(boardId)) {
      revokeObjectUrl(previewUrl)
      return null
    }
    const frameSize = resolveImageNodeFrameSize(previewRatio, true)
    const nodeId = options.createNodeAtPosition('image', point.x, point.y, {
      title: resolveImageFileTitle(file),
      width: frameSize.width,
      sourceImageRatio: previewRatio,
      outputStatus: 'idle',
      outputImages: [],
    })
    if (!nodeId) {
      revokeObjectUrl(previewUrl)
      return {
        kind: 'rejected',
        failureMessage: i18n.global.t('canvas.session.uploadFailed'),
      }
    }

    const requestId = beginLocalUpload(nodeId, { boardId, file, previewUrl })
    const result = await materializeLocalUpload(nodeId, requestId)
    if (result.kind === 'ready') return { kind: 'ready', nodeId }
    if (result.kind === 'failed') {
      return { kind: 'awaiting-retry', nodeId, failureMessage: result.message }
    }
    return null
  }

  async function materializeLocalUpload(
    nodeId: string,
    requestId: number,
  ): Promise<LocalUploadMaterializationResult> {
    const localUpload = localUploads.get(nodeId)
    if (!localUpload) return { kind: 'stale' }
    try {
      const uploaded = await uploadImageToBoardForBoard(localUpload.file, localUpload.boardId)
      if (
        !isBoardStillCurrent(localUpload.boardId) ||
        !isNodeUploadRequestCurrent(nodeId, requestId) ||
        !options.getImageNodeSnapshot(nodeId)
      ) {
        // A newer drop may have replaced this call's entry while the upload
        // was in flight. Only release when the stored entry is still this
        // call's; otherwise return stale without destroying the newer upload.
        if (localUploads.get(nodeId) === localUpload) {
          releaseLocalUpload(nodeId)
        }
        return { kind: 'stale' }
      }
      options.updateSnapshotNode(nodeId, (prev) => {
        if (prev.type !== 'image-node') return prev
        const previousWidth = Number(prev.width)
        const width =
          Number.isFinite(previousWidth) && previousWidth > 0
            ? previousWidth
            : resolveImageNodeFrameSize(uploaded.ratio, true).width
        return {
          ...prev,
          title: resolveImageFileTitle(localUpload.file) || prev.title,
          width,
          sourceImageUrl: uploaded.url,
          sourceMediaRef: uploaded.mediaRef,
          sourceImageRatio: uploaded.ratio,
          outputStatus: 'ready',
          outputImages: [],
        }
      })
      options.onNodeMutated?.(nodeId)
      releaseLocalUpload(nodeId)
      return { kind: 'ready' }
    } catch (error) {
      if (!isNodeUploadRequestCurrent(nodeId, requestId)) return { kind: 'stale' }
      const message = userFacingErrorCopy(error, i18n.global.t('canvas.session.uploadFailed'))
      retainFailedLocalUpload(nodeId, message)
      return { kind: 'failed', message }
    } finally {
      if (isNodeUploadRequestCurrent(nodeId, requestId)) {
        nodeUploadRequests.delete(nodeId)
      }
    }
  }

  async function retryLocalUpload(nodeId: string): Promise<boolean> {
    const localUpload = localUploads.get(nodeId)
    if (!localUpload || !options.getImageNodeSnapshot(nodeId)) {
      releaseLocalUpload(nodeId)
      return false
    }
    const requestId = startNodeUploadRequest(nodeId)
    updateLocalUploadState(nodeId, { previewUrl: localUpload.previewUrl })
    const result = await materializeLocalUpload(nodeId, requestId)
    if (result.kind === 'failed') options.onWarning(result.message)
    return result.kind === 'ready'
  }

  function releaseDetachedLocalUploads(
    liveNodeIds: ReadonlySet<string>,
    currentBoardId: string | null | undefined = readCurrentBoardIdRef(options.currentBoardId),
  ) {
    for (const [nodeId, localUpload] of localUploads) {
      if (!liveNodeIds.has(nodeId) || localUpload.boardId !== currentBoardId) {
        releaseLocalUpload(nodeId)
      }
    }
  }

  function disposeLocalUploads() {
    disposed = true
    for (const nodeId of Array.from(localUploads.keys())) releaseLocalUpload(nodeId)
  }

  function hasLocalUploads(): boolean {
    return localUploads.size > 0
  }

  return {
    uploadImageToBoard,
    applyImageFileToImageNode,
    createImageNodeFromFile,
    retryLocalUpload,
    releaseDetachedLocalUploads,
    disposeLocalUploads,
    hasLocalUploads,
  }
}
