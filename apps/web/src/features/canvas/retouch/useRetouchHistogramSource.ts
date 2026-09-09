import { computed, type ComputedRef, type Ref } from 'vue'
import type { RetouchEffectLayer, RetouchLayer, RetouchRenderedPreview } from './types'
import {
  resolveRetouchHistogramSource,
  type RetouchHistogramSourceResolution,
} from './utils/histogramSource'

type UseRetouchHistogramSourceOptions = {
  isAdjustMode: Ref<boolean> | ComputedRef<boolean>
  layers: Ref<readonly RetouchLayer[]> | ComputedRef<readonly RetouchLayer[]>
  selectedEffectLayer: ComputedRef<RetouchEffectLayer | null>
  previewSource: ComputedRef<RetouchRenderedPreview | null>
}

export function useRetouchHistogramSource(options: UseRetouchHistogramSourceOptions) {
  let cachedResolution: RetouchHistogramSourceResolution | null = null

  const retouchHistogramSource = computed<ImageData | null>(() => {
    const previewSource = options.previewSource.value
    const selectedLayer = options.selectedEffectLayer.value
    if (!options.isAdjustMode.value || !previewSource || !selectedLayer) return null

    cachedResolution = resolveRetouchHistogramSource({
      sourceImageData: previewSource.imageData,
      layers: options.layers.value,
      targetLayerId: selectedLayer.id,
      cachedResolution,
    })
    return cachedResolution.imageData
  })

  return {
    retouchHistogramSource,
  }
}
