import { computed, type ComputedRef, type Ref } from 'vue'
import type {
  ImageEditorAdjustmentLayer,
  ImageEditorLayer,
  ImageEditorRenderedPreview,
} from './types'
import {
  resolveImageEditorHistogramSource,
  type ImageEditorHistogramSourceResolution,
} from './utils/imageEditorHistogramSource'

type UseImageEditorHistogramSourceOptions = {
  isAdjustMode: Ref<boolean> | ComputedRef<boolean>
  layers: Ref<readonly ImageEditorLayer[]> | ComputedRef<readonly ImageEditorLayer[]>
  selectedAdjustmentLayer: ComputedRef<ImageEditorAdjustmentLayer | null>
  previewSource: ComputedRef<ImageEditorRenderedPreview | null>
}

export function useImageEditorHistogramSource(options: UseImageEditorHistogramSourceOptions) {
  let cachedResolution: ImageEditorHistogramSourceResolution | null = null

  const imageEditorHistogramSource = computed<ImageData | null>(() => {
    const previewSource = options.previewSource.value
    const selectedLayer = options.selectedAdjustmentLayer.value
    if (!options.isAdjustMode.value || !previewSource || !selectedLayer) return null

    cachedResolution = resolveImageEditorHistogramSource({
      sourceImageData: previewSource.imageData,
      layers: options.layers.value,
      targetLayerId: selectedLayer.id,
      cachedResolution,
    })
    return cachedResolution.imageData
  })

  return {
    imageEditorHistogramSource,
  }
}
