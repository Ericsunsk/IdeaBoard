import { computed, type MaybeRefOrGetter, toValue } from 'vue'

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
  zoomLevel: MaybeRefOrGetter<number>
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
  const maxSide = computed(() =>
    resolveCanvasImagePreviewMaxSide({
      zoomLevel: toValue(options.zoomLevel),
      shouldForceFullQuality: toValue(options.shouldForceFullQuality),
    }),
  )

  return { maxSide }
}
