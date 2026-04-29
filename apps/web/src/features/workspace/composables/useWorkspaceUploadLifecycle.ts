import { ref, type Ref } from 'vue'
import { getAuthStorageUserScope } from '../../../shared/utils/auth'
import type { CanvasBoardSnapshot, CanvasImageItem } from '../types/canvasNodes'

const PENDING_RECYCLE_STORAGE_KEY_PREFIX = 'ideaboard.canvas.pendingUploadRecycle.v1'

export interface PendingRecycleUploadEntry {
  url: string
  userId: number
  boardId: string
}

interface PersistedImageUrlReference {
  path: string
  getUrl: () => string
  setUrl: (url: string) => void
}

interface OutputImageNodeLike {
  outputImages: Array<{ url: string }>
}

interface SourceAndOutputImageNodeLike extends OutputImageNodeLike {
  sourceImageUrl: string
}

interface SourceImageNodeLike {
  sourceImageUrl: string
}

interface UseWorkspaceUploadLifecycleOptions<TUploadResult extends { data?: { url?: string } }> {
  getCurrentBoardId: () => string | null | undefined
  getUserId: () => number
  getUserUid: () => string
  messages: Ref<CanvasImageItem[]>
  imageNodes: Ref<SourceAndOutputImageNodeLike[]>
  maskNodes: Ref<SourceImageNodeLike[]>
  modelNodes: Ref<OutputImageNodeLike[]>
  viewpointNodes: Ref<OutputImageNodeLike[]>
  colorNodes: Ref<OutputImageNodeLike[]>
  enhanceNodes: Ref<OutputImageNodeLike[]>
  editNodes: Ref<OutputImageNodeLike[]>
  fusionNodes: Ref<OutputImageNodeLike[]>
  wearNodes: Ref<OutputImageNodeLike[]>
  createBoardSnapshot: () => CanvasBoardSnapshot
  collectCanvasBoardImageUrls: (snapshot: CanvasBoardSnapshot | null | undefined) => Set<string>
  getCanvasBoardState: (boardId: string) => CanvasBoardSnapshot | null
  isImageUrlReferencedInOtherCanvasBoardStates: (url: string, excludeBoardId?: string) => boolean
  hasHistoryImageUrlReference: (url: string, options?: {
    recentUndoSteps?: number
    includeRedo?: boolean
    includeCurrent?: boolean
  }) => boolean
  uploadImageApi: (file: File, boardId: string) => Promise<TUploadResult>
  recycleImageApi: (url: string, boardId: string) => Promise<unknown>
  warning: (message: string) => void
  historyRecycleGuardSteps?: number
  userDownloadEndpoint?: string
}

