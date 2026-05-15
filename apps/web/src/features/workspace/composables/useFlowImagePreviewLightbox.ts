import { onBeforeUnmount } from 'vue'
import type { ImageNodeItem, ImageNodeOutputItem } from '../types/canvasNodes'
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

function normalizeImageUrl(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
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

function collectOutputItems(outputImages: ImageNodeItem['outputImages']): ImageNodeOutputItem[] {
  return Array.isArray(outputImages)
    ? outputImages.filter((item) => normalizeImageUrl(item?.url).length > 0)
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

    const candidates: LightboxCandidate[] = []
    if (sourceUrl) {
      candidates.push({ src: sourceUrl, ratio: node.sourceImageRatio })
    }
    outputItems.forEach((item) => {
      const outputUrl = normalizeImageUrl(item.url)
      if (!outputUrl) return
      candidates.push({ src: outputUrl, ratio: item.ratio })
    })

    if (candidates.length === 0) return

    const sourceOffset = sourceUrl ? 1 : 0
    const latestOutputIndex =
      outputItems.length > 0 ? sourceOffset + outputItems.length - 1 : Math.max(0, sourceOffset - 1)
    const index = target === 'source' ? 0 : Math.max(0, latestOutputIndex)
    const dataSource = await Promise.all(
      candidates.map((candidate) => createLightboxItem(candidate)),
    )
    if (isDisposed || requestId !== previewRequestId) return

    const instance = ensureLightbox()
    if (instance.pswp) {
      instance.pswp.close()
    }
    instance.loadAndOpen(index, dataSource)
  }

  onBeforeUnmount(() => {
    isDisposed = true
    if (!lightbox) return
    lightbox.destroy()
    lightbox = null
  })

  return {
    openNodeImagePreview,
  }
}
