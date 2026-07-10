import {
  computed,
  onBeforeUnmount,
  shallowRef,
  toValue,
  watch,
  type MaybeRefOrGetter,
  type Ref,
} from 'vue'
import type { ImageEditorLayer, ImageEditorRenderedPreview } from '../types'
import { createImageAdjustmentInlinePreviewRenderer } from '../services/imageAdjustmentInlinePreviewRenderer'
import { buildImageAdjustmentPlan } from '../utils/imageAdjustmentPlanPipeline'

type UseImageAdjustmentRenderedPreviewOptions = {
  isAdjustMode: Ref<boolean>
  layers: Ref<readonly ImageEditorLayer[]>
  sourceUrl?: MaybeRefOrGetter<string | null | undefined>
}

export function useImageAdjustmentRenderedPreview(
  options: UseImageAdjustmentRenderedPreviewOptions,
) {
  const imageEditorRenderedPreview = shallowRef<ImageEditorRenderedPreview | null>(null)
  const imageEditorPreviewSource = shallowRef<ImageEditorRenderedPreview | null>(null)

  const sourceUrl = computed(() => toValue(options.sourceUrl) ?? null)
  const imageAdjustmentPlan = computed(() => {
    if (!options.isAdjustMode.value) return []
    return buildImageAdjustmentPlan(options.layers.value)
  })
  const previewRenderer = createImageAdjustmentInlinePreviewRenderer({
    onPreview(preview) {
      imageEditorRenderedPreview.value = preview
    },
    onSource(source) {
      imageEditorPreviewSource.value = source
        ? {
            type: 'rendered-image',
            imageData: source.imageData,
            width: source.width,
            height: source.height,
          }
        : null
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

  const renderedPreview = computed<ImageEditorRenderedPreview | null>(() => {
    if (!options.isAdjustMode.value || imageAdjustmentPlan.value.length === 0) return null
    return imageEditorRenderedPreview.value
  })
  const previewSource = computed<ImageEditorRenderedPreview | null>(() => {
    if (!options.isAdjustMode.value) return null
    return imageEditorPreviewSource.value
  })

  return {
    imageEditorRenderedPreview: renderedPreview,
    imageEditorPreviewSource: previewSource,
  }
}
