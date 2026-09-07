import {
  computed,
  onBeforeUnmount,
  shallowRef,
  toValue,
  watch,
  type MaybeRefOrGetter,
  type Ref,
} from 'vue'
import type { RetouchLayer, RetouchRenderedPreview } from './types'
import { createImageRetouchInlinePreviewRenderer } from './services/imageRetouchInlinePreviewRenderer'
import { buildImageRetouchPlan } from './utils/imageRetouchPlanPipeline'

type UseImageRetouchRenderedPreviewOptions = {
  isAdjustMode: Ref<boolean>
  layers: Ref<readonly RetouchLayer[]>
  sourceUrl?: MaybeRefOrGetter<string | null | undefined>
}

export function useImageRetouchRenderedPreview(options: UseImageRetouchRenderedPreviewOptions) {
  const retouchRenderedPreview = shallowRef<RetouchRenderedPreview | null>(null)
  const retouchPreviewSource = shallowRef<RetouchRenderedPreview | null>(null)

  const sourceUrl = computed(() => toValue(options.sourceUrl) ?? null)
  const imageRetouchPlan = computed(() => {
    if (!options.isAdjustMode.value) return []
    return buildImageRetouchPlan(options.layers.value)
  })
  const previewRenderer = createImageRetouchInlinePreviewRenderer({
    onPreview(preview) {
      retouchRenderedPreview.value = preview
    },
    onSource(source) {
      retouchPreviewSource.value = source
        ? {
            type: 'rendered-image',
            imageData: source.imageData,
            width: source.width,
            height: source.height,
          }
        : null
    },
    onError(error) {
      console.warn('[retouch] failed to load retouch preview source', error)
    },
  })

  watch(
    [sourceUrl, () => options.isAdjustMode.value],
    ([currentSourceUrl, isAdjustMode]) => {
      previewRenderer.updateSource(isAdjustMode ? currentSourceUrl : null)
    },
    { immediate: true },
  )

  watch(imageRetouchPlan, (plan) => previewRenderer.updatePlan(plan), { immediate: true })

  onBeforeUnmount(() => {
    previewRenderer.dispose()
  })

  const renderedPreview = computed<RetouchRenderedPreview | null>(() => {
    if (!options.isAdjustMode.value || imageRetouchPlan.value.length === 0) return null
    return retouchRenderedPreview.value
  })
  const previewSource = computed<RetouchRenderedPreview | null>(() => {
    if (!options.isAdjustMode.value) return null
    return retouchPreviewSource.value
  })

  return {
    retouchRenderedPreview: renderedPreview,
    retouchPreviewSource: previewSource,
  }
}
