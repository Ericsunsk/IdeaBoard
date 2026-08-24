import { onBeforeUnmount } from 'vue'
import type { ImageNodeItem, ImageNodeOutputItem } from '../../canvas-board/domain'
import { canonicalizeRuntimeUploadRef, resolveAssetUrl } from '../../../shared/config/runtimeUrls'
import {
  getMediaAssetMetadata,
  resolveMediaAssetOriginalURL,
} from '../../../shared/api/mediaAssetDelivery'
import PhotoSwipeLightbox from 'photoswipe/lightbox'
import 'photoswipe/style.css'

type PreviewTarget = 'source' | 'latest-output'

interface ImageNaturalSize {
  width: number
  height: number
}

interface LightboxCandidate {
  src: string
  ratio: number
  mediaAssetId?: number
}

interface LightboxDataSourceItem {
  src: string
  width: number
  height: number
}

const LIGHTBOX_FALLBACK_SHORT_SIDE = 1440
const IMAGE_DIMENSION_TIMEOUT_MS = 1800
const IMAGE_DIMENSION_CACHE_LIMIT = 120
const imageDimensionCache = new Map<string, ImageNaturalSize>()
const imageDimensionTasks = new Map<string, Promise<ImageNaturalSize | null>>()

export function normalizeImageUrl(value: unknown): string {
  if (typeof value !== 'string') return ''
  const trimmed = value.trim()
  if (!trimmed) return ''
  return resolveAssetUrl(canonicalizeRuntimeUploadRef(trimmed)).trim()
}

function normalizeImageRatio(value: unknown): number {
  const ratio = Number(value)
  return Number.isFinite(ratio) && ratio > 0 ? ratio : 1
}

function toLightboxSize(ratio: number): { width: number; height: number } {
  const safeRatio = normalizeImageRatio(ratio)
  if (safeRatio >= 1) {
    return {
      width: LIGHTBOX_FALLBACK_SHORT_SIDE,
      height: Math.round(LIGHTBOX_FALLBACK_SHORT_SIDE * safeRatio),
    }
  }
  return {
    width: Math.round(LIGHTBOX_FALLBACK_SHORT_SIDE / safeRatio),
    height: LIGHTBOX_FALLBACK_SHORT_SIDE,
  }
}

function normalizeNaturalSize(width: number, height: number): ImageNaturalSize | null {
  if (!Number.isFinite(width) || !Number.isFinite(height)) return null
  const roundedWidth = Math.round(width)
  const roundedHeight = Math.round(height)
  if (roundedWidth <= 0 || roundedHeight <= 0) return null
  return {
    width: roundedWidth,
    height: roundedHeight,
  }
}

function rememberImageDimension(url: string, size: ImageNaturalSize) {
  if (imageDimensionCache.has(url)) imageDimensionCache.delete(url)
  imageDimensionCache.set(url, size)
  while (imageDimensionCache.size > IMAGE_DIMENSION_CACHE_LIMIT) {
    const oldestKey = imageDimensionCache.keys().next().value
    if (!oldestKey) break
    imageDimensionCache.delete(oldestKey)
  }
}

function readImageNaturalSize(url: string): Promise<ImageNaturalSize | null> {
  if (imageDimensionCache.has(url)) {
    return Promise.resolve(imageDimensionCache.get(url) ?? null)
  }
  const inFlight = imageDimensionTasks.get(url)
  if (inFlight) return inFlight

  const task = new Promise<ImageNaturalSize | null>((resolve) => {
    const image = new Image()
    let settled = false
    let timeoutId = 0
    function finish(size: ImageNaturalSize | null) {
      if (settled) return
      settled = true
      window.clearTimeout(timeoutId)
      image.onload = null
      image.onerror = null
      if (size) rememberImageDimension(url, size)
      resolve(size)
    }
    timeoutId = window.setTimeout(() => finish(null), IMAGE_DIMENSION_TIMEOUT_MS)
    image.onload = () => finish(normalizeNaturalSize(image.naturalWidth, image.naturalHeight))
    image.onerror = () => finish(null)
    image.src = url
    if (image.complete) {
      finish(normalizeNaturalSize(image.naturalWidth, image.naturalHeight))
    }
  }).finally(() => {
    imageDimensionTasks.delete(url)
  })

  imageDimensionTasks.set(url, task)
  return task
}

