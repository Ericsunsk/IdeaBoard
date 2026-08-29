import {
  executeImageAdjustmentPlan,
  type ImageAdjustmentPlan,
} from '../../utils/imageAdjustmentEngine'

export function executeCpuImageAdjustmentPlanInPlace(
  imageData: ImageData,
  plan: ImageAdjustmentPlan,
): ImageData {
  return executeImageAdjustmentPlan(imageData, plan)
}
