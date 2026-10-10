import { computed, type ComputedRef, type Ref } from 'vue'
import type { RetouchRenderedPreview } from '../stage/runtime/types.ts'
import { isUsableTextureImage } from '../stage/useZeroCopyRetouchStage.ts'

/** Downsampled edge for histogram input: keeps the CPU histogram work small. */
export const STABLE_HISTOGRAM_MAX_SIDE = 128

/**
 * Cache identity of the raw DOM source: URL + decoded size. Effect params
 * are excluded on purpose — dragging the selected layer must never move the
 * histogram shape (Lightroom semantics: the mountain shows the layer input).
 */
export function resolveStableSourceKey(image: HTMLImageElement): string {
  const src = image.currentSrc || image.src || ''
  return `${src} ${image.naturalWidth}x${image.naturalHeight}`
}

/**
 * Decode the raw DOM image (pre-GPU effects) into a small ImageData that
 * keeps the aspect ratio. Null-safe: an unusable or tainted (CORS) source
 * yields null so the histogram stays empty instead of breaking the chain.
 */
export function decodeStableSourceImage(
  image: HTMLImageElement | null | undefined,
  maxSide: number = STABLE_HISTOGRAM_MAX_SIDE,
): ImageData | null {
  if (!isUsableTextureImage(image)) return null
  try {
    const source = image as HTMLImageElement
    const edge = Math.max(source.naturalWidth, source.naturalHeight)
    const scale = Math.min(1, maxSide / edge)
    const width = Math.max(1, Math.floor(source.naturalWidth * scale))
    const height = Math.max(1, Math.floor(source.naturalHeight * scale))
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const context = canvas.getContext('2d')
    if (!context) return null
    context.drawImage(source, 0, 0, width, height)
    return context.getImageData(0, 0, width, height)
  } catch {
    // Tainted canvas reads throw: empty histogram, uninterrupted session.
    return null
  }
}

type UseStableSourceImageOptions = {
  isAdjustMode: Ref<boolean> | ComputedRef<boolean>
  /** Raw on-screen node image, before any GPU effect. */
  resolveDomImage: () => HTMLImageElement | null
  /**
   * Reactive resample trigger (layer identity). The DOM image itself is not
   * reactive, so a late decode is picked up on the next layer/mode change.
   * Same-source resolutions reuse the cached reference, so slider drags
   * never move the histogram shape.
   */
  refreshSource: Ref<unknown> | ComputedRef<unknown>
  /**
   * Panel session identity (node id, '' when there is no node). Within one
   * session the first sampled base is frozen: later `resolveDomImage()` picks
   * (canvas parent vs node stage, thumbnail vs full image, late currentSrc)
   * are ignored so the first drag never moves the histogram. A session change
   * resets the cache. The null -> image transition is still allowed so a
   * late-decoding image is picked up via `refreshSource`.
   */
  sessionKey?: Ref<string> | ComputedRef<string>
}

/**
 * Stable histogram base: samples the DOM source once per image and reuses
 * the same reference until the source itself changes. Only samples while in
 * adjust mode with a decoded image.
 */
export function useStableSourceImage(options: UseStableSourceImageOptions) {
  let cachedKey: string | null = null
  let cachedPreview: RetouchRenderedPreview | null = null
  let cachedSessionKey: string | null = null

  const stablePreviewSource = computed<RetouchRenderedPreview | null>(() => {
    const sessionKey = options.sessionKey?.value ?? null
    if (sessionKey !== cachedSessionKey) {
      cachedSessionKey = sessionKey
      cachedKey = null
      cachedPreview = null
    }
    if (!options.isAdjustMode.value) return null
    // Subscribe so a late-decoding DOM image is picked up on layer changes.
    void options.refreshSource.value
    // Frozen once sampled within a session: ignore later DOM picks, no re-decode.
    if (sessionKey !== null && cachedPreview) return cachedPreview
    const image = options.resolveDomImage()
    if (!image) return null
    const key = resolveStableSourceKey(image)
    if (cachedPreview && cachedKey === key) return cachedPreview
    const imageData = decodeStableSourceImage(image)
    if (!imageData) return null
    cachedKey = key
    cachedPreview = {
      type: 'rendered-image',
      imageData,
      width: imageData.width,
      height: imageData.height,
    }
    return cachedPreview
  })

  return {
    stablePreviewSource,
  }
}
