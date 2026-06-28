import {
  computed,
  onBeforeUnmount,
  shallowRef,
  toValue,
  watch,
  type MaybeRefOrGetter,
  type Ref,
} from 'vue'
import type { ImageEditorFilterPreview, ImageEditorLayer } from '../types'
import { createImageAdjustmentInlinePreviewRenderer } from '../services/imageAdjustmentInlinePreviewRenderer'
import { buildImageAdjustmentPlan } from '../utils/imageAdjustmentPlanPipeline'

type UseImageAdjustmentFilterPreviewsOptions = {
  isAdjustMode: Ref<boolean>
  layers: Ref<readonly ImageEditorLayer[]>
  sourceUrl?: MaybeRefOrGetter<string | null | undefined>
}

export function useImageAdjustmentFilterPreviews(options: UseImageAdjustmentFilterPreviewsOptions) {
  const renderedPreview = shallowRef<ImageEditorFilterPreview | null>(null)

  const sourceUrl = computed(() => toValue(options.sourceUrl) ?? null)
  const imageAdjustmentPlan = computed(() => {
    if (!options.isAdjustMode.value) return []
    return buildImageAdjustmentPlan(options.layers.value)
  })
  const previewRenderer = createImageAdjustmentInlinePreviewRenderer({
    onPreview(preview) {
      renderedPreview.value = preview
    },
    onError(error) {
      console.warn('[image-editor] failed to load adjustment preview source', error)
    },
  })

  watch(
    [sourceUrl, () => options.isAdjustMode.value],
    ([currentSourceUrl, isAdjustMode]) => {
      previewRenderer.updateSource(isAdjustMode ? currentSourceUrl : null)
    },
    { immediate: true },
  )

  watch(imageAdjustmentPlan, (plan) => previewRenderer.updatePlan(plan), { immediate: true })

  onBeforeUnmount(() => {
    previewRenderer.dispose()
  })

  const imageAdjustmentFilterPreviews = computed<ImageEditorFilterPreview[]>(() => {
    if (!options.isAdjustMode.value || imageAdjustmentPlan.value.length === 0) return []
    return renderedPreview.value ? [renderedPreview.value] : []
  })

  return {
    imageAdjustmentFilterPreviews,
    imageAdjustmentPlan,
  }
}
