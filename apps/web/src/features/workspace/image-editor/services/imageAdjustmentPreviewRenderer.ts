import { getImageAdjustmentProcessingRuntime } from '../runtime/imageAdjustmentProcessingRuntime'
import type { RenderPreviewResult } from '../runtime/types'
import type { ImageAdjustmentPlan } from '../utils/imageAdjustmentEngine'

export const IMAGE_ADJUSTMENT_PREVIEW_TASK_ID = 'image-adjustment-preview'

export function renderImageAdjustmentPreview(
  sourceUrl: string,
  plan: ImageAdjustmentPlan,
  options: { maxSide?: number; debounceMs?: number; taskId?: string } = {},
): Promise<RenderPreviewResult> {
  return getImageAdjustmentProcessingRuntime().renderPreview({
    source: { url: sourceUrl },
    plan,
    taskId: options.taskId ?? IMAGE_ADJUSTMENT_PREVIEW_TASK_ID,
    debounceMs: options.debounceMs,
    maxSide: options.maxSide,
  })
}

export function cancelImageAdjustmentPreviewRender(
  taskId = IMAGE_ADJUSTMENT_PREVIEW_TASK_ID,
) {
  getImageAdjustmentProcessingRuntime().cancel(taskId)
}
