import { executeImageRetouchPlan, type ImageRetouchPlan } from '../../utils/imageRetouchEngine'

export function executeCpuImageRetouchPlanInPlace(
  imageData: ImageData,
  plan: ImageRetouchPlan,
): ImageData {
  return executeImageRetouchPlan(imageData, plan)
}
