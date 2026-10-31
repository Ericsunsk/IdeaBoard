import { onBeforeUnmount } from 'vue'
import type { ImageNodeItem, ImageNodeOutputItem } from '../../../../board/ports/canvas'
import {
  canonicalizeRuntimeUploadRef,
  resolveAssetUrl,
} from '../../../../../core/config/runtimeUrls.ts'
import {
  getMediaAssetMetadata,
  resolveMediaAssetOriginalURL,
} from '@/api/media/mediaAssetDelivery'
import { readImageNaturalSize } from '../../../../../core/media/imageDimensions.ts'
import { resolveMotionTiming } from '../../../../../core/motion/motionPolicy'
import PhotoSwipeLightbox from 'photoswipe/lightbox'
import 'photoswipe/style.css'

type PreviewTarget = 'source' | 'latest-output'

interface LightboxCandidate {
  src: string
  ratio: number
  assetId?: string
}

interface LightboxDataSourceItem {
  src: string
  width: number
  height: number
}

const LIGHTBOX_FALLBACK_SHORT_SIDE = 1440

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

async function createLightboxItem(candidate: LightboxCandidate): Promise<LightboxDataSourceItem> {
  const naturalSize = await readImageNaturalSize(candidate.src)
  const size = naturalSize ?? toLightboxSize(candidate.ratio)
  return {
    src: candidate.src,
    width: size.width,
    height: size.height,
  }
}

async function resolveOriginalCandidateSource(
  candidate: LightboxCandidate,
): Promise<string | null> {
  if (!candidate.assetId) return candidate.src
  try {
    const metadata = await getMediaAssetMetadata(candidate.assetId)
    if (metadata.original.status === 'ready' && metadata.original.url) {
      return normalizeImageUrl(metadata.original.url)
    }
    // Authenticated API content URLs are not safe <img> sources when the API
    // origin differs from the app origin. Prefer any ready signed derivative
    // instead of falling back to `/original/content` and triggering CORS.
    for (const tier of ['large-preview', 'preview', 'thumb'] as const) {
      const variant = metadata.derivatives[tier]
      if (variant?.status === 'ready' && variant.url) return normalizeImageUrl(variant.url)
    }
  } catch {
    return null
  }
  return null
}

function collectOutputItems(outputImages: ImageNodeItem['outputImages']): ImageNodeOutputItem[] {
  return Array.isArray(outputImages)
    ? outputImages.filter((item) => {
        const assetId = `${item?.mediaRef?.assetId ?? ''}`.trim()
        return normalizeImageUrl(item?.url).length > 0 || assetId.length > 0
      })
    : []
}

function toAssetCandidate(src: string, ratio: number, assetId?: string): LightboxCandidate {
  const trimmed = assetId?.trim()
  return {
    src: trimmed ? resolveMediaAssetOriginalURL(trimmed) : src,
    ratio,
    ...(trimmed ? { assetId: trimmed } : {}),
  }
}

function collectLightboxCandidates(node: ImageNodeItem): {
  candidates: LightboxCandidate[]
  hasSourceCandidate: boolean
  outputCount: number
} {
  const sourceUrl = normalizeImageUrl(node.sourceImageUrl)
  const outputItems = collectOutputItems(node.outputImages)
  const candidates: LightboxCandidate[] = []
  const hasSourceCandidate = Boolean(sourceUrl || node.sourceMediaRef?.assetId)
  if (hasSourceCandidate) {
    candidates.push(
      toAssetCandidate(sourceUrl, node.sourceImageRatio, node.sourceMediaRef?.assetId),
    )
  }
  for (const item of outputItems) {
    const outputUrl = normalizeImageUrl(item.url)
    const outputAssetId = item.mediaRef?.assetId?.trim()
    if (!outputUrl && !outputAssetId) continue
    candidates.push(toAssetCandidate(outputUrl, item.ratio, outputAssetId))
  }
  return { candidates, hasSourceCandidate, outputCount: outputItems.length }
}

export function useCanvasImageLightbox() {
  let lightbox: PhotoSwipeLightbox | null = null
  let isDisposed = false
  let previewRequestId = 0

  function ensureLightbox(): PhotoSwipeLightbox {
    if (!lightbox) {
      lightbox = new PhotoSwipeLightbox({
        dataSource: [],
        pswpModule: () => import('photoswipe'),
        bgOpacity: 0.9,
        wheelToZoom: true,
        pinchToClose: true,
        showHideAnimationType: 'zoom',
      })
      lightbox.init()
    }
    const enter = resolveMotionTiming('enter')
    const exit = resolveMotionTiming('exit')
    Object.assign(lightbox.options, {
      showAnimationDuration: enter.durationMs,
      hideAnimationDuration: exit.durationMs,
      zoomAnimationDuration: enter.durationMs,
      easing: enter.easing,
    })
    return lightbox
  }

  async function openNodeImagePreview(
    node: ImageNodeItem,
    target: PreviewTarget = 'latest-output',
  ) {
    const requestId = ++previewRequestId
    const { candidates, hasSourceCandidate, outputCount } = collectLightboxCandidates(node)
    if (candidates.length === 0) return

    const sourceOffset = hasSourceCandidate ? 1 : 0
    const latestOutputIndex =
      outputCount > 0 ? sourceOffset + outputCount - 1 : Math.max(0, sourceOffset - 1)
    const requestedIndex = target === 'source' ? 0 : Math.max(0, latestOutputIndex)
    const resolvedCandidates = await Promise.all(
      candidates.map(async (candidate) => {
        const src = await resolveOriginalCandidateSource(candidate)
        return src ? { ...candidate, src } : null
      }),
    )
    const availableOriginalIndexes = resolvedCandidates.reduce<number[]>(
      (indexes, candidate, index) => {
        if (candidate !== null) indexes.push(index)
        return indexes
      },
      [],
    )
    const dataSource = await Promise.all(
      resolvedCandidates
        .filter((candidate): candidate is LightboxCandidate => candidate !== null)
        .map((candidate) => createLightboxItem(candidate)),
    )
    if (dataSource.length === 0) return
    const requestedAvailableIndex = availableOriginalIndexes.indexOf(requestedIndex)
    const index =
      requestedAvailableIndex >= 0
        ? requestedAvailableIndex
        : Math.min(dataSource.length - 1, Math.max(0, requestedIndex))
    if (isDisposed || requestId !== previewRequestId) return

    const instance = ensureLightbox()
    if (instance.pswp) {
      instance.pswp.close()
    }
    instance.loadAndOpen(index, dataSource)
  }

  async function openImageUrlPreview(src: string, ratio = 1, assetId?: string) {
    const candidate: LightboxCandidate = {
      src: normalizeImageUrl(src),
      ratio,
      ...(assetId ? { assetId } : {}),
    }
    const url = await resolveOriginalCandidateSource(candidate)
    if (!url) return
    const requestId = ++previewRequestId
    const dataSource = [await createLightboxItem({ ...candidate, src: url })]
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
