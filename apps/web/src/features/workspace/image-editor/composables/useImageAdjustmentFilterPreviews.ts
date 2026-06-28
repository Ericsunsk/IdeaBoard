import { computed, onBeforeUnmount, ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import type { ImageEditorFilterPreview, ImageEditorLayer } from '../types'
import {
  cancelImageAdjustmentPreviewRender,
  IMAGE_ADJUSTMENT_PREVIEW_TASK_ID,
  renderImageAdjustmentPreview,
} from '../services/imageAdjustmentPreviewRenderer'
import { compileImageAdjustmentPlan } from '../utils/imageAdjustmentEngine'
import { flattenVisibleImageEditorAdjustmentLayers } from '../utils/imageEditorLayerAdjustments'

type UseImageAdjustmentFilterPreviewsOptions = {
  isAdjustMode: Ref<boolean>
  layers: Ref<readonly ImageEditorLayer[]>
  sourceUrl?: MaybeRefOrGetter<string | null | undefined>
}

const PREVIEW_RENDER_DEBOUNCE_MS = 80

export function useImageAdjustmentFilterPreviews(options: UseImageAdjustmentFilterPreviewsOptions) {
  const renderedPreviewUrl = ref<string | null>(null)
  let previousRenderedPreviewUrl: string | null = null
  let renderedPreviewSourceUrl: string | null = null

  const imageAdjustmentPlan = computed(() => {
    if (!options.isAdjustMode.value) return []
    return compileImageAdjustmentPlan(
      flattenVisibleImageEditorAdjustmentLayers(options.layers.value),
    )
  })

  watch(
    [imageAdjustmentPlan, () => toValue(options.sourceUrl)],
    ([plan, sourceUrl]) => {
      if (!sourceUrl || plan.length === 0) {
        cancelImageAdjustmentPreviewRender(IMAGE_ADJUSTMENT_PREVIEW_TASK_ID)
        clearRenderedPreviewUrl()
        return
      }
      if (renderedPreviewSourceUrl !== null && renderedPreviewSourceUrl !== sourceUrl) {
        clearRenderedPreviewUrl()
      }

      void renderImageAdjustmentPreview(sourceUrl, plan, {
        taskId: IMAGE_ADJUSTMENT_PREVIEW_TASK_ID,
        debounceMs: PREVIEW_RENDER_DEBOUNCE_MS,
      })
        .then((result) => {
          if (result.status !== 'completed') {
            if (result.status === 'failed') {
              clearRenderedPreviewUrl()
              console.warn('[image-editor] failed to render adjustment preview', result.error)
            }
            return
          }
          const previewUrl = result.output.objectUrl
          if (!previewUrl) {
            clearRenderedPreviewUrl()
            return
          }
          const stalePreviewUrl = previousRenderedPreviewUrl
          renderedPreviewUrl.value = previewUrl
          previousRenderedPreviewUrl = previewUrl
          renderedPreviewSourceUrl = sourceUrl
          if (stalePreviewUrl !== previewUrl) revokeObjectUrl(stalePreviewUrl)
        })
        .catch((error) => {
          clearRenderedPreviewUrl()
          console.warn('[image-editor] failed to render adjustment preview', error)
        })
    },
    { immediate: true },
  )

  onBeforeUnmount(() => {
    cancelImageAdjustmentPreviewRender(IMAGE_ADJUSTMENT_PREVIEW_TASK_ID)
    clearRenderedPreviewUrl()
  })

  const imageAdjustmentFilterPreviews = computed<ImageEditorFilterPreview[]>(() => {
    if (!options.isAdjustMode.value || imageAdjustmentPlan.value.length === 0) return []
    if (renderedPreviewUrl.value) {
      return [{ type: 'rendered-image', url: renderedPreviewUrl.value }]
    }
    return []
  })

  function clearRenderedPreviewUrl() {
    revokeObjectUrl(previousRenderedPreviewUrl)
    previousRenderedPreviewUrl = null
    renderedPreviewSourceUrl = null
    renderedPreviewUrl.value = null
  }

  return {
    imageAdjustmentFilterPreviews,
    imageAdjustmentPlan,
  }
}

function revokeObjectUrl(url: string | null) {
  if (!url) return
  URL.revokeObjectURL(url)
}
