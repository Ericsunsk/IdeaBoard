import type { ImageEditorAdjustmentLayer } from '../types'
import { getImageAdjustmentProcessingRuntime } from '../runtime/imageAdjustmentProcessingRuntime'
import type { ImageProcessingProgress, RenderFinalInput } from '../runtime/types'
import { buildImageAdjustmentPlan } from '../utils/imageAdjustmentPlanPipeline'

type ImageAdjustmentRenderSource = {
  url: string
  title?: string
}

const IMAGE_ADJUSTMENT_FINAL_TASK_ID = 'image-adjustment-final'

type ImageAdjustmentRenderOptions = {
  scopeId?: string
  onProgress?: (progress: ImageProcessingProgress) => void
}

export async function renderImageAdjustmentFile(
  source: ImageAdjustmentRenderSource,
  layers: readonly ImageEditorAdjustmentLayer[],
  options: ImageAdjustmentRenderOptions = {},
): Promise<File> {
  const plan = buildImageAdjustmentPlan(layers)
  if (plan.length === 0) {
    throw new Error('没有可应用的编辑图层')
  }

  const renderInput: RenderFinalInput = {
    source,
    plan,
    taskId: getImageAdjustmentFinalTaskId(options.scopeId),
    latestOnly: true,
  }
  if (options.onProgress !== undefined) renderInput.onProgress = options.onProgress

  const result = await getImageAdjustmentProcessingRuntime().renderFinal(renderInput)

  if (result.status === 'completed') return result.output.file
  throw new Error(result.error.message)
}

export function cancelImageAdjustmentFinalRender(scopeId?: string) {
  getImageAdjustmentProcessingRuntime().cancel(getImageAdjustmentFinalTaskId(scopeId))
}

function getImageAdjustmentFinalTaskId(scopeId?: string) {
  return scopeId ? `${IMAGE_ADJUSTMENT_FINAL_TASK_ID}:${scopeId}` : IMAGE_ADJUSTMENT_FINAL_TASK_ID
}
