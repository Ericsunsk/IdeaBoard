import {
  collectCanvasBoardImageUrls,
  getCanvasBoardState,
  isImageUrlReferencedInOtherCanvasBoardStates,
} from '../../boards/lib/canvasBoardState'
import { uploadApi } from '../../../shared/api/upload'
import type { CanvasBoardSnapshot } from '../types/canvasNodes'
import { filenameFromImageUrl, isDataUrl, resolveFetchUrl } from './flowMediaIO'
import { prepareImageAssetForUpload } from '../../../shared/media/imageAssetPipeline'

interface StableRef {
  path: string
  get: () => string
  set: (url: string) => void
}

interface FlowWorkspaceUploadLifecycleOptions {
  userId: number
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

function isBoardWorkspaceUploadPath(path: string, userUid: string, boardId: string): boolean {
  const segments = path.split('/').filter(Boolean)
  for (let i = 0; i + 4 < segments.length; i += 1) {
    if (segments[i] !== 'uploads' || segments[i + 1] !== 'workspace') continue
    if (!pathSegmentMatches(segments[i + 2], userUid)) continue
    if (!pathSegmentMatches(segments[i + 3], boardId)) continue
    if (segments[i + 4] !== 'content') continue
    return true
  }
  return false
}

function appendOutputImageRefs(
  refs: StableRef[],
  prefix: string,
  nodes: Array<{ outputImages: Array<{ url: string }> }>,
) {
  nodes.forEach((node, nodeIndex) => {
    node.outputImages.forEach((output, outputIndex) => {
      refs.push({
        path: `${prefix}[${nodeIndex}].outputImages[${outputIndex}].url`,
        get: () => output.url,
        set: (url) => {
          output.url = url
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
    })
  })
  appendOutputImageRefs(refs, 'imageNodes', snapshot.imageNodes)
  appendOutputImageRefs(refs, 'modelNodes', snapshot.modelNodes)
  appendOutputImageRefs(refs, 'viewpointNodes', snapshot.viewpointNodes)
  appendOutputImageRefs(refs, 'editNodes', snapshot.editNodes)
  appendOutputImageRefs(refs, 'fusionNodes', snapshot.fusionNodes)
  appendOutputImageRefs(refs, 'wearNodes', snapshot.wearNodes)
  return refs
}

export function createFlowWorkspaceUploadLifecycle(options: FlowWorkspaceUploadLifecycleOptions) {
  const { userId, userUid, boardId, warning } = options
  const currentUserId = Number(userId)
  const normalizedBoardId = boardId.trim()
  const normalizedUserUid = userUid.trim()

  function isStableWorkspaceUrl(raw: string): boolean {
    const trimmed = raw.trim()
    if (!trimmed) return true
    const path = parsePathname(trimmed)
    if (!path) return false
    return isBoardWorkspaceUploadPath(path, normalizedUserUid, normalizedBoardId)
  }

  async function fetchAndUploadStable(raw: string): Promise<string> {
    const blob = await fetchImageBlob(raw)
    if (!blob.size) {
      throw new Error('下载图片失败：响应为空')
    }
    const file = new File([blob], filenameFromImageUrl(raw, blob.type || 'image/jpeg'), {
      type: blob.type || 'image/jpeg',
    })
    const prepared = await prepareImageAssetForUpload(file)
    prepared.warnings.forEach((message) => warning(message))
    const uploadRes = await uploadApi.upload(prepared.uploadFile, normalizedBoardId)
    if (uploadRes.code !== 0 || !uploadRes.data?.url) {
      throw new Error(uploadRes.msg || '上传图片失败')
    }
    const stableUrl = uploadRes.data.url.trim()
    if (!stableUrl || !isStableWorkspaceUrl(stableUrl)) {
      throw new Error('上传图片失败：返回链接不是当前 board workspace uploads')
    }
    return stableUrl
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
    const next = JSON.parse(JSON.stringify(snapshot)) as CanvasBoardSnapshot
    const refs = collectStableRefs(next)
    const tasks = new Map<string, Promise<string>>()
    for (const ref of refs) {
      const current = ref.get()
      const normalized = current.trim()
      if (!normalized) continue
      if (normalized !== current) ref.set(normalized)
      if (isStableWorkspaceUrl(normalized)) continue
      const taskKey = `default::${normalized}`
      let task = tasks.get(taskKey)
      if (!task) {
        task = fetchAndUploadStable(normalized)
        tasks.set(taskKey, task)
      }
      try {
        ref.set(await task)
      } catch (error) {
        const reason = error instanceof Error ? error.message : '未知错误'
        throw new Error(`保存失败：${ref.path} 使用了不稳定图片链接，${reason}`, { cause: error })
      }
    }
    return next
  }

  function resolveRecyclableUrl(url: string): string | null {
    const normalized = url.trim()
    if (!normalized) return null
    if (!Number.isFinite(currentUserId) || currentUserId <= 0) return null
    const path = parsePathname(normalized)
    if (!path) return null
    if (!isBoardWorkspaceUploadPath(path, normalizedUserUid, normalizedBoardId)) return null
    return normalized
  }

  async function recycleRemovedUploadsAfterSave(
    previousSnapshot: CanvasBoardSnapshot | null | undefined,
    nextSnapshot: CanvasBoardSnapshot,
  ): Promise<void> {
    const previousUrls = collectCanvasBoardImageUrls(previousSnapshot)
    const nextUrls = collectCanvasBoardImageUrls(nextSnapshot)
    const recycleTargets: string[] = []
    previousUrls.forEach((url) => {
      if (nextUrls.has(url)) return
      const recyclable = resolveRecyclableUrl(url)
      if (!recyclable) return
      if (isImageUrlReferencedInOtherCanvasBoardStates(recyclable, normalizedBoardId)) return
      recycleTargets.push(recyclable)
    })
    if (!recycleTargets.length) return
    const results = await Promise.allSettled(
      recycleTargets.map((url) => uploadApi.recycle(url, normalizedBoardId)),
    )
    const failed = results.filter((item) => {
      if (item.status === 'rejected') return true
      const code = item.value?.code
      return typeof code === 'number' && code !== 0
    }).length
    if (failed > 0) {
      warning(`有 ${failed} 张图片回收失败，已保留文件`)
    }
  }

  function readPreviousSnapshot(): CanvasBoardSnapshot | null {
    return getCanvasBoardState(normalizedBoardId)
  }

  return {
    stabilizeSnapshotForSave,
    recycleRemovedUploadsAfterSave,
    readPreviousSnapshot,
  }
}
