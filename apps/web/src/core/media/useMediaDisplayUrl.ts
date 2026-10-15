import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { createSharedComposable, useDevicePixelRatio } from '@vueuse/core'
import { resolveMediaDisplayUrl, type MediaDisplay } from './mediaDisplay'

const useDisplayPixelRatio = createSharedComposable(useDevicePixelRatio)

export function useMediaDisplayUrl(
  display: MaybeRefOrGetter<MediaDisplay | undefined>,
  dimensions: MaybeRefOrGetter<{ width: number; height: number }>,
) {
  const { pixelRatio } = useDisplayPixelRatio()
  return computed(() => {
    const { width, height } = toValue(dimensions)
    return resolveMediaDisplayUrl(toValue(display), Math.max(width, height) * pixelRatio.value)
  })
}
