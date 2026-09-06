import { uploadApi } from '../../../shared/api/upload'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import { i18n } from '../../../shared/i18n'
import { rebuildCanvasBoardMediaRefs, type CanvasBoardSnapshot } from '../../canvas-board/domain'
import { filenameFromImageUrl, isDataUrl, resolveFetchUrl } from '../services/flowMediaIO'
import { prepareImageAssetForUpload } from '../../../shared/media/imageAssetPipeline'
import { buildMediaBrowserURL, type MediaRef } from '../../../shared/media/MediaRef'
import { measureCanvasPerfAsync } from '../utils/canvasPerfProbe'

interface StableRef {
  path: string
  get: () => string
  getMediaRef?: () => MediaRef | undefined
  set: (url: string) => void
  setMediaRef?: (mediaRef: MediaRef) => void
}

interface BoardSessionUploadLifecycleOptions {
  userUid: string
  boardId: string
  warning: (message: string) => void
}

function t(key: string, params?: Record<string, unknown>): string {
  return i18n.global.t(key, params ?? {})
}

function parsePathname(raw: string): string | null {
  const trimmed = raw.trim()
  if (!trimmed) return null
  if (trimmed.startsWith('/')) return trimmed
  try {
    return new URL(trimmed, window.location.origin).pathname || null
  } catch {
    return null
  }
}