async function createLightboxItem(candidate: LightboxCandidate): Promise<LightboxDataSourceItem> {
  const naturalSize = await readImageNaturalSize(candidate.src)
  const size = naturalSize ?? toLightboxSize(candidate.ratio)
  return {
    src: candidate.src,
    width: size.width,
    height: size.height,
  }
}

async function resolveOriginalCandidateSource(candidate: LightboxCandidate): Promise<string> {
  if (!candidate.mediaAssetId) return candidate.src
  try {
    const metadata = await getMediaAssetMetadata(candidate.mediaAssetId)
    if (metadata.original.status === 'ready' && metadata.original.url) {
      return normalizeImageUrl(metadata.original.url)
    }
  } catch {
    return resolveMediaAssetOriginalURL(candidate.mediaAssetId)
  }
  return resolveMediaAssetOriginalURL(candidate.mediaAssetId)
}

function collectOutputItems(outputImages: ImageNodeItem['outputImages']): ImageNodeOutputItem[] {
  return Array.isArray(outputImages)
    ? outputImages.filter((item) => {
        const mediaAssetId = Number(item?.mediaAssetId)
        return (
          normalizeImageUrl(item?.url).length > 0 ||
          (Number.isInteger(mediaAssetId) && mediaAssetId > 0)
        )
      })
    : []
}

export function useFlowImagePreviewLightbox() {
  let lightbox: PhotoSwipeLightbox | null = null
  let isDisposed = false
  let previewRequestId = 0

  function ensureLightbox(): PhotoSwipeLightbox {
    if (lightbox) return lightbox
    lightbox = new PhotoSwipeLightbox({
      dataSource: [],
      pswpModule: () => import('photoswipe'),
      bgOpacity: 0.9,
      wheelToZoom: true,
      pinchToClose: true,
      showHideAnimationType: 'zoom',
    })
    lightbox.init()
    return lightbox
  }

  async function openNodeImagePreview(
    node: ImageNodeItem,
    target: PreviewTarget = 'latest-output',
  ) {
    const requestId = ++previewRequestId
    const sourceUrl = normalizeImageUrl(node.sourceImageUrl)
    const outputItems = collectOutputItems(node.outputImages)

    const hasSourceCandidate = Boolean(sourceUrl || node.sourceMediaAssetId)
    const candidates: LightboxCandidate[] = []
    if (hasSourceCandidate) {
      candidates.push({
        src: node.sourceMediaAssetId
          ? resolveMediaAssetOriginalURL(node.sourceMediaAssetId)
          : sourceUrl,
        ratio: node.sourceImageRatio,
        ...(node.sourceMediaAssetId ? { mediaAssetId: node.sourceMediaAssetId } : {}),
      })
    }
    outputItems.forEach((item) => {
      const outputUrl = normalizeImageUrl(item.url)
      if (!outputUrl && !item.mediaAssetId) return
      candidates.push({
        src: item.mediaAssetId ? resolveMediaAssetOriginalURL(item.mediaAssetId) : outputUrl,
        ratio: item.ratio,
        ...(item.mediaAssetId ? { mediaAssetId: item.mediaAssetId } : {}),
      })
    })

    if (candidates.length === 0) return

    const sourceOffset = hasSourceCandidate ? 1 : 0
    const latestOutputIndex =
      outputItems.length > 0 ? sourceOffset + outputItems.length - 1 : Math.max(0, sourceOffset - 1)
    const index = target === 'source' ? 0 : Math.max(0, latestOutputIndex)
    const dataSource = await Promise.all(
      candidates.map(async (candidate) =>
        createLightboxItem({ ...candidate, src: await resolveOriginalCandidateSource(candidate) }),
      ),
    )
    if (isDisposed || requestId !== previewRequestId) return

    const instance = ensureLightbox()
    if (instance.pswp) {
      instance.pswp.close()
    }
    instance.loadAndOpen(index, dataSource)
  }

  async function openImageUrlPreview(src: string, ratio = 1) {
    const url = normalizeImageUrl(src)
    if (!url) return
    const requestId = ++previewRequestId
    const dataSource = [await createLightboxItem({ src: url, ratio })]
    if (isDisposed || requestId !== previewRequestId) return

    const instance = ensureLightbox()
    if (instance.pswp) {
      instance.pswp.close()
    }
    instance.loadAndOpen(0, dataSource)
  }

  onBeforeUnmount(() => {
    isDisposed = true
    if (!lightbox) return
    lightbox.destroy()
    lightbox = null
  })

  return {
    openNodeImagePreview,
    openImageUrlPreview,
  }
}
