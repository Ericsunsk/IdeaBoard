import { canvasToBlob, fetchImageBlobFromUrl, resolveFetchUrl } from '../../media/canvasMediaIo.ts'
import {
  detectCanvasTransparency,
  resolvePreviewMimeType,
  WEBP_PREVIEW_MIME_TYPE,
  WEBP_PREVIEW_QUALITY,
} from '../../media/displayImagePreviewEncoding.ts'

interface PreviewResource {
  key: string
  url: string | null
  users: number
  controller: AbortController
  result: Promise<string | null>
}

const PREVIEW_CACHE_LIMIT = 80
const PREVIEW_CONCURRENCY = 2
const resources = new Map<string, PreviewResource>()
const previewCache = new Map<string, PreviewResource>()
const resourcesByUrl = new Map<string, PreviewResource>()
const pendingPreviewTasks: Array<() => void> = []
let activePreviewTasks = 0

function trimPreviewCache() {
  for (const [key, resource] of previewCache) {
    if (previewCache.size <= PREVIEW_CACHE_LIMIT) break
    if (resource.users > 0 || !resource.url) continue
    previewCache.delete(key)
    resources.delete(key)
    resourcesByUrl.delete(resource.url)
    URL.revokeObjectURL(resource.url)
  }
}

function releaseResource(resource: PreviewResource) {
  resource.users -= 1
  if (resource.users === 0 && !resource.url) {
    resource.controller.abort()
    if (resources.get(resource.key) === resource) resources.delete(resource.key)
  }
  trimPreviewCache()
}

function acquireResource(sourceUrl: string, maxSide: number): PreviewResource {
  const key = `${maxSide}::${sourceUrl}`
  const existing = resources.get(key)
  if (existing) {
    existing.users += 1
    if (existing.url) {
      previewCache.delete(key)
      previewCache.set(key, existing)
    }
    return existing
  }

  const controller = new AbortController()
  const resource: PreviewResource = {
    key,
    url: null,
    users: 1,
    controller,
    result: enqueuePreviewTask(
      () => createPreviewObjectUrl(sourceUrl, maxSide, controller.signal),
      controller.signal,
    )
      .then((url) => {
        if (!url) return null
        if (controller.signal.aborted) {
          URL.revokeObjectURL(url)
          return null
        }
        resource.url = url
        previewCache.set(key, resource)
        resourcesByUrl.set(url, resource)
        trimPreviewCache()
        return url
      })
      .catch(() => null)
      .finally(() => {
        if (!resource.url && resources.get(key) === resource) resources.delete(key)
      }),
  }
  resources.set(key, resource)
  return resource
}

export function createDisplayPreviewResources() {
  let generated: PreviewResource | null = null
  let displayed: PreviewResource[] = []

  function load(sourceUrl: string, maxSide: number | null): Promise<string | null> {
    const next = sourceUrl && maxSide !== null ? acquireResource(sourceUrl, maxSide) : null
    if (generated) releaseResource(generated)
    generated = next
    return next?.result ?? Promise.resolve(null)
  }

  function display(...urls: string[]) {
    const next: PreviewResource[] = []
    for (const url of new Set(urls)) {
      const resource = resourcesByUrl.get(url)
      if (!resource) continue
      resource.users += 1
      next.push(resource)
    }
    for (const resource of displayed) releaseResource(resource)
    displayed = next
  }

  function dispose() {
    if (generated) releaseResource(generated)
    generated = null
    display()
  }

  return { load, display, dispose }
}

function runNextPreviewTask() {
  while (activePreviewTasks < PREVIEW_CONCURRENCY && pendingPreviewTasks.length > 0) {
    const task = pendingPreviewTasks.shift()
    if (!task) break
    task()
  }
}

function enqueuePreviewTask<T>(taskFactory: () => Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(new DOMException('Aborted', 'AbortError'))
  return new Promise<T>((resolve, reject) => {
    const cleanup = () => signal.removeEventListener('abort', handleAbort)
    const wrappedResolve = (value: T) => {
      cleanup()
      resolve(value)
    }
    const wrappedReject = (reason?: unknown) => {
      cleanup()
      reject(reason)
    }
    const run = () => {
      if (signal.aborted) {
        wrappedReject(new DOMException('Aborted', 'AbortError'))
        return
      }
      activePreviewTasks += 1
      void taskFactory()
        .then(wrappedResolve)
        .catch(wrappedReject)
        .finally(() => {
          activePreviewTasks = Math.max(0, activePreviewTasks - 1)
          runNextPreviewTask()
        })
    }
    const handleAbort = () => {
      const pendingIndex = pendingPreviewTasks.indexOf(run)
      if (pendingIndex >= 0) {
        pendingPreviewTasks.splice(pendingIndex, 1)
        wrappedReject(new DOMException('Aborted', 'AbortError'))
      }
    }
    signal.addEventListener('abort', handleAbort, { once: true })
    pendingPreviewTasks.push(run)
    runNextPreviewTask()
  })
}

async function fetchPreviewSourceBlob(
  sourceUrl: string,
  signal: AbortSignal,
): Promise<Blob | null> {
  if (signal.aborted) return null
  const fetchUrl = resolveFetchUrl(sourceUrl)
  if (!fetchUrl) return null
  try {
    const blob = await fetchImageBlobFromUrl(fetchUrl, { signal })
    if (signal.aborted) return null
    return blob
  } catch {
    return null
  }
}

function resolvePreviewRenderSize(
  bitmap: ImageBitmap,
  maxSide: number,
): { width: number; height: number } | null {
  const longestSide = Math.max(bitmap.width, bitmap.height)
  if (longestSide <= 0 || longestSide <= maxSide) return null
  const scale = maxSide / longestSide
  return {
    width: Math.max(1, Math.round(bitmap.width * scale)),
    height: Math.max(1, Math.round(bitmap.height * scale)),
  }
}

async function renderPreviewObjectUrl(
  sourceBlob: Blob,
  maxSide: number,
  signal: AbortSignal,
): Promise<string | null> {
  let bitmap: ImageBitmap | null = null
  try {
    try {
      bitmap = await createImageBitmap(sourceBlob)
    } catch {
      return null
    }
    if (signal.aborted) return null
    const size = resolvePreviewRenderSize(bitmap, maxSide)
    if (!size) return null
    const canvas = document.createElement('canvas')
    canvas.width = size.width
    canvas.height = size.height
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    if (!ctx) return null
    ctx.drawImage(bitmap, 0, 0, size.width, size.height)
    bitmap.close()
    bitmap = null
    if (signal.aborted) return null
    const hasTransparentPixels = detectCanvasTransparency(ctx, size.width, size.height)
    const previewMime = resolvePreviewMimeType(sourceBlob.type, hasTransparentPixels)
    const previewBlob = await canvasToBlob(
      canvas,
      previewMime,
      previewMime === WEBP_PREVIEW_MIME_TYPE ? WEBP_PREVIEW_QUALITY : undefined,
    )
    if (signal.aborted) return null
    return URL.createObjectURL(previewBlob)
  } finally {
    bitmap?.close()
  }
}

async function createPreviewObjectUrl(
  sourceUrl: string,
  maxSide: number,
  signal: AbortSignal,
): Promise<string | null> {
  if (signal.aborted) return null
  const sourceBlob = await fetchPreviewSourceBlob(sourceUrl, signal)
  if (!sourceBlob || signal.aborted) return null
  if (!sourceBlob.type.startsWith('image/')) return null
  return renderPreviewObjectUrl(sourceBlob, maxSide, signal)
}
