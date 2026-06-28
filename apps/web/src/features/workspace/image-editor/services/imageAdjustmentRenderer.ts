import type { ImageEditorAdjustmentLayer } from '../types'
import { getImageAdjustmentProcessingRuntime } from '../runtime/imageAdjustmentProcessingRuntime'
import type { ImageProcessingProgress, RenderFinalInput } from '../runtime/types'
import { buildImageAdjustmentPlan } from '../utils/imageAdjustmentPlanPipeline'

type ImageAdjustmentRenderSource = {
  url: string
  title?: string
}

export const IMAGE_ADJUSTMENT_FINAL_TASK_ID = 'image-adjustment-final'

type ImageAdjustmentRenderOptions = {
  taskId?: string
  latestOnly?: boolean
  onProgress?: (progress: ImageProcessingProgress) => void
}

export async function renderImageAdjustmentFile(
  source: ImageAdjustmentRenderSource,
  layers: readonly ImageEditorAdjustmentLayer[],
  options: ImageAdjustmentRenderOptions = {},
): Promise<File> {
  const plan = buildImageAdjustmentPlan(layers)
  if (plan.length === 0) {
    throw new Error('没有可应用的调整图层')
  }

  const renderInput: RenderFinalInput = {
    source,
    plan,
    taskId: options.taskId ?? IMAGE_ADJUSTMENT_FINAL_TASK_ID,
  }
  if (options.latestOnly !== undefined) renderInput.latestOnly = options.latestOnly
  if (options.onProgress !== undefined) renderInput.onProgress = options.onProgress

  const result = await getImageAdjustmentProcessingRuntime().renderFinal(renderInput)

  if (result.status === 'completed') return result.output.file
  throw new Error(result.error.message)
}

export function cancelImageAdjustmentFinalRender(taskId = IMAGE_ADJUSTMENT_FINAL_TASK_ID) {
  getImageAdjustmentProcessingRuntime().cancel(taskId)
}
