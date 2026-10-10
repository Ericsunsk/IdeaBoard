import { computed, type ComputedRef, type Ref } from 'vue'
import type { RetouchEffectLayer, RetouchLayer } from '../../core/model.ts'
import type { RetouchRenderedPreview } from '../stage/runtime/types.ts'
import {
  resolveRetouchHistogramSource,
  type RetouchHistogramSourceResolution,
} from '../../core/histogramSource.ts'

type UseRetouchHistogramSourceOptions = {
  isAdjustMode: Ref<boolean> | ComputedRef<boolean>
  layers: Ref<readonly RetouchLayer[]> | ComputedRef<readonly RetouchLayer[]>
  selectedEffectLayer: ComputedRef<RetouchEffectLayer | null>
  previewSource: ComputedRef<RetouchRenderedPreview | null>
  /*
   * Explicit pending slot forwarded to the resolver. Optional so existing
   * callers keep working: without it an unknown target id still resolves to
   * the raw source (legacy id-miss path).
   */
  pendingEffectLayer?:
    Ref<RetouchEffectLayer | null> | ComputedRef<RetouchEffectLayer | null> | undefined
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
      pendingEffectLayer: options.pendingEffectLayer?.value ?? null,
      cachedResolution,
    })
    return cachedResolution.imageData
  })

  return {
    retouchHistogramSource,
  }
}
