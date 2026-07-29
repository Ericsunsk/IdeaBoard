import { uploadApi } from '../../../shared/api/upload'
import {
  rebuildCanvasBoardMediaRefs,
  type CanvasBoardSnapshot,
} from '../../canvas-board/domain'
import { filenameFromImageUrl, isDataUrl, resolveFetchUrl } from '../services/flowMediaIO'
import { prepareImageAssetForUpload } from '../../../shared/media/imageAssetPipeline'
import { measureCanvasPerfAsync } from '../utils/canvasPerfProbe'

interface StableRef {
  path: string
  get: () => string
  set: (url: string) => void
  setMediaAssetId?: (mediaAssetId: number) => void
}

interface BoardSessionUploadLifecycleOptions {
  userUid: string
  boardId: string
  warning: (message: string) => void
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
    if (!pathSegmentMatches(segments[i + 2], userUid)) continue
    if (!pathSegmentMatches(segments[i + 3], boardId)) continue
    if (segments[i + 4] !== 'content') continue
    return true
  }
  return false
}

type StableImageItem = { url: string; mediaAssetId?: number }

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
        set: (url) => {
          item.url = url
        },
        setMediaAssetId: (mediaAssetId) => {
          item.mediaAssetId = mediaAssetId
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
      set: (url) => {
        node.sourceImageUrl = url
      },
      setMediaAssetId: (mediaAssetId) => {
        node.sourceMediaAssetId = mediaAssetId
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

  async function fetchAndUploadStable(raw: string): Promise<{ url: string; mediaAssetId: number }> {
    const blob = await fetchImageBlob(raw)
    if (!blob.size) {
      throw new Error('下载图片失败：响应为空')
    }
    const file = new File([blob], filenameFromImageUrl(raw, blob.type || 'image/jpeg'), {
      type: blob.type || 'image/jpeg',
    })
    const prepared = await prepareImageAssetForUpload(file)
    prepared.warnings.forEach((message) => warning(message))
    const uploaded = await uploadApi.upload(prepared.uploadFile, normalizedBoardId)
    const stableUrl = uploaded.url.trim()
    const mediaAssetId = Number(uploaded.media_asset_id)
    if (!stableUrl || !isStableCanvasUrl(stableUrl)) {
      throw new Error('上传图片失败：返回链接不是当前 board canvas uploads')
    }
    if (!Number.isInteger(mediaAssetId) || mediaAssetId <= 0) {
      throw new Error('上传图片失败：缺少媒体资产标识')
    }
    return { url: stableUrl, mediaAssetId }
  }

  async function fetchImageBlob(raw: string): Promise<Blob> {
    if (isDataUrl(raw)) {
      try {
        const dataUrlResponse = await fetch(raw)
        if (!dataUrlResponse.ok) {
          throw new Error(`读取图片失败：HTTP ${dataUrlResponse.status}`)
        }
        return await dataUrlResponse.blob()
      } catch (error) {
        throw new Error(`读取图片失败：${error instanceof Error ? error.message : '未知错误'}`, {
          cause: error,
        })
      }
    }
    let response: Response
    try {
      response = await fetch(resolveFetchUrl(raw), { credentials: 'include' })
    } catch (error) {
      throw new Error(`下载图片失败：${error instanceof Error ? error.message : '网络异常'}`, {
        cause: error,
      })
    }
    if (!response.ok) {
      throw new Error(`下载图片失败：HTTP ${response.status}`)
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
        const tasks = new Map<string, Promise<{ url: string; mediaAssetId: number }>>()
        for (const ref of refs) {
          const current = ref.get()
          const normalized = current.trim()
          if (!normalized) continue
          if (normalized !== current) ref.set(normalized)
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
            ref.setMediaAssetId?.(uploaded.mediaAssetId)
          } catch (error) {
            const reason = error instanceof Error ? error.message : '未知错误'
            throw new Error(`保存失败：${ref.path} 使用了不稳定图片链接，${reason}`, {
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
