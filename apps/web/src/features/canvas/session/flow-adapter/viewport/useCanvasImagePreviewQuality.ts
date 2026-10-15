import { computed, readonly, ref, type MaybeRefOrGetter, toValue, watch } from 'vue'
import type { CanvasImageQuality } from './useCanvasImageQualityZoom'

const CANVAS_IMAGE_PREVIEW_MAX_SIDE = {
  compact: 256,
  balanced: 640,
  detailed: 1280,
  full: null,
} as const

type CanvasImagePreviewMaxSide =
  (typeof CANVAS_IMAGE_PREVIEW_MAX_SIDE)[keyof typeof CANVAS_IMAGE_PREVIEW_MAX_SIDE]

type ResolveCanvasImagePreviewMaxSideOptions = {
  zoomLevel: number
  shouldForceFullQuality?: boolean | undefined
}

type UseCanvasImagePreviewQualityOptions = {
  imageQuality: CanvasImageQuality
  shouldForceFullQuality?: MaybeRefOrGetter<boolean>
}

export function resolveCanvasImagePreviewMaxSide(
  options: ResolveCanvasImagePreviewMaxSideOptions,
): CanvasImagePreviewMaxSide {
  if (options.shouldForceFullQuality) return CANVAS_IMAGE_PREVIEW_MAX_SIDE.full
  if (!Number.isFinite(options.zoomLevel) || options.zoomLevel < 77) {
    return CANVAS_IMAGE_PREVIEW_MAX_SIDE.compact
  }
  if (options.zoomLevel < 138) return CANVAS_IMAGE_PREVIEW_MAX_SIDE.balanced
  return CANVAS_IMAGE_PREVIEW_MAX_SIDE.detailed
}

export function useCanvasImagePreviewQuality(options: UseCanvasImagePreviewQualityOptions) {
  const { imageQuality } = options
  const shouldForceFullQuality = computed(() => !!toValue(options.shouldForceFullQuality))
  if (shouldForceFullQuality.value) imageQuality.requestSettle()
  watch(shouldForceFullQuality, imageQuality.requestSettle, { flush: 'sync' })
  const requestedMaxSide = computed(() =>
    resolveCanvasImagePreviewMaxSide({
      zoomLevel: imageQuality.zoomLevel.value,
      shouldForceFullQuality: shouldForceFullQuality.value,
    }),
  )
  const maxSide = ref(
    imageQuality.isSettled.value
      ? requestedMaxSide.value
      : resolveCanvasImagePreviewMaxSide({ zoomLevel: imageQuality.zoomLevel.value }),
  )
  watch(
    [imageQuality.isSettled, requestedMaxSide],
    ([isSettled, requested]) => {
      if (isSettled) maxSide.value = requested
    },
    { flush: 'sync' },
  )

  return { maxSide: readonly(maxSide) }
}