export function useWorkspaceUploadLifecycle<TUploadResult extends { data?: { url?: string } }>(
  options: UseWorkspaceUploadLifecycleOptions<TUploadResult>,
) {
  const {
    getCurrentBoardId,
    getUserId,
    getUserUid,
    messages,
    imageNodes,
    maskNodes,
    modelNodes,
    viewpointNodes,
    colorNodes,
    enhanceNodes,
    editNodes,
    fusionNodes,
    wearNodes,
    createBoardSnapshot,
    collectCanvasBoardImageUrls,
    getCanvasBoardState,
    isImageUrlReferencedInOtherCanvasBoardStates,
    hasHistoryImageUrlReference,
    uploadImageApi,
    recycleImageApi,
    warning,
    historyRecycleGuardSteps = 10,
    userDownloadEndpoint = '/api/user/download',
  } = options

  const pendingRecycleEntries = ref<PendingRecycleUploadEntry[]>(readPendingRecycleEntries())
  let pendingRecycleScanInFlight = false
  let pendingRecycleScanQueued = false
  let pendingRecycleScanTimer: number | null = null
  let pendingSaveRecycleEntriesBuffer: PendingRecycleUploadEntry[] = []

  function getPendingRecycleStorageKey(): string | null {
    const scope = getAuthStorageUserScope()
    return scope ? `${PENDING_RECYCLE_STORAGE_KEY_PREFIX}.${scope}` : null
  }

  function isRecord(value: unknown): value is Record<string, unknown> {
    return Boolean(value) && typeof value === 'object'
  }

  function readStringField(record: Record<string, unknown>, key: string): string {
    const value = record[key]
    return typeof value === 'string' ? value.trim() : ''
  }

  function normalizePendingRecycleEntry(entry: unknown): PendingRecycleUploadEntry | null {
    if (!isRecord(entry)) return null
    const rawUrl = readStringField(entry, 'url')
    const rawBoardId = readStringField(entry, 'boardId')
    const rawUserId = Number(entry.userId)
    if (!rawUrl || !rawBoardId || !Number.isFinite(rawUserId) || rawUserId <= 0) return null
    return {
      url: rawUrl,
      userId: Math.floor(rawUserId),
      boardId: rawBoardId,
    }
  }

  function dedupePendingRecycleEntries(entries: unknown[]): PendingRecycleUploadEntry[] {
    const entryMap = new Map<string, PendingRecycleUploadEntry>()
    entries.forEach((entry) => {
      const normalized = normalizePendingRecycleEntry(entry)
      if (!normalized) return
      entryMap.set(normalized.url, normalized)
    })
    return Array.from(entryMap.values())
  }

  function readPendingRecycleEntries(): PendingRecycleUploadEntry[] {
    if (typeof window === 'undefined') return []
    const storageKey = getPendingRecycleStorageKey()
    if (!storageKey) return []
    const raw = localStorage.getItem(storageKey)
    if (!raw) return []
    try {
      const parsed = JSON.parse(raw)
      return Array.isArray(parsed) ? dedupePendingRecycleEntries(parsed) : []
    } catch {
      return []
    }
  }

  function writePendingRecycleEntries(entries: PendingRecycleUploadEntry[]) {
    if (typeof window === 'undefined') return
    const storageKey = getPendingRecycleStorageKey()
    if (!storageKey) return
    if (!entries.length) {
      localStorage.removeItem(storageKey)
      return
    }
    localStorage.setItem(storageKey, JSON.stringify(entries))
  }

  function setPendingRecycleEntries(entries: PendingRecycleUploadEntry[]) {
    const nextEntries = dedupePendingRecycleEntries(entries)
    pendingRecycleEntries.value = nextEntries
    writePendingRecycleEntries(nextEntries)
  }

  function reloadPendingRecycleEntries() {
    setPendingRecycleEntries(readPendingRecycleEntries())
  }

  function clearPendingRecycleScanTimer() {
    if (pendingRecycleScanTimer === null) return
    window.clearTimeout(pendingRecycleScanTimer)
    pendingRecycleScanTimer = null
  }

  function parseCanvasImagePath(url: string): string | null {
    const raw = url.trim()
    if (!raw) return null
    if (raw.startsWith('/')) return raw
    try {
      const parsed = new URL(raw, window.location.origin)
      return parsed.pathname || null
    } catch {
      return null
    }
  }

  function encodeWorkspacePathSegment(segment: string): string {
    return encodeURIComponent(segment).replace(
      /[!'()*]/g,
      (char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`,
    )
  }

  function pathSegmentMatches(segment: string, expected: string): boolean {
    const encodedExpected = encodeWorkspacePathSegment(expected)
    if (segment === expected || segment === encodedExpected) return true
    try {
      return decodeURIComponent(segment) === expected
    } catch {
      return false
    }
  }

  function isWorkspaceUploadPath(path: string, boardIdOverride?: string): boolean {
    const boardId = `${boardIdOverride ?? getCurrentBoardId() ?? ''}`.trim()
    if (!boardId) return false
    const storageUid = `${getUserUid()}`.trim()
    const segments = path.split('/').filter(Boolean)
    for (let i = 0; i + 4 < segments.length; i += 1) {
      if (segments[i] !== 'uploads' || segments[i + 1] !== 'workspace') continue
      if (storageUid && !pathSegmentMatches(segments[i + 2], storageUid)) continue
      if (!pathSegmentMatches(segments[i + 3], boardId)) continue
      if (segments[i + 4] !== 'content') continue
      return true
    }
    return false
  }

  function uploadWorkspaceImage(file: File): Promise<TUploadResult> {
    const boardId = `${getCurrentBoardId() ?? ''}`.trim()
    if (!boardId) {
      throw new Error('缺少画布ID')
    }
    return uploadImageApi(file, boardId)
  }

  function resolveAbsoluteCanvasUrl(rawUrl: string): URL | null {
    if (typeof window === 'undefined') return null
    try {
      return new URL(rawUrl, window.location.origin)
    } catch {
      return null
    }
  }

  function isStableWorkspaceUploadUrl(rawUrl: string): boolean {
    if (typeof window === 'undefined') return true
    const normalized = rawUrl.trim()
    if (!normalized) return true
    const path = parseCanvasImagePath(normalized)
    if (path && isWorkspaceUploadPath(path)) return true
    const parsed = resolveAbsoluteCanvasUrl(normalized)
    if (!parsed) return false
    return isWorkspaceUploadPath(parsed.pathname)
  }

  function getProxyTargetUrl(rawUrl: string): string | null {
    const parsed = resolveAbsoluteCanvasUrl(rawUrl.trim())
    if (!parsed) return null
    if (parsed.origin !== window.location.origin || !parsed.pathname.startsWith(userDownloadEndpoint)) {
      return null
    }
    const target = parsed.searchParams.get('url')
    return target?.trim() || null
  }

  function resolveImageFetchRequestUrl(rawUrl: string): string {
    const normalized = rawUrl.trim()
    const proxyTarget = getProxyTargetUrl(normalized)
    if (proxyTarget) {
      return `${userDownloadEndpoint}?url=${encodeURIComponent(proxyTarget)}`
    }
    const parsed = resolveAbsoluteCanvasUrl(normalized)
    if (parsed && (parsed.protocol === 'http:' || parsed.protocol === 'https:')) {
      if (parsed.origin !== window.location.origin) {
        return `${userDownloadEndpoint}?url=${encodeURIComponent(parsed.href)}`
      }
      return parsed.href
    }
    return normalized
  }

  function inferFileExtensionFromMimeType(mimeType: string): string {
    const normalized = mimeType.toLowerCase()
    if (normalized.includes('png')) return 'png'
    if (normalized.includes('webp')) return 'webp'
    if (normalized.includes('gif')) return 'gif'
    if (normalized.includes('bmp')) return 'bmp'
    if (normalized.includes('jpeg') || normalized.includes('jpg')) return 'jpg'
    if (normalized.includes('svg')) return 'svg'
    return 'jpg'
  }

  function toUploadFilename(rawUrl: string, mimeType: string): string {
    const parsed = resolveAbsoluteCanvasUrl(rawUrl.trim())
    const pathname = parsed?.pathname || rawUrl
    const leaf = pathname.split('/').filter(Boolean).pop() || ''
    const safeLeaf = leaf.replace(/[^A-Za-z0-9._-]/g, '')
    const hasExtension = /\.[A-Za-z0-9]{2,8}$/.test(safeLeaf)
    if (hasExtension) return safeLeaf
    return `workspace-image-${Date.now()}.${inferFileExtensionFromMimeType(mimeType)}`
  }

  async function fetchAndUploadStableWorkspaceUrl(rawUrl: string): Promise<string> {
    const requestUrl = resolveImageFetchRequestUrl(rawUrl)
    let response: Response
    try {
      response = await fetch(requestUrl, { credentials: 'include' })
    } catch (error) {
      throw new Error(`下载图片失败：${error instanceof Error ? error.message : '网络异常'}`)
    }
    if (!response.ok) {
      throw new Error(`下载图片失败：HTTP ${response.status}`)
    }
    const blob = await response.blob()
    if (!blob.size) {
      throw new Error('下载图片失败：响应为空')
    }
    const uploadFile = new File([blob], toUploadFilename(rawUrl, blob.type || 'image/jpeg'), {
      type: blob.type || 'image/jpeg',
    })
    let uploadResult: TUploadResult
    try {
      uploadResult = await uploadWorkspaceImage(uploadFile)
    } catch (error) {
      throw new Error(`上传图片失败：${error instanceof Error ? error.message : '未知错误'}`)
    }
    const stableUrl = `${uploadResult?.data?.url ?? ''}`.trim()
    if (!stableUrl) {
      throw new Error('上传图片失败：服务端未返回可用链接')
    }
    if (!isStableWorkspaceUploadUrl(stableUrl)) {
      throw new Error('上传图片失败：返回链接不是 workspace uploads 地址')
    }
    return stableUrl
  }

  function appendOutputImageReferences(
    target: PersistedImageUrlReference[],
    prefix: string,
    nodes: Array<{ outputImages: Array<{ url: string }> }>,
  ) {
    nodes.forEach((node, nodeIndex) => {
      node.outputImages.forEach((output, outputIndex) => {
        target.push({
          path: `${prefix}[${nodeIndex}].outputImages[${outputIndex}].url`,
          getUrl: () => output.url,
          setUrl: (url) => {
            output.url = url
          },
        })
      })
    })
  }

  function collectPersistedImageUrlReferences(): PersistedImageUrlReference[] {
    const references: PersistedImageUrlReference[] = []
    messages.value.forEach((item, index) => {
      references.push({
        path: `messages[${index}].url`,
        getUrl: () => item.url,
        setUrl: (url) => {
          item.url = url
        },
      })
    })
    imageNodes.value.forEach((node, index) => {
      references.push({
        path: `imageNodes[${index}].sourceImageUrl`,
        getUrl: () => node.sourceImageUrl,
        setUrl: (url) => {
          node.sourceImageUrl = url
        },
      })
    })
    maskNodes.value.forEach((node, index) => {
      references.push({
        path: `maskNodes[${index}].sourceImageUrl`,
        getUrl: () => node.sourceImageUrl,
        setUrl: (url) => {
          node.sourceImageUrl = url
        },
      })
    })
    appendOutputImageReferences(references, 'imageNodes', imageNodes.value)
    appendOutputImageReferences(references, 'modelNodes', modelNodes.value)
    appendOutputImageReferences(references, 'viewpointNodes', viewpointNodes.value)
    appendOutputImageReferences(references, 'colorNodes', colorNodes.value)
    appendOutputImageReferences(references, 'enhanceNodes', enhanceNodes.value)
    appendOutputImageReferences(references, 'editNodes', editNodes.value)
    appendOutputImageReferences(references, 'fusionNodes', fusionNodes.value)
    appendOutputImageReferences(references, 'wearNodes', wearNodes.value)
    return references
  }

  async function stabilizePersistedImageUrlsBeforeSave() {
    const normalizeAndUploadTasks = new Map<string, Promise<string>>()
    const references = collectPersistedImageUrlReferences()
    for (const reference of references) {
      const original = reference.getUrl()
      const normalized = original.trim()
      if (!normalized) continue
      if (normalized !== original) {
        reference.setUrl(normalized)
      }
      if (isStableWorkspaceUploadUrl(normalized)) continue
      let uploadTask = normalizeAndUploadTasks.get(normalized)
      if (!uploadTask) {
        uploadTask = fetchAndUploadStableWorkspaceUrl(normalized)
        normalizeAndUploadTasks.set(normalized, uploadTask)
      }
      try {
        const stableUrl = await uploadTask
        reference.setUrl(stableUrl)
      } catch (error) {
        const reason = error instanceof Error && error.message ? error.message : '未知错误'
        throw new Error(`保存失败：${reference.path} 使用了不稳定图片链接，${reason}`)
      }
    }
  }

  function resolveCanvasWorkspaceUploadMeta(url: string): CanvasImageItem['uploadMeta'] | undefined {
    const path = parseCanvasImagePath(url)
    if (!path) return undefined
    if (!isWorkspaceUploadPath(path)) return undefined
    return {
      kind: 'workspace-local-drop-upload',
      userId: Number(getUserId()),
    }
  }

  function resolvePendingRecycleEntryForUrl(
    url: string,
    boardIdOverride?: string,
  ): PendingRecycleUploadEntry | null {
    const normalizedUrl = url.trim()
    if (!normalizedUrl || !isStableWorkspaceUploadUrl(normalizedUrl)) return null
    const currentUserID = Number(getUserId())
    const boardId = `${boardIdOverride ?? getCurrentBoardId() ?? ''}`.trim()
    if (!Number.isFinite(currentUserID) || currentUserID <= 0 || !boardId) return null
    const path = parseCanvasImagePath(normalizedUrl)
    if (!path || !isWorkspaceUploadPath(path, boardId)) return null
    return {
      url: normalizedUrl,
      userId: currentUserID,
      boardId,
    }
  }

  function queueWorkspaceUploadUrlsForRecycle(
    urls: Iterable<string>,
    boardIdOverride?: string,
  ) {
    const nextEntryMap = new Map(
      pendingRecycleEntries.value.map(entry => [entry.url, entry] as const),
    )
    for (const url of urls) {
      const entry = resolvePendingRecycleEntryForUrl(url, boardIdOverride)
      if (!entry) continue
      nextEntryMap.set(entry.url, entry)
    }
    setPendingRecycleEntries(Array.from(nextEntryMap.values()))
  }

  function queueDeletedCanvasUploadsForRecycle(deletedItems: CanvasImageItem[]) {
    if (!deletedItems.length) return
    queueWorkspaceUploadUrlsForRecycle(deletedItems.map(item => item.url))
  }

  async function flushPendingRecycleUploads() {
    if (pendingRecycleScanInFlight) {
      pendingRecycleScanQueued = true
      return
    }
    clearPendingRecycleScanTimer()
    if (pendingRecycleEntries.value.length === 0) return
    pendingRecycleScanInFlight = true
    try {
      const activeBoardId = `${getCurrentBoardId() ?? ''}`.trim()
      const activeSnapshotUrls = collectCanvasBoardImageUrls(createBoardSnapshot())
      const recycleTargets: PendingRecycleUploadEntry[] = []
      const retainedEntries: PendingRecycleUploadEntry[] = []

      pendingRecycleEntries.value.forEach((entry) => {
        const normalized = normalizePendingRecycleEntry(entry)
        if (!normalized) return
        const { url } = normalized

        if (activeSnapshotUrls.has(url.trim())) return

        if (hasHistoryImageUrlReference(url, {
          recentUndoSteps: historyRecycleGuardSteps,
          includeRedo: true,
          includeCurrent: false,
        })) {
          retainedEntries.push(normalized)
          return
        }

        const excludeBoardId = normalized.boardId === activeBoardId ? activeBoardId : undefined
        if (isImageUrlReferencedInOtherCanvasBoardStates(url, excludeBoardId)) {
          retainedEntries.push(normalized)
          return
        }

        recycleTargets.push(normalized)
      })

      setPendingRecycleEntries(retainedEntries)
      if (!recycleTargets.length) return

      const results = await Promise.allSettled(recycleTargets.map(entry => recycleImageApi(entry.url, entry.boardId)))
      const failedCount = results.filter(result => result.status === 'rejected').length
      if (failedCount > 0) {
        warning(`有 ${failedCount} 张图片回收失败，已保留文件`)
        results.forEach((result, index) => {
          if (result.status === 'rejected') retainedEntries.push(recycleTargets[index])
        })
        setPendingRecycleEntries(retainedEntries)
      }
    } finally {
      pendingRecycleScanInFlight = false
      if (pendingRecycleScanQueued) {
        pendingRecycleScanQueued = false
        if (pendingRecycleEntries.value.length > 0) {
          void flushPendingRecycleUploads()
        }
      }
    }
  }

  function schedulePendingRecycleScan() {
    if (pendingRecycleEntries.value.length === 0) return
    if (pendingRecycleScanTimer !== null) return
    pendingRecycleScanTimer = window.setTimeout(() => {
      pendingRecycleScanTimer = null
      void flushPendingRecycleUploads()
    }, 120)
  }

  function collectRemovedWorkspaceUploadEntries(
    previousSnapshot: CanvasBoardSnapshot | null | undefined,
    nextSnapshot: CanvasBoardSnapshot | null | undefined,
    boardId: string,
  ): PendingRecycleUploadEntry[] {
    const previousUrls = collectCanvasBoardImageUrls(previousSnapshot)
    const nextUrls = collectCanvasBoardImageUrls(nextSnapshot)
    const removedEntries: PendingRecycleUploadEntry[] = []

    previousUrls.forEach((url) => {
      if (nextUrls.has(url)) return
      const entry = resolvePendingRecycleEntryForUrl(url, boardId)
      if (entry) removedEntries.push(entry)
    })
    return removedEntries
  }

  async function prepareBoardSnapshotBeforePersist() {
    const boardId = `${getCurrentBoardId() ?? ''}`.trim()
    const previousSnapshot = boardId ? getCanvasBoardState(boardId) : null
    await stabilizePersistedImageUrlsBeforeSave()
    pendingSaveRecycleEntriesBuffer = boardId
      ? collectRemovedWorkspaceUploadEntries(previousSnapshot, createBoardSnapshot(), boardId)
      : []
  }

  async function handleBoardPersisted() {
    if (pendingSaveRecycleEntriesBuffer.length > 0) {
      setPendingRecycleEntries([
        ...pendingRecycleEntries.value,
        ...pendingSaveRecycleEntriesBuffer,
      ])
      pendingSaveRecycleEntriesBuffer = []
    }
    schedulePendingRecycleScan()
  }

  return {
    uploadWorkspaceImage,
    resolveCanvasWorkspaceUploadMeta,
    queueWorkspaceUploadUrlsForRecycle,
    queueDeletedCanvasUploadsForRecycle,
    prepareBoardSnapshotBeforePersist,
    handleBoardPersisted,
    schedulePendingRecycleScan,
    clearPendingRecycleScanTimer,
    flushPendingRecycleUploads,
    reloadPendingRecycleEntries,
  }
}