function encodePathSegment(segment: string): string {
  return encodeURIComponent(segment).replace(
    /[!'()*]/g,
    (ch) => `%${ch.charCodeAt(0).toString(16).toUpperCase()}`,
  )
}

function pathSegmentMatches(segment: string, expected: string): boolean {
  const encoded = encodePathSegment(expected)
  if (segment === expected || segment === encoded) return true
  try {
    return decodeURIComponent(segment) === expected
  } catch {
    return false
  }
}

function isBoardCanvasUploadPath(path: string, userUid: string, boardId: string): boolean {
  const segments = path.split('/').filter(Boolean)
  for (let i = 0; i + 4 < segments.length; i += 1) {
    if (segments[i] !== 'uploads' || segments[i + 1] !== 'canvas') continue
    if (!pathSegmentMatches(segments[i + 2] ?? '', userUid)) continue
    if (!pathSegmentMatches(segments[i + 3] ?? '', boardId)) continue
    if (segments[i + 4] !== 'content') continue
    return true
  }
  return false
}

type StableImageItem = { url: string; mediaRef?: MediaRef }

function appendImageItemRefs(
  refs: StableRef[],
  prefix: string,
  nodes: Array<Partial<Record<'inputImages' | 'outputImages', StableImageItem[]>>>,
  collection: 'inputImages' | 'outputImages',
) {
  nodes.forEach((node, nodeIndex) => {
    node[collection]?.forEach((item, itemIndex) => {
      refs.push({
        path: `${prefix}[${nodeIndex}].${collection}[${itemIndex}].url`,
        get: () => item.url,
        getMediaRef: () => item.mediaRef,
        set: (url) => {
          item.url = url
        },
        setMediaRef: (mediaRef) => {
          item.mediaRef = mediaRef
        },
      })
    })
  })
}

function collectStableRefs(snapshot: CanvasBoardSnapshot): StableRef[] {
  const refs: StableRef[] = []
  snapshot.imageNodes.forEach((node, index) => {
    refs.push({
      path: `imageNodes[${index}].sourceImageUrl`,
      get: () => node.sourceImageUrl,
      getMediaRef: () => node.sourceMediaRef,
      set: (url) => {
        node.sourceImageUrl = url
      },
      setMediaRef: (mediaRef) => {
        node.sourceMediaRef = mediaRef
      },
    })
  })
  appendImageItemRefs(refs, 'imageNodes', snapshot.imageNodes, 'outputImages')
  appendImageItemRefs(refs, 'promptNodes', snapshot.promptNodes, 'inputImages')
  appendImageItemRefs(refs, 'fusionNodes', snapshot.fusionNodes, 'inputImages')
  appendImageItemRefs(refs, 'fusionNodes', snapshot.fusionNodes, 'outputImages')
  return refs
}

export function createBoardSessionUploadLifecycle(options: BoardSessionUploadLifecycleOptions) {
  const { userUid, boardId, warning } = options
  const normalizedBoardId = boardId.trim()
  const normalizedUserUid = userUid.trim()

  function isStableCanvasUrl(raw: string): boolean {
    const trimmed = raw.trim()
    if (!trimmed) return true
    const path = parsePathname(trimmed)
    if (!path) return false
    return isBoardCanvasUploadPath(path, normalizedUserUid, normalizedBoardId)
  }

  async function fetchAndUploadStable(raw: string): Promise<{ url: string; mediaRef: MediaRef }> {
    const blob = await fetchImageBlob(raw)
    if (!blob.size) {
      throw new Error(t('canvas.session.downloadEmptyResponse'))
    }
    const file = new File([blob], filenameFromImageUrl(raw, blob.type || 'image/jpeg'), {
      type: blob.type || 'image/jpeg',
    })
    const prepared = await prepareImageAssetForUpload(file)
    prepared.warnings.forEach((message) => warning(message))
    const uploaded = await uploadApi.upload(prepared.uploadFile, normalizedBoardId)
    const assetId = uploaded.asset_id?.trim()
    if (!assetId) {
      throw new Error(t('canvas.session.uploadMissingMediaAsset'))
    }
    const mediaRef: MediaRef = {
      assetId,
      derivativeTier: 'preview',
      derivativeFormat: 'webp',
    }
    return { url: buildMediaBrowserURL(mediaRef), mediaRef }
  }

  async function fetchImageBlob(raw: string): Promise<Blob> {
    if (isDataUrl(raw)) {
      try {
        const dataUrlResponse = await fetch(raw)
        if (!dataUrlResponse.ok) {
          throw new Error(t('canvas.session.readImageHttpFailed', { code: dataUrlResponse.status }))
        }
        return await dataUrlResponse.blob()
      } catch (error) {
        throw new Error(
          t('canvas.session.readImageFailed', {
            reason: userFacingErrorCopy(error, t('canvas.session.unknownError')),
          }),
          {
            cause: error,
          },
        )
      }
    }
    let response: Response
    try {
      response = await fetch(resolveFetchUrl(raw), { credentials: 'include' })
    } catch (error) {
      throw new Error(
        t('canvas.session.downloadImageFailed', {
          reason: userFacingErrorCopy(error, t('canvas.session.networkError')),
        }),
        {
          cause: error,
        },
      )
    }
    if (!response.ok) {
      throw new Error(t('canvas.session.downloadImageHttpFailed', { code: response.status }))
    }
    return await response.blob()
  }

  async function stabilizeSnapshotForSave(
    snapshot: CanvasBoardSnapshot,
  ): Promise<CanvasBoardSnapshot> {
    let unstableRefCount = 0
    return measureCanvasPerfAsync(
      'board.save.stabilizeSnapshot',
      () => ({
        imageNodes: snapshot.imageNodes.length,
        promptNodes: snapshot.promptNodes.length,
        fusionNodes: snapshot.fusionNodes.length,
        unstableRefs: unstableRefCount,
      }),
      async () => {
        const refs = collectStableRefs(snapshot)
        const tasks = new Map<string, Promise<{ url: string; mediaRef: MediaRef }>>()
        for (const ref of refs) {
          const current = ref.get()
          const normalized = current.trim()
          if (!normalized) continue
          if (normalized !== current) ref.set(normalized)
          const mediaRef = ref.getMediaRef?.()
          if (mediaRef) {
            const stableUrl = buildMediaBrowserURL(mediaRef)
            if (normalized !== stableUrl) ref.set(stableUrl)
            continue
          }
          if (isStableCanvasUrl(normalized)) continue
          unstableRefCount += 1
          const taskKey = `default::${normalized}`
          let task = tasks.get(taskKey)
          if (!task) {
            task = fetchAndUploadStable(normalized)
            tasks.set(taskKey, task)
          }
          try {
            const uploaded = await task
            ref.set(uploaded.url)
            ref.setMediaRef?.(uploaded.mediaRef)
          } catch (error) {
            const reason = userFacingErrorCopy(error, t('canvas.session.unknownError'))
            throw new Error(t('canvas.session.unstableImageLink', { path: ref.path, reason }), {
              cause: error,
            })
          }
        }
        rebuildCanvasBoardMediaRefs(snapshot)
        return snapshot
      },
      50,
    )
  }

  return {
    stabilizeSnapshotForSave,
  }
}
