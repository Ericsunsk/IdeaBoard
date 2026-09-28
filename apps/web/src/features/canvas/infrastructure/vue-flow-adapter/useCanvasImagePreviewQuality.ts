import { computed, getCurrentScope, onScopeDispose, ref, type MaybeRefOrGetter, toValue } from 'vue'
import { IMAGE_NODE_EMPTY_SIZE } from '../../../board'
import { resolveCanvasViewportZoomScale } from './useCanvasViewportContext'

export const CANVAS_IMAGE_PREVIEW_MAX_SIDE = {
  compact: 320,
  balanced: 640,
  detailed: 1280,
  full: null,
} as const

const CANVAS_IMAGE_PREVIEW_RENDERED_SIDE_LIMIT = {
  compact: 240,
  balanced: 520,
  detailed: 960,
} as const

export type CanvasImagePreviewMaxSide =
  (typeof CANVAS_IMAGE_PREVIEW_MAX_SIDE)[keyof typeof CANVAS_IMAGE_PREVIEW_MAX_SIDE]

type ResolveCanvasImagePreviewMaxSideOptions = {
  stageWidth: number
  stageHeight: number
  zoomLevel: number
  devicePixelRatio?: number
  forceFullQuality?: boolean | undefined
}

type UseCanvasImagePreviewQualityOptions = {
  stageWidth: MaybeRefOrGetter<number>
  stageHeight: MaybeRefOrGetter<number>
  zoomLevel: MaybeRefOrGetter<number>
  forceFullQuality?: MaybeRefOrGetter<boolean>
  devicePixelRatio?: MaybeRefOrGetter<number | undefined>
}

function normalizePositiveNumber(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : fallback
}

export function resolveCanvasImagePreviewMaxSide(
  options: ResolveCanvasImagePreviewMaxSideOptions,
): CanvasImagePreviewMaxSide {
  if (options.forceFullQuality) return CANVAS_IMAGE_PREVIEW_MAX_SIDE.full

  const stageWidth = normalizePositiveNumber(options.stageWidth, IMAGE_NODE_EMPTY_SIZE)
  const stageHeight = normalizePositiveNumber(options.stageHeight, stageWidth)
  const zoomScale = resolveCanvasViewportZoomScale(options.zoomLevel)
  const devicePixelRatio = normalizePositiveNumber(options.devicePixelRatio, 1)
  const longestRenderedSide = Math.max(stageWidth, stageHeight) * zoomScale * devicePixelRatio

  if (longestRenderedSide <= CANVAS_IMAGE_PREVIEW_RENDERED_SIDE_LIMIT.compact) {
    return CANVAS_IMAGE_PREVIEW_MAX_SIDE.compact
  }
  if (longestRenderedSide <= CANVAS_IMAGE_PREVIEW_RENDERED_SIDE_LIMIT.balanced) {
    return CANVAS_IMAGE_PREVIEW_MAX_SIDE.balanced
  }
  if (longestRenderedSide <= CANVAS_IMAGE_PREVIEW_RENDERED_SIDE_LIMIT.detailed) {
    return CANVAS_IMAGE_PREVIEW_MAX_SIDE.detailed
  }
  return CANVAS_IMAGE_PREVIEW_MAX_SIDE.full
}

function readSystemDevicePixelRatio(): number {
  if (typeof window === 'undefined') return 1
  return normalizePositiveNumber(window.devicePixelRatio, 1)
}

export function useCanvasImagePreviewQuality(options: UseCanvasImagePreviewQualityOptions) {
  // window.devicePixelRatio is not reactive: without a listener, moving the
  // window across monitors or changing page zoom leaves the tier stale until
  // the next zoom/stage change. Track it via matchMedia so the tier follows.
  const systemDevicePixelRatio = ref(readSystemDevicePixelRatio())
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    let query: MediaQueryList | null = null
    const handleChange = () => {
      query?.removeEventListener('change', handleChange)
      systemDevicePixelRatio.value = readSystemDevicePixelRatio()
      query = window.matchMedia(`(resolution: ${systemDevicePixelRatio.value}dppx)`)
      query.addEventListener('change', handleChange)
    }
    query = window.matchMedia(`(resolution: ${systemDevicePixelRatio.value}dppx)`)
    query.addEventListener('change', handleChange)
    if (getCurrentScope()) {
      onScopeDispose(() => {
        query?.removeEventListener('change', handleChange)
        query = null
      })
    }
  }

  const maxSide = computed(() =>
    resolveCanvasImagePreviewMaxSide({
      stageWidth: toValue(options.stageWidth),
      stageHeight: toValue(options.stageHeight),
      zoomLevel: toValue(options.zoomLevel),
      devicePixelRatio:
        toValue(options.devicePixelRatio) ?? systemDevicePixelRatio.value,
      forceFullQuality: toValue(options.forceFullQuality),
    }),
  )

  return {
    maxSide,
  }
}
