import { computed, type ComputedRef, type Ref } from 'vue'
import type {
  ImageEditorAdjustmentLayer,
  ImageEditorLayer,
  ImageEditorRenderedPreview,
} from '../types'
import { executeImageAdjustmentPlan } from '../utils/imageAdjustmentEngine'
import { buildImageAdjustmentPlanFromAdjustmentLayers } from '../utils/imageAdjustmentPlanPipeline'
import { flattenVisibleImageEditorAdjustmentLayersBeforeLayer } from '../utils/imageEditorLayerAdjustments'
import { cloneImageData } from '../utils/imageDataUtils'

type UseImageEditorHistogramSourceOptions = {
  isAdjustMode: Ref<boolean> | ComputedRef<boolean>
  layers: Ref<readonly ImageEditorLayer[]> | ComputedRef<readonly ImageEditorLayer[]>
  selectedAdjustmentLayer: ComputedRef<ImageEditorAdjustmentLayer | null>
  previewSource: ComputedRef<ImageEditorRenderedPreview | null>
}

export function useImageEditorHistogramSource(options: UseImageEditorHistogramSourceOptions) {
  let cache: {
    sourceImageData: ImageData
    targetLayerId: string
    upstreamLayers: readonly ImageEditorAdjustmentLayer[]
    imageData: ImageData
  } | null = null

  const imageEditorHistogramSource = computed<ImageData | null>(() => {
    const previewSource = options.previewSource.value
    const selectedLayer = options.selectedAdjustmentLayer.value
    if (!options.isAdjustMode.value || !previewSource || !selectedLayer) return null

    const upstreamLayers = flattenVisibleImageEditorAdjustmentLayersBeforeLayer(
      options.layers.value,
      selectedLayer.id,
    )
    if (
      cache &&
      cache.sourceImageData === previewSource.imageData &&
      cache.targetLayerId === selectedLayer.id &&
      areSameLayerRefs(cache.upstreamLayers, upstreamLayers)
    ) {
      return cache.imageData
    }

    const imageData = cloneImageData(previewSource.imageData)
    const plan = buildImageAdjustmentPlanFromAdjustmentLayers(upstreamLayers)
    if (plan.length > 0) executeImageAdjustmentPlan(imageData, plan)
    cache = {
      sourceImageData: previewSource.imageData,
      targetLayerId: selectedLayer.id,
      upstreamLayers,
      imageData,
    }
    return imageData
  })

  return {
    imageEditorHistogramSource,
  }
}

function areSameLayerRefs(
  first: readonly ImageEditorAdjustmentLayer[],
  second: readonly ImageEditorAdjustmentLayer[],
) {
  return first.length === second.length && first.every((layer, index) => layer === second[index])
}
